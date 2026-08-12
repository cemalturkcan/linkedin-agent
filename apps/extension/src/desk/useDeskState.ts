import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { AgentClient, type Failure, type Result } from '@/lib/agent/client'
import { afterStatus, AgentStream, NEVER_LIVE, type StreamStatus } from '@/lib/agent/stream'
import { coalesce } from '@/lib/coalesce'
import { announceNow } from '@/lib/worker'
import type {
  CredentialStatus,
  Note,
  PlanState,
  Posting,
  ProfileState,
  Settings,
  SettingsDocument,
  SetupState,
  Trace,
  TraceList,
} from '@/lib/agent/types'
import { queueOnOpen } from '@/lib/lists'
import { openTab } from '@/lib/open'

export interface Resource<T> {
  value: T | null
  live: boolean
  at: number | null
  failure: Failure | null
}

export function pending<T>(): Resource<T> {
  return { value: null, live: false, at: null, failure: null }
}

export function outageLine(message: string): string {
  return `${message}. everything below is the last thing it said, not what is true now.`
}

function settle<T>(previous: Resource<T>, result: Result<T>): Resource<T> {
  if (result.ok) return { value: result.value, live: true, at: Date.now(), failure: null }
  return { value: previous.value, live: false, at: previous.at, failure: result.failure }
}

export interface RunningCall {
  id: number
  purpose: string
  startedAt: number
  output: string
  capped: boolean
}

export interface DeskState {
  setup: Resource<SetupState>
  credentials: Resource<CredentialStatus>
  profile: Resource<ProfileState>
  settings: Resource<SettingsDocument>
  plan: Resource<PlanState>
  traces: Resource<TraceList>
  jobs: Resource<Posting[]>
  notes: Note[]
  stream: StreamStatus
  notice: string | null
  busy: string | null
  running: RunningCall | null
  unreachable: string | null
}

export interface DeskActions {
  refresh(): Promise<void>
  reconnect(): void
  chooseFolder(dir: string): Promise<boolean>
  pasteCredentials(value: string): Promise<boolean>
  clearCredentials(): Promise<void>
  reindex(force: boolean): Promise<void>
  saveSettings(next: Settings): Promise<boolean>
  deleteNote(id: number): Promise<void>
  recordOutcome(id: string, outcome: string, note: string): Promise<boolean>
  openPosting(posting: Posting): Promise<void>
  reflect(): Promise<string>
  readTrace(id: number): Promise<Result<Trace>>
  say(message: string | null): void
}

const NOTHING_RUNNING: RunningCall | null = null

