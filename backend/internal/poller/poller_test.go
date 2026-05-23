package poller

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"runway/reviews/internal/store"
)

// Minimal valid Apple RSS JSON responses used by mock feed servers.
const (
	emptyFeedJSON = `{"feed":{"author":{"name":{"label":"App"}},"entry":[]}}`

	// One review with a past date — accepted unconditionally because the poller
	// passes a zero LastRefreshAt (no since filter) on the first poll.
	oneReviewFeedJSON = `{"feed":{"author":{"name":{"label":"App"}},"entry":[` +
		`{"id":{"label":"https://itunes.apple.com/us/review/id111?type=Purple+Software"},` +
		`"title":{"label":"Great"},"content":{"label":"body"},"im:rating":{"label":"5"},` +
		`"im:version":{"label":"1.0"},"author":{"name":{"label":"tester"},"uri":{"label":""}},` +
		`"updated":{"label":"2024-01-01T00:00:00Z"}}]}}`
)

func newTestStore(t *testing.T) *store.Store {
	t.Helper()
	s := store.New(t.TempDir())
	if err := s.Load(); err != nil {
		t.Fatalf("store.Load: %v", err)
	}
	return s
}

func feedServer(t *testing.T, body string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
}

// countingFeedServer wraps a feed server that counts how many requests it
// has received. Lets tests assert that a no-op PollNow truly did not poll.
func countingFeedServer(t *testing.T) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var count atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		count.Add(1)
		_, _ = w.Write([]byte(emptyFeedJSON))
	}))
	return ts, &count
}

// waitForStatus polls the store until the named app reaches the expected
// status or the timeout elapses. The initial poll runs immediately on
// Start, so transitions happen within a few hundred ms — no time.Sleep
// guesswork.
func waitForStatus(t *testing.T, s *store.Store, appID string, want store.PollerStatus, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if app, ok := s.GetApp(appID); ok && app.Poller.Status == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	app, _ := s.GetApp(appID)
	t.Fatalf("timed out waiting for app %s to reach status %q (current: %q)", appID, want, app.Poller.Status)
}

// TestStart_ImmediatelyPollsAllApps verifies Start runs the first poll
// without waiting for the ticker — both apps flip to status=ok well
// before the 1-hour interval elapses.
func TestStart_ImmediatelyPollsAllApps(t *testing.T) {
	ts := feedServer(t, emptyFeedJSON)
	defer ts.Close()

	s := newTestStore(t)
	if err := s.AddApp(store.App{ID: "10", Name: "App1", RSSUrl: ts.URL + "/json"}); err != nil {
		t.Fatal(err)
	}
	if err := s.AddApp(store.App{ID: "11", Name: "App2", RSSUrl: ts.URL + "/json"}); err != nil {
		t.Fatal(err)
	}

	m := New(s, time.Hour)
	m.Start()
	defer m.Stop()

	waitForStatus(t, s, "10", store.PollerStatusOK, 2*time.Second)
	waitForStatus(t, s, "11", store.PollerStatusOK, 2*time.Second)
}

// TestStart_Idempotent verifies a second Start while running is a no-op:
// the first context is preserved.
func TestStart_Idempotent(t *testing.T) {
	ts := feedServer(t, emptyFeedJSON)
	defer ts.Close()

	s := newTestStore(t)
	m := New(s, time.Hour)
	m.Start()
	defer m.Stop()

	firstCtx := m.ctx
	m.Start() // second call must NOT replace the context
	if m.ctx != firstCtx {
		t.Errorf("Start() replaced the manager context on a second call — should be idempotent")
	}
}

// TestStop_BeforeStart_NoOp guards against a panic if Stop is called on a
// manager that was never started (e.g., a fast-fail in main before Start).
func TestStop_BeforeStart_NoOp(t *testing.T) {
	s := newTestStore(t)
	m := New(s, time.Hour)
	m.Stop() // must not panic
}

// TestStop_BlocksUntilInFlightPollExits is the load-bearing concurrency
// guarantee: after Stop returns, no goroutine started by the manager is
// still running. The test holds the feed handler open with a channel,
// triggers Stop, and verifies Stop returned only after the run loop
// released its resources.
func TestStop_BlocksUntilInFlightPollExits(t *testing.T) {
	entered := make(chan struct{}, 1)
	unblock := make(chan struct{})

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		select {
		case entered <- struct{}{}:
		default:
		}
		select {
		case <-unblock:
		case <-time.After(5 * time.Second):
		}
		_, _ = w.Write([]byte(emptyFeedJSON))
	}))
	defer ts.Close()
	defer close(unblock)

	s := newTestStore(t)
	if err := s.AddApp(store.App{ID: "20", Name: "App", RSSUrl: ts.URL + "/json"}); err != nil {
		t.Fatal(err)
	}

	m := New(s, time.Hour)
	m.Start()

	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("poller did not start the feed call within timeout")
	}

	stopped := make(chan struct{})
	go func() {
		m.Stop()
		close(stopped)
	}()
	select {
	case <-stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("Stop did not return within timeout after context cancel")
	}

	select {
	case <-m.done:
	default:
		t.Error("Stop returned but done channel is not closed")
	}
}

