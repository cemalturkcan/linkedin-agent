import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { AgentClient, type Failure, type Result } from '@/lib/agent/client'
import { afterStatus, AgentStream, NEVER_LIVE, type StreamStatus } from '@/lib/agent/stream'
import type {
  PlanState,
  Posting,
  ProfileState,
  SettingsDocument,
  SetupState,
  Transition,
} from '@/lib/agent/types'
import {
  armPreset,
  ATTEMPT_KEY,
  disarm as forgetArmed,
  loadArmed,
  loadAttempt,
  type Armed,
  type Attempt,
} from '@/lib/armed'
import {
  cacheHandled,
  handledIn,
  HANDLED_KEY,
  knownIn,
  loadHandled,
  loadOpened,
  markOpened,
} from '@/lib/handled'
import { countLists, groupLists, queueOnOpen, type ListKey } from '@/lib/lists'
import { openTab } from '@/lib/open'
import { watchKey } from '@/lib/storage'
import {
  BROAD_PRESET,
  cachePresets,
  cachedPresets,
  derivePresets,
  isDerived,
  presetById,
  type Preset,
} from '@/lib/presets'
import { lastRoundSeen, NO_WORKER, runRoundNow, watchRound, announceNow } from '@/lib/worker'
import type { RoundReport, RoundUpdate } from '@/lib/linkedin/round'

export { LISTS, type ListKey } from '@/lib/lists'

export interface Resource<T> {
  value: T | null
  live: boolean
  at: number | null
  failure: Failure | null
}

function pending<T>(): Resource<T> {
  return { value: null, live: false, at: null, failure: null }
}

function settle<T>(previous: Resource<T>, result: Result<T>): Resource<T> {
  if (result.ok) return { value: result.value, live: true, at: Date.now(), failure: null }
  return { value: previous.value, live: false, at: previous.at, failure: result.failure }
}

export const GATE_REASONS = [
  'paused',
  'executor-offline',
  'no-profile',
  'daily-cap',
  'round-cap',
]

export function clearsWithTheGate(reason: string): boolean {
  return GATE_REASONS.includes(reason)
}

export interface Notice {
  text: string
  reason: string
}

function gateHasCleared(plan: Resource<PlanState>): boolean {
  return plan.live && plan.value !== null && plan.value.blocked === null
}

export function noticeStands(notice: Notice | null, plan: Resource<PlanState>): Notice | null {
  if (!notice || !clearsWithTheGate(notice.reason)) return notice
  return gateHasCleared(plan) ? null : notice
}

export function reportStands(
  report: RoundReport | null,
  plan: Resource<PlanState>,
): RoundReport | null {
  if (!report?.refused || !clearsWithTheGate(report.reason)) return report
  return gateHasCleared(plan) ? null : report
}

export interface PanelState {
  jobs: Resource<Posting[]>
  plan: Resource<PlanState>
  profile: Resource<ProfileState>
  settings: Resource<SettingsDocument>
  setup: Resource<SetupState>
  stream: StreamStatus
  notice: string | null
  busy: boolean
  presets: Preset[]
  presetsDerived: boolean
  armedPreset: Armed | null
  attempt: Attempt | null
  opened: Set<string>
  handled: Set<string>
  known: Set<string>
  lists: Record<ListKey, Posting[]>
  counts: Record<ListKey, number>
  staleCount: number
  reachable: boolean
  phase: RoundUpdate['phase'] | null
  lastReport: RoundReport | null
}

export interface PanelActions {
  refresh(): Promise<void>
  screen(): Promise<void>
  rescreen(): Promise<void>
  move(posting: Posting, to: Transition): Promise<boolean>
  record(posting: Posting, outcome: string): Promise<void>
  runRound(): Promise<void>
  setPaused(paused: boolean): Promise<void>
  arm(preset: Preset): Promise<void>
  disarm(): Promise<void>
  open(posting: Posting, from: ListKey): Promise<void>
  held(id: string): Promise<void>
  dismissNotice(): void
  reconnect(): void
}

