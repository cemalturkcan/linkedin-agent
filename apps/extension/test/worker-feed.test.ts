import { expect, test } from 'bun:test'
import { AgentClient } from '@/lib/agent/client'
import { LinkedInFailure } from '@/lib/linkedin/failure'
import { NO_KNOBS } from '@/lib/linkedin/feed'
import type { PlaceHit } from '@/lib/linkedin/geo'
import { clockAt } from '@/lib/linkedin/round'
import type { SearchPage, SearchQuery } from '@/lib/linkedin/voyager'
import { feedPage, feedPlaces, rememberChosenPlace, type FeedPorts } from '@/worker/feed'
import typeahead from './fixtures/typeahead.json'

const BASE = 'http://127.0.0.1:9787'
const NOW = 1_770_000_000_000

interface Recorded {
  ports: FeedPorts
  searches: { query: SearchQuery; start: number }[]
  places: string[]
  posted: { path: string; body: Record<string, unknown> }[]
  backoffUntil: number
  forgot: boolean
}

function harness(
  options: {
    search?: (query: SearchQuery, start: number) => Promise<SearchPage>
    place?: (name: string) => Promise<{ locationId: string; label: string; cached: boolean }>
    lookup?: () => Promise<unknown>
    backoffUntil?: number
  } = {},
): Recorded {
  const searches: { query: SearchQuery; start: number }[] = []
  const places: string[] = []
  const posted: { path: string; body: Record<string, unknown> }[] = []
  const state = { backoffUntil: options.backoffUntil ?? 0, forgot: false }

  const fetcher = (async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = new URL(String(input))
    posted.push({
      path: url.pathname,
      body: init?.body ? (JSON.parse(String(init.body)) as Record<string, unknown>) : {},
    })
    return new Response(JSON.stringify({ stored: 1, places: [], known: 1 }), {
      status: 200,
      headers: { 'content-type': 'application/json' },
    })
  }) as typeof fetch

  const recorded: Recorded = {
    searches,
    places,
    posted,
    get backoffUntil() {
      return state.backoffUntil
    },
    get forgot() {
      return state.forgot
    },
    ports: {
      agent: new AgentClient({ base: async () => BASE, fetcher }),
      linkedin: {
        async search(query, start) {
          searches.push({ query, start })
          if (options.search) return options.search(query, start)
          return { jobs: [], total: 0 }
        },
        async posting(id) {
          return { id, description: '', applied: false, closed: false }
        },
        async place(name) {
          places.push(name)
          if (options.place) return options.place(name)
          return { locationId: '102135806', label: name, cached: false }
        },
        forgetSession() {
          state.forgot = true
        },
      },
      lookup: {
        async json() {
          if (options.lookup) return options.lookup()
          return typeahead
        },
      },
      backoff: {
        async held() {
          return state.backoffUntil > NOW ? state.backoffUntil : 0
        },
        async escalate() {
          state.backoffUntil = NOW + 300_000
          return state.backoffUntil
        },
        async clear() {
          state.backoffUntil = 0
        },
      },
      timeoutMs: 15_000,
    },
  }
  return recorded
}

test('a page carries the knobs and the offset, and nothing else', async () => {
  const test_ = harness({
    search: async () => ({
      jobs: [
        {
          id: '1',
          title: 'Senior Software Engineer',
          company: 'northwind',
          location: 'remote',
          url: 'https://www.linkedin.com/jobs/view/1/',
          listedAt: NOW,
          reposted: false,
          easyApply: true,
        },
      ],
      total: 120,
    }),
  })

  const answer = await feedPage(
    test_.ports,
    {
      place: 'somewhere',
      locationId: '102135806',
      keyword: '',
      range: 'r3600',
      easyApply: true,
      remoteOnly: false,
    },
    25,
  )

  expect(answer.ok).toBe(true)
  expect(test_.searches[0]).toEqual({
    query: {
      locationId: '102135806',
      keyword: '',
      range: 'r3600',
      easyApply: true,
      remoteOnly: false,
    },
    start: 25,
  })
  expect(test_.places).toEqual([])
  if (answer.ok) {
    expect(answer.total).toBe(120)
    expect(answer.cards.map((card) => card.title)).toEqual(['Senior Software Engineer'])
  }
})

test('a place typed but never picked is resolved once, by name', async () => {
  const test_ = harness()
  await feedPage(test_.ports, { ...NO_KNOBS, place: 'somewhere' }, 0)

  expect(test_.places).toEqual(['somewhere'])
  expect(test_.searches[0]?.query.locationId).toBe('102135806')
})

test('a place linkedin does not know says so and spends no fetch', async () => {
  const test_ = harness({
    place: async (name) => {
      throw new LinkedInFailure('unknown-place', `linkedin's place lookup knows nowhere called ${name}`)
    },
  })
  const answer = await feedPage(test_.ports, { ...NO_KNOBS, place: 'nowhere' }, 0)

  expect(answer.ok).toBe(false)
  expect(test_.searches.length).toBe(0)
  if (!answer.ok) {
    expect(answer.message).toBe("linkedin's place lookup knows nowhere called nowhere")
  }
})

test('a rate limit backs the feed off and says until when', async () => {
  const test_ = harness({
    search: async () => {
      throw new LinkedInFailure('rate-limited', 'linkedin is rate limiting this session')
    },
  })
  const answer = await feedPage(test_.ports, { ...NO_KNOBS, locationId: '102135806' }, 0)

  expect(answer.ok).toBe(false)
  if (!answer.ok) {
    expect(answer.message).toBe(
      `linkedin is rate limiting this session, holding off until ${clockAt(NOW + 300_000)}`,
    )
  }
  expect(test_.backoffUntil).toBeGreaterThan(NOW)
})

test('a session still backing off fetches nothing at all', async () => {
  const test_ = harness({ backoffUntil: NOW + 300_000 })
  const answer = await feedPage(test_.ports, { ...NO_KNOBS, locationId: '102135806' }, 0)

  expect(answer.ok).toBe(false)
  expect(test_.searches.length).toBe(0)
  if (!answer.ok) expect(answer.message).toContain('the feed is off until')
})

test('a lost session drops the token and says to sign in again', async () => {
  const test_ = harness({
    search: async () => {
      throw new LinkedInFailure('signed-out', 'the linkedin session is gone, sign in again')
    },
  })
  const answer = await feedPage(test_.ports, { ...NO_KNOBS, locationId: '102135806' }, 0)

  expect(answer.ok).toBe(false)
  expect(test_.forgot).toBe(true)
})

test('the typeahead answers names, and picking one tells the agent about it', async () => {
  const test_ = harness()
  const found = await feedPlaces(test_.ports, 'rotter')

  expect(found.ok).toBe(true)
  expect((found.hits ?? []).length).toBeGreaterThan(0)

  const hit = (found.hits ?? [])[0] as PlaceHit
  await rememberChosenPlace(test_.ports, hit, 'rotter')

  const saved = test_.posted.find((call) => call.path === '/api/places')
  expect(saved?.body).toEqual({
    places: [
      { name: hit.label, asked: 'rotter', ring: '', locationId: hit.locationId, source: 'hand' },
    ],
  })
})