export function useDeskState(client: AgentClient): DeskState & { actions: DeskActions } {
  const [setup, setSetup] = useState<Resource<SetupState>>(pending<SetupState>())
  const [credentials, setCredentials] = useState<Resource<CredentialStatus>>(
    pending<CredentialStatus>(),
  )
  const [profile, setProfile] = useState<Resource<ProfileState>>(pending<ProfileState>())
  const [settings, setSettings] = useState<Resource<SettingsDocument>>(pending<SettingsDocument>())
  const [plan, setPlan] = useState<Resource<PlanState>>(pending<PlanState>())
  const [traces, setTraces] = useState<Resource<TraceList>>(pending<TraceList>())
  const [jobs, setJobs] = useState<Resource<Posting[]>>(pending<Posting[]>())
  const [stream, setStream] = useState<StreamStatus>('connecting')
  const [notice, setNotice] = useState<string | null>(null)
  const [busy, setBusy] = useState<string | null>(null)
  const [running, setRunning] = useState<RunningCall | null>(NOTHING_RUNNING)

  const streamRef = useRef<AgentStream | null>(null)

  const loadSetup = useCallback(async () => {
    const result = await client.setup()
    setSetup((previous) => settle(previous, result))
  }, [client])

  const loadCredentials = useCallback(async () => {
    const result = await client.credentials()
    setCredentials((previous) => settle(previous, result))
  }, [client])

  const loadProfile = useCallback(async () => {
    const result = await client.profile()
    setProfile((previous) => settle(previous, result))
  }, [client])

  const loadSettings = useCallback(async () => {
    const result = await client.settings()
    setSettings((previous) => settle(previous, result))
  }, [client])

  const loadPlan = useCallback(async () => {
    const result = await client.planState()
    setPlan((previous) => settle(previous, result))
  }, [client])

  const loadTraces = useCallback(async () => {
    const result = await client.traces()
    setTraces((previous) => settle(previous, result))
  }, [client])

  const loadJobs = useCallback(async () => {
    const result = await client.jobs('all')
    setJobs((previous) =>
      settle(previous, result.ok ? { ok: true, value: result.value.jobs ?? [] } : result),
    )
  }, [client])

  const refresh = useCallback(async () => {
    await Promise.all([
      loadSetup(),
      loadCredentials(),
      loadProfile(),
      loadSettings(),
      loadPlan(),
      loadTraces(),
      loadJobs(),
    ])
  }, [loadCredentials, loadJobs, loadPlan, loadProfile, loadSettings, loadSetup, loadTraces])

  useEffect(() => {
    void announceNow()
    void refresh()
  }, [refresh])

  useEffect(() => {
    const source = new AgentStream({ url: (cursor) => client.eventsUrl(cursor) })
    streamRef.current = source

    const burstJobs = coalesce(() => void loadJobs())
    const burstPlan = coalesce(() => void loadPlan())
    const burstTraces = coalesce(() => void loadTraces())
    const burstProfile = coalesce(() => {
      void loadProfile()
      void loadSetup()
    })

    let track = NEVER_LIVE
    const offStatus = source.onStatus((status) => {
      setStream(status)
      const seen = afterStatus(track, status)
      track = seen.track
      if (seen.refetch) void refresh()
    })
    const offState = source.onState((frame) => {
      if (frame.name === 'error') {
        const text = frame.data.message
        setNotice(typeof text === 'string' && text ? text : 'the agent reported a failure')
        return
      }
      switch (frame.route) {
        case '/api/setup':
          void loadSetup()
          void loadCredentials()
          return
        case '/api/profile':
          burstProfile()
          return
        case '/api/settings':
          void loadSettings()
          return
        case '/api/plan':
        case '/api/plugin':
          burstPlan()
          return
        case '/api/jobs':
          burstJobs()
          return
        default:
          return
      }
    })

    const offDelta = source.onDelta((frame) => {
      if (frame.name === 'trace') {
        const id = Number(frame.data.id)
        const state = String(frame.data.state ?? '')
        if (!Number.isFinite(id)) return
        if (state === 'running') {
          setRunning({
            id,
            purpose: String(frame.data.purpose ?? 'model call'),
            startedAt: Date.now(),
            output: '',
            capped: false,
          })
          return
        }
        setRunning((previous) => (previous && previous.id === id ? null : previous))
        burstTraces()
        return
      }
      const id = Number(frame.data.id)
      if (!Number.isFinite(id)) return
      const text = typeof frame.data.text === 'string' ? frame.data.text : ''
      const capped = frame.data.capped === true
      setRunning((previous) => {
        if (!previous || previous.id !== id) {
          return {
            id,
            purpose: String(frame.data.purpose ?? 'model call'),
            startedAt: Date.now(),
            output: text,
            capped,
          }
        }
        return { ...previous, output: previous.output + text, capped: previous.capped || capped }
      })
    })

    source.start()
    return () => {
      offStatus()
      offState()
      offDelta()
      burstJobs.cancel()
      burstPlan.cancel()
      burstTraces.cancel()
      burstProfile.cancel()
      source.stop()
      streamRef.current = null
    }
  }, [client, loadCredentials, loadJobs, loadPlan, loadProfile, loadSettings, loadSetup, loadTraces])

  const unreachable = useMemo(() => {
    for (const resource of [setup, settings, profile, plan, traces, jobs]) {
      if (resource.failure?.kind === 'unreachable') return resource.failure.message
    }
    return null
  }, [jobs, plan, profile, setup, settings, traces])

  const notes = plan.value?.notes ?? []

  const actions = useMemo<DeskActions>(
    () => ({
      refresh,
      reconnect() {
        streamRef.current?.retryNow()
        void refresh()
      },
      async chooseFolder(dir) {
        setBusy('scanning')
        const result = await client.chooseFolder(dir)
        setBusy(null)
        if (!result.ok) {
          setNotice(result.failure.message)
          return false
        }
        setSetup((previous) => settle(previous, result))
        setNotice(null)
        await Promise.all([loadProfile(), loadSettings()])
        return true
      },
      async pasteCredentials(value) {
        setBusy('checking')
        const result = await client.storeCredentials(value)
        setBusy(null)
        if (!result.ok) {
          setNotice(result.failure.message)
          return false
        }
        setCredentials((previous) => settle(previous, result))
        setNotice(null)
        await loadSetup()
        return true
      },
      async clearCredentials() {
        const result = await client.forgetCredentials()
        if (!result.ok) return setNotice(result.failure.message)
        setCredentials((previous) => settle(previous, result))
        await loadSetup()
      },
      async reindex(force) {
        setBusy('indexing')
        const result = await client.index(force)
        setBusy(null)
        if (!result.ok) setNotice(result.failure.message)
        await Promise.all([loadProfile(), loadSetup()])
      },
      async saveSettings(next) {
        setBusy('saving')
        const result = await client.saveSettings(next as unknown as Record<string, unknown>)
        setBusy(null)
        if (!result.ok) {
          setNotice(result.failure.message)
          return false
        }
        setSettings((previous) => settle(previous, result))
        setNotice(null)
        return true
      },
      async deleteNote(id) {
        const result = await client.deleteNote(id)
        if (!result.ok) return setNotice(result.failure.message)
        await loadPlan()
      },
      async openPosting(posting) {
        if (!posting.url) return
        openTab(posting.url)
        if (!queueOnOpen(posting.status)) return
        const result = await client.move(posting.id, 'queue')
        if (!result.ok) return setNotice(result.failure.message)
        await loadJobs()
      },
      async recordOutcome(id, outcome, note) {
        const result = await client.recordOutcome(id, outcome, note)
        if (!result.ok) {
          setNotice(result.failure.message)
          return false
        }
        await loadJobs()
        return true
      },
      async reflect() {
        setBusy('reflecting')
        const result = await client.reflect(true)
        setBusy(null)
        if (!result.ok) {
          setNotice(result.failure.message)
          return result.failure.message
        }
        await loadPlan()
        if (!result.value.ran) {
          return result.value.reason || 'the reflector had nothing to read'
        }
        return `${result.value.written} written, ${result.value.retired} retired, from ${result.value.read} outcomes`
      },
      readTrace(id) {
        return client.trace(id)
      },
      say(message) {
        setNotice(message)
      },
    }),
    [client, loadJobs, loadPlan, loadProfile, loadSettings, loadSetup, refresh],
  )

  return {
    setup,
    credentials,
    profile,
    settings,
    plan,
    traces,
    jobs,
    notes,
    stream,
    notice,
    busy,
    running,
    unreachable,
    actions,
  }
}
