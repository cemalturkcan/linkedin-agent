import { agent } from '@/lib/agent/client'
import { ATTEMPT_KEY, type Attempt } from '@/lib/armed'
import { dueForCheck, toSweep, TAB_GAP_MS } from '@/lib/applied'
import { markOpened } from '@/lib/handled'
import { note } from '@/lib/log'
import { harvestedFrom } from '@/lib/linkedin/feed'
import { resolvePlace } from '@/lib/linkedin/geo'
import { minutesOf, nextWakeAfter, paceFrom } from '@/lib/linkedin/pace'
import { postingIdFrom } from '@/lib/linkedin/posting'
import {
  createRunner,
  type RoundPorts,
  type RoundReport,
  type RoundUpdate,
} from '@/lib/linkedin/round'
import {
  backoffStore,
  lastRound,
  openRoundStore,
  rememberRound,
  seenStore,
} from '@/lib/linkedin/store'
import { Voyager, viewUrl, type Card } from '@/lib/linkedin/voyager'
import { readKey, writeKeys } from '@/lib/storage'
import { feedPage, feedPlaces, rememberChosenPlace, type FeedPorts } from '@/worker/feed'
import {
  attachFor,
  ATTACHED_KEY,
  ATTACHED_MAX,
  previewFor,
  syncApplied,
  type AttachedPort,
  type PagePort,
} from '@/worker/apply'
import { workerHold, WATCH_ALARM } from '@/worker/hold'
import {
  attachResume,
  readForm,
  selectDocument,
  type FormReading,
  type InjectionResult,
} from '@/worker/inject'

function firstResult(
  results: chrome.scripting.InjectionResult<InjectionResult>[],
  fallback: InjectionResult,
): InjectionResult {
  const outcomes = results
    .map((frame) => frame.result)
    .filter((outcome): outcome is InjectionResult => Boolean(outcome))
  return outcomes.find((outcome) => outcome.ok) ?? outcomes[0] ?? fallback
}

const ROUND_ALARM = 'round'
const SESSION_HOST = 'https://www.linkedin.com'
const APPLIED_KEY = 'appliedChecked'
const APPLIED_TTL_MS = 24 * 60 * 60_000

const voyager = new Voyager({
  session: async () => {
    const cookie = await chrome.cookies.get({ url: SESSION_HOST, name: 'JSESSIONID' })
    return cookie?.value ?? null
  },
})

const ports: RoundPorts = {
  agent,
  linkedin: {
    search: (query, start, timeoutMs) => voyager.search(query, start, timeoutMs),
    posting: (id, timeoutMs) => voyager.posting(id, timeoutMs),
    place: (name, timeoutMs) => resolvePlace(name, voyager, { timeoutMs }),
    forgetSession: () => voyager.forgetSession(),
  },
  seen: seenStore(),
  backoff: backoffStore(),
  openRound: openRoundStore(),
  hold: workerHold(),
  sleep: (ms) => new Promise((resolve) => setTimeout(resolve, ms)),
  now: () => Date.now(),
  random: () => Math.random(),
  notify: (update) => void broadcast(update),
}

const runner = createRunner(ports)

const feed: FeedPorts = {
  agent,
  linkedin: ports.linkedin,
  lookup: voyager,
  backoff: ports.backoff,
  timeoutMs: 15_000,
}

const UNSEEN_KEY = 'unseenInFeed'

async function broadcast(update: RoundUpdate): Promise<void> {
  if (update.phase === 'finished') {
    await rememberRound(update.report)
    await badge(update.report)
    note('round', update.report.message || 'the round finished with nothing to say')
  }
  try {
    await chrome.runtime.sendMessage({ type: 'round:update', update })
  } catch {
    return
  }
}

async function paint(text: string, blocked: boolean, title: string): Promise<void> {
  await chrome.action.setBadgeBackgroundColor({ color: blocked ? '#8c2f2f' : '#101010' })
  await chrome.action.setBadgeText({ text })
  await chrome.action.setTitle({ title })
}

async function badge(report: RoundReport): Promise<void> {
  const blocked = report.blocked !== '' || report.failed
  if (blocked) {
    await paint('!', true, report.message || 'linkedin agent')
    return
  }
  await showUnseen(await readKey<number>(UNSEEN_KEY, 0), report.message || 'linkedin agent')
}

async function showUnseen(unseen: number, title: string): Promise<void> {
  const count = Number.isFinite(unseen) && unseen > 0 ? Math.min(Math.trunc(unseen), 99) : 0
  await writeKeys({ [UNSEEN_KEY]: count })
  await paint(count > 0 ? String(count) : '', false, count > 0 ? `${count} unseen postings` : title)
}

async function recordOpened(cards: Card[], code: string, lang: string): Promise<boolean> {
  const ids = cards.map((card) => card.id).filter(Boolean)
  if (ids.length === 0) return false
  await markOpened(ids)
  const held = await agent.recordOpened(cards.map(harvestedFrom), code, lang)
  return held.ok
}

