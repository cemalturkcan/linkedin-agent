import { AgentClient } from '@/lib/agent/client'
import type { Plan, Query } from '@/lib/agent/types'
import type { ResolvedPlace } from '@/lib/linkedin/geo'
import type { OpenRound, RoundPorts, RoundUpdate } from '@/lib/linkedin/round'
import { PAGE_SIZE, type Card, type PostingBody, type SearchPage, type SearchQuery } from '@/lib/linkedin/voyager'

export const BASE = 'http://127.0.0.1:9787'
export const START_AT = 1_770_000_000_000

export interface Call {
  method: string
  path: string
  body: Record<string, unknown>
}

export interface Answer {
  status: number
  body: unknown
}

export type Route = (body: Record<string, unknown>, call: number) => Answer

export interface HarnessOptions {
  plan?: Partial<Plan>
  routes?: Record<string, Route>
  search?: (query: SearchQuery, start: number) => Promise<SearchPage>
  posting?: (id: string) => Promise<PostingBody>
  place?: (name: string) => Promise<ResolvedPlace>
  seen?: string[]
  described?: string[]
  backoffUntil?: number
  openRound?: OpenRound | null
  startAt?: number
}

export function query(overrides: Partial<Query> = {}): Query {
  return {
    label: 'city-engineer',
    ring: 'city',
    place: 'Rotterdam',
    range: 'r86400',
    want: 25,
    reason: 'the near ring first',
    ...overrides,
  }
}

export function plannedRound(overrides: Partial<Plan> = {}, startAt = START_AT): Plan {
  const deadlineMs = overrides.pacing?.roundDeadlineMs ?? 240_000
  return {
    ok: true,
    cycleId: 7,
    startedAt: new Date(startAt).toISOString(),
    deadlineAt: new Date(startAt + deadlineMs).toISOString(),
    nextRunAt: null,
    rationale: 'start close and widen if it runs dry',
    screenBudget: 40,
    queries: [query()],
    pacing: {
      newJobTarget: 25,
      maxWideningSteps: 2,
      roundDeadlineMs: deadlineMs,
      requestDelayMs: 900,
      requestTimeoutMs: 15_000,
      cycleMinutes: 10,
      maxPagesPerQuery: 2,
    },
    apply: { autoAttach: true, uploadFileName: 'resume.pdf', uploadFileNameMode: 'one-name' },
    knownIds: [],
    ...overrides,
  }
}

export function cards(ids: string[], title = 'Backend Engineer'): Card[] {
  return ids.map((id) => ({
    id,
    title,
    company: 'northwind',
    location: 'remote',
    url: `https://www.linkedin.com/jobs/view/${id}/`,
    listedAt: START_AT,
    reposted: false,
    easyApply: true,
  }))
}

export function page(jobs: Card[], total = jobs.length): SearchPage {
  return { jobs, total }
}

export function fullPage(prefix: string, from: number): SearchPage {
  const ids: string[] = []
  for (let index = 0; index < PAGE_SIZE; index += 1) ids.push(`${prefix}${from + index}`)
  return { jobs: cards(ids), total: 500 }
}

export interface Harness {
  ports: RoundPorts
  calls: Call[]
  updates: RoundUpdate[]
  searches: { query: SearchQuery; start: number }[]
  postings: string[]
  places: string[]
  holds: string[]
  stored: { open: OpenRound | null; seen: Set<string>; described: Set<string>; backoffUntil: number }
  pathsOf(method: string): string[]
  bodyOf(path: string): Record<string, unknown> | null
  advance(ms: number): void
  at(): number
}

