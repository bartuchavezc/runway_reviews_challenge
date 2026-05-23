package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"time"

	"runway/reviews/internal/feed"
	"runway/reviews/internal/poller"
	"runway/reviews/internal/store"
)

var rssURLRegex = regexp.MustCompile(`^https://itunes\.apple\.com/[a-z]{2}/rss/customerreviews/id=(\d+)/`)

// appIDRegex ensures app IDs from the URL path are strictly numeric (Apple app IDs always are).
// This prevents path traversal attacks since the ID is used to construct file paths in the store.
var appIDRegex = regexp.MustCompile(`^\d+$`)

const (
	defaultWindowHours  = 48
	maxWindowHours      = 8760    // 1 year
	maxRequestBodyBytes = 1 << 13 // 8 KB

	// appleValidationBudget is the total time budget shared by all outbound Apple API
	// calls during a single POST /api/apps request (feed reachability check +
	// app-name lookup). Both calls share one context so the combined add-app
	// latency is bounded regardless of how slowly each individual call responds.
	appleValidationBudget = 8 * time.Second
)

type Handler struct {
	store  *store.Store
	poller *poller.Manager
}

func NewHandler(s *store.Store, p *poller.Manager) *Handler {
	return &Handler{store: s, poller: p}
}

// RegisterRoutes uses Go 1.22 method+path routing — no manual dispatch needed.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("OPTIONS /api/", h.preflight)
	mux.HandleFunc("GET /api/apps", h.withCORS(h.listApps))
	mux.HandleFunc("POST /api/apps", h.withCORS(h.addApp))
	mux.HandleFunc("DELETE /api/apps/{id}", h.withCORS(h.deleteApp))
	mux.HandleFunc("GET /api/apps/{id}/reviews", h.withCORS(h.getReviews))
	mux.HandleFunc("GET /api/health", h.withCORS(h.listHealth))
}

func (h *Handler) preflight(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) withCORS(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		next(w, r)
	}
}

func (h *Handler) listApps(w http.ResponseWriter, _ *http.Request) {
	apps := h.store.Apps()
	response := make([]store.AppResponse, 0, len(apps))
	for _, app := range apps {
		response = append(response, store.AppResponse{App: app})
	}
	respondJSON(w, http.StatusOK, response)
}

// listHealth is a control-plane ping: aggregates poller state across all
// tracked apps into a single status flag plus the most recent successful
// refresh timestamp. Suitable for Kubernetes liveness/readiness probes or
// uptime monitors — no per-app detail, no disk reads.
//
// status = "sync"   — zero apps tracked OR every app's last poll succeeded
// status = "unsync" — at least one app is in PollerStatusError
func (h *Handler) listHealth(w http.ResponseWriter, _ *http.Request) {
	type healthResponse struct {
		Status          string    `json:"status"`
		LastSuccessSync time.Time `json:"last_success_sync"`
	}

	apps := h.store.Apps()
	status := "sync"
	var lastSync time.Time
	for _, a := range apps {
		if a.Poller.Status == store.PollerStatusError {
			status = "unsync"
		}
		if a.Poller.LastRefreshAt.After(lastSync) {
			lastSync = a.Poller.LastRefreshAt
		}
	}
	respondJSON(w, http.StatusOK, healthResponse{
		Status:          status,
		LastSuccessSync: lastSync,
	})
}

