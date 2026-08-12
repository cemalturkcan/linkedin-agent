import { useCallback, useEffect, useState } from 'react'
import { relativeAge } from '@/lib/format'
import { clearLog, logging, readLog, setLogging, type LogEntry } from '@/lib/log'
import { Action, Note } from '@/desk/components/Fields'

export function LogView() {
  const [rows, setRows] = useState<LogEntry[]>([])
  const [on, setOn] = useState(true)

  const load = useCallback(async () => {
    const [entries, live] = await Promise.all([readLog(), logging()])
    setRows([...entries].reverse())
    setOn(live)
  }, [])

  useEffect(() => {
    void load()
    const timer = setInterval(() => void load(), 2000)
    return () => clearInterval(timer)
  }, [load])

  return (
    <div className="w-full max-w-[46rem]">
      <p className="max-w-[42rem] text-body text-muted">
        what the extension did, newest first: the rounds it ran, the cvs it attached or refused to
        attach, and the times the agent did not answer. it is kept in the browser, never sent
        anywhere, and it holds the last few hundred lines.
      </p>

      <div className="mt-4 flex flex-wrap items-center gap-3">
        <Action onClick={() => void setLogging(!on).then(load)}>
          {on ? 'stop recording' : 'start recording'}
        </Action>
        <Action onClick={() => void clearLog().then(load)}>clear</Action>
        <span className="grid-num text-meta text-muted">{`${rows.length} lines`}</span>
      </div>

      {!on ? <Note term="off">nothing is being recorded while this is off.</Note> : null}

      {rows.length === 0 ? (
        <p className="mt-6 text-meta text-muted">
          nothing recorded yet. a round, an attach or an outage writes the first line.
        </p>
      ) : (
        <ul className="mt-4 -mx-2 divide-y divide-hairline border-y border-hairline">
          {rows.map((entry, index) => (
            <li
              key={`${entry.at}-${index}`}
              className="flex items-baseline gap-3 px-2 py-1.5 hover:bg-hover"
            >
              <span className="label w-14 shrink-0 text-muted">{entry.area}</span>
              <span className="min-w-0 flex-1 text-meta text-ink">{entry.text}</span>
              <span className="grid-num shrink-0 text-micro text-muted">
                {relativeAge(entry.at)}
              </span>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}