export function harness(options: HarnessOptions = {}): Harness {
  const calls: Call[] = []
  const updates: RoundUpdate[] = []
  const searches: { query: SearchQuery; start: number }[] = []
  const postings: string[] = []
  const places: string[] = []
  const holds: string[] = []
  const counts = new Map<string, number>()

  let clock = options.startAt ?? START_AT
  const stored = {
    open: options.openRound ?? null,
    seen: new Set(options.seen ?? []),
    described: new Set(options.described ?? []),
    backoffUntil: options.backoffUntil ?? 0,
  }

  const defaults: Record<string, Route> = {
    'POST /api/cycle/start': () => ({ status: 200, body: plannedRound(options.plan ?? {}, clock) }),
    'POST /api/cycle/widen': () => ({
      status: 409,
      body: { ok: false, reason: 'widening-spent', message: 'no widening left' },
    }),
    'POST /api/cycle/skip': () => ({ status: 200, body: { skipped: 1 } }),
    'POST /api/cycle/finish': (body) => ({
      status: 200,
      body: { cycle: { id: body.cycleId, status: 'done', terminationReason: body.reason } },
    }),
    'POST /api/jobs': (body) => {
      const jobs = (body.jobs as unknown[]) ?? []
      return {
        status: 200,
        body: {
          received: jobs.length,
          inserted: jobs.length,
          duplicates: 0,
          collapsed: 0,
          judged: 0,
          newIds: jobs.map((job) => String((job as { id: string }).id)),
          roundNew: jobs.length,
          newJobTarget: 25,
          stop: false,
        },
      }
    },
    'GET /api/jobs/pending-descriptions': () => ({ status: 200, body: { jobs: [] } }),
    'POST /api/jobs/screen': () => ({
      status: 200,
      body: {
        paused: false,
        triaged: 0,
        kept: 0,
        droppedAtTriage: 0,
        gated: 0,
        screened: 0,
        picked: 0,
        manual: 0,
        skipped: 0,
        flagged: 0,
        corrected: 0,
        nudges: 0,
        awaitingDescription: 0,
        screenKey: 'harness',
        verdicts: [],
      },
    }),
    'POST /api/jobs/descriptions': () => ({ status: 200, body: { stored: 1, missing: 0, unknown: 0 } }),
    'POST /api/places': () => ({ status: 200, body: { stored: 1, places: [], known: 1 } }),
  }

  const routes = { ...defaults, ...(options.routes ?? {}) }

  const fetcher = (async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = new URL(String(input))
    const method = init?.method ?? 'GET'
    const key = `${method} ${url.pathname}`
    const body = init?.body ? (JSON.parse(String(init.body)) as Record<string, unknown>) : {}
    calls.push({ method, path: url.pathname, body })

    const route = routes[key]
    if (!route) return new Response(JSON.stringify({ error: `no route for ${key}` }), { status: 404 })

    const seen = (counts.get(key) ?? 0) + 1
    counts.set(key, seen)
    const answer = route(body, seen)
    return new Response(JSON.stringify(answer.body), {
      status: answer.status,
      headers: { 'content-type': 'application/json' },
    })
  }) as typeof fetch

  const ports: RoundPorts = {
    agent: new AgentClient({ base: async () => BASE, fetcher }),
    linkedin: {
      async search(query, start) {
        searches.push({ query, start })
        if (options.search) return options.search(query, start)
        return page(cards(['1', '2', '3']))
      },
      async posting(id) {
        postings.push(id)
        if (options.posting) return options.posting(id)
        return { id, description: 'a body', applied: false, closed: false }
      },
      async place(name) {
        places.push(name)
        if (options.place) return options.place(name)
        return { locationId: '102135806', label: name, cached: false }
      },
      forgetSession() {
        holds.push('forget-session')
      },
    },
    seen: {
      async load() {
        return new Set(stored.seen)
      },
      async remember(ids) {
        for (const id of ids) stored.seen.add(id)
      },
      async described() {
        return new Set(stored.described)
      },
      async rememberDescribed(ids) {
        for (const id of ids) stored.described.add(id)
      },
    },
    backoff: {
      async held() {
        return stored.backoffUntil > clock ? stored.backoffUntil : 0
      },
      async escalate() {
        stored.backoffUntil = clock + 300_000
        return stored.backoffUntil
      },
      async clear() {
        stored.backoffUntil = 0
      },
    },
    openRound: {
      async read() {
        return stored.open
      },
      async write(round) {
        stored.open = round
      },
      async clear() {
        stored.open = null
      },
    },
    hold: {
      start() {
        holds.push('start')
      },
      stop() {
        holds.push('stop')
      },
    },
    async sleep(ms) {
      clock += ms
      await Promise.resolve()
    },
    now: () => clock,
    random: () => 0,
    notify: (update) => {
      updates.push({ phase: update.phase, report: { ...update.report } })
    },
  }

  return {
    ports,
    calls,
    updates,
    searches,
    postings,
    places,
    holds,
    stored,
    pathsOf: (method) => calls.filter((call) => call.method === method).map((call) => call.path),
    bodyOf: (path) => calls.find((call) => call.path === path)?.body ?? null,
    advance: (ms) => {
      clock += ms
    },
    at: () => clock,
  }
}
