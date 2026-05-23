# Frontend — Reviews workbench

The React app that consumes the backend's `/api` and displays recent iOS
App Store reviews. See [the repo root README](../README.md) for the
architecture, the rationale behind the design decisions, and the full
quick-start. This file is the practical scope-it / run-it / test-it
reference for the frontend in isolation.

## Stack

- **Vite + React 19 + TypeScript** with strict mode and the React Compiler
  *not* enabled (default for this Vite template).
- **Zero runtime dependencies beyond `react` / `react-dom`.** No router,
  no state library, no design-system package, no fetch client, no i18n
  framework. Style is plain CSS modules + a single tokens file.
- **Vitest + jsdom** for tests. The plain-function tests use `jsdom` as a
  default environment; no React Testing Library is installed (the brief
  scope didn't justify the dependency).

## Layout

```
src/
├── main.tsx              app bootstrap, mounts <ErrorBoundary> → <App>
├── App.tsx               orchestration: view, active app, filter state, K shortcut
├── App.module.css
├── ErrorBoundary.tsx     top-level boundary; reloads on click for the fallback
├── types.ts              App, Review, Filters, PollerMeta, RatingFilter, View
├── vite-env.d.ts         ambient declarations for *.module.css
├── styles/
│   ├── global.css        reset, focus rings, prefers-reduced-motion
│   └── tokens.css        colors, radii, shadows, font families
├── lib/                  pure modules, paired with co-located *.test.ts files
│   ├── utils.ts          timeAgo (the only date helper)
│   ├── display.ts        initials(name), tint(id), windowLabel(hours)
│   ├── filters.ts        applyFilters(reviews, filters) — pure pipeline
│   └── validation.ts     RSS_REGEX shared with Settings
├── api/client.ts         fetch wrapper, NetworkError, 30s timeout, AbortError pass-through
├── hooks/
│   ├── useApps.ts        single-shot /api/apps load + add/remove mutations
│   └── useReviews.ts     per-app fetch + module-level cache + AbortController
└── components/
    ├── Sidebar/          app list + "+ Add app" entry (K shortcut)
    ├── AppHeader/        active-app title + 3 stat tiles
    ├── FilterBar/        Window/Rating dropdowns (via ListboxChip), search, sort
    ├── ReviewList/       score block, title/body/meta, shimmer, error banner
    └── Settings/         Add-app input + Tracked-apps table
```

## How it talks to the backend

The API client calls `/api/...` as relative paths in every environment:

- **`npm run dev`** — Vite's proxy at `vite.config.ts` forwards `/api/*`
  to `http://localhost:8080`. Run the backend separately
  (`cd ../backend && go run ./cmd/server`).
- **`docker compose up`** — the production nginx in the frontend image
  reverse-proxies `/api/*` to the `backend` service.
- **No third-party fetch wrapper.** `src/api/client.ts` is ~50 lines of
  hand-rolled `fetch` with three intentional features: a `NetworkError`
  class so the offline UX can branch on transport failures, a 30 s
  timeout composed with the caller's signal, and an `AbortError`
  pass-through so a deliberate cancel doesn't flip the offline state.

## Run, build, test, lint

```sh
npm install         # one-time
npm run dev         # vite dev server on http://localhost:5173
npm run build       # type-check + production build into dist/
npm run preview     # serve the built dist locally
npm test            # vitest run, all tests once
npm run lint        # eslint
```

All four (build / test / lint / type-check) are clean as of this commit.
`npm test` reports 42 tests across 4 files (`utils`, `display`, `filters`,
`validation`).

## What's tested

The frontend test suite is intentionally scoped to pure functions and
shared logic that benefits most from regression coverage:

| File                         | Covers                                                          |
|------------------------------|-----------------------------------------------------------------|
| `src/lib/utils.test.ts`      | `timeAgo` boundary cases, with `vi.useFakeTimers()` for determinism. |
| `src/lib/display.test.ts`    | `initials`, `tint` (determinism + palette spread), `windowLabel`. |
| `src/lib/filters.test.ts`    | Every branch of the rating filter, query matching across title/body/author, AND-composition, no-mutation. |
| `src/lib/validation.test.ts` | `RSS_REGEX` — accepts canonical URL, captures app id, rejects http/wrong-host/non-numeric/missing-id/uppercase-country/missing-slash. |

Component-level interaction tests (FilterBar keyboard nav, Settings
delete-confirm, `useReviews` cache behaviour) are out of scope for the
current package set — adding them would require `@testing-library/react`,
which would be the right call once the app grows beyond a take-home.

## Things to know

- **No `Last polled` indicator anywhere.** Polling state is operator-side
  data; surfacing it in the UI was deliberately removed for noise reasons
  (see the root README's *Key decisions* section).
- **Data refreshes on full page reload.** No 30 s health poll, no SSE.
  The `useReviews` cache is narrowly scoped to making rapid app-toggle
  feel instant within a single session.
- **Display fields (tint, initials) live in `src/display.ts`,** not the
  backend. They're presentation concerns, computed at render time.
- **K opens the Add-app page** and focuses the URL input. Only global
  shortcut. The Add-app entry button advertises it via `aria-keyshortcuts`.
- **Geist + Geist Mono are loaded from Google Fonts.** Acceptable for
  the take-home; a production deployment should self-host (e.g. via
  `fontsource/geist`) for privacy and CSP cleanliness.
- **Desktop-only.** No responsive media queries — the design target is
  a 1280×800 internal workbench.

## Known scope cuts

These are deliberate, not bugs. They're called out so a reviewer can find
them quickly without digging:

- No telemetry (Sentry / Datadog / RUM). The ErrorBoundary `console.error`s.
- No CSP headers in the nginx config — the production deploy should add them.
- No `noUncheckedIndexedAccess` in `tsconfig.app.json`. Tightening it would
  be a senior follow-up.
- The `FilterBar` `ListboxChip` does not use roving tabindex; options are
  natural-tab-order focusable buttons. Internally consistent and SR-friendly,
  just not the textbook ARIA listbox pattern.
- `frontend/data/` — none. State persists on the backend.
