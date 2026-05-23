import { describe, it, expect } from 'vitest'
import { initials, tint, windowLabel } from './display'

describe('initials', () => {
  it('returns ?? for the empty string', () => {
    expect(initials('')).toBe('??')
  })

  it('first two letters for a single-word name', () => {
    expect(initials('Snapchat')).toBe('SN')
    expect(initials('duolingo')).toBe('DU')
  })

  it('first letter of first two letter-starting words', () => {
    expect(initials('Spotify Music')).toBe('SM')
    expect(initials('Tab - The simple bill splitter')).toBe('TT')
  })

  it('drops em-dashes and trailing punctuation', () => {
    // "Notion – Notes, Docs, Tasks" → first two letter-words are "Notion" + "Notes"
    expect(initials('Notion – Notes, Docs, Tasks')).toBe('NN')
  })

  it('handles single-letter names by repeating the letter', () => {
    expect(initials('A')).toBe('AA')
  })

  it('uppercase output regardless of input case', () => {
    expect(initials('abc def')).toBe('AD')
    expect(initials('apple')).toBe('AP')
  })
})

describe('tint', () => {
  it('returns a hex color from the palette', () => {
    expect(tint('1')).toMatch(/^#[0-9a-f]{6}$/i)
  })

  it('is deterministic — same id always yields the same color', () => {
    expect(tint('595068606')).toBe(tint('595068606'))
    expect(tint('snap')).toBe(tint('snap'))
  })

  it('distributes across the palette — different ids usually differ', () => {
    // The palette has 8 entries; 8 distinct ids should produce >1 unique color.
    const colors = new Set(['a', 'b', 'c', 'd', 'e', 'f', 'g', 'h'].map(tint))
    expect(colors.size).toBeGreaterThan(1)
  })

  it('never returns undefined / null for any non-empty id', () => {
    for (const id of ['1', '12345', 'abc', 'A', 'snap']) {
      expect(tint(id)).toBeTruthy()
    }
  })
})

describe('windowLabel', () => {
  it('returns short labels for the canonical window sizes', () => {
    expect(windowLabel(24)).toBe('24h')
    expect(windowLabel(48)).toBe('48h')
    expect(windowLabel(72)).toBe('3d')
    expect(windowLabel(168)).toBe('1W')
    expect(windowLabel(720)).toBe('1M')
  })

  it('falls back to "Nh" for non-canonical values', () => {
    expect(windowLabel(96)).toBe('96h')
    expect(windowLabel(1)).toBe('1h')
  })
})
