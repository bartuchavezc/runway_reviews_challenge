package poller

import (
	"context"
	"log"
	"sync"
	"time"

	"runway/reviews/internal/feed"
	"runway/reviews/internal/store"
)

// Manager polls every tracked app on a single ticker, sequentially.
type Manager struct {
	store    *store.Store
	interval time.Duration

	mu     sync.Mutex
	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}
	wg     sync.WaitGroup // tracks in-flight PollNow goroutines
}

func New(s *store.Store, interval time.Duration) *Manager {
	return &Manager{store: s, interval: interval}
}

// Start launches the polling loop. First poll runs immediately so apps
// loaded from disk don't wait an interval after a restart; subsequent
// polls fire every `interval`. Idempotent.
func (m *Manager) Start() {
	m.mu.Lock()
	if m.ctx != nil {
		m.mu.Unlock()
		return
	}
	m.ctx, m.cancel = context.WithCancel(context.Background())
	m.done = make(chan struct{})
	m.mu.Unlock()

	log.Printf("[struct:Manager][method:Start][message:poller starting (interval %s)]", m.interval)
	go m.run()
}

// Stop cancels the polling context (aborting any in-flight HTTP call)
// and blocks until the main loop and all PollNow goroutines exit.
// Safe to call multiple times.
//
// cancel() runs inside m.mu so the PollNow ↔ Stop race is closed:
// either PollNow saw the cancelled context before its wg.Add (then it
// returned without spawning) or PollNow did wg.Add (then Stop's
// wg.Wait below blocks for it).
func (m *Manager) Stop() {
	m.mu.Lock()
	if m.cancel == nil {
		m.mu.Unlock()
		return
	}
	cancel := m.cancel
	done := m.done
	m.cancel = nil // mark stopped so a second Stop is a no-op
	cancel()
	m.mu.Unlock()
	if done != nil {
		<-done
	}
	m.wg.Wait()
	log.Printf("[struct:Manager][method:Stop][message:poller stopped]")
}

// PollNow polls a single app in a background goroutine. Used by the
// AddApp handler so a new app's reviews land without waiting for the
// next ticker. No-op if Start hasn't run or Stop has.
//
// The ctx check AND the wg.Add(1) both run under m.mu so a concurrent
// Stop can't complete wg.Wait() (with wg at 0) between our check and
// our Add — which would let a goroutine outlive Stop. Stop also cancels
// the context inside m.mu, making the two orderings race-free.
func (m *Manager) PollNow(appID string) {
	m.mu.Lock()
	if m.ctx == nil || m.ctx.Err() != nil {
		m.mu.Unlock()
		return
	}
	ctx := m.ctx
	m.wg.Add(1)
	m.mu.Unlock()
	go func() {
		defer m.wg.Done()
		m.pollApp(ctx, appID)
	}()
}

// run is the single polling goroutine. Immediate first poll, then
// ticker-driven. Exits when the context is cancelled.
func (m *Manager) run() {
	defer close(m.done)
	m.pollAll()
	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			m.pollAll()
		case <-m.ctx.Done():
			return
		}
	}
}

// pollAll polls every app in the current snapshot, sequentially. The
// ctx.Done() check between iterations lets Stop preempt a long pass.
func (m *Manager) pollAll() {
	apps := m.store.Apps()
	for _, app := range apps {
		if m.ctx.Err() != nil {
			return
		}
		m.pollApp(m.ctx, app.ID)
	}
}

func (m *Manager) pollApp(ctx context.Context, appID string) {
	if ctx.Err() != nil {
		return
	}

	// Snapshot for the latest LastRefreshAt — used as FetchAll's since hint.
	app, ok := m.store.GetApp(appID)
	if !ok {
		return // app was removed between snapshot and poll
	}

	// Bounded inline retry absorbs transient feed flakes (one-off 503, etc.)
	// so a single bad response doesn't pin the poller in "error" for a full
	// POLL_INTERVAL_MINUTES. Short backoff so a real outage exits promptly.
	const maxAttempts = 3
	const retryBackoff = 2 * time.Second
	var reviews []store.Review
	var err error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		reviews, _, err = feed.FetchAll(ctx, app.RSSUrl)
		if err == nil || ctx.Err() != nil {
			break
		}
		if attempt < maxAttempts {
			log.Printf("[struct:Manager][method:pollApp][message:transient feed error for %s (attempt %d/%d): %v]", appID, attempt, maxAttempts, err)
			select {
			case <-time.After(retryBackoff):
			case <-ctx.Done():
				return
			}
		}
	}
	if err != nil {
		if ctx.Err() != nil {
			return // cancelled — not a real error
		}
		if metaErr := m.store.UpdatePollerMeta(appID, store.PollerMeta{
			Status:        store.PollerStatusError,
			LastRefreshAt: app.Poller.LastRefreshAt,
		}); metaErr != nil {
			log.Printf("[struct:Manager][method:pollApp][message:failed to persist error status for %s: %v]", appID, metaErr)
		}
		log.Printf("[struct:Manager][method:pollApp][message:error polling %s (%s) after %d attempts: %v]", app.Name, appID, maxAttempts, err)
		return
	}

	for i := range reviews {
		reviews[i].AppID = appID
	}

	// MergeReviews is the DELETE-race boundary: if the app was removed
	// while we were fetching, it returns ErrNotFound (deleted app is
	// invisible under s.mu) and the orphan reviews drop. This is what
	// makes sequential polling safe alongside concurrent DELETE.
	added, err := m.store.MergeReviews(appID, reviews)
	if err != nil {
		log.Printf("[struct:Manager][method:pollApp][message:merge error for %s: %v]", appID, err)
		return
	}

	if metaErr := m.store.UpdatePollerMeta(appID, store.PollerMeta{
		Status:        store.PollerStatusOK,
		LastRefreshAt: time.Now().UTC(),
	}); metaErr != nil {
		log.Printf("[struct:Manager][method:pollApp][message:failed to persist ok status for %s: %v]", appID, metaErr)
	}

	if added > 0 {
		log.Printf("[struct:Manager][method:pollApp][message:app %s (%s) +%d new reviews]", app.Name, appID, added)
	} else {
		log.Printf("[struct:Manager][method:pollApp][message:app %s (%s) no new reviews]", app.Name, appID)
	}
}
