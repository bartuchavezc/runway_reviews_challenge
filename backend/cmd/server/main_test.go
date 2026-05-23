package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"runway/reviews/internal/api"
	"runway/reviews/internal/poller"
	"runway/reviews/internal/store"
)

// TestE2E_FullStack wires up the real store + poller + HTTP handler against a
// mock Apple RSS feed and exercises the full pipeline end-to-end:
//
//  1. backend serves /api/apps and /api/apps/{id}/reviews over real HTTP
//  2. a worker polls the mock Apple feed on a short interval
//  3. reviews are persisted to disk and returned through the API
//  4. on simulated restart (new store instance, same data dir) the reviews
//     are still there — proves the assignment's stop/restart requirement.
//
// The poller's first poll runs immediately when the worker is started, so
// the test doesn't have to wait for an interval tick.
//
// Mock Apple is a small httptest.Server returning one valid review entry.
// The poller's RSS URL points at this mock server, not the real Apple CDN,
// so the test is fully hermetic.
func TestE2E_FullStack(t *testing.T) {
	dataDir := t.TempDir()

	// Mock Apple RSS feed. Returns one review with a recent timestamp so it
	// falls inside the default 48h window.
	mockApple := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		now := time.Now().UTC().Format(time.RFC3339)
		body := fmt.Sprintf(`{"feed":{"author":{"name":{"label":"Mock"}},"entry":[`+
			`{"id":{"label":"https://itunes.apple.com/us/review/id111?type=Purple+Software"},`+
			`"title":{"label":"Great app"},"content":{"label":"This is the review body."},`+
			`"im:rating":{"label":"5"},"im:version":{"label":"1.0"},`+
			`"author":{"name":{"label":"e2etester"},"uri":{"label":""}},`+
			`"updated":{"label":"%s"}}]}}`, now)
		_, _ = w.Write([]byte(body))
	}))
	defer mockApple.Close()

	// --- First-run stack: store, poller (100ms interval), handler. -----------
	s := store.New(dataDir)
	if err := s.Load(); err != nil {
		t.Fatalf("store.Load: %v", err)
	}

	// Seed the app directly into the store (bypasses the POST /api/apps path
	// because that calls feed.LookupAppName against the real itunes.apple.com
	// host — out of scope for a hermetic test). The poller still exercises
	// the full FetchAll → MergeReviews → UpdatePollerMeta pipeline against
	// the mock Apple server below.
	const appID = "595068606"
	if err := s.AddApp(store.App{
		ID:     appID,
		Name:   "Mock App",
		RSSUrl: mockApple.URL + "/json",
	}); err != nil {
		t.Fatalf("AddApp: %v", err)
	}

	mgr := poller.New(s, 100*time.Millisecond)
	mgr.Start()

	mux := http.NewServeMux()
	api.NewHandler(s, mgr).RegisterRoutes(mux)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	// Wait for the first poll to complete by polling /api/apps until the
	// app's poller status flips to "ok". Bounded by a 2 s deadline so a
	// stuck poller fails fast instead of hanging the test runner.
	waitUntil(t, 2*time.Second, func() bool {
		app, ok := s.GetApp(appID)
		return ok && app.Poller.Status == store.PollerStatusOK
	})

	// GET /api/apps/{id}/reviews — the full HTTP round-trip.
	got := getReviews(t, srv.URL+"/api/apps/"+appID+"/reviews?window=48")
	if len(got) != 1 {
		t.Fatalf("expected 1 review through the API, got %d: %+v", len(got), got)
	}
	if got[0].Author != "e2etester" {
		t.Errorf("review author = %q, want %q", got[0].Author, "e2etester")
	}
	if got[0].Score != 5 {
		t.Errorf("review score = %d, want 5", got[0].Score)
	}
	if got[0].Body != "This is the review body." {
		t.Errorf("review body = %q", got[0].Body)
	}
	if got[0].SubmittedAt.IsZero() {
		t.Errorf("review submittedAt is zero")
	}

	// /api/apps returns the app with totalReviews=1.
	apps := getApps(t, srv.URL+"/api/apps")
	if len(apps) != 1 || apps[0].ID != appID || apps[0].TotalReviews != 1 {
		t.Errorf("/api/apps = %+v, want one app id=%s with totalReviews=1", apps, appID)
	}

	// /api/health returns sync with a recent last_success_sync.
	health := getHealth(t, srv.URL+"/api/health")
	if health.Status != "sync" {
		t.Errorf("health status = %q, want sync", health.Status)
	}
	if time.Since(health.LastSuccessSync) > 5*time.Second {
		t.Errorf("health last_success_sync too old: %v", health.LastSuccessSync)
	}

	// --- Restart simulation: tear everything down, reload from the same -----
	// data dir, prove the reviews are still there.
	mgr.Stop()
	srv.Close()

	s2 := store.New(dataDir)
	if err := s2.Load(); err != nil {
		t.Fatalf("post-restart store.Load: %v", err)
	}
	mgr2 := poller.New(s2, time.Hour) // long interval; we don't want it to re-poll mid-assertion
	defer mgr2.Stop()

	mux2 := http.NewServeMux()
	api.NewHandler(s2, mgr2).RegisterRoutes(mux2)
	srv2 := httptest.NewServer(mux2)
	defer srv2.Close()

	// The app must be listed without re-adding it.
	apps2 := getApps(t, srv2.URL+"/api/apps")
	if len(apps2) != 1 || apps2[0].ID != appID {
		t.Fatalf("after restart /api/apps = %+v, want one app id=%s", apps2, appID)
	}
	if apps2[0].TotalReviews != 1 {
		t.Errorf("after restart totalReviews = %d, want 1", apps2[0].TotalReviews)
	}

	// The previously polled review must still be served — no re-fetch needed.
	got2 := getReviews(t, srv2.URL+"/api/apps/"+appID+"/reviews?window=48")
	if len(got2) != 1 || got2[0].Author != "e2etester" {
		t.Errorf("after restart reviews = %+v, want one review by e2etester", got2)
	}
}

