import { agentBase } from '@/lib/agent/base'
import type {
  CredentialStatus,
  Harvested,
  Health,
  Hello,
  IndexResult,
  Ingested,
  ListStatus,
  Note,
  OpenedRecord,
  PlaceResolution,
  Plan,
  PlanState,
  PluginState,
  Posting,
  PostingList,
  ProfileState,
  PromptPreview,
  Reflection,
  ResumeList,
  Round,
  ScreenResult,
  SettingsDocument,
  SetupState,
  Staleness,
  Trace,
  TraceList,
  Transition,
  Widening,
} from '@/lib/agent/types'

export type FailureKind = 'unreachable' | 'refused' | 'rejected'

export interface Failure {
  kind: FailureKind
  message: string
  status: number
  reason: string
}

export type Result<T> = { ok: true; value: T } | { ok: false; failure: Failure }

export interface ClientOptions {
  base?: () => Promise<string>
  fetcher?: typeof fetch
  now?: () => number
}

const READ_TIMEOUT = 60_000
const WRITE_TIMEOUT = 120_000
const MODEL_TIMEOUT = 2 * 60 * 60_000
const FILE_TIMEOUT = 120_000

function unreachable(base: string, detail: string): Failure {
  return {
    kind: 'unreachable',
    status: 0,
    reason: 'unreachable',
    message: `the agent is not answering on ${base}, ${detail}`,
  }
}

function sentence(body: unknown, fallback: string): string {
  if (body && typeof body === 'object') {
    const record = body as Record<string, unknown>
    if (typeof record.message === 'string' && record.message) return record.message
    if (typeof record.error === 'string' && record.error) return record.error
  }
  return fallback
}

function refusalReason(body: unknown): string {
  if (body && typeof body === 'object') {
    const record = body as Record<string, unknown>
    if (typeof record.reason === 'string' && record.reason) return record.reason
  }
  return 'refused'
}

interface Call {
  method?: string
  body?: unknown
  timeout?: number
  query?: Record<string, string | number | undefined>
}

function search(query: Call['query']): string {
  if (!query) return ''
  const params = new URLSearchParams()
  for (const [key, value] of Object.entries(query)) {
    if (value === undefined || value === '') continue
    params.set(key, String(value))
  }
  const encoded = params.toString()
  return encoded ? `?${encoded}` : ''
}

export class AgentClient {
  private readonly resolveBase: () => Promise<string>
  private readonly fetcher: typeof fetch

  constructor(options: ClientOptions = {}) {
    this.resolveBase = options.base ?? agentBase
    this.fetcher = options.fetcher ?? globalThis.fetch.bind(globalThis)
  }

  async url(path: string, query?: Call['query']): Promise<string> {
    return `${await this.resolveBase()}${path}${search(query)}`
  }

  private async call<T>(path: string, call: Call = {}): Promise<Result<T>> {
    const base = await this.resolveBase()
    const timeout = call.timeout ?? (call.method && call.method !== 'GET' ? WRITE_TIMEOUT : READ_TIMEOUT)
    const controller = new AbortController()
    const timer = setTimeout(() => controller.abort(), timeout)

    let response: Response
    try {
      response = await this.fetcher(`${base}${path}${search(call.query)}`, {
        method: call.method ?? 'GET',
        headers: call.body === undefined ? undefined : { 'content-type': 'application/json' },
        body: call.body === undefined ? undefined : JSON.stringify(call.body),
        signal: controller.signal,
      })
    } catch {
      clearTimeout(timer)
      return {
        ok: false,
        failure: controller.signal.aborted
          ? unreachable(base, `it took longer than ${Math.round(timeout / 1000)} seconds to answer`)
          : unreachable(base, 'start it and the panel picks up where it left off'),
      }
    }
    clearTimeout(timer)

    let body: unknown = null
    const text = await response.text().catch(() => '')
    if (text) {
      try {
        body = JSON.parse(text)
      } catch {
        body = null
      }
    }

    if (response.ok) return { ok: true, value: body as T }

    if (response.status === 409) {
      return {
        ok: false,
        failure: {
          kind: 'refused',
          status: 409,
          reason: refusalReason(body),
          message: sentence(body, 'the agent refused that, and gave no reason'),
        },
      }
    }
    return {
      ok: false,
      failure: {
        kind: 'rejected',
        status: response.status,
        reason: 'rejected',
        message: sentence(body, `the agent answered ${response.status} and said nothing more`),
      },
    }
  }

  health(): Promise<Result<Health>> {
    return this.call('/api/health')
  }

  setup(): Promise<Result<SetupState>> {
    return this.call('/api/setup')
  }

  chooseFolder(dir: string): Promise<Result<SetupState>> {
    return this.call('/api/setup', { method: 'POST', body: { dir } })
  }

  credentials(): Promise<Result<CredentialStatus>> {
    return this.call('/api/credentials')
  }

  storeCredentials(value: string): Promise<Result<CredentialStatus>> {
    return this.call('/api/credentials', { method: 'PUT', body: { value } })
  }

  forgetCredentials(): Promise<Result<CredentialStatus>> {
    return this.call('/api/credentials', { method: 'DELETE' })
  }

  hello(manifest: Hello): Promise<Result<{ plugin: PluginState }>> {
    return this.call('/api/plugin/hello', { method: 'POST', body: manifest })
  }

  plugin(): Promise<Result<{ plugin: PluginState }>> {
    return this.call('/api/plugin')
  }

  resumes(): Promise<Result<ResumeList>> {
    return this.call('/api/resumes')
  }

  resumeFileUrl(code: string, lang: string): Promise<string> {
    return this.url(`/api/resumes/${encodeURIComponent(code)}/${encodeURIComponent(lang)}/file`)
  }