export function useAgentState(client: AgentClient): PanelState & { actions: PanelActions } {
  const [jobs, setJobs] = useState<Resource<Posting[]>>(pending<Posting[]>())
  const [plan, setPlan] = useState<Resource<PlanState>>(pending<PlanState>())
  const [profile, setProfile] = useState<Resource<ProfileState>>(pending<ProfileState>())
  const [settings, setSettings] = useState<Resource<SettingsDocument>>(pending<SettingsDocument>())
  const [setup, setSetup] = useState<Resource<SetupState>>(pending<SetupState>())
  const [stream, setStream] = useState<StreamStatus>('connecting')
  const [notice, setNotice] = useState<Notice | null>(null)
  const [busy, setBusy] = useState(false)
  const [presets, setPresets] = useState<Preset[]>([BROAD_PRESET])
  const [armedPreset, setArmedPreset] = useState<Armed | null>(null)
  const [attempt, setAttempt] = useState<Attempt | null>(null)
  const [opened, setOpened] = useState<Set<string>>(new Set())
  const [cached, setCached] = useState<Set<string>>(new Set())
  const [phase, setPhase] = useState<RoundUpdate['phase'] | null>(null)
  const [lastReport, setLastReport] = useState<RoundReport | null>(null)

  const streamRef = useRef<AgentStream | null>(null)

  const loadJobs = useCallback(async () => {
    const result = await client.jobs('all')
    setJobs((previous) =>
      settle(previous, result.ok ? { ok: true, value: result.value.jobs ?? [] } : result),
    )
    if (!result.ok) return
    setCached(await cacheHandled(handledIn(result.value.jobs ?? [])))
  }, [client])

  const loadPlan = useCallback(async () => {
    const result = await client.planState()
    setPlan((previous) => settle(previous, result))
    if (!result.ok || result.value.blocked !== null) return
    setNotice((previous) => (previous && clearsWithTheGate(previous.reason) ? null : previous))
    setLastReport((previous) =>
      previous?.refused && clearsWithTheGate(previous.reason) ? null : previous,
    )
  }, [client])

  const loadProfile = useCallback(async () => {
    const result = await client.profile()
    setProfile((previous) => settle(previous, result))
    if (!result.ok) return
    const derived = derivePresets(result.value)
    if (isDerived(derived)) {
      setPresets(derived)
      await cachePresets(derived)
    }
  }, [client])

  const loadSettings = useCallback(async () => {
    const result = await client.settings()
    setSettings((previous) => settle(previous, result))
  }, [client])

  const loadSetup = useCallback(async () => {
    const result = await client.setup()
    setSetup((previous) => settle(previous, result))
  }, [client])

  const refresh = useCallback(async () => {
    await Promise.all([loadJobs(), loadPlan(), loadProfile(), loadSettings(), loadSetup()])
  }, [loadJobs, loadPlan, loadProfile, loadSettings, loadSetup])

  useEffect(() => {
    let cancelled = false
    void (async () => {
      const [presetCache, armed, held, ownOpens, lastTry] = await Promise.all([
        cachedPresets(),
        loadArmed(),
        loadHandled(),
        loadOpened(),
        loadAttempt(),
      ])
      if (cancelled) return
      setPresets(presetCache)
      setArmedPreset(armed)
      setAttempt(lastTry)
      setCached(held)
      setOpened(ownOpens)
      void announceNow()
      await refresh()
    })()
    return () => {
      cancelled = true
    }
  }, [refresh])

  useEffect(() => {
    let cancelled = false
    void (async () => {
      const seen = await lastRoundSeen()
      if (cancelled || !seen) return
      setLastReport(seen.report)
      setPhase(seen.running ? 'harvesting' : null)
    })()
    return () => {
      cancelled = true
    }
  }, [])

  useEffect(
    () =>
      watchRound((update) => {
        setLastReport(update.report)
        setPhase(update.phase === 'finished' ? null : update.phase)
        if (update.phase !== 'finished') return
        if (update.report.blocked || update.report.failed) {
          setNotice({ text: update.report.message, reason: update.report.reason })
        }
        void loadPlan()
        void loadJobs()
      }),
    [loadJobs, loadPlan],
  )

  useEffect(
    () =>
      watchKey(HANDLED_KEY, (value) => {
        if (!Array.isArray(value)) return
        setCached(new Set(value.filter((id): id is string => typeof id === 'string')))
      }),
    [],
  )

  useEffect(
    () =>
      watchKey(ATTEMPT_KEY, (value) => {
        setAttempt((value as Attempt | undefined) ?? null)
      }),
    [],
  )

  useEffect(() => {
    const source = new AgentStream({ url: (cursor) => client.eventsUrl(cursor) })
    streamRef.current = source

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
        setNotice({
          text: typeof text === 'string' && text ? text : 'the agent reported a failure',
          reason: '',
        })
        return
      }
      switch (frame.route) {
        case '/api/jobs':
          void loadJobs()
          return
        case '/api/plan':
          void loadPlan()
          return
        case '/api/plugin':
          void loadPlan()
          return
        case '/api/profile':
          void loadProfile()
          return
        case '/api/settings':
          void loadSettings()
          return
        case '/api/setup':
          void loadSetup()
          return
        default:
          return
      }
    })

    source.start()
    return () => {
      offStatus()
      offState()
      source.stop()
      streamRef.current = null
    }
  }, [client, loadJobs, loadPlan, loadProfile, loadSettings, loadSetup])

  const rows = jobs.value ?? []

  const lists = useMemo(() => groupLists(rows, opened), [rows, opened])
  const counts = useMemo(() => countLists(lists), [lists])
  const handled = useMemo(() => {
    const merged = new Set(cached)
    for (const id of handledIn(rows)) merged.add(id)
    for (const id of opened) merged.add(id)
    return merged
  }, [cached, opened, rows])
  const known = useMemo(() => {
    const merged = knownIn(rows)
    for (const id of cached) merged.add(id)
    return merged
  }, [cached, rows])

  const staleCount = useMemo(() => rows.filter((row) => row.stale).length, [rows])
  const shownNotice = useMemo(() => noticeStands(notice, plan), [notice, plan])
  const shownReport = useMemo(() => reportStands(lastReport, plan), [lastReport, plan])

  const report = useCallback((failure: Failure) => {
    setNotice({ text: failure.message, reason: failure.reason })
  }, [])

  const actions = useMemo<PanelActions>(
    () => ({
      refresh,
      async screen() {
        setBusy(true)
        const result = await client.screen()
        setBusy(false)
        if (!result.ok) return report(result.failure)
        setNotice(null)
        await loadJobs()
      },
      async rescreen() {
        const result = await client.rescreen()
        if (!result.ok) return report(result.failure)
        await loadJobs()
      },
      async move(posting, to) {
        const result = await client.move(posting.id, to)
        if (!result.ok) {
          report(result.failure)
          return false
        }
        setOpened(await markOpened([posting.id]))
        await loadJobs()
        return true
      },
      async record(posting, outcome) {
        const result = await client.recordOutcome(posting.id, outcome)
        if (!result.ok) return report(result.failure)
        await loadJobs()
      },
      async runRound() {
        setBusy(true)
        setNotice(null)
        const answer = await runRoundNow()
        setBusy(false)
        setPhase(null)
        if (!answer) {
          setNotice({ text: NO_WORKER, reason: '' })
          return
        }
        if (answer.report && (answer.report.blocked || answer.report.failed)) {
          setNotice({ text: answer.report.message, reason: answer.report.reason })
        }
        await Promise.all([loadPlan(), loadJobs()])
      },
      async setPaused(paused) {
        const result = await client.saveSettings({ apply: { paused } })
        if (!result.ok) return report(result.failure)
        setSettings((previous) => settle(previous, result))
      },
      async arm(preset) {
        setArmedPreset(await armPreset(preset))
      },
      async disarm() {
        await forgetArmed()
        setArmedPreset(null)
      },
      async open(posting, from) {
        openTab(posting.url, false)
        setOpened(await markOpened([posting.id]))
        if (queueOnOpen(from)) {
          const result = await client.move(posting.id, 'queue')
          if (!result.ok) return report(result.failure)
          await loadJobs()
        }
      },
      async held(id) {
        setOpened(await markOpened([id]))
        await loadJobs()
      },
      dismissNotice() {
        setNotice(null)
      },
      reconnect() {
        streamRef.current?.retryNow()
        void refresh()
      },
    }),
    [client, loadJobs, loadPlan, refresh, report],
  )

  return {
    jobs,
    plan,
    profile,
    settings,
    setup,
    stream,
    notice: shownNotice?.text ?? null,
    busy,
    presets,
    presetsDerived: isDerived(presets),
    armedPreset,
    attempt,
    opened,
    handled,
    known,
    lists,
    counts,
    staleCount,
    reachable: jobs.live || plan.live || settings.live,
    phase,
    lastReport: shownReport,
    actions,
  }
}

export function presetFor(presets: Preset[], id: string | null): Preset | null {
  return presetById(presets, id)
}
