import { useEffect, useState } from 'react'
import type { Result } from '@/lib/agent/client'
import type { Trace, TraceList } from '@/lib/agent/types'
import { duration, relativeAge } from '@/lib/format'
import { cn } from '@/lib/utils'
import { Note, Output } from '@/desk/components/Fields'
import type { Resource, RunningCall } from '@/desk/useDeskState'

const SECTIONS: { label: string; of(detail: Trace): string }[] = [
  { label: 'system prompt, exactly as sent', of: (detail) => detail.system ?? '' },
  { label: 'user message, exactly as sent', of: (detail) => detail.user ?? '' },
  { label: 'what came back', of: (detail) => detail.output ?? '' },
]

function useElapsed(startedAt: number): number {
  const [now, setNow] = useState(() => Date.now())

  useEffect(() => {
    setNow(Date.now())
    const timer = setInterval(() => setNow(Date.now()), 500)
    return () => clearInterval(timer)
  }, [startedAt])

  return Math.max(0, now - startedAt)
}

function Running({ call }: { call: RunningCall }) {
  const elapsed = useElapsed(call.startedAt)

  return (
    <div className="mt-4 border border-hairline">
      <div className="flex flex-wrap items-baseline gap-x-4 gap-y-1 border-b border-hairline bg-accent px-2 py-1.5">
        <span className="label text-ink">running</span>
        <span className="text-row text-ink">{call.purpose || 'model call'}</span>
        <span className="grid-num ml-auto text-meta text-ink">{duration(elapsed)}</span>
      </div>
      <div className="px-2 pt-1 pb-2">
        {call.output ? (
          <Output label="arriving now" text={call.output} follow height="max-h-[18rem]" />
        ) : (
          <p className="pt-1 text-meta text-muted">nothing back from the model yet.</p>
        )}
        {call.capped ? (
          <p className="mt-1 text-meta text-muted">
            the start of this call has scrolled out of the buffer. the whole of it is in the record
            once it ends.
          </p>
        ) : null}
      </div>
    </div>
  )
}

function Idle({ known }: { known: boolean }) {
  return (
    <div className="mt-4 flex flex-wrap items-baseline gap-x-4 gap-y-1 border border-hairline px-2 py-1.5">
      <span className="label text-muted">{known ? 'idle' : 'unknown'}</span>
      <span className="min-w-0 flex-1 text-meta text-muted">
        {known
          ? 'no model call is running. the next one streams into this box while it works.'
          : 'the agent is not answering, so a call could be running and this page would not know.'}
      </span>
    </div>
  )
}

function Detail({ id, read }: { id: number; read(id: number): Promise<Result<Trace>> }) {
  const [detail, setDetail] = useState<Trace | null>(null)
  const [failure, setFailure] = useState<string | null>(null)
  const [reading, setReading] = useState(true)

  useEffect(() => {
    let cancelled = false
    setReading(true)
    setDetail(null)
    setFailure(null)
    void read(id).then((result) => {
      if (cancelled) return
      setReading(false)
      if (result.ok) setDetail(result.value)
      else setFailure(result.failure.message)
    })
    return () => {
      cancelled = true
    }
  }, [id, read])

  const filled = detail ? SECTIONS.filter((section) => section.of(detail) !== '') : []

  return (
    <div className="border-t border-hairline bg-raised px-2 pt-1 pb-2">
      {reading ? <p className="label py-1 text-muted">reading</p> : null}
      {failure ? <Note term="error">{failure}</Note> : null}
      {detail && !reading && filled.length === 0 ? (
        <p className="py-1 text-meta text-muted">this call was recorded without its prompt.</p>
      ) : null}
      {detail
        ? filled.map((section) => (
            <Output key={section.label} label={section.label} text={section.of(detail)} />
          ))
        : null}
    </div>
  )
}

interface TraceViewProps {
  traces: Resource<TraceList>
  running: RunningCall | null
  cursor: number
  openedId: number | null
  onOpen(id: number | null): void
  onCursor(index: number): void
  read(id: number): Promise<Result<Trace>>
}

export function TraceView({
  traces,
  running,
  cursor,
  openedId,
  onOpen,
  onCursor,
  read,
}: TraceViewProps) {
  const recent = traces.value?.traces ?? []

  return (
    <div className="w-full max-w-[46rem]">
      <p className="max-w-[42rem] text-body text-muted">
        every model call runs through one place: reading your cvs, planning a round, widening it,
        triage, screening, reviewing and drawing lessons. this is what the agent sent, what came
        back and how long it took.
      </p>

      {traces.failure ? <Note term="error">{traces.failure.message}</Note> : null}

      {running ? <Running call={running} /> : <Idle known={traces.failure === null} />}

      <section className="mt-6">
        <div className="flex items-baseline justify-between">
          <h2 className="label text-ink">recent calls</h2>
          <span className="grid-num text-meta text-muted">
            {recent.length}
            {!traces.live && traces.value ? ' cached' : ''}
          </span>
        </div>
        <p className="mt-1 max-w-[42rem] text-meta text-muted">
          open one to read the exact prompt that was sent and what the model answered with. the
          agent keeps the newest and drops the rest.
        </p>

        {recent.length === 0 ? (
          <p className="mt-2 text-meta text-muted">
            {traces.value
              ? 'nothing recorded yet.'
              : 'the agent has not answered, so no call is listed. this is silence, not an empty list.'}
          </p>
        ) : (
          <ul className="mt-2 -mx-2 divide-y divide-hairline border-y border-hairline">
            {recent.map((call, index) => {
              const open = call.id === openedId
              return (
                <li key={call.id}>
                  <button
                    type="button"
                    onClick={() => {
                      onCursor(index)
                      onOpen(open ? null : call.id)
                    }}
                    aria-expanded={open}
                    className={cn(
                      'flex w-full items-baseline gap-3 px-2 py-2 text-left',
                      index === cursor ? 'bg-select' : 'hover:bg-hover',
                    )}
                  >
                    <span className="label w-[4.5rem] shrink-0 text-ink">{call.purpose}</span>
                    <span className="min-w-0 flex-1 truncate text-meta text-muted">
                      {call.error ?? call.state}
                    </span>
                    <span className="grid-num shrink-0 text-micro text-muted">
                      {`${call.inputTokens} in, ${call.outputTokens} out`}
                    </span>
                    <span className="grid-num w-14 shrink-0 text-right text-micro text-muted">
                      {duration(call.durationMs ?? 0)}
                    </span>
                    <span className="grid-num w-8 shrink-0 text-right text-micro text-muted">
                      {relativeAge(call.startedAt)}
                    </span>
                  </button>
                  {open ? <Detail id={call.id} read={read} /> : null}
                </li>
              )
            })}
          </ul>
        )}
      </section>
    </div>
  )
}
