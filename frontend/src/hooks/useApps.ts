import { useState, useEffect, useCallback } from 'react'
import { api } from '../api/client'
import type { App } from '../types'

// Single-shot fetch on mount; data refreshes only when the user reloads
// the page. The backend's poller produces fresh data on disk every 15
// minutes per app — exposing that liveness in the UI was more noise than
// signal, so we deliberately don't poll /api/health here. Operator-facing
// freshness data lives in the backend's apps.json / /api/health endpoint.
export function useApps() {
  const [apps, setApps] = useState<App[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const fetchApps = useCallback(async () => {
    try {
      const data = await api.getApps()
      setApps(data)
      setError(null)
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to load apps')
    } finally {
      setLoading(false)
    }
  }, [])

  /* eslint-disable react-hooks/set-state-in-effect */
  useEffect(() => { fetchApps() }, [fetchApps])
  /* eslint-enable react-hooks/set-state-in-effect */

  const addApp = useCallback(async (rssUrl: string): Promise<App> => {
    const app = await api.addApp(rssUrl)
    setApps(prev => [...prev, app])
    return app
  }, [])

  const removeApp = useCallback(async (id: string) => {
    await api.deleteApp(id)
    setApps(prev => prev.filter(a => a.id !== id))
  }, [])

  return { apps, loading, error, addApp, removeApp, refresh: fetchApps }
}
