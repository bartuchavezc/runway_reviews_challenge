# Recent iOS App Store reviews viewer

A small full-stack tool that polls the App Store Connect customer-reviews RSS feed for any number of iOS apps and surfaces the recent reviews in a read-only React workbench.

The assignment text lives in [`challenge.txt`](./challenge.txt). The design reference lives in [`design_handoff_reviews_workbench/`](./design_handoff_reviews_workbench/).

---

## Quick start

The repo ships with a `docker-compose.yml`. From the repo root:

```sh
docker compose up --build
```

- Frontend: <http://localhost:5173>
- Backend:  <http://localhost:8080/api/health>

Review state is persisted to `./data/` on the host (mounted into the backend container at `/data`). Stop everything with `docker compose down`; state survives. Wipe state by deleting the `./data/` directory.

For local development without Docker:

```sh
# terminal 1
cd backend && go run ./cmd/server

# terminal 2
cd frontend && npm install && npm run dev
```

The Vite dev server proxies `/api/*` to `localhost:8080` so the same relative-path API client works in both environments.

---

## What it does

- The backend runs a single polling loop on one ticker (default every 15 minutes). On each tick it iterates the tracked apps **sequentially**, hitting the Apple RSS feed for each in turn, merging new reviews into a per-app JSON file on disk, and deduping by Apple's review ID. Transient feed flakes (a one-off 503 from Apple's CDN, say) are absorbed by an inline retry-with-backoff inside the per-app poll — not by parallelism. State is fully restartable: kill the process, restart it, and the next tick picks up where it left off without losing or re-emitting anything.
- The frontend lists the tracked apps in a sidebar, lets you switch between them, filter by rating, search within author / title / body, and pick a window from 24 h to 1 month. The default window is the assignment-required 48 h.
- Add and remove apps from the Add-app page. Paste an App Store Connect reviews RSS URL; the backend resolves the human name via the iTunes lookup API, validates the feed is reachable, and triggers an immediate poll (`PollNow`) so the new app's reviews show up without waiting for the next tick.

---

## Architecture

```
┌─────────────────────────┐         ┌────────────────────────────┐
│  React app (Vite)       │  /api   │  Go backend                │
│                         ├────────►│                            │
│  Sidebar / AppHeader    │         │  HTTP handlers             │
│  FilterBar / ReviewList │         │  Poller (single ticker,    │
│  Settings (Add-app)     │         │   iterates apps in turn,   │
│                         │         │   retry-with-backoff inside)│
│  Module-level cache     │◄────────┤  Store (in-memory metadata │
│  AbortController-guarded│  JSON   │   + per-app JSON files)    │
└─────────────────────────┘         │  Apple RSS client          │
                                    └──────────┬─────────────────┘
                                               │
                                               ▼
                                    ./data/apps.json
                                    ./data/reviews/{appId}.json
```

### Backend (`backend/`)

Layout follows the conventional Go `cmd/` + `internal/` split.

| Package                | Responsibility                                                            |
|------------------------|---------------------------------------------------------------------------|
| `cmd/server`           | Entry point: env-var config, store load, poller start, HTTP server, graceful two-phase shutdown. |
| `internal/api`         | HTTP handlers, route registration, request validation, CORS.              |
| `internal/poller`      | Single polling goroutine, sequential pass over all tracked apps each tick. `Start` / `Stop` lifecycle; `PollNow(appID)` for the post-AddApp immediate poll. Inline retry-with-backoff absorbs transient feed failures. |
| `internal/store`       | In-memory app metadata + per-app review files on disk; atomic JSON writes via temp-file + `fsync` + rename; dedup on merge. |
| `internal/feed`        | Apple RSS client: `FetchPage`, `FetchAll`, `LookupAppName`, URL canonicalisation, pagination, response-shape normalisation, SSRF and body-size defences. |
| `internal/env`         | Minimal env-var helpers.                                                  |

Zero third-party dependencies — `go.mod` confirms it.

### Frontend (`frontend/`)

