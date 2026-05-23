import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { timeAgo } from './utils'

// Pin Date.now() so the minute/hour/day boundaries are exact. Without this
// these tests would flake any time a CI runner ran the assertion across a
// millisecond boundary that flipped 60s→59s.
const FROZEN = new Date('2026-05-22T12:00:00.000Z')

describe('timeAgo', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.setSystemTime(FROZEN)
  })
  afterEach(() => vi.useRealTimers())

  it('returns "never" for empty string', () => {
    expect(timeAgo('')).toBe('never')
  })

  it('returns "never" for Go zero time (0001-...)', () => {
    expect(timeAgo('0001-01-01T00:00:00Z')).toBe('never')
  })

  it('returns "just now" for time less than 1 minute ago', () => {
    const iso = new Date(FROZEN.getTime() - 30_000).toISOString()
    expect(timeAgo(iso)).toBe('just now')
  })

  it('returns minutes ago for 1–59 min', () => {
    const iso = new Date(FROZEN.getTime() - 5 * 60_000).toISOString()
    expect(timeAgo(iso)).toBe('5m ago')
  })

  it('returns hours ago for 1–23 h', () => {
    const iso = new Date(FROZEN.getTime() - 2 * 60 * 60_000).toISOString()
    expect(timeAgo(iso)).toBe('2h ago')
  })

  it('returns "1m ago" at the 1 minute boundary', () => {
    const iso = new Date(FROZEN.getTime() - 60_000).toISOString()
    expect(timeAgo(iso)).toBe('1m ago')
  })

  it('returns "1h ago" at the 60-minute boundary', () => {
    const iso = new Date(FROZEN.getTime() - 60 * 60_000).toISOString()
    expect(timeAgo(iso)).toBe('1h ago')
  })

  it('returns "23h ago" just below the 24-hour boundary', () => {
    const iso = new Date(FROZEN.getTime() - 23 * 60 * 60_000).toISOString()
    expect(timeAgo(iso)).toBe('23h ago')
  })

  it('returns days ago for >= 24 h', () => {
    const iso = new Date(FROZEN.getTime() - 3 * 24 * 60 * 60_000).toISOString()
    expect(timeAgo(iso)).toBe('3d ago')
  })

  it('returns "30d ago" for 30-day window boundary', () => {
    const iso = new Date(FROZEN.getTime() - 30 * 24 * 60 * 60_000).toISOString()
    expect(timeAgo(iso)).toBe('30d ago')
  })
})
