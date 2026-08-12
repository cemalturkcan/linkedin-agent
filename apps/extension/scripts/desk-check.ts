import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { AgentClient, type Failure, type Result } from '../src/lib/agent/client'
import { AgentStream, type DeltaFrame } from '../src/lib/agent/stream'
import type { Posting, SetupState, Trace, TraceList } from '../src/lib/agent/types'
import { groupLists, LISTS, postingAge } from '../src/lib/lists'
import { landingPlace } from '../src/desk/Desk'
import { PostingsView, filterPostings } from '../src/desk/components/PostingsView'
import { ProfileView } from '../src/desk/components/ProfileView'
import { RoundsView } from '../src/desk/components/RoundsView'
import { SettingsView } from '../src/desk/components/SettingsView'
import { SetupView } from '../src/desk/components/SetupView'
import { TraceView } from '../src/desk/components/TraceView'
import { outageLine, pending, type Resource } from '../src/desk/useDeskState'
import { FetchEventSource } from './sse'

const base = process.argv[2] ?? process.env.AGENT_BASE ?? 'http://127.0.0.1:8787'
const offlineOnly = process.argv.includes('--down')
const client = new AgentClient({ base: async () => base })

const wait = (ms: number) => new Promise((resolve) => setTimeout(resolve, ms))

function line(label: string, body: string): void {
  console.log(`${label.padEnd(28)} ${body}`)
}

function show<T>(label: string, result: Result<T>, summary: (value: T) => string): void {
  if (result.ok) line(label, summary(result.value))
  else line(label, `${result.failure.kind}: ${result.failure.message}`)
}

function live<T>(result: Result<T>): Resource<T> {
  if (result.ok) return { value: result.value, live: true, at: Date.now(), failure: null }
  return { value: null, live: false, at: null, failure: result.failure }
}

const OUTAGE: Failure = {
  kind: 'unreachable',
  status: 0,
  reason: 'unreachable',
  message: `the agent is not answering on ${base}, start it and the desk picks up where it left off`,
}

function stale<T>(resource: Resource<T>): Resource<T> {
  return { value: resource.value, live: false, at: resource.at, failure: OUTAGE }
}

