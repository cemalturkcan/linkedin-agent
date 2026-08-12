import { expect, test } from 'bun:test'
import { AgentClient } from '@/lib/agent/client'
import type { Settings } from '@/lib/agent/types'
import { attachFor, base64Of, syncApplied, type ApplyPorts } from '@/worker/apply'
import type { FormReading, InjectionResult } from '@/worker/inject'

const BASE = 'http://127.0.0.1:9787'

interface Stage {
  settings?: Partial<Settings['apply']>
  jobs?: Record<string, unknown>[]
  resumes?: { code: string; label: string; role: string; languages: string[] }[]
  reading?: Partial<FormReading>
  select?: InjectionResult
  attach?: InjectionResult
  attached?: string[]
  file?: { ok: boolean }
  armed?: { code: string; label: string; languages: string[]; at: number } | null
}

function stage(options: Stage = {}) {
  const calls: string[] = []
  const selected: string[] = []
  const uploaded: { name: string; base64: string }[] = []
  const remembered: string[] = [...(options.attached ?? [])]

  const fetcher = (async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = new URL(String(input))
    calls.push(`${init?.method ?? 'GET'} ${url.pathname}`)

    if (url.pathname === '/api/settings') {
      return Response.json({
        settings: {
          apply: {
            autoAttach: true,
            uploadFileName: 'resume.pdf',
            uploadFileNameMode: 'per-variant',
            resumeLanguages: [],
            paused: false,
            ...(options.settings ?? {}),
          },
        },
        enums: {},
      })
    }
    if (url.pathname === '/api/jobs') {
      return Response.json({ jobs: options.jobs ?? [] })
    }
    if (url.pathname === '/api/resumes') {
      return Response.json({
        resumeDir: '/cv',
        resumes: options.resumes ?? [
          { code: 'BE', label: 'backend', role: 'backend', languages: ['en'] },
        ],
      })
    }
    if (url.pathname.endsWith('/file')) {
      if (options.file && !options.file.ok) return new Response('', { status: 404 })
      return new Response(new Uint8Array([37, 80, 68, 70]))
    }
    if (url.pathname.endsWith('/applied')) {
      return Response.json({ job: { id: 'x', status: 'applied' } })
    }
    return new Response(JSON.stringify({ error: 'no route' }), { status: 404 })
  }) as typeof fetch

  const ports: ApplyPorts = {
    agent: new AgentClient({ base: async () => BASE, fetcher }),
    page: {
      async read() {
        return { fileInput: true, options: [], ...(options.reading ?? {}) }
      },
      async select(_tabId, name) {
        selected.push(name)
        return options.select ?? { ok: true, reason: 'selected', matches: 1 }
      },
      async attach(_tabId, base64, name) {
        uploaded.push({ name, base64 })
        return options.attach ?? { ok: true, reason: 'uploaded', matches: 1 }
      },
    },
    attached: {
      async has(id) {
        return remembered.includes(id)
      },
      async remember(id) {
        remembered.push(id)
      },
    },
    armed: async () => options.armed ?? null,
  }
  return { ports, calls, selected, uploaded, remembered }
}

const assigned = [{ id: '1', status: 'inbox', resumeCode: 'BE', resumeLang: 'en' }]

test('the pause switch stops the attach before anything is read from the page', async () => {
  const test_ = stage({ settings: { paused: true }, jobs: assigned })
  const outcome = await attachFor(test_.ports, '1', 9)

  expect(outcome).toEqual({
    ok: false,
    reason: 'paused',
    why: 'the agent is paused, so nothing is attached',
  })
  expect(test_.calls).toEqual(['GET /api/settings'])
})

test('attaching switched off in settings is honoured', async () => {
  const test_ = stage({ settings: { autoAttach: false }, jobs: assigned })
  const outcome = await attachFor(test_.ports, '1', 9)
  expect(outcome.ok).toBe(false)
  if (!outcome.ok) expect(outcome.reason).toBe('attach-off')
})

