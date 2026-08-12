import { useCallback, useEffect, useMemo, useState } from 'react'
import { agent } from '@/lib/agent/client'
import type { SetupState } from '@/lib/agent/types'
import { openTab } from '@/lib/open'
import { useHotkeys, type HotkeyHandler } from '@/lib/useHotkeys'
import { cn } from '@/lib/utils'
import { Keycap } from '@/panel/components/Keycap'
import {
  ANY_COUNTRY,
  ANY_TAG,
  PostingsView,
  filterPostings,
  countriesIn,
  tagsIn,
  type PostingFilter,
} from '@/desk/components/PostingsView'
import { ProfileView } from '@/desk/components/ProfileView'
import { RoundsView } from '@/desk/components/RoundsView'
import { SettingsView } from '@/desk/components/SettingsView'
import { SetupView } from '@/desk/components/SetupView'
import { LogView } from '@/desk/components/LogView'
import { TraceView } from '@/desk/components/TraceView'
import { outageLine, useDeskState, type Resource } from '@/desk/useDeskState'

const VIEWS = ['work', 'rounds', 'settings'] as const
const SECTIONS = ['settings', 'setup', 'profile', 'model', 'log'] as const

export type ViewId = (typeof VIEWS)[number]
export type SectionId = (typeof SECTIONS)[number]

export interface Place {
  view: ViewId
  section: SectionId
}

const TITLES: Record<ViewId, string> = {
  work: 'work',
  rounds: 'rounds',
  settings: 'settings',
}

const SECTION_TITLES: Record<SectionId, string> = {
  settings: 'preferences',
  setup: 'setup',
  profile: 'profile',
  model: 'trace',
  log: 'log',
}

const SECTION_KEYS: Record<SectionId, string> = {
  settings: ',',
  setup: 'c',
  profile: 'p',
  model: 'm',
  log: 'l',
}

const FIRST_SECTION: SectionId = 'settings'

interface Hint {
  keys: string[]
  label: string
}

const SHARED: Hint[] = [
  { keys: ['1', '2', '3'], label: 'views' },
  { keys: ['r'], label: 'refresh' },
  { keys: ['⇧P'], label: 'pause' },
]

const PER_VIEW: Record<ViewId, Hint[]> = {
  work: [
    { keys: ['j', 'k'], label: 'postings' },
    { keys: ['↵'], label: 'open on linkedin' },
    { keys: ['d'], label: 'details' },
    { keys: ['esc'], label: 'close them' },
  ],
  rounds: [{ keys: ['j', 'k'], label: 'rounds' }],
  settings: [{ keys: [',', 'c', 'p', 'm', 'l'], label: 'sections' }],
}

const PER_SECTION: Record<SectionId, Hint[]> = {
  settings: [{ keys: ['tab'], label: 'fields' }],
  setup: [{ keys: ['i'], label: 'read the cvs' }],
  profile: [{ keys: ['i'], label: 'read the cvs' }],
  model: [
    { keys: ['j', 'k'], label: 'calls' },
    { keys: ['↵', 'd'], label: 'open a call' },
    { keys: ['esc'], label: 'close it' },
  ],
  log: [],
}

