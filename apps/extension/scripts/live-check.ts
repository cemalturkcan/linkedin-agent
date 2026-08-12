import { AgentClient, type Result } from '../src/lib/agent/client'
import { AgentStream, type StateFrame } from '../src/lib/agent/stream'
import type { Posting } from '../src/lib/agent/types'
import { hello } from '../src/lib/capabilities'
import { relativeAge, resumeTag } from '../src/lib/format'
import { loadHandled, markOpened } from '../src/lib/handled'
import { countLists, groupLists } from '../src/lib/lists'
import { cachePresets, cachedPresets, derivePresets, isDerived } from '../src/lib/presets'
import { FetchEventSource } from './sse'

const base = process.argv[2] ?? process.env.AGENT_BASE ?? 'http://127.0.0.1:8787'
const offlineOnly = process.argv.includes('--down')
const outageAt = process.argv.indexOf('--outage')
const outagePid = outageAt === -1 ? 0 : Number(process.argv[outageAt + 1] ?? 0)
const client = new AgentClient({ base: async () => base })

function line(label: string, body: string): void {
  console.log(`${label.padEnd(26)} ${body}`)
}

function show<T>(label: string, result: Result<T>, summary: (value: T) => string): void {
  if (result.ok) line(label, summary(result.value))
  else line(label, `${result.failure.kind}: ${result.failure.message}`)
}

const wait = (ms: number) => new Promise((resolve) => setTimeout(resolve, ms))

async function readEverything(): Promise<void> {
  console.log(`\n== reads against ${base} ==`)
  show('GET /api/health', await client.health(), (value) => `ok=${value.ok}`)
  show(
    'GET /api/setup',
    await client.setup(),
    (value) =>
      `configured=${value.configured} ready=${value.ready} indexState=${value.indexState} missing=${JSON.stringify(value.missing)}`,
  )
  show(
    'GET /api/profile',
    await client.profile(),
    (value) => `indexState=${value.indexState} resumes=${value.resumes.length} error=${value.error ?? 'none'}`,
  )
  show(
    'GET /api/settings',
    await client.settings(),
    (value) =>
      `paused=${value.settings.apply.paused} cycleMinutes=${value.settings.budget.cycleMinutes} rings=${JSON.stringify(value.enums.placeRings)}`,
  )
  show(
    'GET /api/plan',
    await client.planState(),
    (value) =>
      `everSeen=${value.plugin.everSeen} connected=${value.plugin.connected} blocked=${value.blocked?.reason ?? 'none'} budget=${value.budget.used}/${value.budget.cap}`,
  )
  show('GET /api/jobs?status=all', await client.jobs('all'), (value) => `jobs=${value.jobs.length}`)
  show('GET /api/places', await client.places(), (value) => `places=${value.places.length}`)
  show('GET /api/notes', await client.notes(), (value) => `notes=${value.notes.length}`)
  show('GET /api/traces', await client.traces(), (value) => `traces=${value.traces.length}`)
}

async function announceAndPlan(): Promise<void> {
  console.log('\n== the executor announces itself, then asks for a round ==')
  const id = 'a'.repeat(32)
  const announced = await client.hello(hello(id, '0.1.0'))
  show(
    'POST /api/plugin/hello',
    announced,
    (value) =>
      `everSeen=${value.plugin.everSeen} connected=${value.plugin.connected} version=${value.plugin.version} rings=${JSON.stringify(value.plugin.capabilities?.fetch.rings)} seenSet=${value.plugin.capabilities?.seenSet}`,
  )
  show(
    'GET /api/plan',
    await client.planState(),
    (value) => `blocked=${value.blocked?.reason ?? 'none'}: ${value.blocked?.message ?? ''}`,
  )
  show('POST /api/cycle/start', await client.startRound(), (value) => `cycleId=${value.cycleId}`)
  show('POST /api/jobs/0/queue', await client.move('0', 'queue'), () => 'moved')
}

async function presets(): Promise<void> {
  console.log('\n== resume presets, derived then cached ==')
  const profile = await client.profile()
  const derived = derivePresets(profile.ok ? profile.value : null)
  line('derived presets', derived.map((preset) => `${preset.id}:${preset.label}`).join(', '))
  line('derived from a real index', String(isDerived(derived)))
  await cachePresets(derived)
  const cached = await cachedPresets()
  line('cached presets', cached.map((preset) => preset.id).join(', '))
}

function row(posting: Posting): string {
  const badges = [posting.easyApply ? '' : 'manual', posting.stale ? 'stale' : '']
    .filter(Boolean)
    .join(' ')
  const tag = resumeTag(posting.resumeCode, posting.resumeLang)
  const score = posting.score === null ? '-' : String(posting.score)
  return [
    posting.title.padEnd(28),
    posting.company.padEnd(12),
    badges.padEnd(7),
    tag.padEnd(6),
    score.padStart(3),
    (relativeAge(posting.listedAt) || '-').padStart(4),
  ].join('  ')
}