test('a posting with no assignment and nothing armed is left alone', async () => {
  const test_ = stage({ jobs: [{ id: '1', status: 'inbox', resumeCode: null, resumeLang: null }] })
  const outcome = await attachFor(test_.ports, '1', 9)
  expect(outcome.ok).toBe(false)
  if (!outcome.ok) expect(outcome.reason).toBe('no-assignment')
  expect(test_.uploaded).toEqual([])
})

test('the armed resume stands in when the agent has no verdict yet', async () => {
  const test_ = stage({
    jobs: [],
    armed: { code: 'BE', label: 'backend', languages: ['en'], at: 0 },
  })
  const outcome = await attachFor(test_.ports, '1', 9)
  expect(outcome.ok).toBe(true)
  if (outcome.ok) expect(outcome.name).toBe('resume-be-en.pdf')
})

test('a posting already judged elsewhere is never attached to', async () => {
  const test_ = stage({ jobs: [{ id: '1', status: 'skipped', resumeCode: 'BE', resumeLang: 'en' }] })
  const outcome = await attachFor(test_.ports, '1', 9)
  expect(outcome.ok).toBe(false)
  if (!outcome.ok) expect(outcome.reason).toBe('judged-elsewhere')
})

test('a posting the person opened themselves still takes the armed cv', async () => {
  const test_ = stage({
    jobs: [{ id: '1', status: 'opened', resumeCode: null, resumeLang: null }],
    armed: { code: 'BE', label: 'backend', languages: ['en'], at: 0 },
  })
  const outcome = await attachFor(test_.ports, '1', 9)
  expect(outcome.ok).toBe(true)
  if (outcome.ok) expect(outcome.name).toBe('resume-be-en.pdf')
})

test('one attach per posting, and a second ask is refused', async () => {
  const test_ = stage({ jobs: assigned, attached: ['1'] })
  const outcome = await attachFor(test_.ports, '1', 9)
  expect(outcome.ok).toBe(false)
  if (!outcome.ok) expect(outcome.reason).toBe('already-attached')
  expect(test_.uploaded).toEqual([])
})

test('a page that is not at the cv step is left alone', async () => {
  const test_ = stage({ jobs: assigned, reading: { fileInput: false } })
  const outcome = await attachFor(test_.ports, '1', 9)
  expect(outcome.ok).toBe(false)
  if (!outcome.ok) expect(outcome.reason).toBe('no-cv-step')
})

test('the copy linkedin already holds is selected, and nothing is uploaded', async () => {
  const test_ = stage({
    jobs: assigned,
    reading: {
      options: [
        { text: 'resume-fe-en.pdf Uploaded 1/2/2026', selected: false },
        { text: 'resume-be-en.pdf Uploaded 3/4/2026', selected: false },
      ],
    },
  })
  const outcome = await attachFor(test_.ports, '1', 9)

  expect(outcome).toEqual({
    ok: true,
    action: 'select',
    name: 'resume-be-en.pdf',
    why: 'linkedin already holds resume-be-en.pdf, so it is selected rather than uploaded again',
  })
  expect(test_.selected).toEqual(['resume-be-en.pdf'])
  expect(test_.uploaded).toEqual([])
  expect(test_.calls).not.toContain('GET /api/resumes/BE/en/file')
})

test('a store with nothing matching uploads one copy under the derived name', async () => {
  const test_ = stage({
    jobs: assigned,
    reading: { options: [{ text: 'resume-fe-tr.pdf', selected: false }] },
  })
  const outcome = await attachFor(test_.ports, '1', 9)

  expect(outcome.ok).toBe(true)
  if (outcome.ok) {
    expect(outcome.action).toBe('upload')
    expect(outcome.name).toBe('resume-be-en.pdf')
    expect(outcome.why).toBe('linkedin holds no file called resume-be-en.pdf')
  }
  expect(test_.selected).toEqual([])
  expect(test_.uploaded[0]?.name).toBe('resume-be-en.pdf')
  expect(test_.uploaded[0]?.base64).toBe('JVBERg==')
})

