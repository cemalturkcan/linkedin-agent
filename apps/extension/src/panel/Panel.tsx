import { useEffect, useMemo, useRef, useState } from 'react'
import { agent } from '@/lib/agent/client'
import type { Posting, Transition } from '@/lib/agent/types'
import { attachmentFor, type PickPreview } from '@/lib/armed'
import { followKeyOf, showsPosting } from '@/lib/linkedin/posting'
import { uploadName } from '@/lib/linkedin/resume'
import { previewPick } from '@/lib/worker'
import { openDesk, openTab } from '@/lib/open'
import { derivePlaceNames, deriveTerms } from '@/lib/presets'
import { useHotkeys, type HotkeyHandler } from '@/lib/useHotkeys'
import { Feed } from '@/panel/components/Feed'
import { FeedKnobs } from '@/panel/components/FeedKnobs'
import { Header, type Tab } from '@/panel/components/Header'
import { Legend } from '@/panel/components/Legend'
import { ManualFeed } from '@/panel/components/ManualFeed'
import { Notices } from '@/panel/components/Notices'
import { Presets } from '@/panel/components/Presets'
import { ResumeStep } from '@/panel/components/ResumeStep'
import { RoundPanel } from '@/panel/components/RoundPanel'
import { useActiveTab } from '@/panel/useActiveTab'
import { LISTS, useAgentState, type ListKey } from '@/panel/useAgentState'
import { useManualFeed } from '@/panel/useManualFeed'

type Selection = Record<ListKey, string | null>

const NO_SELECTION: Selection = { inbox: null, manual: null, queue: null, applied: null }

const EMPTY_TEXT: Record<ListKey, string> = {
  inbox: 'nothing in the inbox. a round brings postings in, screening decides which ones land here.',
  manual: 'nothing waiting to be sent by hand.',
  queue: 'nothing queued. opening a posting from the inbox queues it.',
  applied: 'nothing recorded as applied yet.',
}

function neighbourId(rows: { id: string }[], id: string): string | null {
  const index = rows.findIndex((row) => row.id === id)
  if (index === -1) return rows[0]?.id ?? null
  return rows[index + 1]?.id ?? rows[index - 1]?.id ?? null
}

