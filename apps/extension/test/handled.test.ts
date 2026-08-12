import { beforeEach, expect, test } from 'bun:test'
import type { Posting } from '@/lib/agent/types'
import {
  cacheHandled,
  handledIn,
  hideHandled,
  knownIn,
  loadHandled,
  loadOpened,
  markOpened,
} from '@/lib/handled'
import { resetMemoryStore } from '@/lib/storage'

function posting(id: string, status: Posting['status']): Posting {
  return {
    id,
    title: `posting ${id}`,
    company: 'somewhere',
    location: '',
    url: `https://www.linkedin.com/jobs/view/${id}/`,
    dupeOf: null,
    description: '',
    descriptionState: 'ok',
    listedAt: null,
    easyApply: true,
    status,
    stage: 'deep',
    score: 70,
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
  posting('a', 'new'),
  posting('b', 'inbox'),
  posting('c', 'skipped'),
  posting('d', 'opened'),
  posting('e', 'reading'),
  posting('f', 'queue'),
  posting('g', 'applied'),
]

beforeEach(() => {
  resetMemoryStore()
})

test('handled is every status but new, whoever took the posting', () => {
  expect([...handledIn(rows)].sort()).toEqual(['b', 'c', 'd', 'e', 'f', 'g'])
})

test('known is every posting the store holds, judged or not', () => {
  expect([...knownIn(rows)].sort()).toEqual(['a', 'b', 'c', 'd', 'e', 'f', 'g'])
})

test('a handled posting never comes back to a feed', () => {
  const cards = [{ id: 'a' }, { id: 'b' }, { id: 'd' }, { id: 'z' }]
  expect(hideHandled(cards, handledIn(rows)).map((card) => card.id)).toEqual(['a', 'z'])
})

test('the agent’s answer is cached, so a feed still hides them with the agent down', async () => {
  await cacheHandled(handledIn(rows))
  expect([...(await loadHandled())].sort()).toEqual(['b', 'c', 'd', 'e', 'f', 'g'])
})

test('what the person opens is handled at once and kept apart from what the agent judged', async () => {
  await cacheHandled(handledIn(rows))
  await markOpened(['a'])

  expect((await loadHandled()).has('a')).toBe(true)
  expect([...(await loadOpened())]).toEqual(['a'])
})

test('opening the same posting twice keeps one entry and the newest position', async () => {
  await markOpened(['a', 'b'])
  const after = await markOpened(['a'])

  expect([...after]).toEqual(['b', 'a'])
})
