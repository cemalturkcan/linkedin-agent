import type { Harvested } from '@/lib/agent/types'
import { hideHandled } from '@/lib/handled'
import { PAGE_SIZE, RANGES, termOf, type Card, type SearchQuery } from '@/lib/linkedin/voyager'

export const FEED_PAGES_MAX = 20
export const DEFAULT_RANGE = 'r86400'

export const RANGE_WORDS: Record<string, string> = {
  r3600: 'last hour',
  r21600: 'last 6 hours',
  r86400: 'last 24 hours',
  r604800: 'last week',
}

export interface FeedKnobs {
  place: string
  locationId: string
  keyword: string
  range: string
  easyApply: boolean
  remoteOnly: boolean
}

export const NO_KNOBS: FeedKnobs = {
  place: '',
  locationId: '',
  keyword: '',
  range: DEFAULT_RANGE,
  easyApply: false,
  remoteOnly: false,
}

export interface FeedRow {
  card: Card
  unseen: boolean
}

export interface FeedPage {
  cards: Card[]
  total: number
  start: number
}

export function rangeOrDefault(value: string): string {
  return (RANGES as readonly string[]).includes(value) ? value : DEFAULT_RANGE
}

export function searchFor(knobs: FeedKnobs): SearchQuery {
  return {
    locationId: knobs.locationId,
    keyword: termOf(knobs.keyword),
    range: rangeOrDefault(knobs.range),
    easyApply: knobs.easyApply,
    remoteOnly: knobs.remoteOnly,
  }
}

export function knobsKey(knobs: FeedKnobs): string {
  return [
    knobs.locationId,
    termOf(knobs.keyword),
    rangeOrDefault(knobs.range),
    knobs.easyApply ? 'easy' : '',
    knobs.remoteOnly ? 'remote' : '',
  ].join('|')
}

export function mergePages(held: Card[], arrived: Card[]): Card[] {
  const seen = new Set(held.map((card) => card.id))
  const merged = [...held]
  for (const card of arrived) {
    if (seen.has(card.id)) continue
    seen.add(card.id)
    merged.push(card)
  }
  return merged
}

export function rowsFor(cards: Card[], handled: Set<string>, known: Set<string>): FeedRow[] {
  return hideHandled(cards, handled).map((card) => ({ card, unseen: !known.has(card.id) }))
}

export function unseenCount(rows: FeedRow[]): number {
  return rows.filter((row) => row.unseen).length
}

export function morePages(cards: Card[], page: FeedPage): boolean {
  if (page.cards.length < PAGE_SIZE) return false
  if (page.total > 0 && page.start + PAGE_SIZE >= page.total) return false
  return cards.length < PAGE_SIZE * FEED_PAGES_MAX
}

export function harvestedFrom(card: Card): Harvested {
  return {
    id: card.id,
    title: card.title,
    company: card.company,
    location: card.location,
    url: card.url,
    description: '',
    listedAt: card.listedAt,
    easyApply: card.easyApply,
  }
}
