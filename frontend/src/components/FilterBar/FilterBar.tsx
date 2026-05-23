import { useRef, useEffect } from 'react'
import type { Filters, RatingFilter } from '../../types'
import { ListboxChip } from './ListboxChip'
import styles from './FilterBar.module.css'

interface Props {
  filters: Filters
  onChange: (f: Filters) => void
  total: number
  shown: number
}

const RATING_OPTIONS: { value: RatingFilter; label: string }[] = [
  { value: 'any',  label: 'Any rating' },
  { value: 'eq5',  label: '★★★★★  5 stars' },
  { value: 'ge4',  label: '★★★★☆  4 stars & up' },
  { value: 'eq3',  label: '★★★☆☆  3 stars' },
  { value: 'le2',  label: '★★☆☆☆  2 stars & below' },
]

const WINDOW_OPTIONS: { value: number; label: string; short: string }[] = [
  { value: 24,   label: 'Last 24h',     short: '24h' },
  { value: 48,   label: 'Last 48h',     short: '48h' },
  { value: 72,   label: 'Last 3 days',  short: '3d'  },
  { value: 168,  label: 'Last 1 week',  short: '1W'  },
  { value: 720,  label: 'Last 1 month', short: '1M'  },
  { value: 8760, label: 'Last 1 year',  short: '1Y'  },
  { value: 0,    label: 'All time',     short: 'All' },
]

export function FilterBar({ filters, onChange, total, shown }: Props) {
  // Keep a ref to the latest filters so the debounce closure never captures a stale copy.
  // Without this, a window change that arrives within the 200ms debounce window would be
  // overwritten when the timeout fires using the old filters spread.
  const filtersRef = useRef(filters)
  useEffect(() => { filtersRef.current = filters }, [filters])

  // Debounced search — timer cleared on unmount to avoid calling onChange after teardown
  const searchTimer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined)
  useEffect(() => () => clearTimeout(searchTimer.current), [])
  const handleSearch = (val: string) => {
    clearTimeout(searchTimer.current)
    searchTimer.current = setTimeout(() => onChange({ ...filtersRef.current, query: val }), 200)
  }

  const selectedRating = RATING_OPTIONS.find(o => o.value === filters.rating) ?? RATING_OPTIONS[0]
  const selectedWindow = WINDOW_OPTIONS.find(o => o.value === filters.window) ?? WINDOW_OPTIONS[1]

  return (
    <div className={styles.bar}>
      <ListboxChip
        label="Window"
        displayValue={selectedWindow.short}
        options={WINDOW_OPTIONS}
        value={filters.window}
        onChange={v => onChange({ ...filters, window: v })}
        active={filters.window !== 48}
        ariaLabel={`Time window: ${selectedWindow.label}`}
        popupLabel="Time window"
      />

      <ListboxChip
        label="Rating"
        // "★★★★★  5 stars" → "5 stars" for the compact chip display.
        displayValue={filters.rating === 'any' ? 'Any' : selectedRating.label.split('  ')[1]}
        options={RATING_OPTIONS}
        value={filters.rating}
        onChange={v => onChange({ ...filters, rating: v })}
        active={filters.rating !== 'any'}
        ariaLabel={`Rating filter: ${selectedRating.label}`}
        popupLabel="Rating filter"
      />

      {/* Search */}
      <div className={styles.search}>
        <SearchIcon />
        <input
          className={styles.searchInput}
          aria-label="Search reviews by author, title, or content"
          placeholder="Search author, title, or content…"
          defaultValue={filters.query}
          onChange={e => handleSearch(e.target.value)}
        />
      </div>

      <div className={styles.spacer} />

      {/* Footer count */}
      <span className={styles.count}>
        — {shown} of {total} reviews · {selectedWindow.label.toLowerCase()} —
      </span>

      {/* Sort */}
      <div className={styles.sort} aria-label="Sort order: Newest first">
        <span className={styles.sortLabel}>Sort</span>
        <span className={styles.sortValue}>Newest first</span>
      </div>
    </div>
  )
}

function SearchIcon() {
  return (
    <svg width="13" height="13" viewBox="0 0 13 13" fill="none" aria-hidden="true">
      <circle cx="5.5" cy="5.5" r="4" stroke="currentColor" strokeWidth="1.3" />
      <path d="M8.5 8.5l2.5 2.5" stroke="currentColor" strokeWidth="1.3" strokeLinecap="round" />
    </svg>
  )
}
