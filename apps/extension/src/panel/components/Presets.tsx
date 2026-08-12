import type { Armed } from '@/lib/armed'
import type { Preset } from '@/lib/presets'
import { cn } from '@/lib/utils'

interface PresetsProps {
  presets: Preset[]
  derived: boolean
  armed: Armed | null
  live: boolean
  attachesAs: string | null
  consequence: string
  onArm(preset: Preset): void
  onDisarm(): void
}

export function Presets({
  presets,
  derived,
  armed,
  live,
  attachesAs,
  consequence,
  onArm,
  onDisarm,
}: PresetsProps) {
  const chosen = presets.find((preset) => preset.code && armed && preset.code === armed.code) ?? null

  return (
    <section className="shrink-0 border-b border-hairline px-3 py-2">
      <div className="flex items-baseline justify-between">
        <h2 className="label text-muted">resume presets</h2>
        {derived && !live ? <span className="label text-muted">cached</span> : null}
      </div>

      <div className="mt-1.5 flex flex-wrap gap-1">
        {presets.map((preset) => {
          const active = chosen?.id === preset.id
          return (
            <button
              key={preset.id}
              type="button"
              title={preset.aim || preset.label}
              aria-pressed={active}
              onClick={() => (active ? onDisarm() : onArm(preset))}
              className={cn(
                'flex items-baseline gap-1 border px-1.5 py-1',
                active
                  ? 'border-ink bg-select text-ink'
                  : 'border-hairline text-muted hover:border-rule hover:text-ink',
              )}
            >
              <span className="max-w-[9rem] truncate text-meta">{preset.label}</span>
              {preset.code ? <span className="label text-muted">{preset.code}</span> : null}
            </button>
          )
        })}
      </div>

      {!derived ? (
        <p className="mt-1.5 text-meta text-muted">
          no cv has been read yet, so there is one broad preset and nothing to arm. the desk page
          points the agent at your cv folder.
        </p>
      ) : null}

      <div className="mt-1.5 flex items-baseline justify-between gap-2">
        <p className="min-w-0 flex-1 truncate text-meta text-muted">
          {armed
            ? `armed ${armed.code}, attached to the next posting the agent has no verdict for`
            : 'nothing armed, so only the agent’s own pick is attached'}
        </p>
        {armed ? (
          <button type="button" onClick={onDisarm} className="label shrink-0 text-muted hover:text-ink">
            disarm
          </button>
        ) : null}
      </div>

      {attachesAs ? (
        <p className="mt-1 text-meta text-muted">
          {`attaches as ${attachesAs}. ${consequence}.`}
        </p>
      ) : null}
    </section>
  )
}