| Module                                    | Responsibility                                          |
|-------------------------------------------|---------------------------------------------------------|
| `src/App.tsx`                             | Orchestration: app selection, view switching, filter state, keyboard shortcut, focus management. |
| `src/hooks/useApps.ts`                    | Loads `/api/apps` once on mount, plus `addApp` / `removeApp` mutations. |
| `src/hooks/useReviews.ts`                 | Per-app review fetch with a module-level cache for instant app-toggle, `AbortController` plumbing, `activeKey` guard against stale writes. |
| `src/api/client.ts`                       | Typed fetch wrapper: `NetworkError` classification, 30 s timeout, AbortError pass-through. |
| `src/lib/display.ts`                      | Pure display helpers — `tint(id)`, `initials(name)`, `windowLabel(hours)`. |
| `src/lib/filters.ts` / `validation.ts`    | Pure modules for the filter pipeline and the RSS-URL regex; co-located test files in the same folder. |
| `src/components/`                         | `Sidebar`, `AppHeader`, `FilterBar`, `ReviewList`, `Settings`. Each component co-located with its CSS module. |
| `src/ErrorBoundary.tsx`                   | Top-level boundary so a render crash doesn't blank the tab. |
| `src/styles/tokens.css` / `global.css`    | Design tokens + global reset and reduced-motion respect.|

Zero runtime dependencies beyond React itself.

---

## Key decisions and why

These were deliberate calls. Some diverge from the design handoff or from a "more is always better" instinct — included so a reviewer can quickly find the reasoning.

**Sequential polling, not one worker per app.** A single goroutine walks the tracked-apps snapshot in order on each tick. Spawning N goroutines for an Apple-RSS workload (~80 KB per poll, ~ms of disk I/O) was speculative parallelism with real costs: per-app lifecycle, shutdown choreography, a `closed`-flag race against late `StartApp` calls. Sequential is simpler and the inline retry-with-backoff in `pollApp` (up to 3 attempts × 2 s) is what actually bounds the cost of a transient feed failure — that's the job per-app goroutines were nominally doing. If the workload ever genuinely needs concurrency, the right next step is a bounded worker pool inside the poller, not one goroutine per app.

**Two-phase shutdown.** `srv.Shutdown` first (drains HTTP, so no new `PollNow` calls can land), then `poller.Stop`. `Stop` cancels the poller's context — both the ticker loop and any in-flight `PollNow` goroutines — and `wg.Wait`s for them to fully release the store before returning. `main` then blocks on a `shutdownDone` channel before exiting, so the process never outlives an in-flight `MergeReviews`.

**Path-traversal defense in depth.** The HTTP handler validates app IDs as `^\d+$` (`api/handler.go`), and the store layer applies the same check at every filesystem-touching helper (`store.go`). If a future internal caller bypasses the handler, the store still refuses to construct a path that could escape `DATA_DIR`.

**Persistence is per-app JSON files with atomic temp-file + fsync + rename.** Crash-safe per file. The cross-file invariant between `apps.json` and `reviews/*.json` is reconciled on `Load` by re-reading the review files to compute `TotalReviews` — so a crash between two writes self-heals on next start. Merge is dedupe-on-ID, idempotent.

**Per-app file locking, POSIX-atomic reads.** The store has two locks: a coarse `RWMutex` on the in-memory `apps` map (held only for brief critical sections — never across disk I/O) and a `map[appID]*sync.Mutex` guarding writes to each `reviews/{id}.json`. Reads of review files take *no* file lock: `atomicWriteJSON` uses POSIX temp-file + rename, so a concurrent `os.ReadFile` either sees the old inode or the new inode, never a torn read. The upshot: a slow `MergeReviews` for app A no longer blocks a `GetReviews` for app B (or even for app A itself — the reader just gets an old-or-new coherent snapshot). And the per-poll-cycle disk write count dropped from 3 (review file + 2 × apps.json) to 2: `MergeReviews` only writes the review file, and the following `UpdatePollerMeta` carries the apps.json persist for the whole cycle.

**Apple feed handled defensively.** SSRF block via `CheckRedirect: ErrUseLastResponse`, a 5 MB `io.LimitReader` cap on response bodies (unbounded `NewDecoder` is a real DoS surface even under a 10 s timeout), a custom unmarshaller for the single-object-`entry` quirk on low-volume feeds, and `CanonicalizeURL` that rewrites any submitted URL into `/sortBy=mostRecent/json` so the pagination early-stop is sound.

**`MAX_PAGES` is configurable but Apple's hard cap is 10.** Apple's CDN returns empty past page 10 (~500 most-recent reviews per app); the default matches reality but exposing the var means an operator can tune for tests or if Apple ever changes.