async function reschedule(after: number): Promise<void> {
  await chrome.alarms.create(ROUND_ALARM, {
    delayInMinutes: minutesOf(after),
    periodInMinutes: minutesOf(after),
  })
}

async function cycle(manual: boolean): Promise<RoundReport | null> {
  await sweepApplied()
  const state = await agent.planState()
  if (!state.ok) note('agent', `the agent did not answer: ${state.failure.message}`)
  const next = paceFrom(state.ok ? state.value : null, Date.now())
  if (!manual && !next.run) {
    await reschedule(next.minutes)
    return null
  }
  note('round', manual ? 'a round you asked for is starting' : 'a scheduled round is starting')
  const report = await runner.run()
  await reschedule(nextWakeAfter(report.nextRunAt, report.cycleMinutes, Date.now()))
  return report
}

async function wake(): Promise<void> {
  const orphan = await runner.resolveOrphan()
  if (orphan.found) {
    note('round', orphan.message)
    await chrome.action.setBadgeBackgroundColor({ color: '#8c2f2f' })
    await chrome.action.setBadgeText({ text: '!' })
    await chrome.action.setTitle({ title: orphan.message })
  }
  await cycle(false)
}

const page: PagePort = {
  async read(tabId) {
    const results = await chrome.scripting.executeScript({
      target: { tabId, allFrames: true },
      func: readForm,
    })
    const merged: FormReading = { fileInput: false, options: [] }
    for (const frame of results) {
      const reading = frame.result as FormReading | undefined
      if (!reading) continue
      merged.fileInput = merged.fileInput || reading.fileInput
      merged.options.push(...reading.options)
    }
    return merged
  },
  async select(tabId, name) {
    return firstResult(
      await chrome.scripting.executeScript({
        target: { tabId, allFrames: true },
        func: selectDocument,
        args: [name],
      }),
      { ok: false, reason: 'not-listed', matches: 0 },
    )
  },
  async attach(tabId, base64, name) {
    return firstResult(
      await chrome.scripting.executeScript({
        target: { tabId, allFrames: true },
        func: attachResume,
        args: [base64, name],
      }),
      { ok: false, reason: 'no-input', matches: 0 },
    )
  },
}

const DESCRIBED_MAX = 50
const described = new Map<string, { title: string; body: string } | null>()

async function describe(id: string): Promise<{ title: string; body: string } | null> {
  const held = described.get(id)
  if (held !== undefined) return held
  try {
    const body = await voyager.posting(id)
    const read = { title: body.title, body: body.description }
    described.set(id, read)
    if (described.size > DESCRIBED_MAX) {
      const oldest = described.keys().next().value
      if (oldest !== undefined) described.delete(oldest)
    }
    return read
  } catch {
    return null
  }
}

async function rememberAttempt(
  id: string,
  outcome: { ok: boolean; action?: string; name?: string; why: string },
): Promise<void> {
  const attempt: Attempt = {
    id,
    at: Date.now(),
    ok: outcome.ok,
    why: outcome.ok ? `${outcome.action === 'select' ? 'selected' : 'uploaded'} ${outcome.name}` : outcome.why,
  }
  note('attach', `posting ${id}: ${attempt.why}`)
  await writeKeys({ [ATTEMPT_KEY]: attempt })
}

const attached: AttachedPort = {
  async has(id) {
    const stored = await readKey<string[]>(ATTACHED_KEY, [])
    return Array.isArray(stored) && stored.includes(id)
  },
  async remember(id) {
    const stored = await readKey<string[]>(ATTACHED_KEY, [])
    const kept = (Array.isArray(stored) ? stored : []).filter((known) => known !== id)
    kept.push(id)
    await writeKeys({ [ATTACHED_KEY]: kept.slice(-ATTACHED_MAX) })
  },
}

async function checked(): Promise<Record<string, number>> {
  const stored = await readKey<Record<string, number>>(APPLIED_KEY, {})
  return stored && typeof stored === 'object' ? stored : {}
}

async function rememberCheck(id: string): Promise<void> {
  const stored = await readKey<Record<string, number>>(APPLIED_KEY, {})
  const kept: Record<string, number> = {}
  const now = Date.now()
  for (const [known, at] of Object.entries(stored ?? {})) {
    if (Number(at) > now - APPLIED_TTL_MS) kept[known] = Number(at)
  }
  kept[id] = now
  await writeKeys({ [APPLIED_KEY]: kept })
}

async function applied(id: string): Promise<boolean> {
  try {
    return (await voyager.posting(id)).applied
  } catch {
    return false
  }
}

async function checkApplied(id: string): Promise<boolean> {
  if (!dueForCheck(await checked(), id, TAB_GAP_MS, Date.now())) return false
  await rememberCheck(id)
  const moved = await syncApplied({ agent, applied }, id)
  if (moved) note('applied', `posting ${id} is applied on linkedin, so it moved to applied here`)
  return moved
}

