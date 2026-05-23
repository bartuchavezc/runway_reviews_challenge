import { useState, useRef, useEffect } from 'react'
import styles from './FilterBar.module.css'

export interface ListboxOption<T extends string | number> {
  value: T
  label: string
}

interface Props<T extends string | number> {
  // Visible "Window" / "Rating" label rendered before the value.
  label: string
  // Shortened display text shown after the label (e.g. "1W" or "Any").
  displayValue: string
  options: ReadonlyArray<ListboxOption<T>>
  value: T
  onChange: (v: T) => void
  // Whether the chip should render in its accent-coloured "active" state.
  active: boolean
  // Full sentence read by screen readers when the trigger receives focus.
  ariaLabel: string
  // Label announced for the listbox popup itself.
  popupLabel: string
}

// A single Window/Rating-style filter chip with an attached listbox popup.
// Implements the WAI-ARIA listbox pattern: aria-haspopup on the trigger,
// role="listbox" + role="option" with aria-selected, Arrow-key cycling,
// Escape closes and restores focus to the trigger, Tab closes without
// trapping. Outside-click also closes. Refactored out of FilterBar so the
// Window and Rating chips share a single implementation.
export function ListboxChip<T extends string | number>({
  label,
  displayValue,
  options,
  value,
  onChange,
  active,
  ariaLabel,
  popupLabel,
}: Props<T>) {
  const [open, setOpen] = useState(false)
  const wrapperRef = useRef<HTMLDivElement>(null)
  const triggerRef = useRef<HTMLButtonElement>(null)
  const itemRefs = useRef<(HTMLButtonElement | null)[]>([])

  // Outside-click closes the popup.
  useEffect(() => {
    if (!open) return
    const handler = (e: MouseEvent) => {
      if (!wrapperRef.current?.contains(e.target as Node)) setOpen(false)
    }
    document.addEventListener('mousedown', handler)
    return () => document.removeEventListener('mousedown', handler)
  }, [open])

  // Focus the currently selected option when the popup opens. Deliberately
  // does NOT depend on `value` — the focus restoration should only run on
  // open, not when the user picks a different option mid-interaction.
  useEffect(() => {
    if (!open) return
    const idx = options.findIndex(o => o.value === value)
    itemRefs.current[Math.max(0, idx)]?.focus()
  }, [open]) // eslint-disable-line react-hooks/exhaustive-deps

  function handleKey(e: React.KeyboardEvent, index: number) {
    if (e.key === 'Escape') {
      e.preventDefault()
      setOpen(false)
      triggerRef.current?.focus()
    } else if (e.key === 'Tab') {
      // WAI-ARIA listbox: Tab commits focus to the next sequential control
      // rather than trapping inside the popup. We just close so the visual
      // state matches where focus has gone.
      setOpen(false)
    } else if (e.key === 'ArrowDown') {
      e.preventDefault()
      itemRefs.current[(index + 1) % options.length]?.focus()
    } else if (e.key === 'ArrowUp') {
      e.preventDefault()
      itemRefs.current[(index - 1 + options.length) % options.length]?.focus()
    }
  }

  return (
    <div className={styles.dropWrapper} ref={wrapperRef}>
      <button
        ref={triggerRef}
        className={`${styles.chip} ${active ? styles.chipActive : ''}`}
        onClick={() => setOpen(o => !o)}
        aria-haspopup="listbox"
        aria-expanded={open}
        aria-label={ariaLabel}
      >
        <span className={styles.chipLabel}>{label}</span>
        <span className={styles.chipValue}>{displayValue}</span>
        <ChevronIcon />
      </button>

      {open && (
        <div
          className={styles.dropdown}
          role="listbox"
          aria-label={popupLabel}
        >
          {options.map((opt, i) => (
            <button
              key={String(opt.value)}
              ref={el => { itemRefs.current[i] = el }}
              role="option"
              aria-selected={value === opt.value}
              className={`${styles.dropItem} ${value === opt.value ? styles.dropItemActive : ''}`}
              onClick={() => {
                onChange(opt.value)
                setOpen(false)
                // Return focus to the trigger so keyboard users land on a
                // sensible next element instead of document.body.
                triggerRef.current?.focus()
              }}
              onKeyDown={e => handleKey(e, i)}
            >
              <span>{opt.label}</span>
              {value === opt.value && <CheckIcon />}
            </button>
          ))}
        </div>
      )}
    </div>
  )
}

function ChevronIcon() {
  return (
    <svg width="9" height="9" viewBox="0 0 9 9" fill="none" style={{ opacity: 0.45 }} aria-hidden="true">
      <path d="M2 3.5l2.5 2.5 2.5-2.5" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  )
}

function CheckIcon() {
  return (
    <svg width="13" height="13" viewBox="0 0 13 13" fill="none" aria-hidden="true">
      <path d="M2.5 6.5l3 3 5-5" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  )
}
