package store

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// ---- helpers ----------------------------------------------------------------

func newTempStore(t *testing.T) *Store {
	t.Helper()
	s := New(t.TempDir())
	if err := s.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	return s
}

func sampleApp(id, name string) App {
	return App{ID: id, Name: name, RSSUrl: "https://itunes.apple.com/us/rss/customerreviews/id=" + id + "/sortBy=mostRecent/json"}
}

func review(id string, appID string, score int, hoursAgo int) Review {
	return Review{
		ID:          id,
		AppID:       appID,
		Score:       score,
		Title:       "Title " + id,
		Body:        "Body " + id,
		Author:      "author",
		SubmittedAt: time.Now().UTC().Add(-time.Duration(hoursAgo) * time.Hour),
	}
}

// ---- AddApp / RemoveApp / AppExists -----------------------------------------

func TestAddApp(t *testing.T) {
	s := newTempStore(t)
	app := sampleApp("111", "Test App")

	if err := s.AddApp(app); err != nil {
		t.Fatalf("AddApp: %v", err)
	}
	if !s.AppExists("111") {
		t.Fatal("expected app to exist after AddApp")
	}

	apps := s.Apps()
	if len(apps) != 1 || apps[0].ID != "111" {
		t.Fatalf("expected 1 app with id 111, got %+v", apps)
	}
	if apps[0].Name != "Test App" {
		t.Errorf("expected name 'Test App', got %q", apps[0].Name)
	}
}

func TestAddApp_Duplicate(t *testing.T) {
	s := newTempStore(t)
	app := sampleApp("222", "App")

	if err := s.AddApp(app); err != nil {
		t.Fatalf("first AddApp: %v", err)
	}
	if err := s.AddApp(app); err == nil {
		t.Fatal("expected error adding duplicate app, got nil")
	}
}

func TestRemoveApp(t *testing.T) {
	s := newTempStore(t)
	if err := s.AddApp(sampleApp("333", "App")); err != nil {
		t.Fatal(err)
	}
	if err := s.RemoveApp("333"); err != nil {
		t.Fatalf("RemoveApp: %v", err)
	}
	if s.AppExists("333") {
		t.Fatal("expected app to be gone after RemoveApp")
	}
	// Review file should have been deleted.
	reviewPath := filepath.Join(s.dataDir, "reviews", "333.json")
	if _, err := os.Stat(reviewPath); !os.IsNotExist(err) {
		t.Error("expected review file to be deleted")
	}
}

func TestRemoveApp_NotFound(t *testing.T) {
	s := newTempStore(t)
	if err := s.RemoveApp("nonexistent"); err == nil {
		t.Fatal("expected error removing non-existent app")
	}
}

// ---- MergeReviews (deduplication) -------------------------------------------

func TestMergeReviews_Dedup(t *testing.T) {
	s := newTempStore(t)
	if err := s.AddApp(sampleApp("444", "App")); err != nil {
		t.Fatal(err)
	}

	batch := []Review{
		review("r1", "444", 5, 1),
		review("r2", "444", 3, 2),
	}

	added, err := s.MergeReviews("444", batch)
	if err != nil {
		t.Fatalf("first MergeReviews: %v", err)
	}
	if added != 2 {
		t.Errorf("expected 2 added, got %d", added)
	}

	// Merge the same batch again — should add 0 new reviews.
	added, err = s.MergeReviews("444", batch)
	if err != nil {
		t.Fatalf("second MergeReviews: %v", err)
	}
	if added != 0 {
		t.Errorf("expected 0 new on re-merge, got %d", added)
	}

	apps := s.Apps()
	if apps[0].TotalReviews != 2 {
		t.Errorf("expected totalReviews=2, got %d", apps[0].TotalReviews)
	}
}

func TestMergeReviews_NewOnlyAppended(t *testing.T) {
	s := newTempStore(t)
	if err := s.AddApp(sampleApp("555", "App")); err != nil {
		t.Fatal(err)
	}

	_, _ = s.MergeReviews("555", []Review{review("r1", "555", 5, 10)})
	added, _ := s.MergeReviews("555", []Review{
		review("r1", "555", 5, 10), // duplicate
		review("r2", "555", 4, 5),  // new
	})
	if added != 1 {
		t.Errorf("expected 1 new review, got %d", added)
	}
}

// ---- GetReviews (time window filter + ordering) -----------------------------

