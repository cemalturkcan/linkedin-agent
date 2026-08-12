import type { StreamStatus } from '@/lib/agent/stream'
import { cn } from '@/lib/utils'
import { LISTS, type ListKey } from '@/panel/useAgentState'

export type Tab = ListKey | 'feed'

export const TABS: Tab[] = [...LISTS, 'feed']

const STREAM_WORD: Record<StreamStatus, string> = {
  live: 'live',
  connecting: 'connecting',
  down: 'no stream',
}

interface HeaderProps {
  tab: Tab
  counts: Record<ListKey, number>
  unseen: number
  stream: StreamStatus
  busy: boolean
  following: boolean
  onTab(tab: Tab): void
  onDesk(): void
}

export function Header({
  tab,
  counts,
  unseen,
  stream,
  busy,
  following,
  onTab,
  onDesk,
}: HeaderProps) {
  return (
    <header className="shrink-0 border-b border-hairline">
      <div className="flex items-center justify-between px-3 pt-3 pb-2">
        <h1 className="label text-ink">agent</h1>
        <div className="flex items-center gap-3">
          <span className={cn('label', stream === 'live' ? 'text-muted' : 'text-ink')}>
            {busy ? 'working' : STREAM_WORD[stream]}
          </span>
          <button type="button" onClick={onDesk} className="label text-muted hover:text-ink">
            desk
          </button>
        </div>
      </div>
      <nav className="flex items-stretch border-t border-hairline">
        {TABS.map((key) => (
          <button
            key={key}
            type="button"
            aria-pressed={key === tab && !following}
            onClick={() => onTab(key)}
            className={cn(
              'flex flex-1 items-baseline justify-center gap-1 border-r border-hairline py-1.5 last:border-r-0',
              key === tab && !following
                ? 'bg-active text-onactive'
                : 'text-muted hover:bg-hover hover:text-ink',
            )}
          >
            <span className="label">{key}</span>
            <span className="grid-num text-micro">
              {key === 'feed' ? unseen : counts[key as ListKey]}
            </span>
          </button>
        ))}
      </nav>
    </header>
  )
}
