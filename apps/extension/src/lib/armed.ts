import type { Posting } from '@/lib/agent/types'
import { consequenceOf, pickLanguage, uploadName } from '@/lib/linkedin/resume'
import type { Preset } from '@/lib/presets'
import { forgetKeys, readKey, writeKeys } from '@/lib/storage'

export const ARMED_KEY = 'armedResume'
export const ATTEMPT_KEY = 'lastAttach'

export interface Attempt {
  id: string
  at: number
  ok: boolean
  why: string
}

export async function loadAttempt(): Promise<Attempt | null> {
  const stored = await readKey<Attempt | null>(ATTEMPT_KEY, null)
  if (!stored || typeof stored.id !== 'string' || stored.id === '') return null
  return stored
}

export function attemptLine(attempt: Attempt | null, postingId: string | null): string {
  if (!attempt || !postingId || attempt.id !== postingId) return ''
  return attempt.ok ? `last try: ${attempt.why}` : `last try failed: ${attempt.why}`
}

export interface Armed {
  code: string
  label: string
  languages: string[]
  at: number
}

export type PickSource = 'agent' | 'armed' | 'closest'

export interface ResumePick {
  code: string
  lang: string | null
  source: PickSource
}

export async function loadArmed(): Promise<Armed | null> {
  const stored = await readKey<Armed | null>(ARMED_KEY, null)
  if (!stored || typeof stored.code !== 'string' || stored.code === '') return null
  return stored
}

export async function armPreset(preset: Preset): Promise<Armed | null> {
  if (!preset.code) {
    await disarm()
    return null
  }
  const armed: Armed = {
    code: preset.code,
    label: preset.label,
    languages: preset.languages ?? [],
    at: Date.now(),
  }
  await writeKeys({ [ARMED_KEY]: armed })
  return armed
}

export async function disarm(): Promise<void> {
  await forgetKeys([ARMED_KEY])
}

export function resumeFor(
  posting: Pick<Posting, 'resumeCode' | 'resumeLang'>,
  armed: Armed | null,
): ResumePick | null {
  if (posting.resumeCode) {
    return { code: posting.resumeCode, lang: posting.resumeLang ?? null, source: 'agent' }
  }
  if (armed) {
    return { code: armed.code, lang: armed.languages[0] ?? null, source: 'armed' }
  }
  return null
}

export interface ApplySettings {
  autoAttach: boolean
  paused: boolean
  uploadFileName: string
  uploadFileNameMode: string
  resumeLanguages: string[]
}

export type AttachSource = PickSource | 'chosen' | 'none'

export interface PickPreview {
  code: string | null
  lang: string | null
  source: AttachSource
  why: string
}

export interface Attachment {
  code: string | null
  lang: string | null
  source: AttachSource
  known: boolean
  why: string
  attachesAs: string
  consequence: string
  blocked: string
}

export const ATTACHABLE = ['inbox', 'queue', 'manual', 'opened']

function blockedBy(posting: Posting | null, apply: ApplySettings | null): string {
  if (!apply) return 'the agent is not answering, so nothing attaches'
  if (apply.paused) return 'the agent is paused, so nothing attaches until you resume it'
  if (!apply.autoAttach) return 'attaching is switched off in settings, so nothing attaches'
  if (posting && !ATTACHABLE.includes(posting.status)) {
    return `this posting sits in ${posting.status}, so nothing attaches`
  }
  return ''
}

export function attachmentFor(
  posting: Posting | null,
  armed: Armed | null,
  apply: ApplySettings | null,
): Attachment {
  const pick = resumeFor(
    { resumeCode: posting?.resumeCode ?? null, resumeLang: posting?.resumeLang ?? null },
    armed,
  )
  const known = posting !== null
  const blocked = blockedBy(posting, apply)

  if (!pick) {
    return {
      code: null,
      lang: null,
      source: 'none',
      known,
      why: known
        ? 'the agent judged this one but assigned no cv, and nothing is armed'
        : 'the agent has never judged this posting, which is normal for one you found yourself, and nothing is armed',
      attachesAs: '',
      consequence: '',
      blocked,
    }
  }

  const lang = pick.lang ?? pickLanguage(armed?.languages ?? [], apply?.resumeLanguages ?? [])
  const byHand = posting?.stage === 'hand'
  const source: AttachSource = pick.source === 'agent' && byHand ? 'chosen' : pick.source
  const why =
    source === 'chosen'
      ? 'this is the cv you chose when you opened this posting'
      : source === 'agent'
        ? 'the agent picked this cv for this posting'
        : known
          ? 'the agent assigned no cv to this posting, so the cv closest to its text attaches, and the armed one when nothing is close'
          : 'the agent has never judged this posting, which is normal for one you found yourself, so the cv closest to its text attaches, and the armed one when nothing is close'

  return {
    code: pick.code,
    lang: lang || null,
    source,
    known,
    why,
    attachesAs: apply
      ? uploadName(apply.uploadFileName, pick.code, lang, apply.uploadFileNameMode)
      : '',
    consequence: apply ? consequenceOf(apply.uploadFileNameMode) : '',
    blocked,
  }
}
