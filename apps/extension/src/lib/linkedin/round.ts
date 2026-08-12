import type { AgentClient, Failure, Result } from '@/lib/agent/client'
import type { Harvested, Plan, Query } from '@/lib/agent/types'
import { backsOff, endsSession, kindOf, sentenceOf } from '@/lib/linkedin/failure'
import { NO_PLACE, type ResolvedPlace } from '@/lib/linkedin/geo'
import {
  MAX_PAGES_PER_QUERY,
  PAGE_SIZE,
  RANGES,
  RINGS,
  termOf,
  WORLDWIDE,
  type PostingBody,
  type SearchPage,
  type SearchQuery,
} from '@/lib/linkedin/voyager'

export const HARVEST_SHARE = 0.7
export const KNOWN_ID_LIMIT = 4_000

export const Reason = {
  targetReached: 'target-reached',
  queriesExhausted: 'queries-exhausted',
  minedOut: 'mined-out',
  wideningSpent: 'widening-spent',
  deadline: 'deadline',
  rateLimited: 'rate-limited',
  challenge: 'challenge',
  signedOut: 'signed-out',
  noUsableQuery: 'no-usable-query',
  everyPlaceUnknown: 'every-place-unknown',
  backingOff: 'backing-off',
  refused: 'refused',
  unreachable: 'agent-unreachable',
  rejected: 'agent-rejected',
  failed: 'failed',
  reclaimed: 'worker-reclaimed',
} as const

const BLOCKING: string[] = [
  Reason.rateLimited,
  Reason.challenge,
  Reason.signedOut,
  Reason.deadline,
  Reason.unreachable,
  Reason.rejected,
  Reason.noUsableQuery,
]

export interface OpenRound {
  cycleId: number
  startedAt: number
  deadlineAt: number
}

export interface QueryReport {
  label: string
  ring: string
  place: string
  keyword: string
  pages: number
  fetched: number
  collected: number
  inserted: number
  widened: boolean
  skipped: string
  minedOut: boolean
  exhausted: boolean
  error: string
}

export interface ScreenReport {
  passes: number
  triaged: number
  screened: number
  picked: number
  manual: number
  skipped: number
  awaiting: number
  stopped: string
  error: string
}

export interface RoundReport {
  cycleId: number | null
  opened: boolean
  closed: boolean
  failed: boolean
  refused: boolean
  reason: string
  message: string
  blocked: string
  startedAt: number
  endedAt: number
  pages: number
  collected: number
  inserted: number
  read: number
  widenings: number
  nextRunAt: string | null
  cycleMinutes: number
  queries: QueryReport[]
  screening: ScreenReport
}

export interface RoundUpdate {
  phase: 'planning' | 'harvesting' | 'widening' | 'reading' | 'screening' | 'finished'
  report: RoundReport
}

export interface LinkedInPort {
  search(query: SearchQuery, start: number, timeoutMs: number): Promise<SearchPage>
  posting(id: string, timeoutMs: number): Promise<PostingBody>
  place(name: string, timeoutMs: number): Promise<ResolvedPlace>
  forgetSession(): void
}

export interface SeenPort {
  load(): Promise<Set<string>>
  remember(ids: string[]): Promise<void>
  described(): Promise<Set<string>>
  rememberDescribed(ids: string[]): Promise<void>
}

export interface BackoffPort {
  held(): Promise<number>
  escalate(): Promise<number>
  clear(): Promise<void>
}

export interface OpenRoundPort {
  read(): Promise<OpenRound | null>
  write(round: OpenRound): Promise<void>
  clear(): Promise<void>
}

export interface HoldPort {
  start(): void
  stop(): void
}

export interface RoundPorts {
  agent: AgentClient
  linkedin: LinkedInPort
  seen: SeenPort
  backoff: BackoffPort
  openRound: OpenRoundPort
  hold: HoldPort
  sleep(ms: number): Promise<void>
  now(): number
  random(): number
  notify(update: RoundUpdate): void
}

