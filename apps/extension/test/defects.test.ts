import { expect, test } from 'bun:test'
import type { PlanState } from '@/lib/agent/types'
import { nearRingsFirst, ringDistance, executeRound, Reason } from '@/lib/linkedin/round'
import {
  clearsWithTheGate,
  noticeStands,
  reportStands,
  type Notice,
  type Resource,
} from '@/panel/useAgentState'
import { opensWith } from '@/panel/useManualFeed'
import { deriveTerms, derivePlaceNames } from '@/lib/presets'
import { spend } from '@/lib/format'
import { POSTING_FILTERS, filterPostings } from '@/desk/components/PostingsView'
import type { Posting } from '@/lib/agent/types'

const mixed = [
  { id: 'a', status: 'skipped', listedAt: 1 },
  { id: 'b', status: 'inbox', listedAt: 2 },
  { id: 'c', status: 'opened', listedAt: 3 },
] as Posting[]
import { buildQuery } from '@/lib/linkedin/voyager'
import { searchFor, NO_KNOBS } from '@/lib/linkedin/feed'
import { cards, harness, page, plannedRound, query } from './support/round-harness'

test('defect 1: the executor works the near rings before the far ones', () => {
  const planned = [
    { ring: 'worldwide', label: 'worldwide-remote-day' },
    { ring: 'region', label: 'europe' },
    { ring: 'city', label: 'istanbul-week' },
    { ring: 'country', label: 'turkiye-week' },
  ]
  expect(nearRingsFirst(planned).map((entry) => entry.label)).toEqual([
    'istanbul-week',
    'turkiye-week',
    'europe',
    'worldwide-remote-day',
  ])
  expect(ringDistance('city')).toBeLessThan(ringDistance('country'))
  expect(ringDistance('country')).toBeLessThan(ringDistance('region'))
  expect(ringDistance('region')).toBeLessThan(ringDistance('worldwide'))
})

test('defect 1: order inside one ring is the order the planner asked for', () => {
  const planned = [
    { ring: 'city', label: 'istanbul-java' },
    { ring: 'worldwide', label: 'worldwide' },
    { ring: 'city', label: 'istanbul-frontend' },
  ]
  expect(nearRingsFirst(planned).map((entry) => entry.label)).toEqual([
    'istanbul-java',
    'istanbul-frontend',
    'worldwide',
  ])
})

test('defect 1: the worldwide query cannot eat the round before the home rings run', async () => {
  const test_ = harness({
    plan: {
      pacing: { ...plannedRound().pacing, newJobTarget: 3, maxPagesPerQuery: 1 },
      queries: [
        query({ label: 'worldwide-remote-day', ring: 'worldwide', place: '', remoteOnly: true }),
        query({ label: 'istanbul-week', ring: 'city', place: 'Istanbul' }),
        query({ label: 'turkiye-week', ring: 'country', place: 'Türkiye' }),
      ],
    },
    search: async () => page(cards(['1', '2', '3'])),
  })

  const report = await executeRound(test_.ports)

  expect(report.reason).toBe(Reason.targetReached)
  expect(report.queries[0]?.label).toBe('istanbul-week')
  expect(report.queries[0]?.collected).toBe(3)
  expect(report.queries.some((entry) => entry.ring === 'worldwide' && entry.pages > 0)).toBe(false)
  expect(test_.searches.length).toBe(1)
  expect(test_.places[0]).toBe('Istanbul')
})

test('defect 2: the keyword reaches linkedin as linkedin own matching', () => {
  const built = buildQuery(
    searchFor({ ...NO_KNOBS, locationId: '102135806', keyword: 'Java Spring Boot' }),
  )
  expect(built).toContain('keywords:Java%20Spring%20Boot')
  expect(buildQuery(searchFor({ ...NO_KNOBS, locationId: '102135806' }))).not.toContain('keywords')
})

test('defect 2: a term is sanitised without breaking the restli tuple', () => {
  const built = buildQuery({ locationId: '102135806', keyword: 'C# (.NET), Go', range: 'r86400' })
  expect(built).toContain('keywords:')
  expect(built.split('keywords:')[1]?.split(',')[0]).not.toContain('(')
  expect(built.startsWith('(')).toBe(true)
  expect(built.endsWith(')')).toBe(true)
})

test('defect 2: the chips are derived from the indexed cvs, never invented', () => {
  const profile = {
    resumeDir: '/cv',
    candidate: {
      yearsExperience: 5,
      seniorityBand: 'senior',
      coreStack: ['Java', 'Go'],
      secondaryStack: [],
      domains: [],
      workLanguages: [],
      places: [{ name: 'Istanbul', ring: 'city' }],
      headline: '',
    },
    indexedAt: '',
    resumes: [
      {
        code: 'BA',
        label: 'Backend',
        fileLanguages: ['en'],
        indexed: true,
        profile: {
          code: 'BA',
          targetRole: 'Senior Backend Developer',
          seniorityClaimed: 'senior',
          coreStack: ['Java', 'Kotlin'],
          secondaryStack: [],
          domains: [],
          languages: [],
          yearsClaimed: 5,
          earliestStart: '',
          places: [{ name: 'Türkiye', ring: 'country' }],
          summary: '',
        },
      },
    ],
    indexState: 'current' as const,
    indexing: false,
    progress: { phase: '', done: 0, total: 0 },
  }

  const terms = deriveTerms(profile)
  expect(terms).toContain('Senior Backend Developer')
  expect(terms).toContain('Java')
  expect(terms.filter((term) => term === 'Java').length).toBe(1)
  expect(derivePlaceNames(profile)).toEqual(['Istanbul', 'Türkiye'])
  expect(deriveTerms(null)).toEqual([])
})

