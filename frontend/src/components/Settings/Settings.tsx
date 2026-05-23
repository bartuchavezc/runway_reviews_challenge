import { useState } from 'react'
import type { App } from '../../types'
import { initials, tint } from '../../lib/display'
import { RSS_REGEX } from '../../lib/validation'
import styles from './Settings.module.css'

interface Props {
  apps: App[]
  onAdd: (rssUrl: string) => Promise<App>
  onRemove: (id: string) => Promise<void>
  rssInputRef?: React.RefObject<HTMLInputElement | null>
}

export function Settings({ apps, onAdd, onRemove, rssInputRef }: Props) {
  const [url, setUrl] = useState('')
  const [status, setStatus] = useState<'idle' | 'loading' | 'success' | 'error'>('idle')
  const [message, setMessage] = useState('')

  const handleAdd = async () => {
    if (!RSS_REGEX.test(url)) {
      setStatus('error')
      setMessage('Invalid RSS URL — must be an itunes.apple.com customer reviews link')
      return
    }
    setStatus('loading')
    try {
      const app = await onAdd(url)
      setStatus('success')
      setMessage(`Parsed appId ${app.id} · resolves to ${app.name}`)
      setUrl('')
    } catch (e) {
      setStatus('error')
      setMessage(e instanceof Error ? e.message : 'Failed to add app')
    }
  }

  const handleRemove = async (id: string) => {
    try {
      await onRemove(id)
    } catch (e) {
      setStatus('error')
      setMessage(e instanceof Error ? e.message : 'Failed to remove app')
    }
  }

  return (
    <div className={styles.page}>
      <div className={styles.pageHeader}>
        <h1 className={styles.h1}>Settings</h1>
        <p className={styles.subtitle}>Manage tracked apps and review polling.</p>
      </div>

      <div className={styles.body}>
        {/* Add an app */}
        <section className={styles.section}>
          <h2 className={styles.h2}>Add an app</h2>
          <p className={styles.desc}>
            Paste an App Store Connect customer reviews RSS URL to start tracking an app.
          </p>

          <label className={styles.fieldLabel} htmlFor="rss-url-input">RSS URL</label>
          <div className={styles.inputRow}>
            <input
              id="rss-url-input"
              ref={rssInputRef}
              className={styles.input}
              value={url}
              onChange={e => { setUrl(e.target.value); setStatus('idle') }}
              onKeyDown={e => e.key === 'Enter' && handleAdd()}
              placeholder="https://itunes.apple.com/us/rss/customerreviews/id=…/json"
              spellCheck={false}
            />
            <button
              className={styles.addBtn}
              onClick={handleAdd}
              disabled={status === 'loading'}
            >
              {status === 'loading' ? 'Adding…' : 'Add app'}
            </button>
          </div>

          {/* role="status" + aria-live so screen readers announce the
              add-app result without taking focus away from the input. */}
          <div role="status" aria-live="polite" aria-atomic="true">
            {status === 'success' && (
              <div className={`${styles.chip} ${styles.chipGreen}`}>
                <CheckIcon />
                <span>{message}</span>
              </div>
            )}
            {status === 'error' && (
              <div className={`${styles.chip} ${styles.chipRed}`}>
                <CrossIcon />
                <span>{message}</span>
              </div>
            )}
          </div>
        </section>

        {/* Tracked apps */}
        <section className={styles.section}>
          <div className={styles.tableHeader}>
            <h2 className={styles.h2}>Tracked apps</h2>
            <span className={styles.tableMeta}>{apps.length} {apps.length === 1 ? 'app' : 'apps'}</span>
          </div>

          <div className={styles.table}>
            <div className={`${styles.tableRow} ${styles.tableHead}`}>
              <span />
              <span>App</span>
              <span>App ID</span>
              <span className={styles.right}>Reviews</span>
              <span />
            </div>

            {apps.length === 0 && (
              <div className={styles.tableEmpty}>No apps tracked yet</div>
            )}

            {apps.map(app => (
              <div key={app.id} className={styles.tableRow}>
                <span
                  className={styles.tableIcon}
                  style={{ background: tint(app.id) }}
                >
                  {initials(app.name)}
                </span>
                <span className={styles.tableName}>{app.name}</span>
                <span className={`${styles.tableMono} ${styles.mute}`}>{app.id}</span>
                <span className={`${styles.tableMono} ${styles.right}`}>{app.totalReviews}</span>
                <span>
                  <button
                    className={styles.removeBtn}
                    onClick={() => handleRemove(app.id)}
                    aria-label={`Remove ${app.name}`}
                  >×</button>
                </span>
              </div>
            ))}
          </div>
        </section>
      </div>
    </div>
  )
}

function CheckIcon() {
  return (
    <svg width="12" height="12" viewBox="0 0 12 12" fill="none" aria-hidden="true">
      <path d="M2 6l3 3 5-5" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  )
}

function CrossIcon() {
  return (
    <svg width="12" height="12" viewBox="0 0 12 12" fill="none" aria-hidden="true">
      <path d="M2 2l8 8M10 2l-8 8" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" />
    </svg>
  )
}