interface Prepared {
  label: string
  ring: string
  place: string
  keyword: string
  range: string
  easyApply: boolean
  remoteOnly: boolean
  want: number
  widened: boolean
}

interface Context {
  ports: RoundPorts
  report: RoundReport
  known: Set<string>
  toRead: Set<string>
  deadline: number
  harvestDeadline: number
  requestDelayMs: number
  requestTimeoutMs: number
  maxPages: number
  target: number
  screenBudget: number
  lastRequest: number
}

export function clockAt(at: number): string {
  return new Date(at).toTimeString().slice(0, 5)
}

const MESSAGES: Record<string, string> = {
  [Reason.targetReached]: 'the round found what it was asked for and stopped',
  [Reason.queriesExhausted]: 'every planned query ran out of pages',
  [Reason.minedOut]: 'every window came back with nothing this round had not already seen',
  [Reason.wideningSpent]: 'the widening steps are spent, so the round stopped',
  [Reason.deadline]: 'the round hit its deadline and stopped where it was',
  [Reason.rateLimited]: 'linkedin rate limited this session, so the round stopped',
  [Reason.challenge]: 'linkedin wants a security check, so the round stopped',
  [Reason.signedOut]: 'the linkedin session is gone, so the round stopped',
  [Reason.noUsableQuery]: 'the plan carried no query this build can run',
  [Reason.everyPlaceUnknown]: 'every planned query named a place linkedin does not know',
  [Reason.unreachable]: 'the agent stopped answering, so the round stopped',
  [Reason.rejected]: 'the agent refused what the round harvested, so it stopped',
  [Reason.reclaimed]: 'the browser reclaimed the worker while the round was running',
}

export function messageFor(reason: string): string {
  return MESSAGES[reason] ?? reason
}

function newReport(startedAt: number): RoundReport {
  return {
    cycleId: null,
    opened: false,
    closed: false,
    failed: false,
    refused: false,
    reason: '',
    message: '',
    blocked: '',
    startedAt,
    endedAt: startedAt,
    pages: 0,
    collected: 0,
    inserted: 0,
    read: 0,
    widenings: 0,
    nextRunAt: null,
    cycleMinutes: 0,
    queries: [],
    screening: {
      passes: 0,
      triaged: 0,
      screened: 0,
      picked: 0,
      manual: 0,
      skipped: 0,
      awaiting: 0,
      stopped: '',
      error: '',
    },
  }
}

function positive(value: unknown, fallback: number, cap: number): number {
  const number = Number(value)
  if (!Number.isFinite(number) || number <= 0) return fallback
  return Math.min(Math.trunc(number), cap)
}

export function prepareQuery(query: Query, maxWant: number, widened: boolean): Prepared | string {
  const label = String(query?.label ?? '').trim()
  if (!label) return 'a planned query arrived without a label'

  const ring = String(query.ring ?? '').toLowerCase()
  const place = String(query.place ?? '').trim()
  if (!(RINGS as readonly string[]).includes(ring)) return `unknown ring '${query.ring}'`
  if (ring === WORLDWIDE && place) return `worldwide carries no place, this one named '${place}'`
  if (ring !== WORLDWIDE && !place) return NO_PLACE
  if (!(RANGES as readonly string[]).includes(String(query.range))) {
    return `unknown time window '${query.range}'`
  }

  return {
    label,
    ring,
    place,
    keyword: termOf(query.keyword),
    range: String(query.range),
    easyApply: query.easyApply === true,
    remoteOnly: query.remoteOnly === true,
    want: positive(query.want, maxWant, maxWant),
    widened,
  }
}

export function ringDistance(ring: string): number {
  const found = (RINGS as readonly string[]).indexOf(String(ring ?? '').toLowerCase())
  return found === -1 ? RINGS.length : found
}

export function nearRingsFirst<T extends { ring: string }>(queries: T[]): T[] {
  return queries
    .map((query, position) => ({ query, position }))
    .sort(
      (left, right) =>
        ringDistance(left.query.ring) - ringDistance(right.query.ring) ||
        left.position - right.position,
    )
    .map((entry) => entry.query)
}