async function feed(): Promise<void> {
  console.log('\n== the feed, harvested into the real store then shaped by the panel ==')
  const listedAt = Date.now() - 3 * 60 * 60 * 1000
  const harvested = await client.harvest({
    cycleId: null,
    queryLabel: 'manual',
    pages: 1,
    jobs: [
      {
        id: 'live-1',
        title: 'Backend Engineer',
        company: 'northwind',
        location: 'remote',
        url: 'https://www.linkedin.com/jobs/view/live-1/',
        description: '',
        listedAt,
        easyApply: true,
      },
      {
        id: 'live-2',
        title: 'Yazılım Geliştirici',
        company: 'meridian',
        location: 'hybrid',
        url: 'https://www.linkedin.com/jobs/view/live-2/',
        description: '',
        listedAt: listedAt - 20 * 60 * 60 * 1000,
        easyApply: false,
      },
      {
        id: 'live-3',
        title: 'Platform Engineer',
        company: 'harborlight',
        location: 'onsite',
        url: 'https://www.linkedin.com/jobs/view/live-3/',
        description: '',
        listedAt,
        easyApply: true,
      },
    ],
  })
  show(
    'POST /api/jobs',
    harvested,
    (value) =>
      `received=${value.received} inserted=${value.inserted} duplicates=${value.duplicates} roundNew=${value.roundNew} target=${value.newJobTarget}`,
  )

  show('POST /api/jobs/live-1/queue', await client.move('live-1', 'queue'), (value) => `status=${value.job.status}`)
  show(
    'POST /api/jobs/live-2/applied',
    await client.move('live-2', 'applied'),
    (value) => `status=${value.job.status}`,
  )

  const all = await client.jobs('all')
  if (!all.ok) return show('GET /api/jobs?status=all', all, () => '')

  await markOpened(['live-3'])
  const opened = await loadHandled()
  const lists = groupLists(all.value.jobs, opened)
  line('counts the panel shows', JSON.stringify(countLists(lists)))
  for (const [name, rows] of Object.entries(lists)) {
    for (const posting of rows) line(`  ${name}`, row(posting))
  }
  line('handled and hidden', [...opened].join(', '))
  line(
    'statuses in the store',
    all.value.jobs.map((posting) => `${posting.id}=${posting.status}`).join(', '),
  )
}

async function streamCheck(): Promise<void> {
  console.log('\n== the event stream ==')
  const frames: StateFrame[] = []
  const statuses: string[] = []

  const stream = new AgentStream({
    url: (cursor) => client.eventsUrl(cursor),
    open: (url) => {
      line('stream opened', url)
      return new FetchEventSource(url)
    },
    backoff: [200],
  })
  stream.onStatus((status) => statuses.push(status))
  stream.onState((frame) => frames.push(frame))
  stream.start()

  await wait(400)
  await client.saveSettings({ apply: { paused: true } })
  await wait(400)
  await client.saveSettings({ apply: { paused: false } })
  await wait(400)

  line('status seen', statuses.join(' -> '))
  line(
    'state frames',
    frames.map((frame) => `${frame.name}->${frame.route} ${JSON.stringify(frame.data)}`).join(' | '),
  )
  const cursor = stream.cursor()
  line('cursor after frames', cursor)

  stream.stop()
  await wait(100)

  console.log('\n== a dropped connection replays from the last id ==')
  const replayed: StateFrame[] = []
  const resumed = new AgentStream({
    url: async () => client.eventsUrl(cursor),
    open: (url) => {
      line('reconnected with', url)
      return new FetchEventSource(url)
    },
    backoff: [200],
  })
  resumed.onState((frame) => replayed.push(frame))
  resumed.start()
  await wait(400)
  await client.saveSettings({ companies: { excludeAgencies: true } })
  await wait(500)
  line('frames after the cursor', replayed.map((frame) => frame.name).join(', ') || 'none')
  await client.saveSettings({ companies: { excludeAgencies: false } })

  const fromZero: StateFrame[] = []
  const full = new AgentStream({
    url: async () => client.eventsUrl(''),
    open: (url) => new FetchEventSource(url),
    backoff: [200],
  })
  full.onState((frame) => fromZero.push(frame))
  full.start()
  await wait(500)
  line('replay from id 0', `${fromZero.length} frames: ${fromZero.map((f) => f.name).join(', ')}`)

  resumed.stop()
  full.stop()
  await wait(100)
}

async function offline(): Promise<void> {
  console.log(`\n== with the agent down at ${base} ==`)
  show('GET /api/health', await client.health(), (value) => `ok=${value.ok}`)
  show('GET /api/jobs?status=all', await client.jobs('all'), (value) => `jobs=${value.jobs.length}`)
  show('GET /api/plan', await client.planState(), () => 'read')
  show('POST /api/cycle/start', await client.startRound(), () => 'planned')

  const cached = await cachedPresets()
  line('presets from cache', cached.map((preset) => `${preset.id}:${preset.label}`).join(', '))
  line('manual path', 'presets, arming and opening a posting need no agent')

  const frames: string[] = []
  const stream = new AgentStream({
    url: (cursor) => client.eventsUrl(cursor),
    open: (url) => new FetchEventSource(url),
    backoff: [150, 300],
  })
  stream.onStatus((status) => frames.push(status))
  stream.start()
  await wait(900)
  stream.stop()
  line('stream status', frames.join(' -> '))
}

if (offlineOnly) {
  await offline()
} else {
  await readEverything()
  await announceAndPlan()
  await presets()
  await feed()
  await streamCheck()

  if (outagePid > 0) {
    console.log(`\n== stopping the agent, pid ${outagePid} ==`)
    process.kill(outagePid, 'SIGTERM')
    for (let attempt = 0; attempt < 40; attempt += 1) {
      const alive = await client.health()
      if (!alive.ok) break
      await wait(250)
    }
    await offline()
  }
}
