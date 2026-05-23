package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"runway/reviews/internal/poller"
	"runway/reviews/internal/store"
)

// newTestHandler creates a Handler backed by a real store in a temp directory.
// This avoids mocking while keeping tests hermetic (no real Apple API calls).
func newTestHandler(t *testing.T) *Handler {
	t.Helper()
	s := store.New(t.TempDir())
	if err := s.Load(); err != nil {
		t.Fatalf("store.Load: %v", err)
	}
	p := poller.New(s, 15*time.Minute)
	return NewHandler(s, p)
}

func doRequest(t *testing.T, h *Handler, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("encode body: %v", err)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rr := httptest.NewRecorder()
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	mux.ServeHTTP(rr, req)
	return rr
}

// ---- GET /api/apps ----------------------------------------------------------

func TestListApps_Empty(t *testing.T) {
	h := newTestHandler(t)
	rr := doRequest(t, h, http.MethodGet, "/api/apps", nil)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	var apps []store.AppResponse
	if err := json.NewDecoder(rr.Body).Decode(&apps); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(apps) != 0 {
		t.Errorf("expected empty list, got %d apps", len(apps))
	}
}

// ---- POST /api/apps (validation only — no real Apple API calls) -------------

func TestAddApp_InvalidURL_RandomHost(t *testing.T) {
	h := newTestHandler(t)
	rr := doRequest(t, h, http.MethodPost, "/api/apps", map[string]string{
		"rssUrl": "https://notapple.com/rss/customerreviews/id=123/json",
	})
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for non-Apple host, got %d", rr.Code)
	}
}

func TestAddApp_InvalidURL_MissingAppId(t *testing.T) {
	h := newTestHandler(t)
	rr := doRequest(t, h, http.MethodPost, "/api/apps", map[string]string{
		"rssUrl": "https://itunes.apple.com/us/rss/customerreviews/sortBy=mostRecent/json",
	})
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing appId, got %d", rr.Code)
	}
}

func TestAddApp_InvalidURL_EmptyBody(t *testing.T) {
	h := newTestHandler(t)
	rr := doRequest(t, h, http.MethodPost, "/api/apps", map[string]string{
		"rssUrl": "",
	})
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for empty rssUrl, got %d", rr.Code)
	}
}

func TestAddApp_MalformedJSON(t *testing.T) {
	h := newTestHandler(t)
	req := httptest.NewRequest(http.MethodPost, "/api/apps", bytes.NewBufferString("not json"))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for malformed JSON, got %d", rr.Code)
	}
}

// ---- DELETE /api/apps/:id ---------------------------------------------------

func TestDeleteApp_NotFound(t *testing.T) {
	h := newTestHandler(t)
	rr := doRequest(t, h, http.MethodDelete, "/api/apps/99999", nil)
	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404 for unknown app, got %d", rr.Code)
	}
}

func TestDeleteApp_RemovesApp(t *testing.T) {
	h := newTestHandler(t)

	// Seed an app directly via the store (bypasses Apple API).
	app := store.App{
		ID:     "42",
		Name:   "TestApp",
		RSSUrl: "https://itunes.apple.com/us/rss/customerreviews/id=42/json",
	}
	if err := h.store.AddApp(app); err != nil {
		t.Fatalf("AddApp: %v", err)
	}

	rr := doRequest(t, h, http.MethodDelete, "/api/apps/42", nil)
	if rr.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", rr.Code)
	}
	if h.store.AppExists("42") {
		t.Error("expected app to be removed after DELETE")
	}
}

// ---- GET /api/apps/:id/reviews ----------------------------------------------

func TestGetReviews_UnknownApp(t *testing.T) {
	h := newTestHandler(t)
	rr := doRequest(t, h, http.MethodGet, "/api/apps/99999/reviews", nil)
	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
}