function trackQuery(context: Context, prepared: Prepared): QueryReport {
  const entry: QueryReport = {
    label: prepared.label,
    ring: prepared.ring,
    place: prepared.place,
    keyword: prepared.keyword,
    pages: 0,
    fetched: 0,
    collected: 0,
    inserted: 0,
    widened: prepared.widened,
    skipped: '',
    minedOut: false,
    exhausted: false,
    error: '',
  }
  context.report.queries.push(entry)
  return entry
}

async function pace(context: Context): Promise<void> {
  const gap = context.requestDelayMs + Math.floor(context.ports.random() * context.requestDelayMs)
  const wait = context.lastRequest + gap - context.ports.now()
  if (wait > 0) await context.ports.sleep(wait)
  context.lastRequest = context.ports.now()
}

async function onLinkedInFailure(context: Context, error: unknown): Promise<string> {
  if (kindOf(error) === null) throw error
  const message = sentenceOf(error)
  if (backsOff(error)) {
    const until = await context.ports.backoff.escalate()
    context.report.blocked = `${message}, holding off until ${clockAt(until)}`
    return kindOf(error) === 'challenge' ? Reason.challenge : Reason.rateLimited
  }
  if (endsSession(error)) {
    context.ports.linkedin.forgetSession()
    context.report.blocked = message
    return Reason.signedOut
  }
  return ''
}

async function skipQuery(context: Context, entry: QueryReport, reason: string): Promise<void> {
  entry.skipped = reason
  if (context.report.cycleId === null) return
  await context.ports.agent.skipQuery({
    cycleId: context.report.cycleId,
    queryLabel: entry.label,
    place: entry.place,
    ring: entry.ring,
    reason,
  })
}

async function locate(
  context: Context,
  prepared: Prepared,
  entry: QueryReport,
): Promise<string | null> {
  if (prepared.ring === WORLDWIDE) return ''
  try {
    await pace(context)
    const place = await context.ports.linkedin.place(prepared.place, context.requestTimeoutMs)
    entry.place = place.label || prepared.place
    if (!place.cached) {
      await context.ports.agent.savePlaces([
        {
          name: place.label,
          asked: prepared.place,
          ring: prepared.ring,
          locationId: place.locationId,
          source: 'round',
        },
      ])
    }
    return place.locationId
  } catch (error) {
    if (kindOf(error) !== 'unknown-place') throw error
    await skipQuery(context, entry, sentenceOf(error))
    return null
  }
}

function toHarvested(card: {
  id: string
  title: string
  company: string
  location: string
  url: string
  listedAt: number | null
  easyApply: boolean
}): Harvested {
  return {
    id: card.id,
    title: card.title,
    company: card.company,
    location: card.location,
    url: card.url,
    description: '',
    listedAt: card.listedAt,
    easyApply: card.easyApply,
  }
}