async function sweepApplied(): Promise<void> {
  const listed = await agent.jobs('all')
  if (!listed.ok) return
  const { take, left } = toSweep(listed.value.jobs, await checked(), Date.now())
  let moved = 0
  for (const posting of take) {
    await rememberCheck(posting.id)
    if (await syncApplied({ agent, applied }, posting.id)) moved += 1
  }
  if (left > 0) note('applied', `${left} postings were left for the next sweep`)
  if (moved > 0) {
    note('applied', `${moved} of the ${take.length} postings looked at are applied on linkedin`)
  }
}

chrome.alarms.onAlarm.addListener((alarm) => {
  if (alarm.name === ROUND_ALARM) void cycle(false)
  if (alarm.name === WATCH_ALARM) void wake()
})

async function reinjectOpenTabs(): Promise<void> {
  const scripts = chrome.runtime.getManifest().content_scripts ?? []
  for (const entry of scripts) {
    const files = entry.js ?? []
    if (!files.length) continue
    const tabs = await chrome.tabs.query({ url: entry.matches ?? [] })
    for (const tab of tabs) {
      if (typeof tab.id !== 'number') continue
      try {
        await chrome.scripting.executeScript({ target: { tabId: tab.id }, files })
      } catch {
        continue
      }
    }
  }
}

chrome.runtime.onStartup.addListener(() => {
  void reinjectOpenTabs()
  void wake()
})
chrome.runtime.onInstalled.addListener(() => {
  void reinjectOpenTabs()
  void wake()
})

chrome.runtime.onMessage.addListener((message, sender, respond) => {
  if (!message || typeof message !== 'object') return false

  if (message.type === 'round:run') {
    cycle(true)
      .then((report) => respond({ ok: true, report }))
      .catch((error) => respond({ ok: false, reason: (error as Error).message }))
    return true
  }

  if (message.type === 'round:last') {
    lastRound()
      .then((report) => respond({ ok: true, report, running: runner.running() }))
      .catch(() => respond({ ok: false, report: null, running: runner.running() }))
    return true
  }

  if (message.type === 'feed:places') {
    feedPlaces(feed, String(message.name ?? ''))
      .then(respond)
      .catch((error) => respond({ ok: false, message: (error as Error).message }))
    return true
  }

  if (message.type === 'feed:place') {
    rememberChosenPlace(feed, message.hit, String(message.asked ?? ''))
      .then(() => respond({ ok: true }))
      .catch(() => respond({ ok: false }))
    return true
  }

  if (message.type === 'feed:page') {
    feedPage(feed, message.knobs, Number(message.start) || 0)
      .then(respond)
      .catch((error) => respond({ ok: false, message: (error as Error).message }))
    return true
  }

  if (message.type === 'feed:opened') {
    recordOpened(
      Array.isArray(message.cards) ? message.cards : [],
      String(message.resumeCode ?? ''),
      String(message.resumeLang ?? ''),
    )
      .then((ok) => respond({ ok }))
      .catch(() => respond({ ok: false }))
    return true
  }

  if (message.type === 'feed:unseen') {
    showUnseen(Number(message.unseen) || 0, 'linkedin agent')
      .then(() => respond({ ok: true }))
      .catch(() => respond({ ok: false }))
    return true
  }

  if (message.type === 'posting-opened') {
    const href = String(message.href ?? '')
    const id = postingIdFrom(href)
    if (!id) {
      respond({ ok: false })
      return true
    }
    recordOpened(
      [
        {
          id,
          title: '',
          company: '',
          location: '',
          url: viewUrl(id),
          listedAt: null,
          reposted: false,
          easyApply: false,
        },
      ],
      '',
      '',
    )
      .then((ok) => respond({ ok }))
      .catch(() => respond({ ok: false }))
    return true
  }

  if (message.type === 'apply:step') {
    const tabId = sender.tab?.id
    if (typeof tabId !== 'number') {
      respond({ ok: false, reason: 'no-tab', why: 'this page has no tab to attach into' })
      return true
    }
    attachFor({ agent, page, attached, describe }, String(message.id), tabId, message.rearm === true)
      .then(async (outcome) => {
        await rememberAttempt(String(message.id), outcome)
        respond(outcome)
      })
      .catch(async (error) => {
        const outcome = { ok: false as const, reason: 'failed', why: (error as Error).message }
        await rememberAttempt(String(message.id), outcome)
        respond(outcome)
      })
    return true
  }

  if (message.type === 'apply:pick') {
    previewFor({ agent, page, attached, describe }, String(message.id))
      .then((preview) => respond({ ok: true, preview }))
      .catch(() => respond({ ok: false, preview: null }))
    return true
  }

  if (message.type === 'apply:check') {
    checkApplied(String(message.id))
      .then((moved) => respond({ ok: true, moved }))
      .catch(() => respond({ ok: false, moved: false }))
    return true
  }

  return false
})

void wake()