func (h *Handler) addApp(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
	var body struct {
		RSSUrl string `json:"rssUrl"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	matches := rssURLRegex.FindStringSubmatch(body.RSSUrl)
	if matches == nil {
		writeError(w, http.StatusBadRequest, "invalid RSS URL — expected https://itunes.apple.com/{country}/rss/customerreviews/id={appId}/...")
		return
	}
	appID := matches[1]

	// Rewrite the URL into the canonical /sortBy=mostRecent/json form. The
	// poller's per-page early-stop assumes newest-first ordering inside a
	// page, which only holds for sortBy=mostRecent. Canonicalizing here
	// instead of rejecting in the regex keeps the user-facing input flexible.
	canonicalURL := feed.CanonicalizeURL(body.RSSUrl)

	if h.store.AppExists(appID) {
		writeError(w, http.StatusConflict, fmt.Sprintf("app %s is already tracked", appID))
		return
	}

	// Both Apple calls share one context so the total add-app latency is bounded
	// by appleValidationBudget even if each individual call is slow.
	// We deliberately do NOT merge page-1 reviews here or set LastRefreshAt —
	// the poller starts immediately after AddApp and fetches all pages from zero.
	appleCtx, cancel := context.WithTimeout(r.Context(), appleValidationBudget)
	defer cancel()

	if _, _, err := feed.FetchPage(appleCtx, canonicalURL, 1); err != nil {
		writeError(w, http.StatusBadGateway, fmt.Sprintf("could not reach RSS feed: %v", err))
		return
	}

	appName := feed.LookupAppName(appleCtx, appID)
	if appName == "" {
		appName = "App " + appID
	}

	// Display-layer concerns (initials, swatch tint) are computed on the
	// frontend from id + name. The persistence struct stays minimal.
	if err := h.store.AddApp(store.App{
		ID:     appID,
		Name:   appName,
		RSSUrl: canonicalURL,
	}); err != nil {
		if errors.Is(err, store.ErrAlreadyExists) {
			writeError(w, http.StatusConflict, fmt.Sprintf("app %s is already tracked", appID))
			return
		}
		// Internal error strings can leak filesystem paths and implementation
		// details. Log the real cause; return a generic message.
		log.Printf("[struct:Handler][method:addApp][message:store.AddApp failed for %s: %v]", appID, err)
		writeError(w, http.StatusInternalServerError, "failed to add app")
		return
	}

	app, _ := h.store.GetApp(appID) // always succeeds: AddApp returned nil above
	// Kick a one-shot poll so the new app's reviews show up without the
	// user waiting a full POLL_INTERVAL_MINUTES for the next tick. The
	// ticker still picks the app up on subsequent cycles via store.Apps().
	h.poller.PollNow(appID)
	respondJSON(w, http.StatusCreated, store.AppResponse{App: app})
}

func (h *Handler) deleteApp(w http.ResponseWriter, r *http.Request) {
	appID := r.PathValue("id")
	if !appIDRegex.MatchString(appID) {
		writeError(w, http.StatusBadRequest, "invalid appId")
		return
	}

	// No need to stop a per-app worker first: the poller is a single
	// sequential loop. If a poll for this app is mid-fetch when DELETE
	// arrives, RemoveApp deletes from the store atomically (under s.mu),
	// and the poller's subsequent MergeReviews call returns ErrNotFound
	// without writing anything. The race window can't produce an orphan
	// review file.
	if err := h.store.RemoveApp(appID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "app not found")
			return
		}
		// Other errors (disk permission, etc) can leak filesystem paths.
		log.Printf("[struct:Handler][method:deleteApp][message:RemoveApp failed for %s: %v]", appID, err)
		writeError(w, http.StatusInternalServerError, "failed to remove app")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// getReviews reads reviews from disk on every call. Review content is
// intentionally not cached in memory on the backend (keeps the server footprint
// bounded). Client-side caching lives in the frontend's useReviews hook.
func (h *Handler) getReviews(w http.ResponseWriter, r *http.Request) {
	appID := r.PathValue("id")
	if !appIDRegex.MatchString(appID) {
		writeError(w, http.StatusBadRequest, "invalid appId")
		return
	}

	windowHours := defaultWindowHours
	if wStr := r.URL.Query().Get("window"); wStr != "" {
		if n, err := strconv.Atoi(wStr); err == nil && n >= 0 {
			if n > maxWindowHours {
				n = maxWindowHours
			}
			windowHours = n
		}
	}

	// window=0 → no time filter, return everything we have stored.
	var since time.Time
	if windowHours > 0 {
		since = time.Now().UTC().Add(-time.Duration(windowHours) * time.Hour)
	}
	reviews, err := h.store.GetReviews(appID, since)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	if err != nil {
		log.Printf("[struct:Handler][method:getReviews][message:GetReviews failed for %s: %v]", appID, err)
		writeError(w, http.StatusInternalServerError, "failed to load reviews")
		return
	}
	if reviews == nil {
		reviews = []store.Review{}
	}
	respondJSON(w, http.StatusOK, reviews)
}

// respondJSON writes a JSON response to an HTTP response writer.
func respondJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	respondJSON(w, status, map[string]string{"error": msg})
}
