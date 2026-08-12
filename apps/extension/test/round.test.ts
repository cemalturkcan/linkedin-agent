import { expect, test } from 'bun:test'
import { LinkedInFailure } from '@/lib/linkedin/failure'
import { clockAt, createRunner, executeRound, Reason, resolveOrphan } from '@/lib/linkedin/round'
import {
  cards,
  fullPage,
  harness,
  page,
  plannedRound,
  query,
  START_AT,
} from './support/round-harness'

function finishes(calls: { path: string; body: Record<string, unknown> }[]): Record<string, unknown>[] {
  return calls.filter((call) => call.path === '/api/cycle/finish').map((call) => call.body)
}

test('a plain round harvests, reports its yield and closes itself', async () => {
  const test_ = harness({ plan: { pacing: { ...plannedRound().pacing, newJobTarget: 3 } } })
  const report = await executeRound(test_.ports)

  expect(report.opened).toBe(true)
  expect(report.closed).toBe(true)
  expect(report.failed).toBe(false)
  expect(report.collected).toBe(3)
  expect(report.reason).toBe(Reason.targetReached)
  expect(test_.pathsOf('POST')).toEqual([
    '/api/cycle/start',
    '/api/places',
    '/api/jobs',
    '/api/jobs/screen',
    '/api/cycle/finish',
  ])
  expect(finishes(test_.calls)).toEqual([{ cycleId: 7, reason: 'target-reached', error: '' }])
  expect(test_.stored.open).toBeNull()
  expect(test_.holds).toEqual(['start', 'stop'])
})

test('a planner with nothing left to widen ends the round on the widening', async () => {
  const test_ = harness()
  const report = await executeRound(test_.ports)

  expect(report.collected).toBe(3)
  expect(report.reason).toBe(Reason.wideningSpent)
  expect(report.message).toBe('the widening steps are spent, so the round stopped')
  expect(finishes(test_.calls)).toEqual([{ cycleId: 7, reason: 'widening-spent', error: '' }])
})

test('a throw partway through still reports a finished round, with the failure on it', async () => {
  const test_ = harness({
    search: async () => {
      throw new TypeError('cannot read properties of undefined')
    },
  })
  const report = await executeRound(test_.ports)

  expect(report.failed).toBe(true)
  expect(report.reason).toBe(Reason.failed)
  expect(report.message).toBe('cannot read properties of undefined')
  expect(report.closed).toBe(true)
  expect(finishes(test_.calls)).toEqual([
    { cycleId: 7, reason: 'failed', error: 'cannot read properties of undefined' },
  ])
  expect(test_.stored.open).toBeNull()
  expect(test_.holds).toEqual(['start', 'stop'])
})

test('a rejection from the agent still reports a finished round', async () => {
  const test_ = harness({
    routes: {
      'POST /api/jobs': () => ({ status: 500, body: { error: 'the store is not writable' } }),
    },
  })
  const report = await executeRound(test_.ports)

  expect(report.opened).toBe(true)
  expect(report.closed).toBe(true)
  expect(report.reason).toBe(Reason.rejected)
  expect(report.message).toBe('the store is not writable')
  expect(finishes(test_.calls)).toEqual([{ cycleId: 7, reason: 'agent-rejected', error: '' }])
  expect(test_.stored.open).toBeNull()
})

test('an early return still reports a finished round, with the reason', async () => {
  const test_ = harness({
    plan: { queries: [query({ ring: 'city', place: '' })] },
  })
  const report = await executeRound(test_.ports)

  expect(report.opened).toBe(true)
  expect(report.closed).toBe(true)
  expect(report.reason).toBe(Reason.noUsableQuery)
  expect(test_.searches.length).toBe(0)
  expect(test_.bodyOf('/api/cycle/skip')).toEqual({
    cycleId: 7,
    queryLabel: 'city-engineer',
    place: '',
    ring: 'city',
    reason: 'a query that is not worldwide has to name a place',
  })
  expect(finishes(test_.calls)).toEqual([{ cycleId: 7, reason: 'no-usable-query', error: '' }])
})

