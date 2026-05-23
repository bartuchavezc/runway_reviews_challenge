export type PollerStatus = 'pending' | 'ok' | 'error'

export interface PollerMeta {
  status: PollerStatus
  lastRefreshAt: string
}

export interface App {
  id: string
  name: string
  rssUrl: string
  poller: PollerMeta
  totalReviews: number
}

export interface Review {
  id: string
  appId: string
  score: number
  title: string
  body: string
  author: string
  version: string
  submittedAt: string
}

export type RatingFilter = 'any' | 'le2' | 'eq3' | 'ge4' | 'eq5'

export interface Filters {
  rating: RatingFilter
  query: string
  window: number // hours; sent as ?window= to the backend
}

export type View = 'main' | 'settings'
