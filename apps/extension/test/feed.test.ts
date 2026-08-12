import { expect, test } from 'bun:test'
import {
  DEFAULT_RANGE,
  harvestedFrom,
  knobsKey,
  mergePages,
  morePages,
  NO_KNOBS,
  rangeOrDefault,
  rowsFor,
  searchFor,
  unseenCount,
} from '@/lib/linkedin/feed'
import { buildQuery, PAGE_SIZE, type Card } from '@/lib/linkedin/voyager'

function card(id: string, title = `posting ${id}`): Card {
  return {
    id,
    title,
    company: 'northwind',
    location: 'remote',
    url: `https://www.linkedin.com/jobs/view/${id}/`,
    listedAt: 1_770_000_000_000,
    reposted: false,
    easyApply: true,
  }
}

function cards(ids: string[]): Card[] {
  return ids.map((id) => card(id))
}

test('the feed sends a place, a window, a term and the two knobs', () => {
  const query = searchFor({
    place: 'somewhere',
    locationId: '102135806',
    keyword: 'Spring Boot',
    range: 'r21600',
    easyApply: true,
    remoteOnly: true,
  })

  expect(query).toEqual({
    locationId: '102135806',
    keyword: 'Spring Boot',
    range: 'r21600',
    easyApply: true,
    remoteOnly: true,
  })
  const built = buildQuery(query)
  expect(built).toContain('sortBy:List(DD)')
  expect(built).toContain('timePostedRange:List(r21600)')
  expect(built).toContain('applyWithLinkedin:List(true)')
  expect(built).toContain('workplaceType:List(2)')
  expect(built).toContain('keywords:Spring%20Boot')
})

test('an empty term sends no keyword clause at all', () => {
  const built = buildQuery(searchFor({ ...NO_KNOBS, locationId: '102135806' }))
  expect(built).not.toContain('keywords')
})

test('a window linkedin does not take falls back to the default one', () => {
  expect(rangeOrDefault('r604800')).toBe('r604800')
  expect(rangeOrDefault('r1')).toBe(DEFAULT_RANGE)
  expect(searchFor({ ...NO_KNOBS, locationId: '1' }).range).toBe(DEFAULT_RANGE)
})

test('changing any knob is a different feed', () => {
  const base = { ...NO_KNOBS, locationId: '1' }
  expect(knobsKey(base)).toBe(knobsKey({ ...base }))
  expect(knobsKey(base)).not.toBe(knobsKey({ ...base, locationId: '2' }))
  expect(knobsKey(base)).not.toBe(knobsKey({ ...base, range: 'r3600' }))
  expect(knobsKey(base)).not.toBe(knobsKey({ ...base, easyApply: true }))
  expect(knobsKey(base)).not.toBe(knobsKey({ ...base, remoteOnly: true }))
})

test('a later page appends and never repeats a card already held', () => {
  const held = cards(['1', '2'])
  expect(mergePages(held, cards(['2', '3'])).map((entry) => entry.id)).toEqual(['1', '2', '3'])
})

test('what the agent judged or the person opened never appears in the feed', () => {
  const handled = new Set(['2'])
  const known = new Set(['1'])
  const rows = rowsFor(cards(['1', '2', '3']), handled, known)

  expect(rows.map((row) => row.card.id)).toEqual(['1', '3'])
  expect(rows.map((row) => row.unseen)).toEqual([false, true])
  expect(unseenCount(rows)).toBe(1)
})

test('paging stops on a short page, on the total, and on the page ceiling', () => {
  const full = cards(Array.from({ length: PAGE_SIZE }, (_, index) => `f${index}`))

  expect(morePages(full, { cards: full, total: 500, start: 0 })).toBe(true)
  expect(morePages(full, { cards: cards(['a']), total: 500, start: 0 })).toBe(false)
  expect(morePages(full, { cards: full, total: PAGE_SIZE, start: 0 })).toBe(false)

  const ceiling = cards(Array.from({ length: PAGE_SIZE * 20 }, (_, index) => `c${index}`))
  expect(morePages(ceiling, { cards: full, total: 5_000, start: 0 })).toBe(false)
})

test('a card becomes exactly what the agent stores, with no description invented', () => {
  expect(harvestedFrom(card('9'))).toEqual({
    id: '9',
    title: 'posting 9',
    company: 'northwind',
    location: 'remote',
    url: 'https://www.linkedin.com/jobs/view/9/',
    description: '',
    listedAt: 1_770_000_000_000,
    easyApply: true,
  })
})

test('no title ever decides what the feed shows', () => {
  const mixed = [card('1', 'Senior Software Engineer'), card('2', 'Marketing Lead')]
  const rows = rowsFor(mixed, new Set(), new Set())
  expect(rows.map((row) => row.card.title)).toEqual([
    'Senior Software Engineer',
    'Marketing Lead',
  ])
})