// TestPollErrorStatus verifies a non-200 feed (after the retry exhausts)
// transitions poller status to error. maxAttempts=3, retryBackoff=2s → up
// to ~4 s of wall time before status flips.
func TestPollErrorStatus(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer ts.Close()

	s := newTestStore(t)
	if err := s.AddApp(store.App{ID: "30", Name: "App", RSSUrl: ts.URL + "/json"}); err != nil {
		t.Fatal(err)
	}

	m := New(s, time.Hour)
	m.Start()
	defer m.Stop()

	waitForStatus(t, s, "30", store.PollerStatusError, 8*time.Second)
}

// TestPollSuccess_MergesReviews verifies a successful poll persists reviews
// and flips status to ok.
func TestPollSuccess_MergesReviews(t *testing.T) {
	ts := feedServer(t, oneReviewFeedJSON)
	defer ts.Close()

	s := newTestStore(t)
	if err := s.AddApp(store.App{ID: "40", Name: "App", RSSUrl: ts.URL + "/json"}); err != nil {
		t.Fatal(err)
	}

	m := New(s, time.Hour)
	m.Start()
	defer m.Stop()

	waitForStatus(t, s, "40", store.PollerStatusOK, 2*time.Second)

	reviews, err := s.GetReviews("40", time.Time{})
	if err != nil {
		t.Fatalf("GetReviews: %v", err)
	}
	if len(reviews) != 1 {
		t.Errorf("expected 1 review after poll, got %d", len(reviews))
	}
}

// TestPollNow_RunsPollWithoutWaitingForTicker covers the AddApp flow:
// the handler kicks PollNow so a newly added app's reviews land in the
// store without waiting a full ticker interval.
func TestPollNow_RunsPollWithoutWaitingForTicker(t *testing.T) {
	ts := feedServer(t, oneReviewFeedJSON)
	defer ts.Close()

	s := newTestStore(t)
	m := New(s, time.Hour)
	m.Start()
	defer m.Stop()

	// Add the app AFTER Start so the immediate first poll ran on an empty
	// store. PollNow is now the only path that can populate it before the
	// 1 h ticker fires.
	if err := s.AddApp(store.App{ID: "50", Name: "App", RSSUrl: ts.URL + "/json"}); err != nil {
		t.Fatal(err)
	}
	m.PollNow("50")

	waitForStatus(t, s, "50", store.PollerStatusOK, 2*time.Second)
	reviews, _ := s.GetReviews("50", time.Time{})
	if len(reviews) != 1 {
		t.Errorf("expected 1 review after PollNow, got %d", len(reviews))
	}
}

// TestPollNow_BeforeStart_NoOp protects against a startup-order bug where
// a handler accepts a request before the manager is started.
func TestPollNow_BeforeStart_NoOp(t *testing.T) {
	ts, count := countingFeedServer(t)
	defer ts.Close()

	s := newTestStore(t)
	if err := s.AddApp(store.App{ID: "60", Name: "App", RSSUrl: ts.URL + "/json"}); err != nil {
		t.Fatal(err)
	}

	m := New(s, time.Hour)
	m.PollNow("60") // before Start — must not panic and must not poll

	time.Sleep(50 * time.Millisecond)
	if got := count.Load(); got != 0 {
		t.Errorf("PollNow before Start hit the feed %d times, want 0", got)
	}
}

// TestPollNow_AfterStop_NoOp guards the shutdown-race scenario: a
// PollNow call that loses the race against process shutdown must NOT
// spawn a goroutine that outlives Stop.
func TestPollNow_AfterStop_NoOp(t *testing.T) {
	ts, count := countingFeedServer(t)
	defer ts.Close()

	s := newTestStore(t)
	if err := s.AddApp(store.App{ID: "70", Name: "App", RSSUrl: ts.URL + "/json"}); err != nil {
		t.Fatal(err)
	}

	m := New(s, time.Hour)
	m.Start()
	waitForStatus(t, s, "70", store.PollerStatusOK, 2*time.Second)
	m.Stop()
	baseline := count.Load()

	for i := 0; i < 5; i++ {
		m.PollNow("70")
	}
	time.Sleep(100 * time.Millisecond)

	if got := count.Load(); got != baseline {
		t.Errorf("PollNow after Stop made %d extra feed hits, want 0", got-baseline)
	}
}
