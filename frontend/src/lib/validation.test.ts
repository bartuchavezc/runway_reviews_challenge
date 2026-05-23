import { describe, it, expect } from 'vitest'
import { RSS_REGEX } from './validation'

describe('RSS_REGEX', () => {
  it('accepts the canonical assignment URL', () => {
    expect(RSS_REGEX.test('https://itunes.apple.com/us/rss/customerreviews/id=595068606/sortBy=mostRecent/page=1/json')).toBe(true)
  })

  it('accepts a URL with the trailing slash and no extra segments', () => {
    expect(RSS_REGEX.test('https://itunes.apple.com/us/rss/customerreviews/id=123/')).toBe(true)
  })

  it('captures the numeric app id', () => {
    const match = RSS_REGEX.exec('https://itunes.apple.com/us/rss/customerreviews/id=447188370/sortBy=mostRecent/json')
    expect(match?.[1]).toBe('447188370')
  })

  it('rejects http:// (must be https)', () => {
    expect(RSS_REGEX.test('http://itunes.apple.com/us/rss/customerreviews/id=123/')).toBe(false)
  })

  it('rejects a different host', () => {
    expect(RSS_REGEX.test('https://attacker.example.com/us/rss/customerreviews/id=123/')).toBe(false)
  })

  it('rejects an unanchored prefix (no leading garbage)', () => {
    expect(RSS_REGEX.test('foohttps://itunes.apple.com/us/rss/customerreviews/id=123/')).toBe(false)
  })

  it('rejects a non-numeric app id', () => {
    expect(RSS_REGEX.test('https://itunes.apple.com/us/rss/customerreviews/id=abc/json')).toBe(false)
  })

  it('rejects a missing app id', () => {
    expect(RSS_REGEX.test('https://itunes.apple.com/us/rss/customerreviews/id=/json')).toBe(false)
  })

  it('rejects an uppercase country code', () => {
    expect(RSS_REGEX.test('https://itunes.apple.com/US/rss/customerreviews/id=123/')).toBe(false)
  })

  it('rejects a missing trailing slash after the id', () => {
    expect(RSS_REGEX.test('https://itunes.apple.com/us/rss/customerreviews/id=123')).toBe(false)
  })
})