async function runQuery(context: Context, prepared: Prepared): Promise<string> {
  const entry = trackQuery(context, prepared)

  let locationId: string | null
  try {
    locationId = await locate(context, prepared, entry)
  } catch (error) {
    entry.error = sentenceOf(error)
    return await onLinkedInFailure(context, error)
  }
  if (locationId === null) return ''

  const search: SearchQuery = {
    locationId,
    keyword: prepared.keyword,
    range: prepared.range,
    easyApply: prepared.easyApply,
    remoteOnly: prepared.remoteOnly,
  }
  const batch: Harvested[] = []
  const onPage = new Set<string>()
  let start = 0
  let stop = ''

  for (let page = 0; page < context.maxPages; page += 1) {
    if (context.ports.now() >= context.harvestDeadline) {
      stop = Reason.deadline
      break
    }
    if (context.report.collected >= context.target) {
      stop = Reason.targetReached
      break
    }
    if (entry.collected >= prepared.want) break

    let result: SearchPage
    try {
      await pace(context)
      result = await context.ports.linkedin.search(search, start, context.requestTimeoutMs)
    } catch (error) {
      entry.error = sentenceOf(error)
      stop = await onLinkedInFailure(context, error)
      break
    }

    entry.pages += 1
    entry.fetched += result.jobs.length
    context.report.pages += 1

    let fresh = 0
    for (const card of result.jobs) {
      if (onPage.has(card.id)) continue
      onPage.add(card.id)
      if (context.known.has(card.id)) continue
      fresh += 1
      context.known.add(card.id)
      batch.push(toHarvested(card))
      entry.collected += 1
      context.report.collected += 1
    }

    start += PAGE_SIZE
    if (result.jobs.length < PAGE_SIZE || (result.total > 0 && start >= result.total)) {
      entry.exhausted = true
      break
    }
    if (fresh === 0) {
      entry.minedOut = true
      break
    }
  }

  if (batch.length > 0) {
    const ingested = await context.ports.agent.harvest({
      cycleId: context.report.cycleId,
      queryLabel: prepared.label,
      pages: entry.pages,
      jobs: batch,
    })
    await context.ports.seen.remember(batch.map((job) => job.id))
    if (ingested.ok) {
      entry.inserted = ingested.value.inserted
      context.report.inserted += ingested.value.inserted
      if (ingested.value.stop && !stop) stop = ingested.value.reason || Reason.targetReached
    } else {
      entry.error = ingested.failure.message
      context.report.blocked = ingested.failure.message
      stop = ingested.failure.kind === 'unreachable' ? Reason.unreachable : Reason.rejected
    }
  }

  return stop
}

async function widen(context: Context): Promise<string> {
  const cycleId = context.report.cycleId
  if (cycleId === null) return Reason.wideningSpent

  const step = await context.ports.agent.widenRound(cycleId)
  if (!step.ok) {
    return step.failure.kind === 'refused'
      ? step.failure.reason || Reason.wideningSpent
      : Reason.unreachable
  }
  if (!step.value.query) return Reason.wideningSpent

  const widened = step.value.query
  context.report.widenings += 1
  const prepared = prepareQuery(widened, PAGE_SIZE * context.maxPages, true)
  if (typeof prepared === 'string') {
    const entry = trackQuery(context, {
      label: String(widened.label ?? 'widening'),
      ring: String(widened.ring ?? ''),
      place: String(widened.place ?? ''),
      keyword: '',
      range: '',
      easyApply: false,
      remoteOnly: false,
      want: 0,
      widened: true,
    })
    await skipQuery(context, entry, prepared)
    return ''
  }
  context.ports.notify({ phase: 'widening', report: context.report })
  return runQuery(context, prepared)
}

async function readBodies(context: Context): Promise<string> {
  const pending = await context.ports.agent.pendingDescriptions(context.screenBudget)
  if (pending.ok) {
    for (const job of pending.value.jobs ?? []) context.toRead.add(String(job.id))
  }
  const already = await context.ports.seen.described()
  const wanted = [...context.toRead]
    .filter((id) => !already.has(id))
    .slice(0, context.screenBudget)
  if (wanted.length === 0) return ''

  context.ports.notify({ phase: 'reading', report: context.report })
  const bodies: { id: string; description: string; applied: boolean; closed: boolean }[] = []
  let stop = ''

  for (const id of wanted) {
    if (context.ports.now() >= context.deadline) {
      stop = Reason.deadline
      break
    }
    try {
      await pace(context)
      const body = await context.ports.linkedin.posting(id, context.requestTimeoutMs)
      if (body.description || body.applied || body.closed) bodies.push(body)
    } catch (error) {
      const blocking = await onLinkedInFailure(context, error)
      if (blocking) {
        stop = blocking
        break
      }
    }
  }

  if (bodies.length > 0) {
    const stored = await context.ports.agent.storeDescriptions(bodies)
    context.report.read = bodies.length
    if (stored.ok) await context.ports.seen.rememberDescribed(bodies.map((body) => body.id))
  }
  return stop
}