function text(node: Parameters<typeof renderToStaticMarkup>[0]): string {
  return renderToStaticMarkup(node)
    .replace(/<[^>]+>/g, ' ')
    .replace(/&#x27;/g, "'")
    .replace(/&quot;/g, '"')
    .replace(/&amp;/g, '&')
    .replace(/&gt;/g, '>')
    .replace(/&lt;/g, '<')
    .replace(/\s+/g, ' ')
    .trim()
}

function noop() {}
const never = async () => false
const nothing = async () => {}

function views(
  setup: Resource<Awaited<ReturnType<AgentClient['setup']>> extends Result<infer T> ? T : never>,
  credentials: Resource<Awaited<ReturnType<AgentClient['credentials']>> extends Result<infer T> ? T : never>,
  profile: Resource<Awaited<ReturnType<AgentClient['profile']>> extends Result<infer T> ? T : never>,
  settings: Resource<Awaited<ReturnType<AgentClient['settings']>> extends Result<infer T> ? T : never>,
  plan: Resource<Awaited<ReturnType<AgentClient['planState']>> extends Result<infer T> ? T : never>,
  traces: Resource<TraceList>,
  jobs: Resource<Posting[]>,
): { id: string; body: string }[] {
  const rows = filterPostings(jobs.value ?? [], 'all')
  return [
    {
      id: 'work',
      body: text(
        createElement(PostingsView, {
          jobs,
          rows,
          filter: 'all',
          tag: '',
          tags: [],
          country: '',
          countries: [],
          cursor: 0,
          openedId: rows[0]?.id ?? null,
          reflection: null,
          busy: null,
          onFilter: noop,
          onTag: noop,
          onCountry: noop,
          onCursor: noop,
          onOpen: noop,
          onVisit: noop,
          onRecord: async () => false,
          onReflect: noop,
        }),
      ),
    },
    {
      id: 'rounds',
      body: text(createElement(RoundsView, { plan, cursor: 0, onForget: noop })),
    },
    {
      id: 'settings / preferences',
      body: text(createElement(SettingsView, { settings, profile, busy: null, onSave: never })),
    },
    {
      id: 'settings / setup',
      body: text(
        createElement(SetupView, {
          setup,
          credentials,
          profile,
          plugin: plan.value?.plugin ?? null,
          busy: null,
          onFolder: never,
          onPaste: never,
          onClear: nothing,
          onIndex: nothing,
        }),
      ),
    },
    {
      id: 'settings / profile',
      body: text(createElement(ProfileView, { profile, busy: null, onIndex: nothing })),
    },
    {
      id: 'settings / trace',
      body: text(
        createElement(TraceView, {
          traces,
          running: null,
          cursor: 0,
          openedId: null,
          onOpen: noop,
          onCursor: noop,
          read: async () => ({ ok: false, failure: OUTAGE }) as Result<Trace>,
        }),
      ),
    },
  ]
}

function when(posting: Posting): string {
  if (posting.listedAt !== null) return new Date(posting.listedAt).toISOString()
  return posting.createdAt || 'no time at all'
}

function ordering(jobs: Resource<Posting[]>, setup: Resource<SetupState>) {
  console.log('\n== the landing screen, and the order the work is drawn in ==')

  const place = (found: ReturnType<typeof landingPlace>) =>
    found
      ? found.view === 'settings'
        ? `${found.view} / ${found.section}`
        : found.view
      : 'undecided, the setup route has not answered'

  const short: Resource<SetupState> = setup.value
    ? {
        ...setup,
        value: { ...setup.value, ready: false, missing: ['a first read of those cvs'] },
      }
    : setup

  line('chain holds', setup.value ? String(setup.value.ready) : 'unknown, setup did not answer')
  line('lands on', place(landingPlace(setup)))
  line('a short chain lands on', place(landingPlace(short)))
  line('an unanswered setup lands on', place(landingPlace(pending<SetupState>())))

  const rows = filterPostings(jobs.value ?? [], 'all')
  console.log(`\n-- the desk work list, ${rows.length} rows, top first --`)
  for (const [index, posting] of rows.entries()) {
    console.log(
      `${String(index + 1).padStart(3)} ${when(posting)}  ${(posting.status + '        ').slice(0, 9)} ${String(posting.score ?? '-').padStart(3)}  ${posting.title.slice(0, 46)}`,
    )
  }
  const ages = rows.map((posting) => postingAge(posting))
  const rising = ages.every((age, index) => index === 0 || age >= (ages[index - 1] ?? 0))
  line('oldest first', String(rising))

  const grouped = groupLists(jobs.value ?? [], new Set())
  console.log('\n-- the panel feed, per list, top first --')
  for (const list of LISTS) {
    const owned = grouped[list]
    console.log(
      `${list.padEnd(8)} ${owned.map((posting) => `${when(posting)} ${posting.id}`).join('\n         ') || 'empty'}`,
    )
    const times = owned.map((posting) => postingAge(posting))
    line(`  ${list} oldest first`, String(times.every((age, index) => index === 0 || age >= (times[index - 1] ?? 0))))
  }

  const deskFirst = rows.find((posting) => posting.status === 'inbox')?.id ?? 'none'
  const panelFirst = grouped.inbox[0]?.id ?? 'none'
  line('desk and panel agree', `${deskFirst} === ${panelFirst}: ${deskFirst === panelFirst}`)
}

async function readEveryRoute() {
  console.log(`\n== every route a desk view reads, against ${base} ==`)
  const setup = await client.setup()
  const credentials = await client.credentials()
  const profile = await client.profile()
  const settings = await client.settings()
  const plan = await client.planState()
  const traces = await client.traces()
  const jobs = await client.jobs('all')
  const places = await client.places()
  const preview = await client.promptPreview()

  show('GET /api/setup', setup, (value) => `configured=${value.configured} ready=${value.ready} missing=${JSON.stringify(value.missing)}`)
  show('GET /api/credentials', credentials, (value) => `loaded=${value.loaded} source=${value.source} stored=${value.stored} token=${'value' in (value as object) ? 'LEAKED' : 'absent'}`)
  show('GET /api/profile', profile, (value) => `indexState=${value.indexState} indexing=${value.indexing} resumes=${value.resumes.length} candidate=${value.candidate ? 'derived' : 'none'}`)
  show('GET /api/settings', settings, (value) => `groups=${Object.keys(value.settings).join(',')} enums=${Object.keys(value.enums).join(',')}`)
  show('GET /api/plan', plan, (value) => `rounds=${value.history.length} notes=${value.notes.length} budget=${value.budget.used}/${value.budget.cap}`)
  show('GET /api/traces', traces, (value) => `running=${value.running ? value.running.purpose : 'none'} recent=${value.traces.length}`)
  show('GET /api/jobs?status=all', jobs, (value) => `jobs=${value.jobs.length}`)
  show('GET /api/places', places, (value) => `places=${value.places.length}`)
  show('GET /api/prompt/preview', preview, (value) => `ready=${value.ready} prompts=${value.prompts.map((prompt) => prompt.id).join(',')}`)

  return {
    setup: live(setup),
    credentials: live(credentials),
    profile: live(profile),
    settings: live(settings),
    plan: live(plan),
    traces: live(traces),
    jobs: jobs.ok
      ? live<Posting[]>({ ok: true, value: jobs.value.jobs ?? [] })
      : live<Posting[]>(jobs as Result<Posting[]>),
    preview,
  }
}

async function promptPreview() {
  console.log('\n== the prompt preview is the honest assembled text ==')
  const marker = `operator note written by the desk check at ${new Date().toISOString()}`
  const saved = await client.settings()
  if (!saved.ok) return line('settings', saved.failure.message)
  const before = saved.value.settings.operatorNotes

  await client.saveSettings({ operatorNotes: marker })
  const preview = await client.promptPreview()
  if (!preview.ok) {
    line('GET /api/prompt/preview', preview.failure.message)
    return
  }
  for (const prompt of preview.value.prompts) {
    const carries = prompt.system.includes(marker)
    line(
      `prompt ${prompt.id}`,
      `${prompt.system.length} chars, carries the operator note: ${carries}${prompt.unavailable ? `, unavailable: ${prompt.unavailable}` : ''}`,
    )
  }
  const deep = preview.value.prompts.find((prompt) => prompt.id === 'deep')
  if (deep) {
    const at = deep.system.indexOf(marker)
    console.log('\n-- the operator block, exactly as the model sees it --')
    console.log(deep.system.slice(Math.max(0, at - 420), at + marker.length + 40))
  }
  await client.saveSettings({ operatorNotes: before })
}

const DESCRIPTION = [
  'We are hiring a senior backend engineer for a distributed platform.',
  'You will own service design, the data model, the deployment and the on call rotation.',
  'The work is remote inside our own hiring reach, and the team writes and reviews in English.',
  'We ask for real production ownership rather than a list of technologies.',
].join('\n')

async function traceStream() {
  console.log('\n== the trace view against a real model call ==')
  const deltas: { at: number; chars: number }[] = []
  const states: { at: number; state: string; purpose: string }[] = []

  const stream = new AgentStream({
    url: (cursor) => client.eventsUrl(cursor),
    open: (url) => new FetchEventSource(url),
    backoff: [200],
  })
  stream.onDelta((frame: DeltaFrame) => {
    if (frame.name === 'trace-delta') {
      deltas.push({ at: Date.now(), chars: String(frame.data.text ?? '').length })
      return
    }
    states.push({
      at: Date.now(),
      state: String(frame.data.state ?? ''),
      purpose: String(frame.data.purpose ?? ''),
    })
  })
  stream.start()
  await wait(500)

  const listedAt = Date.now() - 2 * 60 * 60 * 1000
  const token = new Date().toISOString().replace(/[^0-9a-z]/gi, '').toLowerCase()
  await client.harvest({
    cycleId: null,
    queryLabel: 'desk-check',
    pages: 1,
    jobs: [
      {
        id: `desk-check-${token}`,
        title: 'Senior Backend Engineer',
        company: `northwind ${token}`,
        location: 'remote',
        url: `https://www.linkedin.com/jobs/view/desk-check-${token}/`,
        description: '',
        listedAt,
        easyApply: true,
      },
    ],
  })

  const triaged = await client.screen(1)
  show('POST /api/jobs/screen', triaged, (value) => `triaged=${value.triaged} kept=${value.kept} awaitingDescription=${value.awaitingDescription}`)

  const wanted = await client.pendingDescriptions(5)
  if (wanted.ok && wanted.value.jobs.length > 0) {
    await client.storeDescriptions(
      wanted.value.jobs.map((job) => ({ id: job.id, description: DESCRIPTION })),
    )
    line('POST /api/jobs/descriptions', `${wanted.value.jobs.length} bodies stored`)
  }

  deltas.length = 0
  const startedAt = Date.now()
  const screened = await client.screen(1)
  const settledAt = Date.now()
  show('POST /api/jobs/screen (deep)', screened, (value) => `screened=${value.screened} picked=${value.picked} manual=${value.manual} skipped=${value.skipped}`)

  await wait(600)
  stream.stop()

  const during = deltas.filter((delta) => delta.at >= startedAt && delta.at <= settledAt)
  line('call window', `${settledAt - startedAt} ms`)
  line('trace state frames', states.map((entry) => `${entry.purpose}:${entry.state}`).join(' -> ') || 'none')
  line('trace-delta frames', String(deltas.length))
  line('deltas during the call', `${during.length} of ${deltas.length}, ${during.reduce((sum, delta) => sum + delta.chars, 0)} chars`)
  if (during.length > 0) {
    const first = during[0] as { at: number }
    const last = during[during.length - 1] as { at: number }
    line('first delta', `${first.at - startedAt} ms after the call opened`)
    line('last delta', `${settledAt - last.at} ms before it settled`)
  }

  const recent = await client.traces()
  if (!recent.ok) return line('GET /api/traces', recent.failure.message)
  const newest = recent.value.traces[0]
  if (!newest) return line('GET /api/traces', 'no call recorded')
  const detail = await client.trace(newest.id)
  show(
    `GET /api/traces/${newest.id}`,
    detail,
    (value) =>
      `purpose=${value.purpose} state=${value.state} duration=${value.durationMs}ms system=${(value.system ?? '').length} user=${(value.user ?? '').length} output=${(value.output ?? '').length}`,
  )
}

function renderViews(label: string, bundle: Awaited<ReturnType<typeof readEveryRoute>>, down: boolean) {
  console.log(`\n== ${label} ==`)
  if (down) console.log(`banner: ${outageLine(OUTAGE.message)}`)
  const shaped = down
    ? views(
        stale(bundle.setup),
        stale(bundle.credentials),
        stale(bundle.profile),
        stale(bundle.settings),
        stale(bundle.plan),
        stale(bundle.traces),
        stale(bundle.jobs),
      )
    : views(
        bundle.setup,
        bundle.credentials,
        bundle.profile,
        bundle.settings,
        bundle.plan,
        bundle.traces,
        bundle.jobs,
      )
  for (const view of shaped) {
    console.log(`\n-- ${view.id} --`)
    console.log(view.body.slice(0, 1400))
    if (down) {
      console.log(`   [cached marker present: ${view.body.includes('cached')}]`)
    }
  }
}

function emptyBundle(): Awaited<ReturnType<typeof readEveryRoute>> {
  return {
    setup: pending(),
    credentials: pending(),
    profile: pending(),
    settings: pending(),
    plan: pending(),
    traces: pending(),
    jobs: pending(),
    preview: { ok: false, failure: OUTAGE },
  } as unknown as Awaited<ReturnType<typeof readEveryRoute>>
}

if (offlineOnly) {
  console.log(`\n== every route with the agent down at ${base} ==`)
  show('GET /api/setup', await client.setup(), () => 'read')
  show('GET /api/credentials', await client.credentials(), () => 'read')
  show('GET /api/profile', await client.profile(), () => 'read')
  show('GET /api/settings', await client.settings(), () => 'read')
  show('GET /api/plan', await client.planState(), () => 'read')
  show('GET /api/traces', await client.traces(), () => 'read')
  show('GET /api/jobs?status=all', await client.jobs('all'), () => 'read')
  show('GET /api/prompt/preview', await client.promptPreview(), () => 'read')
  renderViews('the desk with nothing ever fetched and the agent down', emptyBundle(), true)
} else {
  const bundle = await readEveryRoute()
  await promptPreview()
  await traceStream()
  const after = await readEveryRoute()
  ordering(after.jobs, after.setup)
  renderViews('the desk with the agent answering', after, false)
  renderViews('the same desk with the agent down, drawn from cache', after, true)
  void bundle
}