**API surface is narrow on purpose.** `GET /api/apps`, `POST /api/apps`, `DELETE /api/apps/{id}`, `GET /api/apps/{id}/reviews?window=N`, and `GET /api/health`. The health endpoint is a control-plane ping — `{status, last_success_sync}` only, no per-app detail, no error strings. Error reasons live in server logs, not on the wire, so a malformed Apple response can't accidentally leak filesystem paths.

**`AbortController` everywhere on the frontend.** `fetch` has no default timeout in the browser. The request wrapper composes a 30 s abort signal with any caller signal so a stalled connection can't hang the UI behind the browser's default 60-90 s. `useReviews.refresh` stores the live controller so two rapid Retry clicks can't race.

**Pastel swatch palette with dark text.** Saturated brand-style swatches with `rgba(0,0,0,.65)` text fell below WCAG-AA at the 9 px sidebar size. Pastels with the full-opacity `--text` token push contrast above 7:1 across all swatch sizes.

**Desktop-only.** No responsive media queries. The tool is an internal viewer at a fixed 1280×800 working size, per the design handoff intent. Phone support would require a real layout rethink; rather than ship a half-working stack on small screens, it stays unsupported.

---

## API reference

| Method | Path                              | Body / Query              | Returns                                 |
|--------|-----------------------------------|---------------------------|-----------------------------------------|
| GET    | `/api/apps`                       | —                         | Array of tracked apps with poller state |
| POST   | `/api/apps`                       | `{ "rssUrl": "..." }`     | The added app (201) or 4xx              |
| DELETE | `/api/apps/{id}`                  | —                         | 204                                     |
| GET    | `/api/apps/{id}/reviews?window=N` | `N` in hours (default 48) | Reviews newer than `now - N h`, newest-first |
| GET    | `/api/health`                     | —                         | `{ status, last_success_sync }`         |

### Configuration

All env vars have sensible defaults and are read at startup.

| Variable                | Default     | Purpose                                  |
|-------------------------|-------------|------------------------------------------|
| `PORT`                  | `8080`      | HTTP listen port.                        |
| `DATA_DIR`              | `./data`    | Where `apps.json` and `reviews/*.json` live. Set to `/data` in the Docker image. |
| `POLL_INTERVAL_MINUTES` | `15`        | Per-app polling cadence.                 |
| `MAX_PAGES`             | `10`        | Apple feed page cap (Apple's own ceiling). |

---

## Tests

```sh
# backend
cd backend && go test -race ./...

# frontend
cd frontend && npm test
```

Backend: ~74–90 % statement coverage across production packages (api 74, feed 79, store 80, poller 89, env 90), race-detector clean under `-race -count=2`. Hermetic — every Apple HTTP call is mocked via `httptest.Server`, no real `itunes.apple.com` traffic. Includes an end-to-end test in `backend/cmd/server/main_test.go` that wires up the real stack against a mock feed and proves the stop/restart-without-losing-progress requirement.

Frontend: 42 tests across four pure-function suites under `src/lib/` (`utils`, `display`, `filters`, `validation`). Deterministic across two consecutive runs; `timeAgo` boundary tests use `vi.useFakeTimers()` to pin the clock. Component-level tests are out-of-scope for the package set (no `@testing-library/react`); covered by the explanation in `frontend/README.md`.

---

## AI usage disclosure

This project was built with significant AI assistance. To be precise about who did what:

- **Code authoring — 90 % AI.** Every line of source code in `backend/` and `frontend/` was written by an AI assistant working from human direction.
- **Architecture, scope, and constraints — human.** The package layout, the polling strategy (and the later call to collapse a speculative per-app-worker design into a single sequential loop), the persistence model, the API surface, the env-var configuration, and every other shape-of-the-system call were decided by the human and given to the AI as direction.
- **Design decisions — human-driven.** The visual language reused the provided design handoff. Departures from it (extended window options, removal of the polling indicator, pastel swatch palette, single Add-app entry, etc.) were each an explicit human call after weighing the trade-off.
- **Review and acceptance — human-driven.** The AI was asked to surface findings and write fixes, but every accepted change passed through a human decision: which findings were real, which were noise, what to fix, what to leave intentional. Multiple AI code-review passes were used as a tool, not as a substitute for judgement.