export function Panel() {
  const state = useAgentState(agent)
  const [tab, setTab] = useState<Tab>('inbox')
  const [selection, setSelection] = useState<Selection>(NO_SELECTION)
  const [feedSelection, setFeedSelection] = useState<string | null>(null)
  const [showRound, setShowRound] = useState(true)
  const [overridden, setOverridden] = useState('')

  const following = useActiveTab()

  const followKey = followKeyOf(following)
  const held = useRef(followKey)
  if (held.current !== followKey) {
    held.current = followKey
    if (overridden !== '') setOverridden('')
  }

  const onPosting = showsPosting(following, overridden)
  const feed = useManualFeed(
    state.handled,
    state.known,
    state.armedPreset,
    state.actions.held,
    tab === 'feed' && !onPosting,
  )
  const openPosting = onPosting
    ? ((state.jobs.value ?? []).find((row) => row.id === following.postingId) ?? null)
    : null
  const terms = useMemo(() => deriveTerms(state.profile.value), [state.profile.value])
  const places = useMemo(() => derivePlaceNames(state.profile.value), [state.profile.value])
  const [preview, setPreview] = useState<PickPreview | null>(null)
  useEffect(() => {
    setPreview(null)
    if (!onPosting || !following.postingId) return
    let stale = false
    void previewPick(following.postingId).then((answer) => {
      if (!stale && answer?.ok && answer.preview) setPreview(answer.preview)
    })
    return () => {
      stale = true
    }
  }, [onPosting, following.postingId, state.armedPreset, state.jobs.value])

  const attachment = useMemo(() => {
    const applied = state.settings.value?.settings.apply ?? null
    const base = attachmentFor(openPosting, state.armedPreset, applied)
    if (!preview?.code || base.source === 'agent' || base.source === 'chosen') return base
    const lang = preview.lang ?? base.lang ?? ''
    return {
      ...base,
      code: preview.code,
      lang: lang || null,
      source: preview.source,
      why: preview.why,
      attachesAs: applied
        ? uploadName(applied.uploadFileName, preview.code, lang, applied.uploadFileNameMode)
        : base.attachesAs,
    }
  }, [openPosting, state.armedPreset, state.settings.value, preview])

  function chooseTab(wanted: Tab) {
    setTab(wanted)
    if (following.screen === 'posting') setOverridden(followKey)
  }

  const list = onPosting ? null : tab === 'feed' ? null : tab
  const rows = list ? state.lists[list] : []
  const selectedId = list ? selection[list] : feedSelection
  const selected = list ? (rows.find((row) => row.id === selectedId) ?? null) : null
  const apply = state.settings.value?.settings.apply ?? null
  const paused = apply?.paused ?? false

  const unreachable = useMemo(() => {
    for (const resource of [state.jobs, state.plan, state.settings]) {
      if (resource.failure?.kind === 'unreachable') return resource.failure.message
    }
    return null
  }, [state.jobs, state.plan, state.settings])

  useEffect(() => {
    if (!list) return
    setSelection((previous) => {
      const current = previous[list]
      if (current && rows.some((row) => row.id === current)) return previous
      const first = rows[0]?.id ?? null
      if (current === first) return previous
      return { ...previous, [list]: first }
    })
  }, [list, rows])

  useEffect(() => {
    if (onPosting || tab !== 'feed') return
    setFeedSelection((previous) => {
      if (previous && feed.rows.some((row) => row.card.id === previous)) return previous
      return feed.rows[0]?.card.id ?? null
    })
  }, [feed.rows, tab])

  function select(id: string | null) {
    if (list) {
      setSelection((previous) => ({ ...previous, [list]: id }))
      return
    }
    setFeedSelection(id)
  }

  function step(delta: number) {
    if (onPosting) return
    const walking = list ? rows : feed.rows.map((row) => row.card)
    if (walking.length === 0) return
    const current = walking.findIndex((row) => row.id === selectedId)
    const from = current === -1 ? (delta > 0 ? -1 : 0) : current
    const next = Math.min(walking.length - 1, Math.max(0, from + delta))
    select(walking[next]?.id ?? null)
  }

  function jump(edge: 'first' | 'last') {
    if (onPosting) return
    const walking = list ? rows : feed.rows.map((row) => row.card)
    if (walking.length === 0) return
    const row = edge === 'first' ? walking[0] : walking[walking.length - 1]
    select(row?.id ?? null)
  }

  async function act(to: Transition) {
    const posting = selected
    if (!posting || !list) return
    const fallback = neighbourId(rows, posting.id)
    setSelection((previous) => ({ ...previous, [list]: fallback }))
    const moved = await state.actions.move(posting, to)
    if (!moved) setSelection((previous) => ({ ...previous, [list]: posting.id }))
  }

  async function open(posting: Posting | null) {
    if (!posting || !list) return
    const fallback = neighbourId(rows, posting.id)
    setSelection((previous) => ({ ...previous, [list]: fallback }))
    await state.actions.open(posting, list)
  }

  async function openFromFeed(id: string | null) {
    const row = feed.rows.find((entry) => entry.card.id === id)
    if (!row) return
    setFeedSelection(neighbourId(feed.rows.map((entry) => entry.card), row.card.id))
    openTab(row.card.url, false)
    await feed.open(row.card)
  }

  const listKeys: Record<string, HotkeyHandler | undefined> = {
    j: () => step(1),
    ArrowDown: () => step(1),
    k: () => step(-1),
    ArrowUp: () => step(-1),
    g: () => jump('first'),
    'shift+g': () => jump('last'),
    Home: () => jump('first'),
    End: () => jump('last'),
    Enter: onPosting ? undefined : () => void (list ? open(selected) : openFromFeed(selectedId)),
    x: list && list !== 'applied' ? () => void act('skip') : undefined,
    a: list === 'manual' || list === 'queue' ? () => void act('applied') : undefined,
  }

  const unconfigured = state.setup.value ? !state.setup.value.configured : false
  const sentToSetup = useRef(false)

  useEffect(() => {
    if (!unconfigured || sentToSetup.current) return
    sentToSetup.current = true
    openDesk('setup')
  }, [unconfigured])

  useHotkeys({
    ...listKeys,
    r: () => void (tab === 'feed' ? feed.reload() : state.actions.refresh()),
    s: () => void state.actions.screen(),
    '.': () => setShowRound((previous) => !previous),
    m: () => openDesk('model'),
    p: () => openDesk('profile'),
    ',': () => openDesk('settings'),
    c: () => openDesk('setup'),
    'shift+p': () => void state.actions.setPaused(!paused),
    '1': () => chooseTab(LISTS[0] as ListKey),
    '2': () => chooseTab(LISTS[1] as ListKey),
    '3': () => chooseTab(LISTS[2] as ListKey),
    '4': () => chooseTab(LISTS[3] as ListKey),
    '5': () => chooseTab('feed'),
  })

  if (unconfigured) {
    return (
      <div className="flex h-full flex-col overflow-hidden">
        <Header
          tab={tab}
          counts={state.counts}
          unseen={feed.unseen}
          stream={state.stream}
          busy={state.busy}
          following={false}
          onTab={chooseTab}
          onDesk={() => openDesk('setup')}
        />
        <main className="flex min-h-0 flex-1 flex-col justify-center px-3">
          <p className="text-row text-ink">nothing is set up yet.</p>
          <p className="mt-1 text-meta text-muted">
            the agent needs the folder holding your cv pdfs before it can plan a round, judge a
            posting or attach anything. setup is open in a tab.
          </p>
          <button
            type="button"
            onClick={() => openDesk('setup')}
            className="label mt-3 self-start border border-hairline px-2 py-1 text-ink hover:bg-hover"
          >
            open setup
          </button>
        </main>
      </div>
    )
  }

  return (
    <div className="flex h-full flex-col overflow-hidden">
      <Header
        tab={tab}
        counts={state.counts}
        unseen={feed.unseen}
        stream={state.stream}
        busy={state.busy}
        following={onPosting}
        onTab={chooseTab}
        onDesk={() => openDesk()}
      />

      <Notices
        reachable={unreachable === null}
        unreachable={unreachable}
        paused={paused}
        stale={state.staleCount}
        notice={state.notice}
        onRetry={state.actions.reconnect}
        onResume={() => void state.actions.setPaused(false)}
        onRescreen={() => void state.actions.rescreen()}
        onDismiss={state.actions.dismissNotice}
      />

      {onPosting ? null : tab === 'feed' ? (
        <FeedKnobs
          knobs={feed.knobs}
          hits={feed.hits}
          searching={feed.searching}
          terms={terms}
          places={places}
          onKnobs={(knobs) => {
            feed.setKnobs(knobs)
            void feed.reload()
          }}
          onSuggest={(name) => void feed.suggest(name)}
          onChoose={(hit, asked) => void feed.choose(hit, asked)}
          onChooseName={(name) => void feed.chooseByName(name)}
          onClearPlace={feed.clearPlace}
        />
      ) : (
        <>
          <Presets
            presets={state.presets}
            derived={state.presetsDerived}
            armed={state.armedPreset}
            live={state.profile.live}
            attachesAs={attachment.attachesAs}
            consequence={attachment.consequence}
            onArm={(preset) => void state.actions.arm(preset)}
            onDisarm={() => void state.actions.disarm()}
          />

          {showRound ? (
            <RoundPanel
              plan={state.plan}
              busy={state.busy}
              phase={state.phase}
              report={state.lastReport}
              onRun={() => void state.actions.runRound()}
            />
          ) : null}
        </>
      )}

      <main className="flex min-h-0 flex-1 flex-col overflow-y-auto">
        {onPosting ? (
          <ResumeStep
            postingId={following.postingId}
            posting={openPosting}
            attachment={attachment}
            attempt={state.attempt}
            presets={state.presets}
            armed={state.armedPreset}
            onArm={(preset) => void state.actions.arm(preset)}
            onDisarm={() => void state.actions.disarm()}
            onList={() => setOverridden(followKey)}
          />
        ) : list ? (
          <Feed
            key={list}
            list={list}
            rows={rows}
            selectedId={selectedId}
            empty={
              unreachable
                ? `${EMPTY_TEXT[list]} the agent is not answering, so this is the last thing it said.`
                : EMPTY_TEXT[list]
            }
            onSelect={(id) => select(id)}
            onOpen={(posting) => void open(posting)}
          />
        ) : (
          <ManualFeed
            rows={feed.rows}
            selectedId={feedSelection}
            loading={feed.loading}
            more={feed.more}
            placed={feed.knobs.locationId !== ''}
            notice={feed.notice}
            onSelect={(id) => select(id)}
            onOpen={(row) => void openFromFeed(row.card.id)}
            onMore={() => void feed.loadMore()}
          />
        )}
      </main>

      <Legend tab={tab} />
    </div>
  )
}
