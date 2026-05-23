import { useState, useEffect, useRef } from 'react'
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
  const [confirmDelete, setConfirmDelete] = useState<string | null>(null)
  const cancelDeleteRef = useRef<HTMLButtonElement | null>(null)
  // Per-row × button refs so we can restore focus when the confirm chip
  // dismisses without deletion. Keyed by app id; entries are GC'd
  // implicitly when the row unmounts and React drops the ref callback.
  const removeButtonRefs = useRef<Map<string, HTMLButtonElement | null>>(new Map())
  // Focus fallback for the case where the user deletes the last (or only)
  // app — there's no neighbouring × button to land on, so we focus the
  // Tracked apps heading instead via tabIndex={-1}.
  const trackedAppsHeadingRef = useRef<HTMLHeadingElement>(null)

  // Move focus to the safer "No" button as soon as the inline confirm chip
  // mounts. Keyboard users would otherwise be left focusing the now-unmounted
  // × button and have no clear way to dismiss without reaching for the mouse.
  useEffect(() => {
    if (confirmDelete) cancelDeleteRef.current?.focus()
  }, [confirmDelete])

  // Cancel the inline confirm without deleting. Restores focus to the row's
  // × button so keyboard users don't get dropped on document.body.
  const cancelConfirm = (appId: string) => {
    setConfirmDelete(null)
    queueMicrotask(() => removeButtonRefs.current.get(appId)?.focus())
  }

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
    // Pre-compute the focus target before the row unmounts: try the next
    // row's × button, fall back to the previous row's, then the table
    // heading (which we make focusable via tabIndex={-1}). Without this,
    // focus lands on document.body once the row disappears and keyboard
    // users have to Tab from the top again.
    const idx = apps.findIndex(a => a.id === id)
    const fallbackId = apps[idx + 1]?.id ?? apps[idx - 1]?.id ?? null

    try {
      await onRemove(id)
      setConfirmDelete(null)
      queueMicrotask(() => {
        if (fallbackId) {
          removeButtonRefs.current.get(fallbackId)?.focus()
        } else {
          trackedAppsHeadingRef.current?.focus()
        }
      })
    } catch (e) {
      setStatus('error')
      setMessage(e instanceof Error ? e.message : 'Failed to remove app')
      setConfirmDelete(null)
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
            <h2 className={styles.h2} ref={trackedAppsHeadingRef} tabIndex={-1}>Tracked apps</h2>
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
                  {confirmDelete === app.id ? (
                    <div
                      className={`${styles.chip} ${styles.chipRed} ${styles.inlineConfirm}`}
                      onKeyDown={e => { if (e.key === 'Escape') cancelConfirm(app.id) }}
                    >
                      <span>Remove?</span>
                      <button className={styles.confirmYes} onClick={() => handleRemove(app.id)}>Yes</button>
                      <button
                        ref={cancelDeleteRef}
                        className={styles.confirmNo}
                        onClick={() => cancelConfirm(app.id)}
                      >No</button>
                    </div>
                  ) : (
                    <button
                      ref={el => { removeButtonRefs.current.set(app.id, el) }}
                      className={styles.removeBtn}
                      onClick={() => setConfirmDelete(app.id)}
                      aria-label={`Remove ${app.name}`}
                    >×</button>
                  )}
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
