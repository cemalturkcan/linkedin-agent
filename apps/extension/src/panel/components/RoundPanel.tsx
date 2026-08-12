import type { PlanState, Round } from '@/lib/agent/types'
import { canFetch, EXECUTOR_MISSING } from '@/lib/capabilities'
import { relativeAge, spend } from '@/lib/format'
import type { RoundReport, RoundUpdate, ScreenReport } from '@/lib/linkedin/round'
import type { Resource } from '@/panel/useAgentState'

function Line({ term, children }: { term: string; children: string }) {
  return (
    <p className="flex items-baseline gap-2 text-meta">
      <span className="label w-14 shrink-0 text-muted">{term}</span>
      <span className="min-w-0 flex-1 text-ink">{children}</span>
    </p>
  )
}

function Queries({ round }: { round: Round }) {
  if (round.queries.length === 0) {
    return <p className="text-meta text-muted">the plan carries no query the executor can run.</p>
  }
  return (
    <ul className="divide-y divide-hairline border-t border-hairline">
      {round.queries.map((entry) => (
        <li key={entry.label} className="py-1.5">
          <div className="flex items-baseline gap-2">
            <span className="min-w-0 flex-1 truncate text-meta text-ink">{entry.label}</span>
            <span className="label shrink-0 text-muted">
              {entry.query.ring}
              {entry.query.place ? ` ${entry.query.place}` : ''}
            </span>
            <span className="grid-num shrink-0 text-micro text-muted">
              {entry.skipped ? 'skipped' : `${entry.inserted}/${entry.received}`}
            </span>
          </div>
          <p className="mt-0.5 text-meta text-muted">
            {entry.skipped ? entry.skipReason || 'skipped, no reason recorded' : entry.query.reason}
          </p>
        </li>
      ))}
    </ul>
  )
}

export function screeningLine(screening: ScreenReport): string {
  if (screening.error) return screening.error
  const counted =
    `${screening.triaged} triaged, ${screening.screened} judged, ` +
    `${screening.picked} to the inbox, ${screening.manual} by hand, ${screening.skipped} dropped`
  if (screening.stopped) return `${counted}. ${screening.stopped}`
  if (screening.awaiting > 0) {
    return `${counted}. ${screening.awaiting} still waiting on their body text.`
  }
  return `${counted}.`
}

interface RoundPanelProps {
  plan: Resource<PlanState>
  busy: boolean
  phase: RoundUpdate['phase'] | null
  report: RoundReport | null
  onRun(): void
}

export function RoundPanel({ plan, busy, phase, report, onRun }: RoundPanelProps) {
  const state = plan.value
  const round = state?.cycle ?? state?.history[0] ?? null
  const executorReady = canFetch(state?.plugin.capabilities ?? null)
  const running = phase !== null

  return (
    <section className="max-h-64 shrink-0 overflow-y-auto border-b border-hairline px-3 py-2">
      <div className="flex items-baseline justify-between">
        <h2 className="label text-muted">round</h2>
        <div className="flex items-baseline gap-3">
          {!plan.live && plan.value ? <span className="label text-muted">cached</span> : null}
          <button
            type="button"
            onClick={onRun}
            disabled={busy || running || !executorReady}
            className="label text-ink disabled:text-muted"
          >
            {phase ?? (busy ? 'planning' : 'run a round')}
          </button>
        </div>
      </div>

      {state ? (
        <div className="mt-1.5 space-y-1">
          <Line term="executor">
            {state.plugin.connected
              ? `checked in ${relativeAge(state.plugin.lastSeen) || 'now'} ago, version ${state.plugin.version ?? 'unknown'}`
              : state.plugin.everSeen
                ? `quiet since ${relativeAge(state.plugin.lastSeen)} ago`
                : 'no extension has ever checked in'}
          </Line>
          <Line term="budget">{spend(state.budget.used, state.budget.cap, 'today')}</Line>
          {state.roundBudget.cap > 0 ? (
            <Line term="round cap">
              {state.roundBudget.open
                ? `${state.roundBudget.used} of ${state.roundBudget.cap} model calls used this round`
                : `${state.roundBudget.cap} model calls a round`}
            </Line>
          ) : null}
          {state.blocked ? <Line term="blocked">{state.blocked.message}</Line> : null}
          {!executorReady ? <Line term="fetch">{EXECUTOR_MISSING}</Line> : null}
          {report && report.message ? (
            <Line term="here">
              {`${report.message}${report.opened && !report.closed ? ', and the agent has not taken the close yet' : ''}`}
            </Line>
          ) : null}
          {report && report.screening.passes > 0 ? (
            <Line term="screened">{screeningLine(report.screening)}</Line>
          ) : null}
        </div>
      ) : (
        <p className="mt-1.5 text-meta text-muted">
          the round is read from the agent, and it has not answered yet.
        </p>
      )}

      {round ? (
        <div className="mt-2 space-y-1">
          <Line term={round.status === 'open' ? 'running' : 'last'}>
            {round.rationale || 'no rationale recorded'}
          </Line>
          {round.terminationReason ? (
            <Line term="ended">{round.terminationReason}</Line>
          ) : null}
          {round.error ? <Line term="died">{round.error}</Line> : null}
          <div className="pt-1">
            <Queries round={round} />
          </div>
        </div>
      ) : null}
    </section>
  )
}