func TestGetReviews_ReturnsWindowedResults(t *testing.T) {
	h := newTestHandler(t)

	app := store.App{
		ID: "99", Name: "App",
		RSSUrl: "https://itunes.apple.com/us/rss/customerreviews/id=99/json",
	}
	if err := h.store.AddApp(app); err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()
	reviews := []store.Review{
		{ID: "r1", AppID: "99", Score: 5, Title: "Good", Body: "body", Author: "a", SubmittedAt: now.Add(-10 * time.Hour)},
		{ID: "r2", AppID: "99", Score: 1, Title: "Bad", Body: "body", Author: "b", SubmittedAt: now.Add(-100 * time.Hour)},
	}
	if _, err := h.store.MergeReviews("99", reviews); err != nil {
		t.Fatal(err)
	}

	// Default 48h window — only r1 should appear.
	rr := doRequest(t, h, http.MethodGet, "/api/apps/99/reviews?window=48", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var result []store.Review
	if err := json.NewDecoder(rr.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 {
		t.Errorf("expected 1 review in 48h window, got %d", len(result))
	}
	if result[0].ID != "r1" {
		t.Errorf("expected review r1, got %q", result[0].ID)
	}
}

func TestGetReviews_EmptyListNotNull(t *testing.T) {
	h := newTestHandler(t)
	app := store.App{
		ID: "77", Name: "Empty",
		RSSUrl: "https://itunes.apple.com/us/rss/customerreviews/id=77/json",
	}
	if err := h.store.AddApp(app); err != nil {
		t.Fatal(err)
	}

	rr := doRequest(t, h, http.MethodGet, "/api/apps/77/reviews", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	// Must be [] not null — the frontend depends on this.
	body := rr.Body.String()
	if body == "null\n" {
		t.Error("expected [] not null for empty review list")
	}
}

// ---- App ID validation (path traversal prevention) -------------------------

func TestAppID_NonNumericRejected(t *testing.T) {
	h := newTestHandler(t)
	cases := []string{
		"/api/apps/abc/reviews",
		"/api/apps/12abc/reviews",
		"/api/apps/-1/reviews",
		"/api/apps/1.0/reviews",
	}
	for _, path := range cases {
		rr := doRequest(t, h, http.MethodGet, path, nil)
		if rr.Code != http.StatusBadRequest {
			t.Errorf("path %q: expected 400 for non-numeric appId, got %d", path, rr.Code)
		}
	}
}

// ---- GET /api/health --------------------------------------------------------

// healthResponse mirrors handler.listHealth's response shape for decoding
// in tests. Keep field tags in sync with handler.go.
type healthResponse struct {
	Status          string    `json:"status"`
	LastSuccessSync time.Time `json:"last_success_sync"`
}

func TestListHealth_EmptyStoreIsSync(t *testing.T) {
	h := newTestHandler(t)
	rr := doRequest(t, h, http.MethodGet, "/api/health", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var got healthResponse
	if err := json.NewDecoder(rr.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Status != "sync" {
		t.Errorf("expected status=sync with zero apps, got %q", got.Status)
	}
	if !got.LastSuccessSync.IsZero() {
		t.Errorf("expected zero last_success_sync with no apps, got %v", got.LastSuccessSync)
	}
}

func TestListHealth_AnyErrorMakesUnsync(t *testing.T) {
	h := newTestHandler(t)
	if err := h.store.AddApp(store.App{ID: "11", Name: "Ok"}); err != nil {
		t.Fatal(err)
	}
	if err := h.store.AddApp(store.App{ID: "12", Name: "Broken"}); err != nil {
		t.Fatal(err)
	}
	ok := time.Date(2026, 5, 22, 12, 0, 0, 0, time.UTC)
	errAt := time.Date(2026, 5, 22, 11, 30, 0, 0, time.UTC)
	_ = h.store.UpdatePollerMeta("11", store.PollerMeta{Status: store.PollerStatusOK, LastRefreshAt: ok})
	_ = h.store.UpdatePollerMeta("12", store.PollerMeta{Status: store.PollerStatusError, LastRefreshAt: errAt})

	rr := doRequest(t, h, http.MethodGet, "/api/health", nil)
	var got healthResponse
	_ = json.NewDecoder(rr.Body).Decode(&got)
	if got.Status != "unsync" {
		t.Errorf("expected unsync when any app is in error, got %q", got.Status)
	}
	if !got.LastSuccessSync.Equal(ok) {
		t.Errorf("expected last_success_sync to be the latest LastRefreshAt %v, got %v", ok, got.LastSuccessSync)
	}
}

func TestListHealth_AllOkIsSync(t *testing.T) {
	h := newTestHandler(t)
	if err := h.store.AddApp(store.App{ID: "13", Name: "App"}); err != nil {
		t.Fatal(err)
	}
	when := time.Date(2026, 5, 22, 12, 0, 0, 0, time.UTC)
	_ = h.store.UpdatePollerMeta("13", store.PollerMeta{Status: store.PollerStatusOK, LastRefreshAt: when})

	rr := doRequest(t, h, http.MethodGet, "/api/health", nil)
	var got healthResponse
	_ = json.NewDecoder(rr.Body).Decode(&got)
	if got.Status != "sync" {
		t.Errorf("expected sync, got %q", got.Status)
	}
	if !got.LastSuccessSync.Equal(when) {
		t.Errorf("expected last_success_sync=%v, got %v", when, got.LastSuccessSync)
	}
}

// ---- POST /api/apps — additional edge cases ---------------------------------

// The duplicate check runs before any Apple API call, so seeding the store
// directly is enough to trigger a 409 without network access.
func TestAddApp_Duplicate(t *testing.T) {
	h := newTestHandler(t)
	if err := h.store.AddApp(store.App{
		ID:     "595",
		Name:   "App",
		RSSUrl: "https://itunes.apple.com/us/rss/customerreviews/id=595/json",
	}); err != nil {
		t.Fatal(err)
	}

	rr := doRequest(t, h, http.MethodPost, "/api/apps", map[string]string{
		"rssUrl": "https://itunes.apple.com/us/rss/customerreviews/id=595/json",
	})
	if rr.Code != http.StatusConflict {
		t.Errorf("expected 409 for duplicate app, got %d", rr.Code)
	}
}

func TestAddApp_BodyTooLarge(t *testing.T) {
	h := newTestHandler(t)
	large := strings.Repeat("x", (1<<13)+1) // exceeds maxRequestBodyBytes (8 KB)
	req := httptest.NewRequest(http.MethodPost, "/api/apps", strings.NewReader(large))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for oversized request body, got %d", rr.Code)
	}
}

// ---- DELETE /api/apps/:id — non-numeric ID ----------------------------------

func TestDeleteApp_NonNumericID(t *testing.T) {
	h := newTestHandler(t)
	cases := []string{
		"/api/apps/abc",
		"/api/apps/-1",
		"/api/apps/1.5",
	}
	for _, path := range cases {
		rr := doRequest(t, h, http.MethodDelete, path, nil)
		if rr.Code != http.StatusBadRequest {
			t.Errorf("DELETE %q: expected 400 for non-numeric appId, got %d", path, rr.Code)
		}
	}
}

// ---- GET /api/apps/:id/reviews — custom window ------------------------------

func TestGetReviews_CustomWindow(t *testing.T) {
	h := newTestHandler(t)
	if err := h.store.AddApp(store.App{
		ID:     "22",
		Name:   "App",
		RSSUrl: "https://itunes.apple.com/us/rss/customerreviews/id=22/json",
	}); err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()
	reviews := []store.Review{
		{ID: "r1", AppID: "22", Score: 5, Title: "T", Body: "b", Author: "a", SubmittedAt: now.Add(-10 * time.Hour)},  // inside 24h
		{ID: "r2", AppID: "22", Score: 4, Title: "T", Body: "b", Author: "a", SubmittedAt: now.Add(-30 * time.Hour)},  // outside 24h, inside 48h
		{ID: "r3", AppID: "22", Score: 3, Title: "T", Body: "b", Author: "a", SubmittedAt: now.Add(-100 * time.Hour)}, // outside both
	}
	if _, err := h.store.MergeReviews("22", reviews); err != nil {
		t.Fatal(err)
	}

	rr := doRequest(t, h, http.MethodGet, "/api/apps/22/reviews?window=24", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var result []store.Review
	if err := json.NewDecoder(rr.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 {
		t.Errorf("expected 1 review in 24h window, got %d", len(result))
	}
	if len(result) > 0 && result[0].ID != "r1" {
		t.Errorf("expected review r1 in window, got %q", result[0].ID)
	}
}

// ---- CORS -------------------------------------------------------------------

// withCORS must set the CORS header on every non-OPTIONS response, not just preflight.
func TestCORS_Header_OnRegularResponse(t *testing.T) {
	h := newTestHandler(t)
	rr := doRequest(t, h, http.MethodGet, "/api/apps", nil)
	if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Errorf("expected Access-Control-Allow-Origin: * on regular response, got %q", got)
	}
}

func TestCORS_Preflight(t *testing.T) {
	h := newTestHandler(t)
	req := httptest.NewRequest(http.MethodOptions, "/api/apps", nil)
	rr := httptest.NewRecorder()
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Errorf("expected 204 for OPTIONS preflight, got %d", rr.Code)
	}
	if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Errorf("expected CORS header *, got %q", got)
	}
}
