import type { AgentClient } from '@/lib/agent/client'
import type { Posting } from '@/lib/agent/types'
import { ATTACHABLE, loadArmed, resumeFor } from '@/lib/armed'
import {
  chooseResume,
  decideAttachment,
  readStored,
  uploadName,
} from '@/lib/linkedin/resume'
import type { FormReading, InjectionResult } from '@/worker/inject'

export const ATTACHED_KEY = 'attached'
export const ATTACHED_MAX = 500

export interface PagePort {
  read(tabId: number): Promise<FormReading>
  select(tabId: number, name: string): Promise<InjectionResult>
  attach(tabId: number, base64: string, name: string): Promise<InjectionResult>
}

export interface AttachedPort {
  has(id: string): Promise<boolean>
  remember(id: string): Promise<void>
}

export interface ApplyPorts {
  agent: AgentClient
  page: PagePort
  attached: AttachedPort
  armed?: typeof loadArmed
}

export type ApplyOutcome =
  | { ok: true; action: 'select' | 'upload'; name: string; why: string }
  | { ok: false; reason: string; why: string }

export function base64Of(bytes: Uint8Array): string {
  let binary = ''
  const chunk = 0x8000
  for (let start = 0; start < bytes.length; start += chunk) {
    binary += String.fromCharCode(...bytes.subarray(start, start + chunk))
  }
  return btoa(binary)
}

function refuse(reason: string, why: string): ApplyOutcome {
  return { ok: false, reason, why }
}

export async function attachFor(
  ports: ApplyPorts,
  postingId: string,
  tabId: number,
): Promise<ApplyOutcome> {
  const settings = await ports.agent.settings()
  if (!settings.ok) {
    return refuse('agent-down', 'the agent is not answering, so nothing is attached')
  }
  const apply = settings.value.settings.apply
  if (apply.paused) {
    return refuse('paused', 'the agent is paused, so nothing is attached')
  }
  if (!apply.autoAttach) {
    return refuse('attach-off', 'attaching is switched off in settings')
  }
  if (await ports.attached.has(postingId)) {
    return refuse('already-attached', 'this posting already had its cv attached once')
  }

  const listed = await ports.agent.jobs('all')
  const posting: Posting | undefined = listed.ok
    ? listed.value.jobs.find((entry) => entry.id === postingId)
    : undefined
  if (posting && !ATTACHABLE.includes(posting.status)) {
    return refuse('judged-elsewhere', `this posting sits in ${posting.status}, so nothing is attached`)
  }

  const armed = await (ports.armed ?? loadArmed)()
  const pick = resumeFor(
    { resumeCode: posting?.resumeCode ?? null, resumeLang: posting?.resumeLang ?? null },
    armed,
  )
  if (!pick) {
    return refuse('no-assignment', 'no cv is assigned to this posting and none is armed')
  }

  const resumes = await ports.agent.resumes()
  if (!resumes.ok) {
    return refuse('agent-down', 'the agent could not list the cvs, so nothing is attached')
  }
  const chosen = chooseResume(resumes.value.resumes, pick.code, pick.lang, apply.resumeLanguages)
  if (!chosen) {
    return refuse('unknown-resume', `the agent has no cv filed under ${pick.code}`)
  }

  const wanted = uploadName(
    apply.uploadFileName,
    chosen.code,
    chosen.lang,
    apply.uploadFileNameMode,
  )
  const reading = await ports.page.read(tabId)
  if (!reading.fileInput) {
    return refuse('no-cv-step', 'this page is not showing the cv step, so nothing is attached')
  }

  const decision = decideAttachment(
    readStored(reading.options),
    wanted,
    apply.uploadFileNameMode,
  )
  if (decision.action === 'select') {
    const selected = await ports.page.select(tabId, decision.name)
    if (selected.ok) {
      await ports.attached.remember(postingId)
      return { ok: true, action: 'select', name: decision.name, why: decision.why }
    }
  }

  const bytes = await ports.agent.resumeFile(chosen.code, chosen.lang)
  if (!bytes.ok) {
    return refuse('no-file', bytes.failure.message)
  }
  const uploaded = await ports.page.attach(tabId, base64Of(bytes.value), wanted)
  if (!uploaded.ok) {
    return refuse(uploaded.reason, `the cv field would not take the file: ${uploaded.reason}`)
  }
  await ports.attached.remember(postingId)
  return {
    ok: true,
    action: 'upload',
    name: wanted,
    why: decision.action === 'upload' ? decision.why : 'the stored copy could not be selected, so a fresh one was uploaded',
  }
}

export interface AppliedPorts {
  agent: AgentClient
  applied(id: string): Promise<boolean>
}

export async function syncApplied(ports: AppliedPorts, postingId: string): Promise<boolean> {
  if (!(await ports.applied(postingId))) return false
  const moved = await ports.agent.move(postingId, 'applied')
  return moved.ok
}
