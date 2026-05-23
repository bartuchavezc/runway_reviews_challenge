package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"sync"
	"time"
)

// Defense-in-depth path-traversal check, enforced at every store-internal
// filesystem call. Belt-and-suspenders against the handler's matching regex.
var appIDOnDiskRegex = regexp.MustCompile(`^\d+$`)

func validAppID(id string) bool { return appIDOnDiskRegex.MatchString(id) }

// Store keeps app metadata in memory; reviews live on disk as
// data/reviews/{appId}.json and are read on demand so memory stays bounded.
//
// Two-layer locking: `mu` (RWMutex) guards the apps map and apps.json
// writes — brief critical sections, never held across file I/O.
// `fileLocks` is a per-app Mutex serializing writes to one app's review
// file. Reads of review files take no lock at all — atomicWriteJSON's
// POSIX temp+rename guarantees old-or-new, never torn.
type Store struct {
	mu      sync.RWMutex
	dataDir string
	apps    map[string]*App

	fileLocksMu sync.Mutex
	fileLocks   map[string]*sync.Mutex
}

func New(dataDir string) *Store {
	return &Store{
		dataDir:   dataDir,
		apps:      make(map[string]*App),
		fileLocks: make(map[string]*sync.Mutex),
	}
}

// fileLock returns the per-app mutex serializing writes to that app's
// review file. Lazily created, dropped only by RemoveApp.
func (s *Store) fileLock(appID string) *sync.Mutex {
	s.fileLocksMu.Lock()
	defer s.fileLocksMu.Unlock()
	if l, ok := s.fileLocks[appID]; ok {
		return l
	}
	l := &sync.Mutex{}
	s.fileLocks[appID] = l
	return l
}

func (s *Store) dropFileLock(appID string) {
	s.fileLocksMu.Lock()
	delete(s.fileLocks, appID)
	s.fileLocksMu.Unlock()
}

func (s *Store) Load() error {
	if err := os.MkdirAll(filepath.Join(s.dataDir, "reviews"), 0755); err != nil {
		return fmt.Errorf("create data dirs: %w", err)
	}

	appsPath := filepath.Join(s.dataDir, "apps.json")
	data, err := os.ReadFile(appsPath)
	if os.IsNotExist(err) {
		return nil // fresh start
	}
	if err != nil {
		return fmt.Errorf("read apps.json: %w", err)
	}

	var apps []App
	if err := json.Unmarshal(data, &apps); err != nil {
		return fmt.Errorf("parse apps.json: %w", err)
	}

	for i := range apps {
		s.apps[apps[i].ID] = &apps[i]
	}

	// Reconcile TotalReviews from disk — MergeReviews defers the apps.json
	// write to the next UpdatePollerMeta, so a crash between the two leaves
	// apps.json one cycle behind.
	for id, app := range s.apps {
		if reviews, err := s.loadReviewFile(id); err == nil {
			app.TotalReviews = len(reviews)
		}
	}
	return nil
}

// Apps returns a copy of the metadata; callers can't mutate internal state.
func (s *Store) Apps() []App {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]App, 0, len(s.apps))
	for _, app := range s.apps {
		result = append(result, *app)
	}
	return result
}

func (s *Store) GetApp(appID string) (App, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	app, ok := s.apps[appID]
	if !ok {
		return App{}, false
	}
	return *app, true
}