test('a round the agent refuses opens nothing and spends no fetch', async () => {
  const test_ = harness({
    routes: {
      'POST /api/cycle/start': () => ({
        status: 409,
        body: {
          ok: false,
          reason: 'daily-cap',
          message: '60 of 60 model calls used today. the round is off until the count resets.',
        },
      }),
    },
  })
  const report = await executeRound(test_.ports)

  expect(report.opened).toBe(false)
  expect(report.cycleId).toBeNull()
  expect(report.reason).toBe('daily-cap')
  expect(report.message).toContain('model calls used today')
  expect(test_.searches.length).toBe(0)
  expect(finishes(test_.calls)).toEqual([])
})

test('a close the agent will not take leaves the round for the next wake', async () => {
  const test_ = harness({
    routes: {
      'POST /api/cycle/finish': () => ({ status: 503, body: { error: 'the store is closed' } }),
    },
  })
  const report = await executeRound(test_.ports)

  expect(report.closed).toBe(false)
  expect(report.message).toContain('the next wake resolves it')
  expect(test_.stored.open).toEqual({
    cycleId: 7,
    startedAt: START_AT,
    deadlineAt: START_AT + 240_000,
  })
})

test('an orphaned round from a previous life is resolved on the next wake', async () => {
  const test_ = harness({
    openRound: { cycleId: 41, startedAt: START_AT - 600_000, deadlineAt: START_AT - 360_000 },
  })
  const outcome = await resolveOrphan(test_.ports)

  expect(outcome.found).toBe(true)
  expect(outcome.cycleId).toBe(41)
  expect(outcome.closed).toBe(true)
  expect(outcome.message).toBe('the browser reclaimed the worker while the round was running')
  expect(finishes(test_.calls)).toEqual([
    {
      cycleId: 41,
      reason: 'worker-reclaimed',
      error: 'the browser reclaimed the worker while the round was running',
    },
  ])
  expect(test_.stored.open).toBeNull()
})

test('a round the agent has already forgotten is not resolved for ever', async () => {
  const test_ = harness({
    openRound: { cycleId: 41, startedAt: START_AT, deadlineAt: START_AT },
    routes: {
      'POST /api/cycle/finish': () => ({ status: 404, body: { error: 'no round with id 41' } }),
    },
  })
  const outcome = await resolveOrphan(test_.ports)

  expect(outcome.closed).toBe(false)
  expect(test_.stored.open).toBeNull()
})

test('two concurrent kicks produce one round', async () => {
  const test_ = harness()
  const runner = createRunner(test_.ports)

  const [first, second] = await Promise.all([runner.run(), runner.run()])

  expect(first).toBe(second)
  expect(test_.calls.filter((call) => call.path === '/api/cycle/start').length).toBe(1)
  expect(finishes(test_.calls).length).toBe(1)
  expect(runner.running()).toBe(false)
})

test('a kick resolves an orphan before it opens a new round', async () => {
  const test_ = harness({
    openRound: { cycleId: 41, startedAt: START_AT - 600_000, deadlineAt: START_AT - 360_000 },
  })
  await createRunner(test_.ports).run()

  expect(finishes(test_.calls)).toEqual([
    {
      cycleId: 41,
      reason: 'worker-reclaimed',
      error: 'the browser reclaimed the worker while the round was running',
    },
    { cycleId: 7, reason: 'widening-spent', error: '' },
  ])
})

test('a rate limit stops the round, backs off and says until when', async () => {
  const test_ = harness({
    search: async () => {
      throw new LinkedInFailure('rate-limited', 'linkedin is rate limiting this session')
    },
  })
  const report = await executeRound(test_.ports)

  expect(report.reason).toBe(Reason.rateLimited)
  expect(report.blocked).toBe(
    `linkedin is rate limiting this session, holding off until ${clockAt(START_AT + 300_000)}`,
  )
  expect(report.message).toBe(report.blocked)
  expect(test_.stored.backoffUntil).toBeGreaterThan(START_AT)
  expect(finishes(test_.calls)).toEqual([{ cycleId: 7, reason: 'rate-limited', error: '' }])
})

