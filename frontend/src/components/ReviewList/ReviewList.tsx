import { useState, useEffect } from 'react'
import type { Review } from '../../types'
import { timeAgo } from '../../lib/utils'
import styles from './ReviewList.module.css'

interface Props {
  reviews: Review[]
  loading: boolean
  error: string | null
  onRetry: () => void
}

export function ReviewList({ reviews, loading, error, onRetry }: Props) {
  // If the fetch hangs without erroring (e.g. stalled connection), stop showing
  // the shimmer after 15 s and offer a retry instead of looping forever.
  // setLoadTooLong is the effect's job — synchronizing a derived "has the
  // current load gone on long enough?" flag with the loading prop and a timer.
  const [loadTooLong, setLoadTooLong] = useState(false)
  /* eslint-disable react-hooks/set-state-in-effect */
  useEffect(() => {
    if (!loading) { setLoadTooLong(false); return }
    const id = setTimeout(() => setLoadTooLong(true), 15_000)
    return () => clearTimeout(id)
  }, [loading])
  /* eslint-enable react-hooks/set-state-in-effect */

  if (error || loadTooLong) {
    return (
      <div className={styles.errorBanner}>
        {error ?? 'Server is taking too long'} — <button className={styles.retry} onClick={onRetry}>Retry</button>
      </div>
    )
  }

  if (loading && reviews.length === 0) {
    return (
      // role="status" + aria-live so screen readers announce the loading
      // state. Without this, an SR user just hears silence while the
      // shimmer is up and has no way to know a fetch is in flight.
      <div className={styles.list} role="status" aria-live="polite" aria-busy="true">
        <span className={styles.srOnly}>Loading reviews…</span>
        {[...Array(5)].map((_, i) => <ShimmerRow key={i} />)}
      </div>
    )
  }

  if (reviews.length === 0) {
    return (
      <div className={styles.empty} role="status">
        — no reviews match —
      </div>
    )
  }

  return (
    // role="list" + role="listitem" on each article gives screen readers
    // a posinset/setsize-style count ("list with 24 items") that an
    // anonymous <div> of <article>s otherwise wouldn't expose.
    <div className={styles.list} role="list" aria-label="Reviews">
      {reviews.map(r => <ReviewCard key={r.id} review={r} />)}
    </div>
  )
}

function ReviewCard({ review: r }: { review: Review }) {
  const scoreClass = r.score <= 2 ? styles.red : r.score === 3 ? styles.amber : styles.green

  return (
    <article className={styles.row} role="listitem" aria-label={`${r.score} out of 5 stars — ${r.title}`}>
      <div className={`${styles.scoreBlock} ${scoreClass}`} aria-hidden="true">
        <span className={styles.scoreDigit}>{r.score}</span>
        <div className={styles.stars}>
          {[1,2,3,4,5].map(i => (
            <StarIcon key={i} filled={i <= r.score} />
          ))}
        </div>
      </div>

      <div className={styles.content}>
        <h3 className={styles.title}>{r.title}</h3>
        <p className={styles.body}>{r.body}</p>
        <div className={styles.meta}>
          <span className={styles.author}>@{r.author}</span>
          <span className={styles.sep}>·</span>
          <span className={styles.timeAgo}>{timeAgo(r.submittedAt)}</span>
          {r.version && <>
            <span className={styles.sep}>·</span>
            <span className={styles.timestamp}>v{r.version}</span>
          </>}
          <div className={styles.spacer} />
        </div>
      </div>
    </article>
  )
}

function ShimmerRow() {
  return (
    <div className={styles.shimmerRow}>
      <div className={`${styles.shimmerBlock} ${styles.shimmer}`} />
      <div className={styles.shimmerContent}>
        <div className={`${styles.shimmerTitle} ${styles.shimmer}`} />
        <div className={`${styles.shimmerBody} ${styles.shimmer}`} />
      </div>
    </div>
  )
}

function StarIcon({ filled }: { filled: boolean }) {
  return (
    <svg width="9" height="9" viewBox="0 0 12 12" aria-hidden="true">
      <path
        d="M6 1l1.5 3.2 3.5.4-2.6 2.4.7 3.4L6 8.8 2.9 10.4l.7-3.4L1 4.6l3.5-.4z"
        fill={filled ? 'currentColor' : 'none'}
        stroke="currentColor"
        strokeWidth={filled ? 0 : 1.2}
        opacity={filled ? 1 : 0.3}
      />
    </svg>
  )
}