test('defect 3: a finished round runs screening itself, without anyone pressing anything', async () => {
  const test_ = harness({
    plan: { pacing: { ...plannedRound().pacing, newJobTarget: 3 } },
    routes: {
      'GET /api/jobs/pending-descriptions': () => ({
        status: 200,
        body: { jobs: [{ id: '1', url: 'https://www.linkedin.com/jobs/view/1/' }] },
      }),
      'POST /api/jobs/screen': (_body, call) => ({
        status: 200,
        body: {
          paused: false,
          triaged: call === 1 ? 3 : 0,
          kept: call === 1 ? 1 : 0,
          droppedAtTriage: call === 1 ? 2 : 0,
          gated: 0,
          screened: call === 1 ? 0 : 1,
          picked: call === 1 ? 0 : 1,
          manual: 0,
          skipped: 0,
          flagged: 0,
          corrected: 0,
          nudges: 0,
          awaitingDescription: call === 1 ? 1 : 0,
          screenKey: 'k',
          verdicts: [],
        },
      }),
    },
  })

  const report = await executeRound(test_.ports)
  const screenCalls = test_.calls.filter((call) => call.path === '/api/jobs/screen')

  expect(screenCalls.length).toBe(2)
  expect(report.screening.passes).toBe(2)
  expect(report.screening.triaged).toBe(3)
  expect(report.screening.picked).toBe(1)
  expect(screenCalls[0]?.body.limit).toBe(40)

  const order = test_.calls.map((call) => call.path)
  expect(order.indexOf('/api/jobs/screen')).toBeLessThan(
    order.indexOf('/api/jobs/pending-descriptions'),
  )
  expect(order.lastIndexOf('/api/jobs/screen')).toBeLessThan(order.indexOf('/api/cycle/finish'))
})

test('defect 3: screening is charged to the round, so it runs before the round closes', async () => {
  const test_ = harness({ plan: { pacing: { ...plannedRound().pacing, newJobTarget: 3 } } })
  await executeRound(test_.ports)

  const paths = test_.calls.map((call) => call.path)
  expect(paths).toContain('/api/jobs/screen')
  expect(paths.indexOf('/api/jobs/screen')).toBeGreaterThan(paths.indexOf('/api/cycle/start'))
  expect(paths.indexOf('/api/jobs/screen')).toBeLessThan(paths.indexOf('/api/cycle/finish'))
})

function live(blocked: PlanState['blocked']): Resource<PlanState> {
  return {
    value: { blocked } as PlanState,
    live: true,
    at: Date.now(),
    failure: null,
  }
}

test('defect 4: a refusal that the gate has cleared clears on screen', () => {
  const refusal: Notice = {
    text: 'the cv folder is not indexed yet, so there is no candidate to plan a round for.',
    reason: 'no-profile',
  }
  const stillBlocked = live({
    ok: false,
    reason: 'no-profile',
    message: refusal.text,
  })

  expect(noticeStands(refusal, stillBlocked)).toEqual(refusal)
  expect(noticeStands(refusal, live(null))).toBeNull()
})

test('defect 4: the last round message clears with the refusal that produced it', () => {
  const refused = {
    refused: true,
    reason: 'no-profile',
    message: 'the cv folder is not indexed yet, so there is no candidate to plan a round for.',
  } as never
  expect(reportStands(refused, live(null))).toBeNull()
  expect(reportStands(refused, live({ ok: false, reason: 'no-profile', message: 'x' }))).toBe(refused)
})

test('defect 4: a refusal the gate does not own is never wiped by a clean plan', () => {
  const failed: Notice = { text: 'the planner returned no usable query', reason: 'planner-failed' }
  expect(noticeStands(failed, live(null))).toEqual(failed)
  expect(clearsWithTheGate('planner-failed')).toBe(false)
  expect(clearsWithTheGate('no-profile')).toBe(true)
  expect(clearsWithTheGate('paused')).toBe(true)
})

test('defect 4: a stale refusal is never cleared by a plan read that is not live', () => {
  const refusal: Notice = { text: 'paused', reason: 'paused' }
  const cached: Resource<PlanState> = {
    value: { blocked: null } as PlanState,
    live: false,
    at: 1,
    failure: null,
  }
  expect(noticeStands(refusal, cached)).toEqual(refusal)
})

test('defect 5: the feed loads when it opens, and only once', () => {
  expect(opensWith(true, true, false)).toBe(true)
  expect(opensWith(false, true, false)).toBe(false)
  expect(opensWith(true, false, false)).toBe(false)
  expect(opensWith(true, true, true)).toBe(false)
})

test('defect 6: a ceiling of zero reads as no cap, never as a cap of zero', () => {
  expect(spend(32, 0, 'today')).toBe('32 model calls today, no cap')
  expect(spend(1, 0, 'this round')).toBe('1 model call this round, no cap')
  expect(spend(32, 60, 'today')).toBe('32 of 60 model calls today')
})

test('defect 7: the work view opens on the inbox and keeps all as the last resort', () => {
  expect(POSTING_FILTERS[0]).toBe('inbox')
  expect(POSTING_FILTERS[POSTING_FILTERS.length - 1]).toBe('all')
  expect(filterPostings(mixed, 'inbox').map((row) => row.id)).toEqual(['b'])
  expect(filterPostings(mixed, 'all').map((row) => row.id)).toEqual(['a', 'b', 'c'])
})
