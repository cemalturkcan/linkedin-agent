import { createElement, type ReactElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { AgentClient, type Failure, type Result } from '../src/lib/agent/client'
import type {
  PlanState,
  Posting,
  ProfileState,
  SettingsDocument,
  SetupState,
  TraceList,
} from '../src/lib/agent/types'
import { attachmentFor, type Armed } from '../src/lib/armed'
import { countLists, groupLists, LISTS, type ListKey } from '../src/lib/lists'
import { NO_KNOBS, rowsFor, type FeedKnobs } from '../src/lib/linkedin/feed'
import { ONE_NAME, PER_VARIANT } from '../src/lib/linkedin/resume'
import type { RoundReport } from '../src/lib/linkedin/round'
import type { Card } from '../src/lib/linkedin/voyager'
import { derivePlaceNames, derivePresets, deriveTerms, isDerived } from '../src/lib/presets'
import { Feed } from '../src/panel/components/Feed'
import { FeedKnobs as FeedKnobsView } from '../src/panel/components/FeedKnobs'
import { Header } from '../src/panel/components/Header'
import { Legend } from '../src/panel/components/Legend'
import { ManualFeed } from '../src/panel/components/ManualFeed'
import { Notices } from '../src/panel/components/Notices'
import { Presets } from '../src/panel/components/Presets'
import { ResumeStep } from '../src/panel/components/ResumeStep'
import { RoundPanel } from '../src/panel/components/RoundPanel'
import { noticeStands, reportStands, type Notice } from '../src/panel/useAgentState'
import { opensWith } from '../src/panel/useManualFeed'
import { PostingsView, filterPostings } from '../src/desk/components/PostingsView'
import { ProfileView } from '../src/desk/components/ProfileView'
import { RoundsView } from '../src/desk/components/RoundsView'
import { SettingsView } from '../src/desk/components/SettingsView'
import { SetupView } from '../src/desk/components/SetupView'
import { TraceView } from '../src/desk/components/TraceView'
import { landingPlace } from '../src/desk/Desk'
import type { Resource } from '../src/desk/useDeskState'

const args = process.argv.slice(2)
const flagAt = args.indexOf('--empty')
const emptyBase = flagAt === -1 ? '' : (args[flagAt + 1] ?? '')
const base = args.find((value, index) => !value.startsWith('--') && index !== flagAt + 1)
  ?? process.env.AGENT_BASE
  ?? 'http://127.0.0.1:8787'

const client = new AgentClient({ base: async () => base })
const emptyClient = emptyBase ? new AgentClient({ base: async () => emptyBase }) : null

const failures: string[] = []
let checks = 0

function line(label: string, body: string): void {
  console.log(`${label.padEnd(34)} ${body}`)
}

function heading(title: string): void {
  console.log(`\n${'='.repeat(78)}\n== ${title}\n${'='.repeat(78)}`)
}

function screen(title: string, markup: string): void {
  console.log(`\n-- ${title} --\n${markup}`)
}

function check(name: string, ok: boolean, detail: string): void {
  checks += 1
  console.log(`${ok ? 'PASS' : 'FAIL'}  ${name}${detail ? ` :: ${detail}` : ''}`)
  if (!ok) failures.push(`${name} :: ${detail}`)
}

function mustNotSay(name: string, markup: string, forbidden: string): void {
  const at = markup.toLowerCase().indexOf(forbidden.toLowerCase())
  check(
    name,
    at === -1,
    at === -1 ? `absent: "${forbidden}"` : `found at ${at}: "${markup.slice(at, at + 110)}"`,
  )
}

function mustSay(name: string, markup: string, wanted: string): void {
  const said = markup.toLowerCase().includes(wanted.toLowerCase())
  check(name, said, said ? `present: "${wanted}"` : `missing: "${wanted}"`)
}

function text(node: ReactElement): string {
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

function live<T>(result: Result<T>): Resource<T> {
  if (result.ok) return { value: result.value, live: true, at: Date.now(), failure: null }
  return { value: null, live: false, at: null, failure: result.failure }
}

const noop = () => {}
const never = async () => false
const nothing = async () => {}
const OUTAGE: Failure = {
  kind: 'unreachable',
  status: 0,
  reason: 'unreachable',
  message: 'the agent is not answering',
}

interface Desk {
  setup: Resource<SetupState>
  credentials: Resource<Awaited<ReturnType<AgentClient['credentials']>> extends Result<infer T> ? T : never>
  profile: Resource<ProfileState>
  settings: Resource<SettingsDocument>
  plan: Resource<PlanState>
  traces: Resource<TraceList>
  jobs: Resource<Posting[]>
}

async function readDesk(source: AgentClient): Promise<Desk> {
  const jobs = await source.jobs('all')
  return {
    setup: live(await source.setup()),
    credentials: live(await source.credentials()),
    profile: live(await source.profile()),
    settings: live(await source.settings()),
    plan: live(await source.planState()),
    traces: live(await source.traces()),
    jobs: jobs.ok
      ? live<Posting[]>({ ok: true, value: jobs.value.jobs ?? [] })
      : live<Posting[]>(jobs as Result<Posting[]>),
  }
}

interface PanelShot {
  header: string
  notices: string
  presets: string
  round: string
  lists: Record<ListKey, string>
  all: string
}

interface PanelOptions {
  tab?: ListKey | 'feed'
  notice?: Notice | null
  report?: RoundReport | null
  armed?: Armed | null
  unreachable?: string | null
}

function panelOf(desk: Desk, options: PanelOptions = {}): PanelShot {
  const rows = desk.jobs.value ?? []
  const lists = groupLists(rows, new Set())
  const counts = countLists(lists)
  const presets = derivePresets(desk.profile.value)
  const apply = desk.settings.value?.settings.apply ?? null
  const attachment = attachmentFor(null, options.armed ?? null, apply)
  const shownNotice = noticeStands(options.notice ?? null, desk.plan)
  const shownReport = reportStands(options.report ?? null, desk.plan)
  const tab = options.tab ?? 'inbox'

  const header = text(
    createElement(Header, {
      tab,
      counts,
      unseen: 0,
      stream: 'live',
      busy: false,
      following: false,
      onTab: noop,
      onDesk: noop,
    }),
  )
  const notices = text(
    createElement(Notices, {
      reachable: !options.unreachable,
      unreachable: options.unreachable ?? null,
      paused: apply?.paused ?? false,
      stale: rows.filter((row) => row.stale).length,
      notice: shownNotice?.text ?? null,
      onRetry: noop,
      onResume: noop,
      onRescreen: noop,
      onDismiss: noop,
    }),
  )
  const presetsShot = text(
    createElement(Presets, {
      presets,
      derived: isDerived(presets),
      armed: options.armed ?? null,
      live: desk.profile.live,
      attachesAs: attachment.attachesAs,
      consequence: attachment.consequence,
      onArm: noop,
      onDisarm: noop,
    }),
  )
  const round = text(
    createElement(RoundPanel, {
      plan: desk.plan,
      busy: false,
      phase: null,
      report: shownReport,
      onRun: noop,
    }),
  )

  const drawn: Record<ListKey, string> = { inbox: '', manual: '', queue: '', applied: '' }
  for (const list of LISTS) {
    drawn[list] = text(
      createElement(Feed, {
        list,
        rows: lists[list],
        selectedId: lists[list][0]?.id ?? null,
        empty: `nothing in ${list}`,
        onSelect: noop,
        onOpen: noop,
      }),
    )
  }

  const legend = text(createElement(Legend, { tab }))
  return {
    header,
    notices,
    presets: presetsShot,
    round,
    lists: drawn,
    all: [header, notices, presetsShot, round, ...LISTS.map((list) => drawn[list]), legend].join(' '),
  }
}

function deskOf(desk: Desk): { id: string; body: string }[] {
  const rows = filterPostings(desk.jobs.value ?? [], 'all')
  return [
    {
      id: 'work',
      body: text(
        createElement(PostingsView, {
          jobs: desk.jobs,
          rows,
          filter: 'all',
          tag: '',
          tags: [],
          cursor: 0,
          openedId: rows[0]?.id ?? null,
          reflection: null,
          busy: null,
          onFilter: noop,
          onTag: noop,
          onCursor: noop,
          onOpen: noop,
          onVisit: noop,
          onRecord: async () => false,
          onReflect: noop,
        }),
      ),
    },
    { id: 'rounds', body: text(createElement(RoundsView, { plan: desk.plan, cursor: 0, onForget: noop })) },
    {
      id: 'settings',
      body: text(
        createElement(SettingsView, {
          settings: desk.settings,
          profile: desk.profile,
          busy: null,
          onSave: never,
        }),
      ),
    },
    {
      id: 'setup',
      body: text(
        createElement(SetupView, {
          setup: desk.setup,
          credentials: desk.credentials,
          profile: desk.profile,
          plugin: desk.plan.value?.plugin ?? null,
          busy: null,
          onFolder: never,
          onPaste: never,
          onClear: nothing,
          onIndex: nothing,
        }),
      ),
    },
    { id: 'profile', body: text(createElement(ProfileView, { profile: desk.profile, busy: null, onIndex: nothing })) },
    {
      id: 'trace',
      body: text(
        createElement(TraceView, {
          traces: desk.traces,
          running: null,
          cursor: 0,
          openedId: null,
          onOpen: noop,
          onCursor: noop,
          read: async () => ({ ok: false, failure: OUTAGE }) as never,
        }),
      ),
    },
  ]
}

const NOT_INDEXED = 'not indexed yet'

function assertIndexHonesty(where: string, desk: Desk, markup: string): void {
  const state = desk.profile.value?.indexState
  const count = desk.profile.value?.resumes.length ?? 0
  if (state !== 'current') {
    line(`${where} index state`, `${state ?? 'unknown'}, ${count} resumes, claim not checked`)
    return
  }
  mustNotSay(
    `${where}: no screen claims the folder is unindexed while /api/profile says current (${count} resumes)`,
    markup,
    NOT_INDEXED,
  )
}

async function stateNothingConfigured(): Promise<void> {
  heading('state 1: nothing configured')
  if (!emptyClient) {
    check('state 1 covered', false, 'pass --empty <url> pointing at an api with an empty store')
    return
  }
  const desk = await readDesk(emptyClient)
  line('GET /api/setup', `configured=${desk.setup.value?.configured} missing=${JSON.stringify(desk.setup.value?.missing)}`)
  line('GET /api/profile', `indexState=${desk.profile.value?.indexState} candidate=${desk.profile.value?.candidate ? 'derived' : 'none'}`)

  const panel = panelOf(desk)
  const views = deskOf(desk)
  screen('panel, nothing configured', panel.all.slice(0, 900))
  screen('desk / setup, nothing configured', views.find((view) => view.id === 'setup')!.body.slice(0, 900))

  const landing = landingPlace(desk.setup)
  check(
    'state 1: the desk lands on setup while the chain is short',
    landing?.view === 'settings' && landing.section === 'setup',
    `landed on ${landing ? `${landing.view}/${landing.section ?? ''}` : 'undecided'}`,
  )
  if (desk.setup.value?.configured) {
    check(
      'state 1: the --empty store is actually empty',
      false,
      'that store already has a cv folder set, so this state cannot be rendered. point --empty at a fresh DATA_DIR, since this script configures it in state 2.',
    )
    return
  }
  check(
    'state 1: setup names what is missing',
    (desk.setup.value?.missing.length ?? 0) > 0,
    JSON.stringify(desk.setup.value?.missing ?? []),
  )
  mustNotSay('state 1: the panel never invents a posting', panel.lists.inbox, 'senior')
}

async function stateIndexing(): Promise<void> {
  heading('state 2: folder set and indexing')
  if (!emptyClient) {
    check('state 2 covered', false, 'pass --empty <url> pointing at an api with an empty store')
    return
  }
  const configured = await client.setup()
  const resumeDir = configured.ok ? configured.value.resumeDir : ''
  if (!resumeDir) {
    check('state 2 covered', false, 'the configured api did not report a resume folder to point the empty one at')
    return
  }
  const chosen = await emptyClient.chooseFolder(resumeDir)
  line('POST /api/setup', chosen.ok ? `indexing=${chosen.value.indexing} dir=${chosen.value.resumeDir}` : chosen.failure.message)

  const desk = await readDesk(emptyClient)
  line('GET /api/profile', `indexState=${desk.profile.value?.indexState} indexing=${desk.profile.value?.indexing}`)
  const views = deskOf(desk)
  const setupView = views.find((view) => view.id === 'setup')!.body
  screen('desk / setup, indexing', setupView.slice(0, 900))
  screen('desk / profile, indexing', views.find((view) => view.id === 'profile')!.body.slice(0, 700))

  check(
    'state 2: indexing is reported as work in progress, not as an error',
    desk.profile.value?.indexing === true || desk.profile.value?.indexState !== 'never',
    `indexing=${desk.profile.value?.indexing} indexState=${desk.profile.value?.indexState}`,
  )
  mustNotSay('state 2: indexing never renders as a failure', setupView, 'failed')
}

async function stateReady(desk: Desk): Promise<void> {
  heading('state 3: indexed and connected, no round yet')
  line('GET /api/profile', `indexState=${desk.profile.value?.indexState} resumes=${desk.profile.value?.resumes.length}`)
  line('GET /api/plan', `blocked=${desk.plan.value?.blocked ? desk.plan.value.blocked.reason : 'none'} executor=${desk.plan.value?.plugin.connected}`)

  const panel = panelOf(desk)
  screen('panel, indexed and connected', panel.all.slice(0, 1100))
  assertIndexHonesty('state 3', desk, panel.all)
  for (const view of deskOf(desk)) {
    assertIndexHonesty(`state 3 desk/${view.id}`, desk, view.body)
  }
  check(
    'state 3: the presets are derived from the indexed cvs',
    isDerived(derivePresets(desk.profile.value)),
    derivePresets(desk.profile.value).map((preset) => preset.id).join(','),
  )
  check(
    'state 3: the executor has checked in',
    desk.plan.value?.plugin.connected === true,
    `connected=${desk.plan.value?.plugin.connected}`,
  )
}

async function stateWorked(desk: Desk): Promise<void> {
  heading('state 4: a finished round, postings in every list, one opened by hand')
  const rows = desk.jobs.value ?? []
  const lists = groupLists(rows, new Set())
  for (const list of LISTS) {
    line(`list ${list}`, `${lists[list].length} rows`)
  }
  const opened = rows.filter((row) => row.status === 'opened')
  line('opened by hand', `${opened.length} rows, stage=${opened[0]?.stage ?? 'none'}`)

  const panel = panelOf(desk)
  for (const list of LISTS) {
    screen(`panel / ${list}`, panel.lists[list].slice(0, 600))
  }
  const views = deskOf(desk)
  screen('desk / work', views.find((view) => view.id === 'work')!.body.slice(0, 1200))
  screen('desk / rounds', views.find((view) => view.id === 'rounds')!.body.slice(0, 1200))

  for (const list of LISTS) {
    const first = lists[list][0]
    if (!first) {
      check(`state 4: ${list} has a posting to draw`, false, 'list is empty in this store')
      continue
    }
    mustSay(`state 4: panel ${list} draws its first row`, panel.lists[list], first.title.slice(0, 24))
  }
  const work = views.find((view) => view.id === 'work')!.body
  if (opened[0]) {
    mustSay('state 4: the desk work list carries the hand-opened posting', work, opened[0].title.slice(0, 24))
  } else {
    check('state 4: a hand-opened posting exists', false, 'no posting with status=opened in this store')
  }
  const round = desk.plan.value?.history[0] ?? desk.plan.value?.cycle ?? null
  if (round) {
    mustSay('state 4: the rounds view names the round that found them', views.find((view) => view.id === 'rounds')!.body, String(round.id))
    for (const entry of round.queries) {
      line(`  query ${entry.label}`, `${entry.query.ring} ${entry.query.place || '-'} "${entry.query.keyword ?? ''}" -> ${entry.inserted}/${entry.received}`)
    }
  }
  assertIndexHonesty('state 4', desk, panel.all + ' ' + views.map((view) => view.body).join(' '))
}

async function stateRefusalClears(): Promise<void> {
  heading('state 5: a refusal that then clears (defect 4)')
  const paused = await client.saveSettings({ apply: { paused: true } })
  line('PUT /api/settings', paused.ok ? 'paused=true' : paused.failure.message)

  const blockedDesk = await readDesk(client)
  const refusal = blockedDesk.plan.value?.blocked
  line('GET /api/plan blocked', refusal ? `${refusal.reason}: ${refusal.message}` : 'none')
  check('state 5: the api refuses while paused', Boolean(refusal), refusal?.reason ?? 'no refusal')

  const notice: Notice = {
    text: refusal?.message ?? 'the agent is paused.',
    reason: refusal?.reason ?? 'paused',
  }
  const report = {
    refused: true,
    reason: notice.reason,
    message: notice.text,
    opened: false,
    closed: false,
    failed: false,
    blocked: notice.text,
    screening: { passes: 0, triaged: 0, screened: 0, picked: 0, manual: 0, skipped: 0, awaiting: 0, stopped: '', error: '' },
    queries: [],
  } as unknown as RoundReport

  const whileBlocked = panelOf(blockedDesk, { notice, report })
  screen('panel while the agent refuses', whileBlocked.notices + ' | ' + whileBlocked.round.slice(0, 500))
  mustSay('state 5: the refusal is on screen while it holds', whileBlocked.notices + whileBlocked.round, notice.text.slice(0, 40))

  const resumed = await client.saveSettings({ apply: { paused: false } })
  line('PUT /api/settings', resumed.ok ? 'paused=false' : resumed.failure.message)
  const clearedDesk = await readDesk(client)
  line('GET /api/plan blocked', clearedDesk.plan.value?.blocked ? clearedDesk.plan.value.blocked.reason : 'none')

  const afterwards = panelOf(clearedDesk, { notice, report })
  screen('the same panel once the refusal cleared', afterwards.notices + ' | ' + afterwards.round.slice(0, 500))
  mustNotSay(
    'state 5: the cleared refusal is gone from the banner',
    afterwards.notices,
    notice.text.slice(0, 40),
  )
  mustNotSay(
    'state 5: the cleared refusal is gone from the round panel',
    afterwards.round,
    notice.text.slice(0, 40),
  )

  const stubborn: Notice = { text: 'the planner returned no usable query', reason: 'planner-failed' }
  const kept = panelOf(clearedDesk, { notice: stubborn })
  mustSay(
    'state 5: a refusal the gate does not own survives a clean plan',
    kept.notices,
    'no usable query',
  )
}

function stateFeedFirstOpen(desk: Desk): void {
  heading('state 6: the feed tab on first open (defect 5)')
  const knobs: FeedKnobs = { ...NO_KNOBS, place: 'Istanbul', locationId: '102105699' }
  const opensNow = opensWith(true, true, false)
  line('opens on first activation', String(opensNow))
  check('state 6: opening the feed asks for a page', opensNow, `opensWith(active, restored, everAsked=false)=${opensNow}`)
  check('state 6: it asks once, not on every render', opensWith(true, true, true) === false, 'second activation does not refetch')

  const loading = text(
    createElement(ManualFeed, {
      rows: [],
      selectedId: null,
      loading: true,
      more: false,
      placed: true,
      notice: null,
      onSelect: noop,
      onOpen: noop,
      onMore: noop,
    }),
  )
  screen('panel / feed, first open', loading)
  mustSay('state 6: first open says it is reading the feed', loading, 'reading the feed')
  mustNotSay('state 6: first open never claims the window is empty', loading, 'nothing in this window')

  const knobsView = text(
    createElement(FeedKnobsView, {
      knobs,
      hits: [],
      searching: false,
      terms: deriveTerms(desk.profile.value),
      places: derivePlaceNames(desk.profile.value),
      onKnobs: noop,
      onSuggest: noop,
      onChoose: noop,
      onChooseName: noop,
      onClearPlace: noop,
    }),
  )
  screen('panel / feed knobs, with the cv-derived chips', knobsView)
  const terms = deriveTerms(desk.profile.value)
  if (terms[0]) {
    mustSay('state 6: the terms offered are the ones the cvs carry', knobsView, terms[0])
  }
  const places = derivePlaceNames(desk.profile.value)
  line('terms from the cvs', terms.join(' | ') || 'none')
  line('places from the cvs', places.join(' | ') || 'none')

  const rows = rowsFor(
    [
      {
        id: 'feed-1',
        title: 'Senior Backend Engineer',
        company: 'northwind',
        location: 'Istanbul',
        url: 'https://www.linkedin.com/jobs/view/feed-1/',
        listedAt: Date.now() - 3_600_000,
        reposted: false,
        easyApply: true,
      } satisfies Card,
    ],
    new Set(),
    new Set(),
  )
  const loaded = text(
    createElement(ManualFeed, {
      rows,
      selectedId: rows[0]?.card.id ?? null,
      loading: false,
      more: false,
      placed: true,
      notice: null,
      onSelect: noop,
      onOpen: noop,
      onMore: noop,
    }),
  )
  screen('panel / feed, a page loaded', loaded)
  mustSay('state 6: an unseen posting is marked unseen', loaded, 'unseen')
}

function stateActiveTab(desk: Desk): void {
  heading('state 7: an active tab holding a judged posting, and one never judged')
  const rows = desk.jobs.value ?? []
  const judged = rows.find((row) => row.resumeCode) ?? null
  const apply = desk.settings.value?.settings.apply ?? null
  line('apply mode', `${apply?.uploadFileNameMode ?? 'unknown'} / ${apply?.uploadFileName ?? ''}`)

  if (!judged) {
    check('state 7: a judged posting exists', false, 'no posting carries a resumeCode in this store')
  } else {
    const attachment = attachmentFor(judged, null, apply)
    const shot = text(
      createElement(ResumeStep, {
        postingId: judged.id,
        posting: judged,
        attachment,
        attempt: null,
        presets: derivePresets(desk.profile.value),
        armed: null,
        onArm: noop,
        onDisarm: noop,
        onList: noop,
      }),
    )
    screen('panel / active tab, a posting the agent judged', shot)
    mustSay('state 7: the judged posting names the cv that attaches', shot, attachment.code ?? '')
    mustNotSay('state 7: a judged posting is never called unjudged', shot, 'never judged this posting')
    line('attaches as', attachment.attachesAs)
    line('consequence', attachment.consequence)
  }

  const armed: Armed = { code: 'BA', label: 'Backend', languages: ['en'], at: Date.now() }
  const unjudged = attachmentFor(null, armed, apply)
  const unjudgedShot = text(
    createElement(ResumeStep, {
      postingId: '4100000000',
      posting: null,
      attachment: unjudged,
      attempt: null,
      presets: derivePresets(desk.profile.value),
      armed,
      onArm: noop,
      onDisarm: noop,
      onList: noop,
    }),
  )
  screen('panel / active tab, a posting the agent never judged', unjudgedShot)
  mustSay('state 7: an unjudged posting says so plainly', unjudgedShot, 'never judged this posting')
  mustSay('state 7: the armed cv stands in for it', unjudgedShot, 'the cv you armed')

  const oneName = attachmentFor(null, armed, apply ? { ...apply, uploadFileNameMode: ONE_NAME, uploadFileName: 'my-cv.pdf' } : null)
  const perVariant = attachmentFor(null, armed, apply ? { ...apply, uploadFileNameMode: PER_VARIANT, uploadFileName: 'my-cv.pdf' } : null)
  line('one-name attaches as', `${oneName.attachesAs} (${oneName.consequence})`)
  line('per-variant attaches as', `${perVariant.attachesAs} (${perVariant.consequence})`)
  check(
    'state 7: one name is the same for every variant, per-variant is not',
    oneName.attachesAs === 'my-cv.pdf' && perVariant.attachesAs !== oneName.attachesAs,
    `${oneName.attachesAs} vs ${perVariant.attachesAs}`,
  )
  mustSay(
    'state 7: the panel states the consequence of the naming mode',
    text(
      createElement(Presets, {
        presets: derivePresets(desk.profile.value),
        derived: true,
        armed,
        live: true,
        attachesAs: oneName.attachesAs,
        consequence: oneName.consequence,
        onArm: noop,
        onDisarm: noop,
      }),
    ),
    'uploads a fresh one',
  )
}

function stateAgentDown(desk: Desk): void {
  heading('state 8: the same panel with the agent down')
  const panel = panelOf(desk, { unreachable: OUTAGE.message })
  screen('panel, agent down', panel.notices + ' | ' + panel.round.slice(0, 400))
  mustSay('state 8: the outage is stated plainly', panel.notices, 'not answering')
  assertIndexHonesty('state 8', desk, panel.all)
}

heading(`rendering both surfaces against ${base}`)
if (emptyBase) line('empty store at', emptyBase)

await stateNothingConfigured()
await stateIndexing()

const desk = await readDesk(client)
await stateReady(desk)
await stateWorked(desk)
await stateRefusalClears()
stateFeedFirstOpen(desk)
stateActiveTab(desk)
stateAgentDown(desk)

heading('result')
console.log(`${checks} checks, ${failures.length} failed`)
for (const failure of failures) console.log(`  FAIL ${failure}`)
if (failures.length > 0) process.exit(1)
console.log('every rendered screen agreed with the api')
