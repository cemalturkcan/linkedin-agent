import type { Note as StandingNote, PlanState, Round, RoundQuery } from '@/lib/agent/types'
import { relativeAge, spend } from '@/lib/format'
import { cn } from '@/lib/utils'
import { Note, Remove } from '@/desk/components/Fields'
import type { Resource } from '@/desk/useDeskState'

const WINDOWS: Record<string, string> = {
  r3600: 'the last hour',
  r21600: 'the last 6 hours',
  r86400: 'the last 24 hours',
  r604800: 'the last week',
}

function ago(value: number | string | null): string {
  const age = relativeAge(value)
  if (!age) return ''
  return age === 'now' ? 'just now' : `${age} ago`
}

function dueIn(value: string | null): string {
  if (!value) return ''
  const at = Date.parse(value)
  if (!Number.isFinite(at)) return ''
  const seconds = Math.round((at - Date.now()) / 1000)
  if (seconds <= 0) return 'the next one is due now'
  if (seconds < 90) return `the next one is due in ${seconds} seconds`
  return `the next one is due in ${Math.round(seconds / 60)} minutes`
}

function Line({ term, children }: { term: string; children: string }) {
  if (!children) return null
  return (
    <p className="flex items-baseline gap-2 text-meta">
      <span className="label w-[3.5rem] shrink-0 text-muted">{term}</span>
      <span className="min-w-0 flex-1 text-ink">{children}</span>
    </p>
  )
}

function Query({ entry }: { entry: RoundQuery }) {
  const ran = entry.pages > 0 || entry.received > 0

  return (
    <li className="py-2">
      <div className="flex items-baseline gap-2">
        <span className="min-w-0 flex-1 truncate text-row text-ink">{entry.label || 'query'}</span>
        {entry.widened ? <span className="label shrink-0 text-ink">widened</span> : null}
        {entry.skipped ? <span className="label shrink-0 text-ink">skipped</span> : null}
        <span className="label shrink-0 text-muted">
          {entry.query.ring}
          {entry.query.place ? `, ${entry.query.place}` : ''}
        </span>
      </div>

      <div className="mt-0.5 flex flex-wrap items-baseline gap-x-3 gap-y-0.5">
        <span className="label text-muted">{WINDOWS[entry.query.range] ?? entry.query.range}</span>
        {entry.query.easyApply ? <span className="label text-muted">easy apply</span> : null}
        {entry.query.remoteOnly ? <span className="label text-muted">remote</span> : null}
        {entry.query.want ? <span className="label text-muted">{`wants ${entry.query.want}`}</span> : null}
      </div>

      <div className="mt-1 space-y-0.5">
        <Line term="why">{entry.query.reason}</Line>
        <Line term="skipped">{entry.skipReason ?? ''}</Line>
        <Line term="yield">
          {ran
            ? `${entry.pages} pages, ${entry.received} seen, ${entry.inserted} new, ${entry.duplicates} already known`
            : entry.skipped
              ? 'nothing fetched'
              : 'not run yet'}
        </Line>
      </div>
    </li>
  )
}

function RoundBlock({ round, selected }: { round: Round; selected: boolean }) {
  const running = round.status === 'open' || !round.finishedAt

  return (
    <div className={cn('border-t border-hairline px-2 py-3', selected ? 'bg-select' : '')}>
      <div className="flex flex-wrap items-baseline gap-x-4 gap-y-1">
        <span className="label text-ink">{running ? 'running' : round.status}</span>
        <span className="label text-muted">{`round ${round.id}`}</span>
        <span className="text-meta text-muted">{`planned ${ago(round.startedAt)}`}</span>
        {round.finishedAt ? (
          <span className="text-meta text-muted">{`ended ${ago(round.finishedAt)}`}</span>
        ) : null}
        <span className="grid-num ml-auto text-meta text-muted">
          {`${round.modelCalls} model calls, ${round.received} seen, ${round.inserted} new, ${round.duplicates} known, ${round.wideningSteps} widening`}
        </span>
      </div>

      {round.rationale ? (
        <p className="mt-2 max-w-[42rem] text-body text-ink">{round.rationale}</p>
      ) : null}

      <div className="mt-2 space-y-0.5">
        <Line term="ended">{round.terminationReason ?? ''}</Line>
        <Line term="died">{round.error ?? ''}</Line>
      </div>

      {round.queries.length > 0 ? (
        <ul className="mt-2 divide-y divide-hairline border-t border-hairline">
          {round.queries.map((entry) => (
            <Query key={entry.label} entry={entry} />
          ))}
        </ul>
      ) : (
        <p className="mt-2 text-meta text-muted">this round carries no query, so it searched nothing.</p>
      )}
    </div>
  )
}

