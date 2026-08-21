import { beforeEach, expect, test } from 'bun:test'
import type { Posting } from '@/lib/agent/types'
import { armPreset, attachmentFor, attemptLine, disarm, loadArmed, resumeFor } from '@/lib/armed'
import { BROAD_PRESET, type Preset } from '@/lib/presets'
import { resetMemoryStore } from '@/lib/storage'

const preset: Preset = {
  id: 'AA',
  label: 'Backend Engineer',
  aim: 'payment services',
  code: 'AA',
  languages: ['en', 'tr'],
  leads: ['Kotlin'],
}

beforeEach(() => {
  resetMemoryStore()
})

test('choosing a preset arms its variant and the choice survives a restart', async () => {
  await armPreset(preset)
  const armed = await loadArmed()

  expect(armed?.code).toBe('AA')
  expect(armed?.languages).toEqual(['en', 'tr'])
})

test('the broad preset arms nothing', async () => {
  await armPreset(preset)
  await armPreset(BROAD_PRESET)

  expect(await loadArmed()).toBeNull()
})

test('disarming leaves the agent as the only source of a pick', async () => {
  await armPreset(preset)
  await disarm()

  expect(await loadArmed()).toBeNull()
})

test('an agent assigned posting keeps the agent pick', async () => {
  const armed = await armPreset(preset)
  const pick = resumeFor({ resumeCode: 'ZZ', resumeLang: 'tr' }, armed)

  expect(pick).toEqual({ code: 'ZZ', lang: 'tr', source: 'agent' })
})

test('a posting with no verdict takes the armed variant', async () => {
  const armed = await armPreset(preset)
  const pick = resumeFor({ resumeCode: null, resumeLang: null }, armed)

  expect(pick).toEqual({ code: 'AA', lang: 'en', source: 'armed' })
})

test('nothing armed and no verdict attaches nothing', () => {
  expect(resumeFor({ resumeCode: null, resumeLang: null }, null)).toBeNull()
})

const APPLY = {
  autoAttach: true,
  paused: false,
  uploadFileName: 'resume.pdf',
  uploadFileNameMode: 'per-variant',
  resumeLanguages: ['en'],
}

function judged(overrides: Partial<Posting> = {}): Posting {
  return {
    id: '1',
    title: 'Senior Software Engineer',
    company: 'northwind',
    location: 'remote',
    url: '',
    dupeOf: null,
    description: '',
    descriptionState: 'ok',
    listedAt: null,
    easyApply: true,
    status: 'inbox',
    stage: 'deep',
    score: 80,
    triageReason: null,
    verdictReason: null,
    seniority: null,
    workplace: null,
    contractType: null,
    postingLang: null,
    agency: null,
    statedPay: null,
    resumeCode: 'ZZ',
    resumeLang: 'tr',
    resumeFit: 'strong',
    tailoredResume: null,
    stale: false,
    outcome: null,
    outcomeNote: null,
    cycleId: null,
    queryLabel: null,
    createdAt: '',
    updatedAt: '',
    ...overrides,
  }
}

test('the agent’s own pick wins, and the panel says so in one line', async () => {
  const armed = await armPreset(preset)
  const attachment = attachmentFor(judged(), armed, APPLY)

  expect(attachment.code).toBe('ZZ')
  expect(attachment.lang).toBe('tr')
  expect(attachment.source).toBe('agent')
  expect(attachment.why).toBe('the agent picked this cv for this posting')
  expect(attachment.attachesAs).toBe('resume-zz-tr.pdf')
})

test('a cv on a posting he opened himself is his pick, not the agent’s', async () => {
  const armed = await armPreset(preset)
  const attachment = attachmentFor(
    judged({ status: 'opened', stage: 'hand', resumeCode: 'GO', resumeLang: 'en' }),
    armed,
    APPLY,
  )

  expect(attachment.source).toBe('chosen')
  expect(attachment.code).toBe('GO')
  expect(attachment.why).toBe('this is the cv you chose when you opened this posting')
})

test('the panel states the same status rule the worker enforces', async () => {
  const armed = await armPreset(preset)

  expect(attachmentFor(judged({ status: 'opened', stage: 'hand' }), armed, APPLY).blocked).toBe('')
  expect(attachmentFor(judged({ status: 'skipped' }), armed, APPLY).blocked).toBe(
    'this posting sits in skipped, so nothing attaches',
  )
  expect(attachmentFor(judged({ status: 'applied' }), armed, APPLY).blocked).toBe(
    'this posting sits in applied, so nothing attaches',
  )
})

test('a posting the agent judged without a cv falls to the armed one', async () => {
  const armed = await armPreset(preset)
  const attachment = attachmentFor(judged({ resumeCode: null, resumeLang: null }), armed, APPLY)

  expect(attachment.code).toBe('AA')
  expect(attachment.source).toBe('armed')
  expect(attachment.why).toBe(
    'the agent assigned no cv to this posting, so the cv closest to its text attaches, and the armed one when nothing is close',
  )
})

test('a posting the agent never judged says so plainly', async () => {
  const armed = await armPreset(preset)
  const attachment = attachmentFor(null, armed, APPLY)

  expect(attachment.known).toBe(false)
  expect(attachment.source).toBe('armed')
  expect(attachment.why).toContain('never judged this posting')
  expect(attachment.attachesAs).toBe('resume-aa-en.pdf')
})

test('nothing armed on a posting nobody judged attaches nothing, and says why', () => {
  const attachment = attachmentFor(null, null, APPLY)

  expect(attachment.source).toBe('none')
  expect(attachment.code).toBeNull()
  expect(attachment.why).toContain('nothing is armed')
  expect(attachment.attachesAs).toBe('')
})

test('a paused or switched off agent attaches nothing and names which', async () => {
  const armed = await armPreset(preset)

  expect(attachmentFor(judged(), armed, { ...APPLY, paused: true }).blocked).toBe(
    'the agent is paused, so nothing attaches until you resume it',
  )
  expect(attachmentFor(judged(), armed, { ...APPLY, autoAttach: false }).blocked).toBe(
    'attaching is switched off in settings, so nothing attaches',
  )
  expect(attachmentFor(judged(), armed, null).blocked).toBe(
    'the agent is not answering, so nothing attaches',
  )
  expect(attachmentFor(judged(), armed, APPLY).blocked).toBe('')
})

test('the panel says what the last attach attempt actually did', () => {
  const failed = { id: '1', at: 1, ok: false, why: 'this posting sits in skipped, so nothing is attached' }
  expect(attemptLine(failed, '1')).toBe(
    'last try failed: this posting sits in skipped, so nothing is attached',
  )
  expect(attemptLine(failed, '2')).toBe('')
  expect(attemptLine(null, '1')).toBe('')
  expect(attemptLine({ id: '1', at: 1, ok: true, why: 'uploaded resume.pdf' }, '1')).toBe(
    'last try: uploaded resume.pdf',
  )
})
