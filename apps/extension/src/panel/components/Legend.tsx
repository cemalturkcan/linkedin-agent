import { Keycap } from '@/panel/components/Keycap'
import type { Tab } from '@/panel/components/Header'
import { cn } from '@/lib/utils'

interface Entry {
  keys: string[]
  label: string
  tabs?: Tab[]
}

const ENTRIES: Entry[] = [
  { keys: ['j', 'k'], label: 'move' },
  { keys: ['↵'], label: 'open' },
  { keys: ['a'], label: 'applied', tabs: ['manual', 'queue'] },
  { keys: ['x'], label: 'skip', tabs: ['inbox', 'manual', 'queue'] },
  { keys: ['1', '2', '3', '4', '5'], label: 'lists' },
  { keys: ['s'], label: 'screen' },
  { keys: ['r'], label: 'refresh' },
  { keys: ['.'], label: 'round' },
  { keys: ['m'], label: 'model' },
  { keys: ['p'], label: 'profile' },
  { keys: [','], label: 'settings' },
  { keys: ['⇧P'], label: 'pause' },
  { keys: ['c'], label: 'cv folder' },
]

export function Legend({ tab }: { tab: Tab }) {
  return (
    <footer className="flex h-7 shrink-0 items-center gap-3 overflow-x-auto border-t border-hairline px-3">
      {ENTRIES.map((entry) => {
        const active = !entry.tabs || entry.tabs.includes(tab)
        return (
          <span
            key={entry.label}
            className={cn('flex shrink-0 items-center gap-1', active ? 'opacity-100' : 'opacity-35')}
          >
            <span className="flex items-center gap-0.5">
              {entry.keys.map((key) => (
                <Keycap key={key}>{key}</Keycap>
              ))}
            </span>
            <span className="text-micro text-muted">{entry.label}</span>
          </span>
        )
      })}
    </footer>
  )
}