export function placeFromHash(hash: string, current: Place | null): Place | null {
  const wanted = hash.replace(/^#/, '')
  if ((VIEWS as readonly string[]).includes(wanted)) {
    return { view: wanted as ViewId, section: current?.section ?? FIRST_SECTION }
  }
  if ((SECTIONS as readonly string[]).includes(wanted)) {
    return { view: 'settings', section: wanted as SectionId }
  }
  return null
}

export function landingPlace(setup: Resource<SetupState>): Place | null {
  if (setup.value) {
    return setup.value.ready
      ? { view: 'work', section: FIRST_SECTION }
      : { view: 'settings', section: 'setup' }
  }
  return setup.failure ? { view: 'settings', section: 'setup' } : null
}

export function Desk() {
  const state = useDeskState(agent)
  const [place, setPlace] = useState<Place | null>(() =>
    placeFromHash(typeof location === 'undefined' ? '' : location.hash, null),
  )
  const [cursor, setCursor] = useState(0)
  const [openedTrace, setOpenedTrace] = useState<number | null>(null)
  const [openedPosting, setOpenedPosting] = useState<string | null>(null)
  const [filter, setFilter] = useState<PostingFilter>('inbox')
  const [tag, setTag] = useState<string>(ANY_TAG)
  const [country, setCountry] = useState<string>(ANY_COUNTRY)
  const [reflection, setReflection] = useState<string | null>(null)

  useEffect(() => {
    function onHash() {
      setPlace((previous) => placeFromHash(location.hash, previous) ?? previous)
    }
    window.addEventListener('hashchange', onHash)
    return () => window.removeEventListener('hashchange', onHash)
  }, [])

  useEffect(() => {
    if (place) return
    const landed = landingPlace(state.setup)
    if (landed) setPlace(landed)
  }, [place, state.setup])

  const go = useCallback((next: Place, hash: string) => {
    setPlace(next)
    setCursor(0)
    if (typeof location !== 'undefined') location.hash = hash
  }, [])

  const view = place?.view ?? null
  const section = place?.section ?? FIRST_SECTION

  const show = useCallback(
    (wanted: ViewId) => {
      go({ view: wanted, section }, wanted)
    },
    [go, section],
  )

  const open = useCallback(
    (wanted: SectionId) => {
      go({ view: 'settings', section: wanted }, wanted)
    },
    [go],
  )

  const labels = useMemo(() => {
    const named: Record<string, string> = {}
    for (const resume of state.profile.value?.resumes ?? []) named[resume.code] = resume.label
    return named
  }, [state.profile.value])

  const inList = useMemo(
    () => filterPostings(state.jobs.value ?? [], filter),
    [filter, state.jobs.value],
  )
  const tags = useMemo(
    () => tagsIn(filterPostings(inList, 'all', ANY_TAG, country), labels),
    [inList, labels, country],
  )
  const countries = useMemo(() => countriesIn(filterPostings(inList, 'all', tag)), [inList, tag])
  const postings = useMemo(
    () => filterPostings(state.jobs.value ?? [], filter, tag, country),
    [filter, tag, country, state.jobs.value],
  )
  const traces = state.traces.value?.traces ?? []
  const rounds = (state.plan.value?.cycle ? 1 : 0) + (state.plan.value?.history.length ?? 0)

  const rows =
    view === 'work'
      ? postings.length
      : view === 'rounds'
        ? rounds
        : view === 'settings' && section === 'model'
          ? traces.length
          : 0

  const paused = state.settings.value?.settings.apply.paused ?? false
  const settingsDraft = state.settings.value?.settings ?? null

  function step(delta: number) {
    if (rows === 0) return
    setCursor((previous) => Math.min(rows - 1, Math.max(0, previous + delta)))
  }

  function showDetail() {
    if (view === 'settings' && section === 'model') {
      const call = traces[cursor]
      if (call) setOpenedTrace((previous) => (previous === call.id ? null : call.id))
      return
    }
    if (view === 'work') {
      const posting = postings[cursor]
      if (posting) setOpenedPosting((previous) => (previous === posting.id ? null : posting.id))
    }
  }

  function openSelected() {
    if (view !== 'work') {
      showDetail()
      return
    }
    const posting = postings[cursor]
    if (posting) void state.actions.openPosting(posting)
  }

  const listKeys: Record<string, HotkeyHandler | undefined> = {
    j: () => step(1),
    ArrowDown: () => step(1),
    k: () => step(-1),
    ArrowUp: () => step(-1),
    g: () => setCursor(0),
    'shift+g': () => setCursor(Math.max(0, rows - 1)),
    Enter: () => openSelected(),
    d: () => showDetail(),
    Escape: () => {
      setOpenedTrace(null)
      setOpenedPosting(null)
    },
  }

  useHotkeys({
    ...listKeys,
    '1': () => show(VIEWS[0]),
    '2': () => show(VIEWS[1]),
    '3': () => show(VIEWS[2]),
    o: () => show('work'),
    '.': () => show('rounds'),
    ',': () => open('settings'),
    c: () => open('setup'),
    p: () => open('profile'),
    m: () => open('model'),
    l: () => open('log'),
    r: () => void state.actions.refresh(),
    i:
      view === 'settings' && (section === 'setup' || section === 'profile')
        ? () => void state.actions.reindex(state.profile.value?.indexState === 'current')
        : undefined,
    'shift+p': settingsDraft
      ? () =>
          void state.actions.saveSettings({
            ...settingsDraft,
            apply: { ...settingsDraft.apply, paused: !paused },
          })
      : undefined,
  })

  const hints = view
    ? view === 'settings'
      ? [...PER_VIEW.settings, ...PER_SECTION[section], ...SHARED]
      : [...PER_VIEW[view], ...SHARED]
    : SHARED

  return (
    <div className="flex h-full flex-col overflow-hidden">
      <header className="shrink-0 border-b border-hairline">
        <div className="mx-auto flex w-full max-w-[62rem] items-center justify-between px-6 pt-3 pb-2">
          <h1 className="label text-ink">agent desk</h1>
          <div className="flex items-center gap-3">
            {state.busy ? <span className="label text-ink">{state.busy}</span> : null}
            <span className={cn('label', state.stream === 'live' ? 'text-muted' : 'text-ink')}>
              {state.stream === 'live' ? 'live' : state.stream === 'connecting' ? 'connecting' : 'no stream'}
            </span>
            <button
              type="button"
              onClick={() => openTab('https://www.linkedin.com/jobs/')}
              className="label text-muted hover:text-ink"
            >
              linkedin
            </button>
          </div>
        </div>
        <nav className="mx-auto flex w-full max-w-[62rem] items-stretch border-t border-hairline px-6 pl-2.5">
          {VIEWS.map((key, index) => (
            <button
              key={key}
              type="button"
              aria-pressed={key === view}
              onClick={() => show(key)}
              className={cn(
                'flex items-baseline gap-2 border-r border-hairline px-3.5 py-2 last:border-r-0',
                key === view ? 'bg-active text-onactive' : 'text-muted hover:bg-hover hover:text-ink',
              )}
            >
              <span className={cn('grid-num text-micro', key === view ? 'text-onactive/70' : 'text-muted')}>{index + 1}</span>
              <span className={cn('label', key === view ? 'text-onactive' : '')}>{TITLES[key]}</span>
            </button>
          ))}
        </nav>
        {view === 'settings' ? (
          <nav className="mx-auto flex w-full max-w-[62rem] items-stretch border-t border-hairline px-6 pl-2.5">
            {SECTIONS.map((key) => (
              <button
                key={key}
                type="button"
                aria-pressed={key === section}
                onClick={() => open(key)}
                className={cn(
                  'flex items-baseline gap-2 border-r border-hairline px-3.5 py-1.5 last:border-r-0',
                  key === section ? 'bg-select text-ink' : 'text-muted hover:bg-hover hover:text-ink',
                )}
              >
                <span className="grid-num text-micro text-muted">{SECTION_KEYS[key]}</span>
                <span className="label">{SECTION_TITLES[key]}</span>
              </button>
            ))}
          </nav>
        ) : null}
      </header>

      {state.unreachable ? (
        <div className="shrink-0 border-b border-hairline bg-accent">
          <div className="mx-auto flex w-full max-w-[62rem] items-start gap-3 px-6 py-2">
            <p className="min-w-0 flex-1 text-meta text-ink">{outageLine(state.unreachable)}</p>
            <button
              type="button"
              onClick={state.actions.reconnect}
              className="label shrink-0 text-muted hover:text-ink"
            >
              retry
            </button>
          </div>
        </div>
      ) : null}

      {state.notice ? (
        <div className="shrink-0 border-b border-hairline bg-accent">
          <div className="mx-auto flex w-full max-w-[62rem] items-start gap-3 px-6 py-2">
            <p className="min-w-0 flex-1 text-meta text-ink">{state.notice}</p>
            <button
              type="button"
              onClick={() => state.actions.say(null)}
              className="label shrink-0 text-muted hover:text-ink"
            >
              dismiss
            </button>
          </div>
        </div>
      ) : null}

      <main className="min-h-0 flex-1 overflow-y-auto">
        <div className="mx-auto w-full max-w-[62rem] px-6 py-6">
          {view === null ? (
            <p className="max-w-[42rem] text-body text-muted">
              waiting on the agent to say whether the setup chain holds. this lands on the work when
              it does, on setup when it does not.
            </p>
          ) : null}

          {view === 'work' ? (
            <PostingsView
              jobs={state.jobs}
              rows={postings}
              filter={filter}
              tag={tag}
              tags={tags}
              country={country}
              countries={countries}
              cursor={cursor}
              openedId={openedPosting}
              onVisit={(posting) => void state.actions.openPosting(posting)}
              reflection={reflection}
              busy={state.busy}
              onFilter={(next) => {
                setFilter(next)
                setTag(ANY_TAG)
                setCountry(ANY_COUNTRY)
                setCursor(0)
              }}
              onTag={(next) => {
                setTag(next)
                setCursor(0)
              }}
              onCountry={(next) => {
                setCountry(next)
                setCursor(0)
              }}
              onCursor={setCursor}
              onOpen={setOpenedPosting}
              onRecord={state.actions.recordOutcome}
              onReflect={() => void state.actions.reflect().then(setReflection)}
            />
          ) : null}

          {view === 'rounds' ? (
            <RoundsView
              plan={state.plan}
              cursor={cursor}
              onForget={(id) => void state.actions.deleteNote(id)}
            />
          ) : null}

          {view === 'settings' && section === 'settings' ? (
            <SettingsView
              settings={state.settings}
              profile={state.profile}
              busy={state.busy}
              onSave={state.actions.saveSettings}
            />
          ) : null}

          {view === 'settings' && section === 'setup' ? (
            <SetupView
              setup={state.setup}
              credentials={state.credentials}
              profile={state.profile}
              plugin={state.plan.value?.plugin ?? null}
              busy={state.busy}
              onFolder={state.actions.chooseFolder}
              onPaste={state.actions.pasteCredentials}
              onClear={state.actions.clearCredentials}
              onIndex={state.actions.reindex}
            />
          ) : null}

          {view === 'settings' && section === 'profile' ? (
            <ProfileView
              profile={state.profile}
              busy={state.busy}
              onIndex={state.actions.reindex}
            />
          ) : null}

          {view === 'settings' && section === 'log' ? <LogView /> : null}

          {view === 'settings' && section === 'model' ? (
            <TraceView
              traces={state.traces}
              running={state.running}
              cursor={cursor}
              openedId={openedTrace}
              onOpen={setOpenedTrace}
              onCursor={setCursor}
              read={state.actions.readTrace}
            />
          ) : null}
        </div>
      </main>

      <footer className="shrink-0 border-t border-hairline">
        <div className="mx-auto flex h-7 w-full max-w-[62rem] items-center gap-3 overflow-x-auto px-6">
          {hints.map((hint) => (
            <span key={hint.label} className="flex shrink-0 items-center gap-1">
              <span className="flex items-center gap-0.5">
                {hint.keys.map((key) => (
                  <Keycap key={key}>{key}</Keycap>
                ))}
              </span>
              <span className="text-micro text-muted">{hint.label}</span>
            </span>
          ))}
        </div>
      </footer>
    </div>
  )
}
