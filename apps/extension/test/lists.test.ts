import { expect, test } from 'bun:test'
import type { Posting } from '@/lib/agent/types'
import { departed } from '@/lib/exit'
import { countLists, groupLists, oldestFirst, postingAge } from '@/lib/lists'

function posting(id: string, status: Posting['status'], listedAt: number | null = null): Posting {
  return {
    id,
    title: id,
    company: '',
    location: '',
    url: '',
    dupeOf: null,
    description: '',
    descriptionState: 'ok',
    listedAt,
    easyApply: true,
    status,
    stage: 'deep',
    score: null,
    triageReason: null,
    verdictReason: null,
    seniority: null,
    workplace: null,
    contractType: null,
    postingLang: null,
    agency: null,
    statedPay: null,
    resumeCode: null,
    resumeLang: null,
    resumeFit: null,
    tailoredResume: null,
    stale: false,
    outcome: null,
    outcomeNote: null,
    cycleId: null,
    queryLabel: null,
    createdAt: '',
    updatedAt: '',
  }
}

const rows = [
  posting('a', 'inbox'),
  posting('b', 'inbox'),
  posting('c', 'manual'),
  posting('d', 'queue'),
  posting('e', 'applied'),
  posting('f', 'new'),
  posting('g', 'skipped'),
]

test('the four lists carry only what the agent put in them', () => {
  const lists = groupLists(rows, new Set())
  expect(countLists(lists)).toEqual({ inbox: 2, manual: 1, queue: 1, applied: 1 })
})

test('a handled posting leaves the lists that propose work and stays in the record', () => {
  const lists = groupLists(rows, new Set(['a', 'c', 'd', 'e']))

  expect(lists.inbox.map((row) => row.id)).toEqual(['b'])
  expect(lists.manual.map((row) => row.id)).toEqual([])
  expect(lists.queue.map((row) => row.id)).toEqual(['d'])
  expect(lists.applied.map((row) => row.id)).toEqual(['e'])
})

test('every list is oldest first, so the longest wait is worked first', () => {
  const hour = 60 * 60 * 1000
  const now = Date.parse('2026-01-01T12:00:00.000Z')
  const feed = [
    posting('newest', 'inbox', now - hour),
    posting('oldest', 'inbox', now - 40 * hour),
    posting('middle', 'inbox', now - 9 * hour),
  ]

  expect(oldestFirst(feed).map((row) => row.id)).toEqual(['oldest', 'middle', 'newest'])
  expect(groupLists(feed, new Set()).inbox.map((row) => row.id)).toEqual([
    'oldest',
    'middle',
    'newest',
  ])
})

test('a posting with no listing time is aged by when it arrived, and ties break by id', () => {
  const arrived = { ...posting('b', 'inbox'), createdAt: '2026-01-01T09:00:00.000Z' }
  const later = { ...posting('a', 'inbox'), createdAt: '2026-01-01T11:00:00.000Z' }

  expect(postingAge(arrived)).toBe(Date.parse('2026-01-01T09:00:00.000Z'))
  expect(oldestFirst([later, arrived]).map((row) => row.id)).toEqual(['b', 'a'])
  expect(oldestFirst([posting('b', 'inbox'), posting('a', 'inbox')]).map((row) => row.id)).toEqual([
    'a',
    'b',
  ])
})

test('a row only leaves the feed when the fresh list no longer holds it', () => {
  const shown = [posting('a', 'inbox'), posting('b', 'inbox'), posting('c', 'inbox')]
  expect(departed(shown, [shown[0]!, shown[2]!])).toEqual(['b'])
  expect(departed(shown, shown)).toEqual([])
  expect(departed(shown, [])).toEqual(['a', 'b', 'c'])
})