async function screenPass(context: Context): Promise<void> {
  const tally = context.report.screening
  context.ports.notify({ phase: 'screening', report: context.report })

  const judged = await context.ports.agent.screen(context.screenBudget)
  tally.passes += 1
  if (!judged.ok) {
    tally.error = judged.failure.message
    return
  }
  const result = judged.value
  tally.error = result.error ?? ''
  tally.stopped = result.stopped ?? ''
  tally.triaged += result.triaged ?? 0
  tally.screened += result.screened ?? 0
  tally.picked += result.picked ?? 0
  tally.manual += result.manual ?? 0
  tally.skipped += result.skipped ?? 0
  tally.awaiting = result.awaitingDescription ?? 0
}

function refusalOf(report: RoundReport, failure: Failure): void {
  report.refused = failure.kind === 'refused'
  report.reason = failure.kind === 'refused' ? failure.reason || Reason.refused : Reason.unreachable
  report.message = failure.message
  report.blocked = failure.message
}

function deadlineOf(plan: Plan, now: number): number {
  const stated = Date.parse(plan.deadlineAt ?? '')
  const fallback = now + positive(plan.pacing?.roundDeadlineMs, 240_000, 15 * 60_000)
  if (!Number.isFinite(stated)) return fallback
  return Math.min(stated, fallback)
}

async function drive(context: RoundPorts, report: RoundReport): Promise<void> {
  const holding = await context.backoff.held()
  if (holding > 0) {
    report.reason = Reason.backingOff
    report.message = `linkedin asked this session to slow down, so nothing runs until ${clockAt(holding)}`
    report.blocked = report.message
    return
  }

  context.notify({ phase: 'planning', report })
  const planned: Result<Plan> = await context.agent.startRound()
  if (!planned.ok) {
    refusalOf(report, planned.failure)
    return
  }

  const plan = planned.value
  report.cycleId = plan.cycleId
  report.opened = true
  report.nextRunAt = plan.nextRunAt ?? null
  report.cycleMinutes = positive(plan.pacing?.cycleMinutes, 10, 720)

  const now = context.now()
  const deadline = deadlineOf(plan, now)
  await context.openRound.write({ cycleId: plan.cycleId, startedAt: now, deadlineAt: deadline })

  const known = await context.seen.load()
  for (const id of (plan.knownIds ?? []).slice(0, KNOWN_ID_LIMIT)) known.add(String(id))

  const state: Context = {
    ports: context,
    report,
    known,
    toRead: new Set<string>(),
    deadline,
    harvestDeadline: now + Math.round((deadline - now) * HARVEST_SHARE),
    requestDelayMs: positive(plan.pacing?.requestDelayMs, 900, 10_000),
    requestTimeoutMs: positive(plan.pacing?.requestTimeoutMs, 15_000, 60_000),
    maxPages: positive(plan.pacing?.maxPagesPerQuery, MAX_PAGES_PER_QUERY, MAX_PAGES_PER_QUERY),
    target: positive(plan.pacing?.newJobTarget, 25, 200),
    screenBudget: positive(plan.screenBudget, 40, 200),
    lastRequest: 0,
  }

  const maxWant = PAGE_SIZE * state.maxPages
  const prepared: Prepared[] = []
  for (const query of plan.queries ?? []) {
    const outcome = prepareQuery(query, maxWant, false)
    if (typeof outcome === 'string') {
      const entry = trackQuery(state, {
        label: String(query.label ?? 'query'),
        ring: String(query.ring ?? ''),
        place: String(query.place ?? ''),
        keyword: '',
        range: '',
        easyApply: false,
        remoteOnly: false,
        want: 0,
        widened: false,
      })
      await skipQuery(state, entry, outcome)
      continue
    }
    prepared.push(outcome)
  }

  context.notify({ phase: 'harvesting', report })
  let stop = prepared.length > 0 ? '' : Reason.noUsableQuery

  for (const query of nearRingsFirst(prepared)) {
    if (stop) break
    stop = await runQuery(state, query)
  }

  const maxWidening = positive(plan.pacing?.maxWideningSteps, 2, 5)
  while (!stop && report.collected < state.target && report.widenings < maxWidening) {
    stop = await widen(state)
  }

  const agentDown = stop === Reason.unreachable || stop === Reason.rejected
  if (!agentDown) await screenPass(state)

  if (!report.blocked) {
    const reading = await readBodies(state)
    if (report.blocked) stop = reading || stop
    else if (report.pages > 0) await context.backoff.clear()
  }

  if (!agentDown && report.read > 0) await screenPass(state)

  report.reason = settled(stop, report, state.target, maxWidening)
  report.message = report.blocked || messageFor(report.reason)
}

