import type { App } from '../../types'
import { initials, tint, windowLabel } from '../../lib/display'
import styles from './AppHeader.module.css'

interface Props {
  app: App
  avgScore: number | null
  // windowHours / windowCount drive the leftmost stat tile so it matches
  // whatever the user picked in the FilterBar (was hardcoded "Reviews (48h)"
  // when only the 48h window existed).
  windowHours: number
  windowCount: number
}

export function AppHeader({ app, avgScore, windowHours, windowCount }: Props) {
  return (
    <header className={styles.header}>
      <div className={styles.icon} style={{ background: tint(app.id) }}>
        {initials(app.name)}
      </div>

      <div className={styles.meta}>
        <h1 className={styles.name}>{app.name}</h1>
      </div>

      <div className={styles.stats}>
        <StatTile label={`Reviews (${windowLabel(windowHours)})`} value={String(windowCount)} />
        <StatTile label="All-time" value={String(app.totalReviews)} />
        <StatTile label="Avg Score" value={avgScore !== null ? avgScore.toFixed(1) : '—'} />
      </div>
    </header>
  )
}

function StatTile({ label, value }: { label: string; value: string }) {
  return (
    <div className={styles.tile}>
      <span className={styles.tileValue}>{value}</span>
      <span className={styles.tileLabel}>{label}</span>
    </div>
  )
}