function Notes({
  notes,
  known,
  onForget,
}: {
  notes: StandingNote[]
  known: boolean
  onForget(id: number): void
}) {
  if (notes.length === 0) {
    return (
      <p className="mt-2 text-meta text-muted">
        {known
          ? 'nothing learned yet. the reflector writes a note only once recorded outcomes support one.'
          : 'the agent has not answered, so no note is listed. this is silence, not an empty memory.'}
      </p>
    )
  }

  return (
    <ul className="mt-2 divide-y divide-hairline border-y border-hairline">
      {notes.map((note) => (
        <li key={note.id} className="flex items-start gap-3 py-1.5">
          <span className="label w-[4.5rem] shrink-0 pt-px text-muted">{note.scope}</span>
          <div className="min-w-0 flex-1">
            <p className="text-body text-ink">{note.text}</p>
            {note.evidence ? <p className="mt-0.5 text-meta text-muted">{note.evidence}</p> : null}
          </div>
          <span className="grid-num shrink-0 pt-px text-micro text-muted">
            {relativeAge(note.createdAt)}
          </span>
          <Remove label={`forget the ${note.scope} note`} onClick={() => onForget(note.id)} />
        </li>
      ))}
    </ul>
  )
}

interface RoundsViewProps {
  plan: Resource<PlanState>
  cursor: number
  onForget(id: number): void
}

export function RoundsView({ plan, cursor, onForget }: RoundsViewProps) {
  const state = plan.value
  const rounds = state ? [...(state.cycle ? [state.cycle] : []), ...state.history] : []
  const seen = new Set<number>()
  const history = rounds.filter((round) => {
    if (seen.has(round.id)) return false
    seen.add(round.id)
    return true
  })

  return (
    <div className="w-full max-w-[46rem]">
      <p className="max-w-[42rem] text-body text-muted">
        the agent plans each round itself: which place, how wide a ring around it, how far back, and
        why that ring earns a slot. the extension runs the plan newest first, and the round stops at
        the first limit it reaches.
      </p>

      {plan.failure ? <Note term="error">{plan.failure.message}</Note> : null}
      {state?.blocked ? <Note term="blocked">{state.blocked.message}</Note> : null}

      {state ? (
        <div className="mt-4 flex flex-wrap items-baseline gap-x-4 gap-y-1 text-meta text-muted">
          <span className="grid-num">{spend(state.budget.used, state.budget.cap, 'today')}</span>
          {state.roundBudget.cap > 0 ? (
            <span className="grid-num">
              {state.roundBudget.open
                ? `${state.roundBudget.used} of ${state.roundBudget.cap} model calls this round`
                : `${state.roundBudget.cap} model calls a round`}
            </span>
          ) : null}
          <span>
            {state.autoCycle
              ? [`a round every ${state.cycleMinutes} minutes`, dueIn(state.nextRunAt)]
                  .filter(Boolean)
                  .join(', ')
              : 'rounds run when you ask'}
          </span>
          <span className="grid-num">{`${state.knownIdCount} postings already judged`}</span>
          {!plan.live ? <span className="label">cached</span> : null}
        </div>
      ) : null}

      {history.length === 0 ? (
        <p className="mt-6 text-meta text-muted">
          {state
            ? 'no round has been planned yet. the panel runs the first one.'
            : 'the agent has not answered, so no round is listed. this is silence, not an empty history.'}
        </p>
      ) : (
        <section className="mt-4 -mx-2">
          {history.map((round, index) => (
            <RoundBlock key={round.id} round={round} selected={index === cursor} />
          ))}
        </section>
      )}

      <section className="mt-8 border-t border-hairline pt-5">
        <div className="flex items-baseline justify-between">
          <h2 className="label text-ink">standing notes</h2>
          <span className="grid-num text-meta text-muted">
            {state ? state.notes.length : 'unknown'}
          </span>
        </div>
        <p className="mt-1 max-w-[42rem] text-meta text-muted">
          what the agent drew from the outcomes you recorded. they ride along in every prompt until
          you delete one, so delete the ones it got wrong.
        </p>
        <Notes notes={state?.notes ?? []} known={state !== null} onForget={onForget} />
      </section>
    </div>
  )
}