test('a session still backing off runs nothing and opens no round', async () => {
  const test_ = harness({ backoffUntil: START_AT + 300_000 })
  const report = await executeRound(test_.ports)

  expect(report.opened).toBe(false)
  expect(report.reason).toBe(Reason.backingOff)
  expect(report.message).toBe(
    `linkedin asked this session to slow down, so nothing runs until ${clockAt(START_AT + 300_000)}`,
  )
  expect(test_.calls.length).toBe(0)
})

test('a lost session stops the round and says to sign in again', async () => {
  const test_ = harness({
    search: async () => {
      throw new LinkedInFailure(
        'signed-out',
        'linkedin signed this session out, open linkedin.com and sign in again',
      )
    },
  })
  const report = await executeRound(test_.ports)

  expect(report.reason).toBe(Reason.signedOut)
  expect(report.message).toBe(
    'linkedin signed this session out, open linkedin.com and sign in again',
  )
  expect(test_.holds).toContain('forget-session')
  expect(test_.postings.length).toBe(0)
  expect(finishes(test_.calls)).toEqual([{ cycleId: 7, reason: 'signed-out', error: '' }])
})

test('the deadline stops a round that is still producing', async () => {
  let served = 0
  const test_ = harness({
    plan: {
      queries: [query({ want: 500 })],
      pacing: {
        newJobTarget: 500,
        maxWideningSteps: 1,
        roundDeadlineMs: 4_000,
        requestDelayMs: 900,
        requestTimeoutMs: 15_000,
        cycleMinutes: 10,
        maxPagesPerQuery: 8,
      },
    },
    search: async () => {
      served += 1
      return fullPage('p', served * 100)
    },
  })
  const report = await executeRound(test_.ports)

  expect(report.reason).toBe(Reason.deadline)
  expect(report.message).toBe('the round hit its deadline and stopped where it was')
  expect(test_.searches.length).toBeGreaterThan(0)
  expect(test_.searches.length).toBeLessThan(8)
  expect(finishes(test_.calls)).toEqual([{ cycleId: 7, reason: 'deadline', error: '' }])
})

test('a full page with nothing new stops that query rather than paging on', async () => {
  const known = fullPage('k', 0).jobs.map((job) => job.id)
  const test_ = harness({
    seen: known,
    search: async () => fullPage('k', 0),
  })
  const report = await executeRound(test_.ports)

  expect(test_.searches.length).toBe(1)
  expect(report.collected).toBe(0)
  expect(report.reason).toBe(Reason.minedOut)
  expect(test_.calls.some((call) => call.path === '/api/jobs')).toBe(false)
})

test('the handled set and the plan digest are the only things filtered before posting', async () => {
  const test_ = harness({
    plan: { knownIds: ['2'] },
    seen: ['1'],
    search: async () => page(cards(['1', '2', '3', '4'])),
  })
  const report = await executeRound(test_.ports)

  expect(report.collected).toBe(2)
  const posted = test_.bodyOf('/api/jobs') as { jobs: { id: string }[] }
  expect(posted.jobs.map((job) => job.id)).toEqual(['3', '4'])
})

test('a title nobody would have guessed is posted like every other one', async () => {
  const test_ = harness({
    search: async () =>
      page([
        ...cards(['1'], 'Senior Software Engineer'),
        ...cards(['2'], 'Marketing Lead'),
        ...cards(['3'], 'Yazılım Uzmanı'),
      ]),
  })
  const report = await executeRound(test_.ports)

  const posted = test_.bodyOf('/api/jobs') as { jobs: { id: string; title: string }[] }
  expect(posted.jobs.map((job) => job.title)).toEqual([
    'Senior Software Engineer',
    'Marketing Lead',
    'Yazılım Uzmanı',
  ])
  expect(report.collected).toBe(3)
})

