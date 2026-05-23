import { useState, useEffect, useRef, useCallback } from 'react'
import { api, NetworkError } from '../api/client'
import type { Review } from '../types'

// Module-level cache, lifetime = browser tab. Purpose is narrow: when a
// user toggles between apps within a single session (snap → spoti → snap
// inside a minute), the previously-loaded reviews render instantly while
// a fresh fetch runs in the background. There is no auto-invalidation —
// stale data is corrected on page reload, which matches the assignment's
// "data refreshes when the user reloads" contract.
// Cache key is `${appId}:${window}` so different window sizes coexist.
const cache = new Map<string, Review[]>()

export function useReviews(appId: string | null, window = 48) {
  const key = appId ? `${appId}:${window}` : null

  const [reviews, setReviews] = useState<Review[]>(() =>
    key ? (cache.get(key) ?? []) : []
  )
  // Start in the loading state on cold mount when there's an app to fetch
  // but no cache hit yet. Without this, the first paint after a hard reload
  // briefly renders the "no reviews match" empty state for a frame before
  // the effect kicks off the fetch and flips loading to true. When the
  // cache already has data we hand it back instantly with loading=false.
  const [loading, setLoading] = useState(() =>
    Boolean(key && !cache.has(key))
  )
  const [error, setError] = useState<string | null>(null)
  const activeKey = useRef(key)
  // Keeps a live handle on the most recent fetch so refresh() can abort
  // an in-flight request before starting a new one. Without this, two
  // rapid Retry clicks race and the slower response would clobber state.
  const inflight = useRef<AbortController | null>(null)

  const fetch_ = useCallback(async (id: string, win: number, signal: AbortSignal) => {
    const k = `${id}:${win}`
    setLoading(true)
    try {
      const data = await api.getReviews(id, win, signal)
      if (signal.aborted) return
      cache.set(k, data)
      if (activeKey.current === k) {
        setReviews(data)
        setError(null)
      }
    } catch (e) {
      if (signal.aborted) return
      if (activeKey.current === k) {
        setError(e instanceof NetworkError ? 'Server unreachable' : e instanceof Error ? e.message : 'Failed to load reviews')
      }
    } finally {
      if (!signal.aborted && activeKey.current === k) setLoading(false)
    }
  }, [])

  useEffect(() => {
    activeKey.current = key
    // When appId becomes null (Settings view) we deliberately keep the last
    // known reviews in local state — returning to main view re-hydrates
    // instantly from cache, no shimmer flash.
    if (!appId || !key) return

    /* eslint-disable react-hooks/set-state-in-effect */
    // Hydrate from the module-level cache before kicking off the refetch.
    // This is the legitimate "sync local React state with an external store"
    // case the lint rule's docs explicitly carve out.
    const cached = cache.get(key)
    if (cached) setReviews(cached)
    /* eslint-enable react-hooks/set-state-in-effect */

    inflight.current?.abort()
    const controller = new AbortController()
    inflight.current = controller
    fetch_(appId, window, controller.signal)
    return () => controller.abort()
  }, [appId, window, key, fetch_])

  const refresh = useCallback(() => {
    if (!appId) return
    inflight.current?.abort()
    const controller = new AbortController()
    inflight.current = controller
    fetch_(appId, window, controller.signal)
  }, [appId, window, fetch_])

  return { reviews, loading, error, refresh }
}
