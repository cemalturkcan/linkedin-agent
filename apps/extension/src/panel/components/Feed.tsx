import { useEffect, useRef, useState } from 'react'
import type { Posting } from '@/lib/agent/types'
import { departed } from '@/lib/exit'
import type { ListKey } from '@/lib/lists'
import { PostingRow } from '@/panel/components/PostingRow'

const EXIT_MS = 220

function reducedMotion(): boolean {
  return typeof matchMedia === 'function' && matchMedia('(prefers-reduced-motion: reduce)').matches
}

function rowId(list: ListKey, id: string): string {
  return `row-${list}-${id}`
}

interface FeedProps {
  list: ListKey
  rows: Posting[]
  selectedId: string | null
  empty: string
  onSelect(id: string): void
  onOpen(posting: Posting): void
}

export function Feed({ list, rows, selectedId, empty, onSelect, onOpen }: FeedProps) {
  const [shown, setShown] = useState<Posting[]>(rows)
  const [leaving, setLeaving] = useState<Set<string>>(() => new Set())
  const shownRef = useRef(shown)
  shownRef.current = shown

  useEffect(() => {
    const gone = departed(shownRef.current, rows)

    if (gone.length === 0) {
      setShown(rows)
      setLeaving((previous) => (previous.size === 0 ? previous : new Set()))
      return
    }

    setLeaving(new Set(gone))
    const timer = setTimeout(
      () => {
        setShown(rows)
        setLeaving(new Set())
      },
      reducedMotion() ? 5 : EXIT_MS,
    )
    return () => clearTimeout(timer)
  }, [rows])

  useEffect(() => {
    if (!selectedId) return
    document.getElementById(rowId(list, selectedId))?.scrollIntoView({ block: 'nearest' })
  }, [list, selectedId])

  if (shown.length === 0) {
    return (
      <div className="flex flex-1 items-start justify-center px-3 py-8">
        <p className="max-w-[18rem] text-center text-meta text-muted">{empty}</p>
      </div>
    )
  }

  return (
    <ul
      role="listbox"
      aria-label={list}
      tabIndex={-1}
      className="flex-1 divide-y divide-hairline overflow-y-auto outline-none"
    >
      {shown.map((posting) => (
        <PostingRow
          key={posting.id}
          posting={posting}
          domId={rowId(list, posting.id)}
          selected={posting.id === selectedId}
          leaving={leaving.has(posting.id)}
          onSelect={() => onSelect(posting.id)}
          onOpen={() => onOpen(posting)}
        />
      ))}
    </ul>
  )
}