test('the fetch carries the place, the ring, the term, the window and the knobs', async () => {
  const test_ = harness({
    plan: {
      queries: [
        query({
          ring: 'city',
          place: 'Rotterdam',
          keyword: 'Spring Boot',
          range: 'r21600',
          easyApply: true,
          remoteOnly: true,
        }),
      ],
    },
  })
  await executeRound(test_.ports)

  expect(test_.places).toEqual(['Rotterdam'])
  expect(test_.searches[0]?.query).toEqual({
    locationId: '102135806',
    keyword: 'Spring Boot',
    range: 'r21600',
    easyApply: true,
    remoteOnly: true,
  })
  expect(test_.searches[0]?.start).toBe(0)
})

test('the round reports every query by place and ring, and posts what came back', async () => {
  const test_ = harness({ search: async () => page(cards(['1', '2', '3'])) })
  const report = await executeRound(test_.ports)

  expect(report.queries.map((entry) => [entry.label, entry.ring, entry.place])).toEqual([
    ['city-engineer', 'city', 'Rotterdam'],
  ])
  expect(report.queries[0]?.fetched).toBe(3)
  expect(report.queries[0]?.collected).toBe(3)
  expect(report.queries[0]?.inserted).toBe(3)
  const posted = test_.bodyOf('/api/jobs') as { queryLabel: string; pages: number; cycleId: number }
  expect(posted.queryLabel).toBe('city-engineer')
  expect(posted.cycleId).toBe(7)
  expect(posted.pages).toBe(1)
})

test('worldwide resolves no place and city resolves one', async () => {
  const worldwide = harness({
    plan: { queries: [query({ label: 'worldwide-remote', ring: 'worldwide', place: '' })] },
  })
  await executeRound(worldwide.ports)
  expect(worldwide.places).toEqual([])
  expect(worldwide.searches[0]?.query.locationId).toBe('')

  const city = harness()
  await executeRound(city.ports)
  expect(city.places).toEqual(['Rotterdam'])
  expect(city.searches[0]?.query.locationId).toBe('102135806')
})

test('a place linkedin does not know is skipped, reported and costs no fetch', async () => {
  const test_ = harness({
    place: async (name) => {
      throw new LinkedInFailure('unknown-place', `linkedin's place lookup knows nowhere called ${name}`)
    },
  })
  const report = await executeRound(test_.ports)

  expect(test_.searches.length).toBe(0)
  expect(test_.bodyOf('/api/cycle/skip')).toEqual({
    cycleId: 7,
    queryLabel: 'city-engineer',
    place: 'Rotterdam',
    ring: 'city',
    reason: "linkedin's place lookup knows nowhere called Rotterdam",
  })
  expect(report.reason).toBe(Reason.everyPlaceUnknown)
  expect(report.message).toBe('every planned query named a place linkedin does not know')
})

test('a resolution the extension obtained is reported so the agent can offer it', async () => {
  const test_ = harness()
  await executeRound(test_.ports)

  expect(test_.bodyOf('/api/places')).toEqual({
    places: [
      {
        name: 'Rotterdam',
        asked: 'Rotterdam',
        ring: 'city',
        locationId: '102135806',
        source: 'round',
      },
    ],
  })
})

test('a cached resolution is not reported again', async () => {
  const test_ = harness({
    place: async (name) => ({ locationId: '102135806', label: name, cached: true }),
  })
  await executeRound(test_.ports)
  expect(test_.calls.some((call) => call.path === '/api/places')).toBe(false)
})

test('a round short of its target asks for one widening step and runs it', async () => {
  const widened = query({ label: 'country-engineer', ring: 'country', place: 'Netherlands' })
  const test_ = harness({
    plan: { pacing: { ...plannedRound().pacing, newJobTarget: 6, maxWideningSteps: 1 } },
    routes: {
      'POST /api/cycle/widen': () => ({
        status: 200,
        body: { ok: true, cycleId: 7, query: widened, relaxed: 'ring', reason: 'the city ran dry', stepsLeft: 0 },
      }),
    },
    search: async (query) => page(cards(query.locationId === '102135806' ? ['1', '2'] : ['5', '6'])),
  })
  const report = await executeRound(test_.ports)

  expect(report.widenings).toBe(1)
  expect(report.queries.map((entry) => entry.label)).toEqual(['city-engineer', 'country-engineer'])
  expect(report.reason).toBe(Reason.wideningSpent)
  expect(test_.places).toEqual(['Rotterdam', 'Netherlands'])
})

