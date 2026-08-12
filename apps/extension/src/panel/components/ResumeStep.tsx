import type { Posting } from '@/lib/agent/types'
import { attemptLine, type Armed, type Attachment, type Attempt } from '@/lib/armed'
import { relativeAge, resumeTag } from '@/lib/format'
import type { Preset } from '@/lib/presets'
import { cn } from '@/lib/utils'

const SOURCE_WORD: Record<Attachment['source'], string> = {
  agent: 'the agent’s own pick',
  chosen: 'the cv you chose for it',
  armed: 'the cv you armed',
  none: 'nothing armed',
}

interface ResumeStepProps {
  postingId: string
  posting: Posting | null
  attachment: Attachment
  attempt: Attempt | null
  presets: Preset[]
  armed: Armed | null
  onArm(preset: Preset): void
  onDisarm(): void
  onList(): void
}

export function ResumeStep({
  postingId,
  posting,
  attachment,
  attempt,
  presets,
  armed,
  onArm,
  onDisarm,
  onList,
}: ResumeStepProps) {
  const tag = attachment.code ? resumeTag(attachment.code, attachment.lang) : ''
  const tried = attemptLine(attempt, postingId)

  return (
    <div className="flex min-h-0 flex-1 flex-col overflow-y-auto">
      <section className="shrink-0 border-b border-hairline px-3 py-2">
        <div className="flex items-baseline justify-between gap-2">
          <h2 className="label text-muted">the posting in this tab</h2>
          <button type="button" onClick={onList} className="label text-muted hover:text-ink">
            lists
          </button>
        </div>
        <p className="mt-1 truncate text-row text-ink">
          {posting?.title || `posting ${postingId}`}
        </p>
        <div className="mt-0.5 flex items-baseline gap-1.5 text-meta text-muted">
          {posting?.company ? (
            <span className="max-w-[10rem] truncate text-ink">{posting.company}</span>
          ) : null}
          {posting?.location ? (
            <span className="max-w-[8rem] truncate">{posting.location}</span>
          ) : null}
          {posting ? <span className="label">{posting.status}</span> : null}
          {posting?.listedAt ? (
            <span className="ml-auto grid-num text-micro">{relativeAge(posting.listedAt)}</span>
          ) : null}
        </div>
      </section>

      <section className="shrink-0 border-b border-hairline px-3 py-2">
        <h2 className="label text-muted">attaches</h2>
        <p className="mt-1 text-row text-ink">
          {tag ? `${tag}, ${SOURCE_WORD[attachment.source]}` : SOURCE_WORD.none}
        </p>
        <p className="mt-0.5 text-meta text-muted">{attachment.why}</p>
        {attachment.blocked ? (
          <p className="mt-1 text-meta text-ink">{attachment.blocked}</p>
        ) : attachment.attachesAs ? (
          <p className="mt-1 text-meta text-muted">
            {`as ${attachment.attachesAs}. ${attachment.consequence}.`}
          </p>
        ) : null}
        {tried ? <p className="mt-1 text-meta text-ink">{tried}</p> : null}
        {posting?.tailoredResume?.needed && posting.tailoredResume.focus ? (
          <p className="mt-1 text-meta text-muted">
            {`a purpose-written cv would lead with ${posting.tailoredResume.focus}`}
          </p>
        ) : null}
      </section>

      <section className="px-3 py-2">
        <h2 className="label text-muted">change it</h2>
        <div className="mt-1.5 flex flex-wrap gap-1">
          {presets.map((preset) => {
            const active = Boolean(preset.code) && preset.code === armed?.code
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
        <p className="mt-1.5 text-meta text-muted">
          arming a cv here changes what attaches on every posting the agent has no pick for, not
          only this one.
        </p>
      </section>
    </div>
  )
}