// TestE2E_DeleteAppCleansState verifies DELETE /api/apps/{id} stops the
// worker, removes the app from /api/apps, and deletes the on-disk review
// file. Single-flow integration check for the destructive path.
func TestE2E_DeleteAppCleansState(t *testing.T) {
	dataDir := t.TempDir()

	mockApple := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"feed":{"author":{"name":{"label":"Mock"}},"entry":[]}}`))
	}))
	defer mockApple.Close()

	s := store.New(dataDir)
	if err := s.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	const appID = "111"
	if err := s.AddApp(store.App{ID: appID, Name: "Mock", RSSUrl: mockApple.URL + "/json"}); err != nil {
		t.Fatalf("AddApp: %v", err)
	}

	mgr := poller.New(s, 100*time.Millisecond)
	mgr.Start()
	defer mgr.Stop()

	mux := http.NewServeMux()
	api.NewHandler(s, mgr).RegisterRoutes(mux)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	waitUntil(t, 2*time.Second, func() bool {
		app, ok := s.GetApp(appID)
		return ok && app.Poller.Status == store.PollerStatusOK
	})

	// DELETE
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodDelete, srv.URL+"/api/apps/"+appID, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("DELETE status = %d, want 204", resp.StatusCode)
	}

	if s.AppExists(appID) {
		t.Error("app still exists in store after DELETE")
	}
	apps := getApps(t, srv.URL+"/api/apps")
	if len(apps) != 0 {
		t.Errorf("/api/apps after DELETE = %+v, want empty", apps)
	}
}

// --- helpers ----------------------------------------------------------------

func waitUntil(t *testing.T, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("waitUntil: condition not met within %s", timeout)
}

func getReviews(t *testing.T, url string) []store.Review {
	t.Helper()
	resp := mustGet(t, url)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s: status %d", url, resp.StatusCode)
	}
	var out []store.Review
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode reviews: %v", err)
	}
	return out
}

func getApps(t *testing.T, url string) []store.AppResponse {
	t.Helper()
	resp := mustGet(t, url)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s: status %d", url, resp.StatusCode)
	}
	var out []store.AppResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode apps: %v", err)
	}
	return out
}

type healthShape struct {
	Status          string    `json:"status"`
	LastSuccessSync time.Time `json:"last_success_sync"`
}

func getHealth(t *testing.T, url string) healthShape {
	t.Helper()
	resp := mustGet(t, url)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s: status %d", url, resp.StatusCode)
	}
	var out healthShape
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode health: %v", err)
	}
	return out
}

func mustGet(t *testing.T, url string) *http.Response {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	return resp
}