test('two copies under one name upload rather than guess', async () => {
  const test_ = stage({
    jobs: assigned,
    reading: {
      options: [
        { text: 'resume-be-en.pdf 1/1/2026', selected: false },
        { text: 'resume-be-en.pdf 2/2/2026', selected: false },
      ],
    },
  })
  const outcome = await attachFor(test_.ports, '1', 9)

  expect(outcome.ok).toBe(true)
  if (outcome.ok) expect(outcome.action).toBe('upload')
  expect(test_.selected).toEqual([])
  expect(test_.uploaded.length).toBe(1)
})

test('a selection the page will not make falls back to uploading', async () => {
  const test_ = stage({
    jobs: assigned,
    reading: { options: [{ text: 'resume-be-en.pdf', selected: false }] },
    select: { ok: false, reason: 'ambiguous', matches: 2 },
  })
  const outcome = await attachFor(test_.ports, '1', 9)

  expect(outcome.ok).toBe(true)
  if (outcome.ok) {
    expect(outcome.action).toBe('upload')
    expect(outcome.why).toBe('the stored copy could not be selected, so a fresh one was uploaded')
  }
})

test('one name uploads even when a file of that name is already stored', async () => {
  const test_ = stage({
    settings: { uploadFileName: 'my-cv.pdf', uploadFileNameMode: 'one-name' },
    jobs: assigned,
    reading: {
      options: [
        { text: 'my-cv.pdf Uploaded 1/1/2026', selected: false },
        { text: 'my-cv.pdf Uploaded 2/2/2026', selected: true },
      ],
    },
  })
  const outcome = await attachFor(test_.ports, '1', 9)

  expect(outcome).toEqual({
    ok: true,
    action: 'upload',
    name: 'my-cv.pdf',
    why: 'every variant attaches under the same name, so nothing tells the stored copies apart and each application uploads a fresh one',
  })
  expect(test_.selected).toEqual([])
  expect(test_.uploaded[0]?.name).toBe('my-cv.pdf')
})

test('per variant selects the copy linkedin already holds under that variant name', async () => {
  const test_ = stage({
    settings: { uploadFileName: 'my-cv.pdf', uploadFileNameMode: 'per-variant' },
    jobs: assigned,
    reading: { options: [{ text: 'my-cv-be-en.pdf Uploaded 1/1/2026', selected: false }] },
  })
  const outcome = await attachFor(test_.ports, '1', 9)

  expect(outcome.ok).toBe(true)
  if (outcome.ok) expect(outcome.action).toBe('select')
  expect(test_.selected).toEqual(['my-cv-be-en.pdf'])
  expect(test_.uploaded).toEqual([])
})

test('a cv field that will not take the file is reported, not retried for ever', async () => {
  const test_ = stage({
    jobs: assigned,
    attach: { ok: false, reason: 'no-resume-field', matches: 1 },
  })
  const outcome = await attachFor(test_.ports, '1', 9)
  expect(outcome.ok).toBe(false)
  if (!outcome.ok) expect(outcome.reason).toBe('no-resume-field')
  expect(test_.remembered).toEqual([])
})

test('linkedin own applied state is what syncs a posting, not a phrase on the page', async () => {
  const test_ = stage({ jobs: assigned })
  const asked: string[] = []

  const notApplied = await syncApplied(
    {
      agent: test_.ports.agent,
      applied: async (id) => {
        asked.push(id)
        return false
      },
    },
    '1',
  )
  expect(notApplied).toBe(false)
  expect(test_.calls).not.toContain('POST /api/jobs/1/applied')

  const applied = await syncApplied({ agent: test_.ports.agent, applied: async () => true }, '1')
  expect(applied).toBe(true)
  expect(test_.calls).toContain('POST /api/jobs/1/applied')
})

test('the bytes reaching the page are the cv the agent served', () => {
  expect(base64Of(new Uint8Array([37, 80, 68, 70]))).toBe('JVBERg==')
})
