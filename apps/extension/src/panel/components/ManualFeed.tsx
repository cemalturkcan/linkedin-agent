import { useEffect } from 'react'
import type { FeedRow } from '@/lib/linkedin/feed'
import { relativeAge } from '@/lib/format'
import { cn } from '@/lib/utils'

interface ManualFeedProps {
  rows: FeedRow[]
  selectedId: string | null
  loading: boolean
  more: boolean
  placed: boolean
  notice: string | null
  onSelect(id: string): void
  onOpen(row: FeedRow): void
  onMore(): void
}

function rowId(id: string): string {
  return `feed-${id}`
}

function empty(placed: boolean, loading: boolean): string {
  if (loading) return 'reading the feed'
  if (!placed) return 'name a place and linkedin answers with its own newest first, nothing else sent'
  return 'nothing in this window that the agent or you have not already handled. widen the window, or wait.'
}

export function ManualFeed({
  rows,
  selectedId,
  loading,
  more,
  placed,
  notice,
  onSelect,
  onOpen,
  onMore,
}: ManualFeedProps) {
  useEffect(() => {
    if (!selectedId) return
    document.getElementById(rowId(selectedId))?.scrollIntoView({ block: 'nearest' })
  }, [selectedId])

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      {notice ? (
        <p className="shrink-0 border-b border-hairline px-3 py-1.5 text-meta text-ink">{notice}</p>
      ) : null}

      {rows.length === 0 ? (
        <div className="flex flex-1 items-start justify-center px-3 py-8">
          <p className="max-w-[18rem] text-center text-meta text-muted">{empty(placed, loading)}</p>
        </div>
      ) : (
        <ul
          role="listbox"
          aria-label="linkedin feed"
          tabIndex={-1}
          className="flex-1 divide-y divide-hairline overflow-y-auto outline-none"
        >
          {rows.map(({ card, unseen }) => (
            <li
              key={card.id}
              id={rowId(card.id)}
              role="option"
              aria-selected={card.id === selectedId}
              onMouseDown={() => onSelect(card.id)}
              onDoubleClick={() => onOpen({ card, unseen })}
              className={cn(
                'select-none px-3 py-1.5',
                card.id === selectedId ? 'bg-select text-ink' : 'hover:bg-hover',
              )}
            >
              <div className="flex items-baseline gap-2">
                <span
                  className={cn(
                    'min-w-0 flex-1 truncate text-row',
                    card.id === selectedId ? 'font-semibold' : '',
                  )}
                >
                  {card.title || 'untitled posting'}
                </span>
                {unseen ? <span className="label shrink-0 text-ink">unseen</span> : null}
              </div>
              <div className="mt-0.5 flex items-baseline gap-1.5 text-meta text-muted">
                <span className="min-w-0 max-w-[10rem] shrink truncate text-ink">
                  {card.company}
                </span>
                {card.location ? (
                  <span className="min-w-0 max-w-[8rem] shrink truncate">{card.location}</span>
                ) : null}
                {card.easyApply ? <span className="label shrink-0">easy</span> : null}
                {card.reposted ? <span className="label shrink-0">reposted</span> : null}
                <span className="ml-auto grid-num shrink-0 text-micro">
                  {relativeAge(card.listedAt)}
                </span>
              </div>
            </li>
          ))}
        </ul>
      )}

      {rows.length > 0 && more ? (
        <div className="shrink-0 border-t border-hairline px-3 py-1.5">
          <button
            type="button"
            onClick={onMore}
            disabled={loading}
            className="label text-ink disabled:text-muted"
          >
            {loading ? 'reading' : 'load more'}
          </button>
        </div>
      ) : null}
    </div>
  )
}
