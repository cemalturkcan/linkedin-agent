import { beforeEach, expect, test } from 'bun:test'
import { AgentClient } from '@/lib/agent/client'

const BASE = 'http://127.0.0.1:9999'

function clientWith(handler: (url: string, init?: RequestInit) => Response | Promise<Response>) {
  const seen: { url: string; init?: RequestInit }[] = []
  const client = new AgentClient({
    base: async () => BASE,
    fetcher: (async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      seen.push({ url, init })
      return handler(url, init)
    }) as typeof fetch,
  })
  return { client, seen }
}

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json' },
  })
}

let originalFetch: typeof fetch

beforeEach(() => {
  originalFetch = globalThis.fetch
  globalThis.fetch = originalFetch
})

test('a list comes back as the resource itself', async () => {
  const { client, seen } = clientWith(() => json({ jobs: [{ id: '1' }] }))
  const result = await client.jobs('inbox')

  expect(result.ok).toBe(true)
  if (result.ok) expect(result.value.jobs.length).toBe(1)
  expect(seen[0]?.url).toBe(`${BASE}/api/jobs?status=inbox`)
})

test('an unreachable agent names the base and what it costs', async () => {
  const { client } = clientWith(() => {
    throw new TypeError('fetch failed')
  })
  const result = await client.planState()

  expect(result.ok).toBe(false)
  if (result.ok) return
  expect(result.failure.kind).toBe('unreachable')
  expect(result.failure.message).toContain(BASE)
})

test('a refusal carries the reason the server named', async () => {
  const { client } = clientWith(() =>
    json(
      {
        ok: false,
        reason: 'executor-offline',
        message: 'no browser extension has ever checked in, so there is nothing to run a round.',
      },
      409,
    ),
  )
  const result = await client.startRound()

  expect(result.ok).toBe(false)
  if (result.ok) return
  expect(result.failure.kind).toBe('refused')
  expect(result.failure.reason).toBe('executor-offline')
  expect(result.failure.message).toContain('checked in')
})

test('an error status carries the sentence the server wrote', async () => {
  const { client } = clientWith(() => json({ error: 'no cv folder chosen yet, pick one in setup' }, 503))
  const result = await client.resumes()

  expect(result.ok).toBe(false)
  if (result.ok) return
  expect(result.failure.kind).toBe('rejected')
  expect(result.failure.status).toBe(503)
  expect(result.failure.message).toBe('no cv folder chosen yet, pick one in setup')
})

test('a write sends json and names its transition in the path', async () => {
  const { client, seen } = clientWith(() => json({ job: { id: '7' } }))
  await client.move('7', 'queue')

  expect(seen[0]?.url).toBe(`${BASE}/api/jobs/7/queue`)
  expect(seen[0]?.init?.method).toBe('POST')
})

test('the event url carries the last id seen and drops it when empty', async () => {
  const { client } = clientWith(() => json({}))
  expect(await client.eventsUrl('41')).toBe(`${BASE}/api/events?lastEventId=41`)
  expect(await client.eventsUrl('')).toBe(`${BASE}/api/events`)
})

test('the default fetcher reaches the network instead of throwing on its receiver', async () => {
  const real = globalThis.fetch
  let calls = 0
  globalThis.fetch = function (this: unknown, _input: RequestInfo | URL, _init?: RequestInit) {
    if (this !== undefined && this !== globalThis) throw new TypeError('Illegal invocation')
    calls += 1
    return Promise.resolve(new Response('{"ok":true}', { status: 200 }))
  } as unknown as typeof fetch

  try {
    const client = new AgentClient()
    const result = await client.health()
    expect(calls).toBe(1)
    expect(result.ok).toBe(true)
  } finally {
    globalThis.fetch = real
  }
})

test('a posting opened by hand is posted to the agent with the cv the person armed', async () => {
  const { client, seen } = clientWith(() =>
    json({ received: 1, recorded: 1, collapsed: 0, held: 0, ids: ['1'] }),
  )
  const result = await client.recordOpened(
    [
      {
        id: '1',
        title: 'Senior Software Engineer',
        company: 'northwind',
        location: 'remote',
        url: 'https://www.linkedin.com/jobs/view/1/',
        description: '',
        listedAt: null,
        easyApply: true,
      },
    ],
    'GO',
    'en',
  )

  expect(result.ok).toBe(true)
  if (result.ok) expect(result.value.recorded).toBe(1)
  expect(seen[0]?.url).toBe(`${BASE}/api/jobs/opened`)
  expect(seen[0]?.init?.method).toBe('POST')
  const body = JSON.parse(String(seen[0]?.init?.body)) as {
    jobs: { id: string }[]
    resumeCode: string
    resumeLang: string
  }
  expect(body.jobs.map((job) => job.id)).toEqual(['1'])
  expect(body.resumeCode).toBe('GO')
  expect(body.resumeLang).toBe('en')
})