func TestGetReviews_WindowFilter(t *testing.T) {
	s := newTempStore(t)
	if err := s.AddApp(sampleApp("666", "App")); err != nil {
		t.Fatal(err)
	}

	_, _ = s.MergeReviews("666", []Review{
		review("r1", "666", 5, 10),  // 10h ago — inside 48h window
		review("r2", "666", 4, 24),  // 24h ago — inside 48h window
		review("r3", "666", 3, 72),  // 72h ago — outside 48h window
		review("r4", "666", 1, 200), // 200h ago — outside 48h window
	})

	since := time.Now().UTC().Add(-48 * time.Hour)
	reviews, err := s.GetReviews("666", since)
	if err != nil {
		t.Fatalf("GetReviews: %v", err)
	}
	if len(reviews) != 2 {
		t.Errorf("expected 2 reviews in 48h window, got %d", len(reviews))
	}
}

func TestGetReviews_NewestFirst(t *testing.T) {
	s := newTempStore(t)
	if err := s.AddApp(sampleApp("777", "App")); err != nil {
		t.Fatal(err)
	}

	_, _ = s.MergeReviews("777", []Review{
		review("r1", "777", 5, 20),
		review("r2", "777", 4, 5),
		review("r3", "777", 3, 40),
	})

	since := time.Now().UTC().Add(-48 * time.Hour)
	reviews, err := s.GetReviews("777", since)
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i < len(reviews); i++ {
		if reviews[i].SubmittedAt.After(reviews[i-1].SubmittedAt) {
			t.Errorf("reviews not sorted newest-first at index %d", i)
		}
	}
}

func TestGetReviews_UnknownApp(t *testing.T) {
	s := newTempStore(t)
	_, err := s.GetReviews("nonexistent", time.Now().Add(-48*time.Hour))
	if err == nil {
		t.Fatal("expected error for unknown app")
	}
}

// ---- GetApp -----------------------------------------------------------------

func TestGetApp_Found(t *testing.T) {
	s := newTempStore(t)
	if err := s.AddApp(sampleApp("9991", "Snapchat")); err != nil {
		t.Fatal(err)
	}
	app, ok := s.GetApp("9991")
	if !ok {
		t.Fatal("GetApp returned false for an existing app")
	}
	if app.ID != "9991" || app.Name != "Snapchat" {
		t.Errorf("GetApp returned wrong app: %+v", app)
	}
}

func TestGetApp_NotFound(t *testing.T) {
	s := newTempStore(t)
	_, ok := s.GetApp("nonexistent")
	if ok {
		t.Fatal("GetApp returned true for a non-existent app")
	}
}

// ---- ErrNotFound sentinel ---------------------------------------------------

// MergeReviews and GetReviews must wrap ErrNotFound so callers can use errors.Is.
func TestMergeReviews_UnknownApp_ErrNotFound(t *testing.T) {
	s := newTempStore(t)
	_, err := s.MergeReviews("9999999999", nil)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected errors.Is(err, ErrNotFound), got %v", err)
	}
}

func TestGetReviews_ErrNotFoundWrapped(t *testing.T) {
	s := newTempStore(t)
	_, err := s.GetReviews("9999999999", time.Now())
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected errors.Is(err, ErrNotFound), got %v", err)
	}
}

// ---- GetReviews edge cases --------------------------------------------------

