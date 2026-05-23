import type { App, View } from '../../types'
import { initials, tint } from '../../lib/display'
import styles from './Sidebar.module.css'

interface Props {
  apps: App[]
  activeAppId: string | null
  view: View
  onSelectApp: (id: string) => void
  onSelectView: (v: View) => void
}

export function Sidebar({ apps, activeAppId, view, onSelectApp, onSelectView }: Props) {
  return (
    <nav className={styles.sidebar} aria-label="Apps and settings">
      <div className={styles.logo}>
        <div className={styles.logoMark} aria-hidden="true">R</div>
        <span className={styles.logoText}>Reviews</span>
      </div>

      <div className={styles.section}>
        <h2 className={styles.sectionHeader}>
          Apps <span className={styles.sectionCount} aria-hidden="true">{apps.length}</span>
          <span className={styles.srOnly}> ({apps.length} tracked)</span>
        </h2>

        <ul className={styles.appList}>
          {apps.map(app => {
            const active = view === 'main' && activeAppId === app.id
            return (
              <li key={app.id}>
                <button
                  className={`${styles.appRow} ${active ? styles.active : ''}`}
                  onClick={() => { onSelectApp(app.id); onSelectView('main') }}
                  aria-current={active ? 'page' : undefined}
                >
                  <span className={styles.swatch} style={{ background: tint(app.id) }} aria-hidden="true">
                    {initials(app.name)}
                  </span>
                  <span className={styles.appName}>{app.name}</span>
                </button>
              </li>
            )
          })}
        </ul>

        {/* K shortcut: open the Add-app page and focus the RSS input.
            This is the only entry into the Add-app view — the redundant
            bottom "Settings" gateway was removed because both routed here. */}
        <button
          className={styles.addButton}
          onClick={() => onSelectView('settings')}
          aria-keyshortcuts="K"
        >
          <PlusIcon /> Add app
          <span className={styles.shortcutHint} aria-hidden="true">
            <span className={styles.pressLabel}>Press</span>
            <kbd className={styles.key}>K</kbd>
          </span>
        </button>
      </div>
    </nav>
  )
}

function PlusIcon() {
  return (
    <svg width="11" height="11" viewBox="0 0 11 11" fill="none" aria-hidden="true">
      <path d="M5.5 1v9M1 5.5h9" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" />
    </svg>
  )
}