test('the agent stopping a harvest stops the round with the agent reason', async () => {
  const test_ = harness({
    routes: {
      'POST /api/jobs': (body) => ({
        status: 200,
        body: {
          received: (body.jobs as unknown[]).length,
          inserted: 3,
          duplicates: 0,
          collapsed: 0,
          judged: 0,
          newIds: ['1', '2', '3'],
          roundNew: 26,
          newJobTarget: 25,
          stop: true,
          reason: 'target-reached',
        },
      }),
    },
  })
  const report = await executeRound(test_.ports)

  expect(report.reason).toBe('target-reached')
  expect(finishes(test_.calls)).toEqual([{ cycleId: 7, reason: 'target-reached', error: '' }])
})

test('the bodies the agent asks for are fetched and posted back', async () => {
  const test_ = harness({
    routes: {
      'GET /api/jobs/pending-descriptions': () => ({
        status: 200,
        body: {
          jobs: [
            { id: '1', url: 'https://www.linkedin.com/jobs/view/1/' },
            { id: '2', url: 'https://www.linkedin.com/jobs/view/2/' },
          ],
        },
      }),
    },
    posting: async (id) => ({
      id,
      description: `body of ${id}`,
      applied: id === '2',
      closed: false,
    }),
  })
  const report = await executeRound(test_.ports)

  expect(test_.postings).toEqual(['1', '2'])
  expect(report.read).toBe(2)
  expect(test_.bodyOf('/api/jobs/descriptions')).toEqual({
    descriptions: [
      { id: '1', description: 'body of 1', applied: false, closed: false },
      { id: '2', description: 'body of 2', applied: true, closed: false },
    ],
  })
  expect([...test_.stored.described]).toEqual(['1', '2'])
})

test('a body already read is not fetched again', async () => {
  const test_ = harness({
    described: ['1'],
    routes: {
      'GET /api/jobs/pending-descriptions': () => ({
        status: 200,
        body: { jobs: [{ id: '1', url: 'u' }, { id: '2', url: 'u' }] },
      }),
    },
  })
  await executeRound(test_.ports)
  expect(test_.postings).toEqual(['2'])
})

test('the panel is told what the round is doing while it runs', async () => {
  const test_ = harness()
  await executeRound(test_.ports)
  expect(test_.updates.map((update) => update.phase)).toEqual([
    'planning',
    'harvesting',
    'screening',
    'finished',
  ])
})

test('the pace comes from the agent, not from a constant here', async () => {
  const test_ = harness({
    plan: {
      nextRunAt: new Date(START_AT + 25 * 60_000).toISOString(),
      pacing: { ...plannedRound().pacing, cycleMinutes: 25 },
    },
  })
  const report = await executeRound(test_.ports)

  expect(report.cycleMinutes).toBe(25)
  expect(report.nextRunAt).toBe(new Date(START_AT + 25 * 60_000).toISOString())
})

test('requests are spaced out with a jitter rather than fired back to back', async () => {
  const gaps: number[] = []
  let last = 0
  const test_ = harness({
    plan: {
      queries: [query({ want: 500 })],
      pacing: {
        ...plannedRound().pacing,
        newJobTarget: 500,
        requestDelayMs: 1_000,
        maxPagesPerQuery: 3,
      },
    },
    search: async () => fullPage('n', gaps.length * 100),
  })
  const original = test_.ports.linkedin.search.bind(test_.ports.linkedin)
  test_.ports.linkedin.search = async (query, start, timeoutMs) => {
    const at = test_.at()
    if (last > 0) gaps.push(at - last)
    last = at
    return original(query, start, timeoutMs)
  }
  test_.ports.random = () => 0.5

  await executeRound(test_.ports)
  expect(gaps.length).toBeGreaterThan(0)
  for (const gap of gaps) expect(gap).toBe(1_500)
})
