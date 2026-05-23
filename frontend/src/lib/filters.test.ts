import { describe, it, expect } from 'vitest'
import { applyFilters } from './filters'
import type { Review, Filters } from '../types'

function review(overrides: Partial<Review> = {}): Review {
  return {
    id: 'r1',
    appId: '1',
    score: 5,
    title: 'Title',
    body: 'Body',
    author: 'alice',
    version: '1.0',
    submittedAt: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

const baseFilters: Filters = { rating: 'any', query: '', window: 48 }

describe('applyFilters', () => {
  it('returns reviews unchanged when rating=any and query is empty', () => {
    const reviews = [review({ id: 'a', score: 5 }), review({ id: 'b', score: 1 })]
    expect(applyFilters(reviews, baseFilters)).toEqual(reviews)
  })

  it('rating eq5 keeps only 5-star reviews', () => {
    const reviews = [
      review({ id: 'a', score: 5 }),
      review({ id: 'b', score: 4 }),
      review({ id: 'c', score: 1 }),
    ]
    const out = applyFilters(reviews, { ...baseFilters, rating: 'eq5' })
    expect(out.map(r => r.id)).toEqual(['a'])
  })

  it('rating ge4 keeps 4-star and above', () => {
    const reviews = [
      review({ id: 'a', score: 5 }),
      review({ id: 'b', score: 4 }),
      review({ id: 'c', score: 3 }),
      review({ id: 'd', score: 1 }),
    ]
    const out = applyFilters(reviews, { ...baseFilters, rating: 'ge4' })
    expect(out.map(r => r.id)).toEqual(['a', 'b'])
  })

  it('rating eq3 keeps only 3-star reviews', () => {
    const reviews = [
      review({ id: 'a', score: 4 }),
      review({ id: 'b', score: 3 }),
      review({ id: 'c', score: 2 }),
    ]
    const out = applyFilters(reviews, { ...baseFilters, rating: 'eq3' })
    expect(out.map(r => r.id)).toEqual(['b'])
  })

  it('rating le2 keeps only 1 and 2-star reviews', () => {
    const reviews = [
      review({ id: 'a', score: 3 }),
      review({ id: 'b', score: 2 }),
      review({ id: 'c', score: 1 }),
    ]
    const out = applyFilters(reviews, { ...baseFilters, rating: 'le2' })
    expect(out.map(r => r.id)).toEqual(['b', 'c'])
  })

  it('query matches case-insensitively against title, body, and author', () => {
    const reviews = [
      review({ id: 'a', title: 'Great app' }),
      review({ id: 'b', body: 'I love this APP so much' }),
      review({ id: 'c', author: 'AppFan99' }),
      review({ id: 'd', title: 'Other', body: 'Other', author: 'someone' }),
    ]
    const out = applyFilters(reviews, { ...baseFilters, query: 'app' })
    expect(out.map(r => r.id)).toEqual(['a', 'b', 'c'])
  })

  it('whitespace-only query is treated as no filter', () => {
    const reviews = [review({ id: 'a' }), review({ id: 'b' })]
    const out = applyFilters(reviews, { ...baseFilters, query: '   ' })
    expect(out).toEqual(reviews)
  })

  it('rating and query compose — both filters apply (AND)', () => {
    const reviews = [
      review({ id: 'a', score: 5, body: 'love it' }),
      review({ id: 'b', score: 5, body: 'meh' }),
      review({ id: 'c', score: 1, body: 'love it' }),
    ]
    const out = applyFilters(reviews, { rating: 'eq5', query: 'love', window: 48 })
    expect(out.map(r => r.id)).toEqual(['a'])
  })

  it('returns an empty array when nothing matches', () => {
    const reviews = [review({ id: 'a', score: 5 })]
    const out = applyFilters(reviews, { ...baseFilters, rating: 'le2' })
    expect(out).toEqual([])
  })

  it('does not mutate the input array', () => {
    const reviews = [review({ id: 'a', score: 5 }), review({ id: 'b', score: 1 })]
    const before = [...reviews]
    applyFilters(reviews, { ...baseFilters, rating: 'eq5' })
    expect(reviews).toEqual(before)
  })
})