// AddApp inserts a new app and persists. Two-step rollback: if the
// review file was created but apps.json fails, drop the orphan file too.
func (s *Store) AddApp(app App) error {
	if !validAppID(app.ID) {
		return fmt.Errorf("invalid app id %q", app.ID)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.apps[app.ID]; exists {
		return fmt.Errorf("%s: %w", app.ID, ErrAlreadyExists)
	}

	app.Poller = PollerMeta{Status: PollerStatusPending}
	app.TotalReviews = 0

	s.apps[app.ID] = &app

	if err := s.writeReviewFile(app.ID, nil); err != nil {
		delete(s.apps, app.ID)
		return fmt.Errorf("create review file: %w", err)
	}
	if err := s.writeAppsFile(); err != nil {
		delete(s.apps, app.ID)
		_ = os.Remove(filepath.Join(s.dataDir, "reviews", app.ID+".json"))
		return err
	}
	return nil
}

// RemoveApp deletes the app and its review file. Holds the per-app
// file lock so a concurrent MergeReviews can't race the unlink.
func (s *Store) RemoveApp(appID string) error {
	if !validAppID(appID) {
		return fmt.Errorf("invalid app id %q", appID)
	}

	// Drain any in-flight MergeReviews before mutating state.
	fl := s.fileLock(appID)
	fl.Lock()
	defer fl.Unlock()

	s.mu.Lock()
	if _, exists := s.apps[appID]; !exists {
		s.mu.Unlock()
		return fmt.Errorf("%s: %w", appID, ErrNotFound)
	}
	delete(s.apps, appID)
	if err := os.Remove(filepath.Join(s.dataDir, "reviews", appID+".json")); err != nil && !os.IsNotExist(err) {
		s.mu.Unlock()
		return fmt.Errorf("remove review file: %w", err)
	}
	err := s.writeAppsFile()
	s.mu.Unlock()

	s.dropFileLock(appID)
	return err
}

// MergeReviews dedupes incoming by ID and writes the merged set to
// the app's review file, updating TotalReviews in memory. apps.json
// is NOT rewritten — the poller's next UpdatePollerMeta carries the
// persist (Load reconciles from disk if a crash splits the two).
// Per-app file lock serializes writes; reads are POSIX-atomic and
// take no lock.
func (s *Store) MergeReviews(appID string, incoming []Review) (int, error) {
	if !validAppID(appID) {
		return 0, fmt.Errorf("invalid app id %q", appID)
	}

	fl := s.fileLock(appID)
	fl.Lock()
	defer fl.Unlock()

	// Re-check existence after taking the file lock — RemoveApp may have run.
	s.mu.RLock()
	_, ok := s.apps[appID]
	s.mu.RUnlock()
	if !ok {
		return 0, fmt.Errorf("%s: %w", appID, ErrNotFound)
	}

	existing, err := s.loadReviewFile(appID)
	if err != nil {
		return 0, fmt.Errorf("read reviews for merge: %w", err)
	}

	seen := make(map[string]struct{}, len(existing))
	for _, r := range existing {
		seen[r.ID] = struct{}{}
	}
	added := 0
	for _, r := range incoming {
		if _, dup := seen[r.ID]; !dup {
			existing = append(existing, r)
			seen[r.ID] = struct{}{}
			added++
		}
	}

	if err := s.writeReviewFile(appID, existing); err != nil {
		return 0, err
	}

	// Update TotalReviews; if RemoveApp won the race, drop our orphan file.
	s.mu.Lock()
	app, ok := s.apps[appID]
	if !ok {
		s.mu.Unlock()
		_ = os.Remove(filepath.Join(s.dataDir, "reviews", appID+".json"))
		return 0, fmt.Errorf("%s: %w", appID, ErrNotFound)
	}
	app.TotalReviews = len(existing)
	s.mu.Unlock()

	return added, nil
}

// UpdatePollerMeta is a no-op if the app was removed mid-poll.
func (s *Store) UpdatePollerMeta(appID string, meta PollerMeta) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	app, ok := s.apps[appID]
	if !ok {
		return nil
	}
	app.Poller = meta
	return s.writeAppsFile()
}

func (s *Store) GetReviews(appID string, since time.Time) ([]Review, error) {
	if !validAppID(appID) {
		return nil, fmt.Errorf("invalid app id %q", appID)
	}

	s.mu.RLock()
	_, ok := s.apps[appID]
	s.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("%s: %w", appID, ErrNotFound)
	}

	all, err := s.loadReviewFile(appID)
	if err != nil {
		return nil, err
	}

	var result []Review
	for _, r := range all {
		// Inclusive on the boundary — Apple sometimes clusters timestamps.
		if !r.SubmittedAt.Before(since) {
			result = append(result, r)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].SubmittedAt.After(result[j].SubmittedAt)
	})
	return result, nil
}

func (s *Store) AppExists(appID string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.apps[appID]
	return ok
}

func (s *Store) loadReviewFile(appID string) ([]Review, error) {
	if !validAppID(appID) {
		return nil, fmt.Errorf("invalid app id %q", appID)
	}
	path := filepath.Join(s.dataDir, "reviews", appID+".json")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var reviews []Review
	if err := json.Unmarshal(data, &reviews); err != nil {
		return nil, err
	}
	return reviews, nil
}

// writeAppsFile must be called with s.mu held — it iterates the apps map.
func (s *Store) writeAppsFile() error {
	apps := make([]App, 0, len(s.apps))
	for _, app := range s.apps {
		apps = append(apps, *app)
	}
	return atomicWriteJSON(filepath.Join(s.dataDir, "apps.json"), apps)
}

func (s *Store) writeReviewFile(appID string, reviews []Review) error {
	if !validAppID(appID) {
		return fmt.Errorf("invalid app id %q", appID)
	}
	if reviews == nil {
		reviews = []Review{}
	}
	return atomicWriteJSON(filepath.Join(s.dataDir, "reviews", appID+".json"), reviews)
}

// atomicWriteJSON writes via temp file + fsync + rename, so a process
// crash never leaves the target partial. No parent-dir fsync — a power
// loss can lose the most recent write, but merge is dedupe-on-ID, so
// the next poll recovers.
func atomicWriteJSON(path string, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}

	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".tmp-")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpName := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("sync temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("close temp file: %w", err)
	}

	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("atomic rename: %w", err)
	}
	return nil
}
