import { memo } from 'react'
import type { Posting, ResumeFit } from '@/lib/agent/types'
import { relativeAge, resumeTag } from '@/lib/format'
import { cn } from '@/lib/utils'

const WEAK_FIT: Record<'partial' | 'none', { mark: string; note: string }> = {
  partial: { mark: '~', note: 'partial, no cv leads with this posting’s core stack' },
  none: { mark: '≠', note: 'none, no cv in the folder covers this posting' },
}

function weakFit(fit: ResumeFit | null) {
  return fit === 'partial' || fit === 'none' ? WEAK_FIT[fit] : null
}

function scoreTone(score: number | null): string {
  if (score === null) return 'text-muted opacity-50'
  if (score >= 85) return 'font-medium text-ink'
  if (score >= 70) return 'text-ink'
  return 'text-muted'
}

interface PostingRowProps {
  posting: Posting
  domId: string
  selected: boolean
  leaving: boolean
  onSelect(): void
  onOpen(): void
}

function PostingRowImpl({ posting, domId, selected, leaving, onSelect, onOpen }: PostingRowProps) {
  const weak = weakFit(posting.resumeFit)
  const tag = resumeTag(posting.resumeCode, posting.resumeLang)
  const reason = posting.verdictReason ?? posting.triageReason ?? ''

  return (
    <li
      id={domId}
      role="option"
      aria-selected={selected}
      onMouseDown={onSelect}
      onDoubleClick={onOpen}
      className={cn(
        'select-none px-3 py-1.5',
        leaving ? 'row-handled' : '',
        selected ? 'bg-select text-ink' : 'hover:bg-hover',
      )}
    >
      <div className="flex items-baseline gap-2">
        <span
          className={cn(
            'min-w-0 flex-1 truncate text-row',
            selected ? 'font-semibold' : '',
          )}
        >
          {posting.title || 'untitled posting'}
        </span>
        <span className={cn('grid-num shrink-0 text-meta', scoreTone(posting.score))}>
          {posting.score === null ? '–' : Math.round(posting.score)}
        </span>
      </div>

      <div className="mt-0.5 flex items-baseline gap-1.5 text-meta text-muted">
        <span className="min-w-0 max-w-[10rem] shrink truncate text-ink">{posting.company}</span>
        {posting.location ? (
          <span className="min-w-0 max-w-[8rem] shrink truncate">{posting.location}</span>
        ) : null}
        {!posting.easyApply ? <span className="label shrink-0">manual</span> : null}
        {posting.stale ? <span className="label shrink-0 text-ink">stale</span> : null}
        <span className="ml-auto flex shrink-0 items-baseline gap-1.5">
          {weak ? (
            <span className="grid-num text-micro" title={weak.note}>
              {weak.mark}
            </span>
          ) : null}
          {tag ? <span className="label">{tag}</span> : null}
          <span className="grid-num w-7 text-right text-micro">
            {relativeAge(posting.listedAt)}
          </span>
        </span>
      </div>

      {reason ? <p className="mt-0.5 truncate text-meta text-muted">{reason}</p> : null}
    </li>
  )
}

export const PostingRow = memo(PostingRowImpl)