function settled(stop: string, report: RoundReport, target: number, maxWidening: number): string {
  if (BLOCKING.includes(stop)) return stop
  if (report.collected >= target) return Reason.targetReached
  const planned = report.queries.filter((entry) => !entry.widened)
  if (report.collected === 0 && planned.length > 0 && planned.every((entry) => entry.skipped !== '')) {
    return Reason.everyPlaceUnknown
  }
  if (report.queries.some((entry) => entry.minedOut)) return Reason.minedOut
  if (stop) return stop
  if (report.widenings >= maxWidening) return Reason.wideningSpent
  return Reason.queriesExhausted
}

async function settle(ports: RoundPorts, report: RoundReport): Promise<void> {
  report.endedAt = ports.now()
  if (report.cycleId === null) {
    await ports.openRound.clear()
    return
  }
  const closed = await ports.agent.finishRound(
    report.cycleId,
    report.reason || Reason.failed,
    report.failed ? report.message : '',
  )
  report.closed = closed.ok
  if (closed.ok) {
    await ports.openRound.clear()
    return
  }
  report.message = `${report.message} the agent did not take the close, so the next wake resolves it.`
}

export async function executeRound(ports: RoundPorts): Promise<RoundReport> {
  const report = newReport(ports.now())
  ports.hold.start()
  try {
    await drive(ports, report)
  } catch (error) {
    report.failed = true
    report.reason = Reason.failed
    report.message = sentenceOf(error)
  } finally {
    try {
      await settle(ports, report)
    } catch (error) {
      report.failed = true
      report.message = `${report.message} closing the round failed too: ${sentenceOf(error)}`
    }
    ports.hold.stop()
    ports.notify({ phase: 'finished', report })
  }
  return report
}

export interface OrphanOutcome {
  found: boolean
  cycleId: number | null
  closed: boolean
  message: string
}

export async function resolveOrphan(ports: RoundPorts): Promise<OrphanOutcome> {
  const orphan = await ports.openRound.read()
  if (!orphan) return { found: false, cycleId: null, closed: false, message: '' }

  const message = messageFor(Reason.reclaimed)
  const closed = await ports.agent.finishRound(orphan.cycleId, Reason.reclaimed, message)
  if (closed.ok || closed.failure.kind === 'rejected') await ports.openRound.clear()
  return { found: true, cycleId: orphan.cycleId, closed: closed.ok, message }
}

export interface Runner {
  run(): Promise<RoundReport>
  running(): boolean
  resolveOrphan(): Promise<OrphanOutcome>
}

export function createRunner(ports: RoundPorts): Runner {
  let inFlight: Promise<RoundReport> | null = null

  return {
    running: () => inFlight !== null,
    run() {
      if (inFlight) return inFlight
      inFlight = (async () => {
        await resolveOrphan(ports)
        return executeRound(ports)
      })().finally(() => {
        inFlight = null
      })
      return inFlight
    },
    resolveOrphan() {
      if (inFlight) return Promise.resolve({ found: false, cycleId: null, closed: false, message: '' })
      return resolveOrphan(ports)
    },
  }
}