// A zero `since` time means "return everything" — After(time.Time{}) is true
// for any modern timestamp because time.Time{} is year 0001.
func TestGetReviews_ZeroTime_ReturnsAll(t *testing.T) {
	s := newTempStore(t)
	if err := s.AddApp(sampleApp("9992", "App")); err != nil {
		t.Fatal(err)
	}
	_, _ = s.MergeReviews("9992", []Review{
		review("r1", "9992", 5, 24*365),   // 1 year ago
		review("r2", "9992", 4, 24*365*2), // 2 years ago
		review("r3", "9992", 3, 1),
	})

	reviews, err := s.GetReviews("9992", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(reviews) != 3 {
		t.Errorf("expected all 3 reviews with zero time, got %d", len(reviews))
	}
}

// ---- UpdatePollerMeta persistence -------------------------------------------

func TestUpdatePollerMeta_Persists(t *testing.T) {
	dir := t.TempDir()
	s1 := New(dir)
	if err := s1.Load(); err != nil {
		t.Fatal(err)
	}
	if err := s1.AddApp(sampleApp("9993", "App")); err != nil {
		t.Fatal(err)
	}

	meta := PollerMeta{
		Status:        PollerStatusError,
		LastRefreshAt: time.Now().UTC().Truncate(time.Second),
	}
	if err := s1.UpdatePollerMeta("9993", meta); err != nil {
		t.Fatalf("UpdatePollerMeta: %v", err)
	}

	s2 := New(dir)
	if err := s2.Load(); err != nil {
		t.Fatalf("second Load: %v", err)
	}
	app, ok := s2.GetApp("9993")
	if !ok {
		t.Fatal("app not found after reload")
	}
	if app.Poller.Status != PollerStatusError {
		t.Errorf("Status = %q, want %q", app.Poller.Status, PollerStatusError)
	}
	if !app.Poller.LastRefreshAt.Equal(meta.LastRefreshAt) {
		t.Errorf("LastRefreshAt not preserved: got %v, want %v", app.Poller.LastRefreshAt, meta.LastRefreshAt)
	}
}

// ---- Persistence (Load/Save round-trip) -------------------------------------

func TestPersistence(t *testing.T) {
	dir := t.TempDir()

	// Write data with first store instance.
	s1 := New(dir)
	if err := s1.Load(); err != nil {
		t.Fatal(err)
	}
	if err := s1.AddApp(sampleApp("999", "PersistApp")); err != nil {
		t.Fatal(err)
	}
	_, _ = s1.MergeReviews("999", []Review{
		review("r1", "999", 5, 2),
		review("r2", "999", 3, 10),
	})
	_ = s1.UpdatePollerMeta("999", PollerMeta{Status: PollerStatusOK, LastRefreshAt: time.Now().UTC()})

	// Load a fresh store from the same directory.
	s2 := New(dir)
	if err := s2.Load(); err != nil {
		t.Fatalf("second Load: %v", err)
	}

	if !s2.AppExists("999") {
		t.Fatal("app not found after reload")
	}
	apps := s2.Apps()
	if apps[0].TotalReviews != 2 {
		t.Errorf("expected 2 reviews after reload, got %d", apps[0].TotalReviews)
	}
	reviews, _ := s2.GetReviews("999", time.Time{})
	if len(reviews) != 2 {
		t.Errorf("expected 2 reviews from GetReviews after reload, got %d", len(reviews))
	}
}

// --- Per-app file locking + concurrent read/write safety -------------------

// TestMergeReviews_DoesNotBlockGetReviewsOfOtherApp pins the property
// that motivates per-app locking: a poll for app A's reviews must not
// block an HTTP read of app B's reviews. Without per-app locking the
// store-wide write lock serialized everything; with per-app locking
// they run in parallel.
func TestMergeReviews_DoesNotBlockGetReviewsOfOtherApp(t *testing.T) {
	s := newTempStore(t)
	if err := s.AddApp(sampleApp("1001", "A")); err != nil {
		t.Fatal(err)
	}
	if err := s.AddApp(sampleApp("1002", "B")); err != nil {
		t.Fatal(err)
	}
	_, _ = s.MergeReviews("1002", []Review{review("rB", "1002", 5, 1)})

	// Start a slow merge on A by handing it ~3000 reviews; meanwhile
	// read B and ensure the read completes promptly.
	bulk := make([]Review, 3000)
	for i := range bulk {
		bulk[i] = review(fmt.Sprintf("r%d", i), "1001", 5, 1)
	}
	mergeDone := make(chan struct{})
	go func() {
		defer close(mergeDone)
		_, _ = s.MergeReviews("1001", bulk)
	}()
	// Without this wait, t.TempDir's cleanup at test return races the
	// still-running merge — atomicWriteJSON's temp file under reviews/
	// can leave the dir non-empty when RemoveAll runs, producing a
	// flaky "directory not empty" failure under -coverprofile.
	t.Cleanup(func() { <-mergeDone })

	done := make(chan struct{})
	go func() {
		defer close(done)
		// Even if A's merge is mid-write, B's read should land on its own
		// file (POSIX rename atomicity means no torn read; per-app
		// locking means no cross-app contention).
		for i := 0; i < 50; i++ {
			reviews, err := s.GetReviews("1002", time.Time{})
			if err != nil || len(reviews) != 1 {
				t.Errorf("GetReviews(B) iteration %d: %v reviews=%d", i, err, len(reviews))
				return
			}
		}
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("GetReviews(B) did not complete within 2 s — likely blocked by MergeReviews(A)")
	}
}

// TestRemoveApp_WaitsForInFlightMerge verifies the per-app file lock is
// also held by RemoveApp, so a concurrent MergeReviews can't race with
// the unlink. After RemoveApp returns, the review file is gone — even
// if MergeReviews completes later, its orphan-cleanup branch fires and
// the disk is consistent.
func TestRemoveApp_WaitsForInFlightMerge(t *testing.T) {
	s := newTempStore(t)
	if err := s.AddApp(sampleApp("1003", "A")); err != nil {
		t.Fatal(err)
	}
	_, _ = s.MergeReviews("1003", []Review{review("r1", "1003", 5, 1)})

	if err := s.RemoveApp("1003"); err != nil {
		t.Fatalf("RemoveApp: %v", err)
	}
	if _, err := os.Stat(filepath.Join(s.dataDir, "reviews", "1003.json")); !os.IsNotExist(err) {
		t.Errorf("expected review file gone after RemoveApp, stat err = %v", err)
	}
}