  async resumeFile(code: string, lang: string): Promise<Result<Uint8Array>> {
    const base = await this.resolveBase()
    const path = `/api/resumes/${encodeURIComponent(code)}/${encodeURIComponent(lang)}/file`
    const controller = new AbortController()
    const timer = setTimeout(() => controller.abort(), FILE_TIMEOUT)
    try {
      const response = await this.fetcher(`${base}${path}`, { signal: controller.signal })
      if (!response.ok) {
        return {
          ok: false,
          failure: {
            kind: 'rejected',
            status: response.status,
            reason: 'rejected',
            message: `the ${code} cv in ${lang} did not come back from the agent`,
          },
        }
      }
      return { ok: true, value: new Uint8Array(await response.arrayBuffer()) }
    } catch {
      return { ok: false, failure: unreachable(base, 'the cv bytes are read from disk by the agent') }
    } finally {
      clearTimeout(timer)
    }
  }

  profile(): Promise<Result<ProfileState>> {
    return this.call('/api/profile')
  }

  index(force = false): Promise<Result<IndexResult>> {
    return this.call('/api/index', { method: 'POST', body: { force }, timeout: MODEL_TIMEOUT })
  }

  settings(): Promise<Result<SettingsDocument>> {
    return this.call('/api/settings')
  }

  saveSettings(patch: Record<string, unknown>): Promise<Result<SettingsDocument>> {
    return this.call('/api/settings', { method: 'PUT', body: patch })
  }

  planState(): Promise<Result<PlanState>> {
    return this.call('/api/plan')
  }

  startRound(): Promise<Result<Plan>> {
    return this.call('/api/cycle/start', { method: 'POST', body: {}, timeout: MODEL_TIMEOUT })
  }

  widenRound(cycleId: number): Promise<Result<Widening>> {
    return this.call('/api/cycle/widen', { method: 'POST', body: { cycleId }, timeout: MODEL_TIMEOUT })
  }

  skipQuery(input: {
    cycleId: number
    queryLabel: string
    place: string
    ring: string
    reason: string
  }): Promise<Result<unknown>> {
    return this.call('/api/cycle/skip', { method: 'POST', body: input })
  }

  finishRound(cycleId: number, reason: string, error = ''): Promise<Result<{ cycle: Round }>> {
    return this.call('/api/cycle/finish', { method: 'POST', body: { cycleId, reason, error } })
  }

  jobs(status: ListStatus = 'all'): Promise<Result<PostingList>> {
    return this.call('/api/jobs', { query: { status } })
  }

  harvest(input: {
    cycleId: number | null
    queryLabel: string
    pages: number
    jobs: Harvested[]
  }): Promise<Result<Ingested>> {
    return this.call('/api/jobs', { method: 'POST', body: input })
  }

  recordOpened(
    jobs: Harvested[],
    resumeCode = '',
    resumeLang = '',
  ): Promise<Result<OpenedRecord>> {
    return this.call('/api/jobs/opened', {
      method: 'POST',
      body: { jobs, resumeCode, resumeLang },
    })
  }

  pendingDescriptions(limit?: number): Promise<Result<{ jobs: { id: string; url: string }[] }>> {
    return this.call('/api/jobs/pending-descriptions', { query: { limit } })
  }

  storeDescriptions(
    descriptions: { id: string; description: string; applied?: boolean; closed?: boolean }[],
  ): Promise<Result<unknown>> {
    return this.call('/api/jobs/descriptions', { method: 'POST', body: { descriptions } })
  }

  screen(limit = 0): Promise<Result<ScreenResult>> {
    return this.call('/api/jobs/screen', { method: 'POST', body: { limit }, timeout: MODEL_TIMEOUT })
  }

  staleness(): Promise<Result<Staleness>> {
    return this.call('/api/jobs/stale')
  }

  rescreen(): Promise<Result<{ reset: number }>> {
    return this.call('/api/jobs/rescreen', { method: 'POST', body: {} })
  }

  move(id: string, transition: Transition): Promise<Result<{ job: Posting }>> {
    return this.call(`/api/jobs/${encodeURIComponent(id)}/${transition}`, { method: 'POST' })
  }

  recordOutcome(id: string, outcome: string, note = ''): Promise<Result<{ job: Posting }>> {
    return this.call(`/api/jobs/${encodeURIComponent(id)}/outcome`, {
      method: 'POST',
      body: { outcome, note },
    })
  }

  places(term = ''): Promise<Result<{ places: { name: string; ring: string }[]; reason?: string }>> {
    return this.call('/api/places', { query: { q: term } })
  }

  savePlaces(places: PlaceResolution[]): Promise<Result<unknown>> {
    return this.call('/api/places', { method: 'POST', body: { places } })
  }

  notes(): Promise<Result<{ notes: Note[] }>> {
    return this.call('/api/notes')
  }

  deleteNote(id: number): Promise<Result<{ notes: Note[] }>> {
    return this.call(`/api/notes/${id}`, { method: 'DELETE' })
  }

  reflect(force = false): Promise<Result<Reflection>> {
    return this.call('/api/reflect', { method: 'POST', body: { force }, timeout: MODEL_TIMEOUT })
  }

  traces(limit?: number): Promise<Result<TraceList>> {
    return this.call('/api/traces', { query: { limit } })
  }

  trace(id: number): Promise<Result<Trace>> {
    return this.call(`/api/traces/${id}`)
  }

  promptPreview(): Promise<Result<PromptPreview>> {
    return this.call('/api/prompt/preview', { timeout: WRITE_TIMEOUT })
  }

  eventsUrl(lastEventId: string): Promise<string> {
    return this.url('/api/events', { lastEventId: lastEventId || undefined })
  }
}

export const agent = new AgentClient()
