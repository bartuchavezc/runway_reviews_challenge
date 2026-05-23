import type { Filters, Review } from '../types'

// applyFilters runs the client-side filter pipeline against an array of
// reviews. Pulled out of App.tsx so the logic is testable in isolation —
// previously inlined inside a useMemo, which made every branch silently
// uncovered. The "any" / empty-query cases are no-ops on the array
// reference, which keeps `useMemo` equality cheap when filters are off.
export function applyFilters(reviews: Review[], filters: Filters): Review[] {
  let result = reviews
  if (filters.rating !== 'any') {
    result = result.filter(r => {
      if (filters.rating === 'eq5') return r.score === 5
      if (filters.rating === 'ge4') return r.score >= 4
      if (filters.rating === 'eq3') return r.score === 3
      if (filters.rating === 'le2') return r.score <= 2
      return true
    })
  }
  const q = filters.query.trim().toLowerCase()
  if (q) {
    result = result.filter(r =>
      r.author.toLowerCase().includes(q) ||
      r.body.toLowerCase().includes(q) ||
      r.title.toLowerCase().includes(q)
    )
  }
  return result
}
