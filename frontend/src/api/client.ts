import type { App, Review } from '../types'

// Thrown when the fetch itself fails (server down, DNS failure, CORS, etc.)
// as opposed to the server returning a non-2xx response.
export class NetworkError extends Error {
  constructor(msg: string) {
    super(msg)
    this.name = 'NetworkError'
  }
}

// Cap every request at 30s so a stalled connection can't pin a goroutine
// or hang the UI behind the browser's default ~60-90s fetch timeout.
const DEFAULT_TIMEOUT_MS = 30_000

async function request<T>(path: string, options?: RequestInit): Promise<T> {
  // Compose the caller's signal (if any) with our own timeout signal so
  // either source can abort. AbortSignal.any is widely supported across
  // the browsers Vite targets.
  const timeout = new AbortController()
  const timer = setTimeout(() => timeout.abort(), DEFAULT_TIMEOUT_MS)
  const signals = options?.signal
    ? AbortSignal.any([options.signal, timeout.signal])
    : timeout.signal

  let res: Response
  try {
    res = await fetch(path, { ...options, signal: signals })
  } catch (e) {
    if (e instanceof DOMException && e.name === 'AbortError') {
      // Two reasons fetch can abort: our timeout fired, OR the caller's
      // own signal cancelled the request (rapid app switch, Retry click).
      // The first is a real failure the user must see; the second is a
      // deliberate cancel that must stay silent.
      if (timeout.signal.aborted) {
        throw new NetworkError('Request timed out')
      }
      throw e
    }
    throw new NetworkError(e instanceof Error ? e.message : 'Network error')
  } finally {
    clearTimeout(timer)
  }
  if (!res.ok) {
    // Gateway errors mean the upstream server is down — treat them like a network failure.
    if (res.status === 502 || res.status === 503 || res.status === 504) {
      throw new NetworkError(`HTTP ${res.status}`)
    }
    const body = await res.json().catch(() => ({})) as { error?: string }
    throw new Error(body.error ?? `HTTP ${res.status}`)
  }
  if (res.status === 204) return undefined as T
  // 200 with malformed body would otherwise surface a raw SyntaxError
  // ("Unexpected token …") to the user. Normalise it into a clean error
  // message — the underlying detail goes into the console for debugging.
  try {
    return await res.json() as T
  } catch (e) {
    console.error('[api] malformed JSON response', path, e)
    throw new NetworkError('Malformed response from server')
  }
}

export const api = {
  getApps: () =>
    request<App[]>('/api/apps'),

  addApp: (rssUrl: string) =>
    request<App>('/api/apps', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ rssUrl }),
    }),

  deleteApp: (id: string) =>
    request<void>(`/api/apps/${id}`, { method: 'DELETE' }),

  getReviews: (appId: string, window = 48, signal?: AbortSignal) =>
    request<Review[]>(`/api/apps/${appId}/reviews?window=${window}`, { signal }),
}
