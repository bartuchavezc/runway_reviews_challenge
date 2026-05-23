import { useState, useEffect, useMemo, useRef, useCallback } from 'react'
import { useApps } from './hooks/useApps'
import { useReviews } from './hooks/useReviews'
import { Sidebar } from './components/Sidebar/Sidebar'
import { AppHeader } from './components/AppHeader/AppHeader'
import { FilterBar } from './components/FilterBar/FilterBar'
import { ReviewList } from './components/ReviewList/ReviewList'
import { Settings } from './components/Settings/Settings'
import type { App, Filters, View } from './types'
import { applyFilters } from './lib/filters'
import styles from './App.module.css'

const STORAGE_KEY = 'activeAppId'

export default function App() {
  const { apps, loading: appsLoading, error: appsError, addApp, removeApp, refresh: retryConnect } = useApps()

  const mainRef = useRef<HTMLElement>(null)
  const rssInputRef = useRef<HTMLInputElement>(null)
  // Imperative focus flag — set by the K-shortcut handler, consumed by the
  // view-change effect. A ref instead of state because nothing else needs to
  // re-render in response to it and storing it as state was failing the
  // react-hooks/set-state-in-effect lint rule for no real benefit.
  const shouldFocusRssInput = useRef(false)

  const [activeAppId, setActiveAppId] = useState<string | null>(() => {
    try { return localStorage.getItem(STORAGE_KEY) } catch { return null }
  })
  const [view, setView] = useState<View>('main')
  const [filters, setFilters] = useState<Filters>({ rating: 'any', query: '', window: 48 })

  const { reviews, loading: reviewsLoading, error, refresh } = useReviews(
    view === 'main' ? activeAppId : null,
    filters.window
  )

  const activeApp = apps.find(a => a.id === activeAppId) ?? null

  // Auto-select the first app once loaded; clears selection when the last app
  // is deleted. This is the textbook "sync local state with external data"
  // effect — apps comes from the server, activeAppId mirrors it. The
  // alternatives (derive at render, useSyncExternalStore over the apps array)
  // would each introduce more accidental complexity than the rule is preventing.
  /* eslint-disable react-hooks/set-state-in-effect */
  useEffect(() => {
    if (appsLoading) return
    if (apps.length === 0 && activeAppId !== null) {
      setActiveAppId(null)
      try { localStorage.removeItem(STORAGE_KEY) } catch { /* ignore */ }
    } else if (apps.length > 0 && !apps.find(a => a.id === activeAppId)) {
      const id = apps[0].id
      setActiveAppId(id)
      try { localStorage.setItem(STORAGE_KEY, id) } catch { /* ignore */ }
    }
  }, [apps, appsLoading, activeAppId])
  /* eslint-enable react-hooks/set-state-in-effect */

  // Move focus to the main content area on view switch so keyboard/screen-reader
  // users don't have to tab through the sidebar again to reach the new content.
  // The K-shortcut also routes through here: if shouldFocusRssInput.current
  // was set, hand focus to the RSS input instead.
  useEffect(() => {
    if (view === 'settings' && shouldFocusRssInput.current) {
      rssInputRef.current?.focus()
      shouldFocusRssInput.current = false
      return
    }
    mainRef.current?.focus()
  }, [view])

  // Global keyboard shortcut — fires when no input element has focus.
  // K → open the Add-app page and focus the RSS input field.
  //
  // Single-letter shortcuts collide with screen-reader quick-nav (NVDA/JAWS
  // browse mode uses single letters to jump between elements). To avoid
  // hijacking those, also bail out when the focused element is contenteditable
  // or when isComposing is true (IME composition). The shortcut is a
  // convenience — the target is reachable via Tab + Enter and advertised
  // via aria-keyshortcuts on the Sidebar button.
  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if (e.target instanceof HTMLInputElement || e.target instanceof HTMLTextAreaElement) return
      if (e.target instanceof HTMLElement && e.target.isContentEditable) return
      if (e.isComposing) return
      if (e.metaKey || e.ctrlKey || e.altKey) return
      if (e.key === 'k' || e.key === 'K') {
        shouldFocusRssInput.current = true
        setView('settings')
      }
    }
    document.addEventListener('keydown', handler)
    return () => document.removeEventListener('keydown', handler)
  }, [])

  const handleSelectApp = (id: string) => {
    setActiveAppId(id)
    try { localStorage.setItem(STORAGE_KEY, id) } catch { /* ignore */ }
    setFilters({ rating: 'any', query: '', window: 48 })
  }

  // After adding an app, navigate directly to its reviews so the user
  // doesn't have to click manually in the sidebar. The initial poll kicks
  // off on the backend immediately; useReviews will auto-retry after 30 s
  // if the first fetch arrives before the poll finishes.
  const handleAddApp = useCallback(async (rssUrl: string): Promise<App> => {
    const app = await addApp(rssUrl)
    setActiveAppId(app.id)
    try { localStorage.setItem(STORAGE_KEY, app.id) } catch { /* ignore */ }
    setFilters({ rating: 'any', query: '', window: 48 })
    setView('main')
    return app
  }, [addApp])

  // Client-side filter pipeline lives in ./filters so the branches can be
  // unit-tested without rendering App.
  const filtered = useMemo(() => applyFilters(reviews, filters), [reviews, filters])

  const avgScore = useMemo(() => {
    if (reviews.length === 0) return null
    return Math.round(reviews.reduce((s, r) => s + r.score, 0) / reviews.length * 10) / 10
  }, [reviews])

  return (
    <div className={styles.layout}>
      {/* Skip link — visible on focus only. Lets keyboard-only users
          bypass the sidebar's app list and land directly on the main
          content (which is itself focusable via tabIndex={-1}). */}
      <a href="#main-content" className={styles.skipLink}>Skip to main content</a>

      <Sidebar
        apps={apps}
        activeAppId={activeAppId}
        view={view}
        onSelectApp={handleSelectApp}
        onSelectView={setView}
      />

      <main
        id="main-content"
        ref={mainRef}
        className={styles.main}
        tabIndex={-1}
        aria-label={view === 'settings' ? 'Settings' : activeApp ? activeApp.name : 'Reviews'}
      >
        {view === 'settings' ? (
          <Settings
            apps={apps}
            onAdd={handleAddApp}
            onRemove={removeApp}
            rssInputRef={rssInputRef}
          />
        ) : !appsLoading && appsError !== null && apps.length === 0 ? (
          <ServerDownState onRetry={retryConnect} />
        ) : (
          <>
            {activeApp ? (
              <>
                <AppHeader
                  app={activeApp}
                  avgScore={avgScore}
                  windowHours={filters.window}
                  windowCount={reviews.length}
                />
                {/* key resets the uncontrolled search input when switching apps */}
                <FilterBar
                  key={activeAppId ?? ''}
                  filters={filters}
                  onChange={setFilters}
                  total={reviews.length}
                  shown={filtered.length}
                />
                <ReviewList
                  reviews={filtered}
                  loading={reviewsLoading}
                  error={error}
                  onRetry={refresh}
                />
              </>
            ) : (
              <div className={styles.empty}>
                {appsLoading ? 'Loading…' : 'Add an app to get started'}
              </div>
            )}
          </>
        )}
      </main>
    </div>
  )
}

function ServerDownState({ onRetry }: { onRetry: () => void }) {
  return (
    <div className={styles.serverDown}>
      <ServerDownIcon />
      <h2 className={styles.serverDownTitle}>Server unreachable</h2>
      <p className={styles.serverDownDesc}>
        Could not connect to the backend.<br />
        Make sure the server is running on :8080.
      </p>
      <button className={styles.serverDownRetry} onClick={onRetry}>
        Retry
      </button>
    </div>
  )
}

function ServerDownIcon() {
  return (
    <svg width="48" height="48" viewBox="0 0 48 48" fill="none" className={styles.serverDownIcon} aria-hidden="true">
      <rect x="5" y="7" width="38" height="12" rx="3" stroke="currentColor" strokeWidth="1.8" />
      <rect x="5" y="23" width="38" height="12" rx="3" stroke="currentColor" strokeWidth="1.8" />
      <circle cx="12" cy="13" r="2" fill="currentColor" opacity="0.4" />
      <circle cx="12" cy="29" r="2" fill="currentColor" opacity="0.4" />
      <path d="M31 38l7 7M38 38l-7 7" stroke="currentColor" strokeWidth="2.2" strokeLinecap="round" />
    </svg>
  )
}
