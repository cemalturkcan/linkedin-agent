import { useState } from 'react'
import type { Outcome, Posting } from '@/lib/agent/types'
import { OUTCOMES } from '@/lib/agent/types'
import { relativeAge, resumeTag } from '@/lib/format'
import { oldestFirst } from '@/lib/lists'
import { cn } from '@/lib/utils'
import { Action, INPUT, Note, blurOnEscape } from '@/desk/components/Fields'
import type { Resource } from '@/desk/useDeskState'

export const POSTING_FILTERS = [
  'inbox',
  'manual',
  'queue',
  'applied',
  'opened',
  'skipped',
  'all',
] as const

export type PostingFilter = (typeof POSTING_FILTERS)[number]

const FIT_WORDS: Record<string, string> = {
  strong: 'strong, a variant already leads with what this posting is built on',
  partial: 'partial, a variant overlaps but leads elsewhere',
  none: 'none, nothing on file speaks to this work',
}

const STAGE_WORDS: Record<string, string> = {
  triage: 'the title alone, the description was never read',
  deep: 'the full description',
  hand: 'nothing, you opened it yourself',
}

const VERDICT_WORDS: Record<string, string> = {
  new: 'not judged yet',
  reading: 'held while the description is read',
  inbox: 'apply',
  manual: 'apply, by hand, this one has no easy apply',
  queue: 'apply, and you opened it',
  applied: 'apply, and you sent it',
  opened: 'yours, you found it and opened it yourself',
  skipped: 'skip',
  duplicate: 'the same role as one already on file',
}

export const ANY_TAG = ''
export const ANY_COUNTRY = ''

const WORKPLACE_SUFFIX = /\s*\([^)]*\)\s*$/

export function countryOf(location: string | null | undefined): string {
  const named = String(location ?? '').replace(WORKPLACE_SUFFIX, '').trim()
  if (!named) return ''
  return (named.split(',').pop() ?? '').trim()
}

export function filterPostings(
  rows: Posting[],
  filter: PostingFilter,
  tag = ANY_TAG,
  country = ANY_COUNTRY,
): Posting[] {
  const inList = filter === 'all' ? rows : rows.filter((row) => row.status === filter)
  const tagged = tag === ANY_TAG ? inList : inList.filter((row) => row.resumeCode === tag)
  const placed =
    country === ANY_COUNTRY ? tagged : tagged.filter((row) => countryOf(row.location) === country)
  return oldestFirst(placed)
}

export interface Tag {
  code: string
  label: string
  count: number
}

function counted(rows: Posting[], keyOf: (row: Posting) => string): Map<string, number> {
  const counts = new Map<string, number>()
  for (const row of rows) {
    const key = keyOf(row)
    if (!key) continue
    counts.set(key, (counts.get(key) ?? 0) + 1)
  }
  return counts
}

function ranked(counts: Map<string, number>, labels: Record<string, string>): Tag[] {
  return [...counts.entries()]
    .map(([code, count]) => ({ code, label: labels[code] ?? code, count }))
    .sort((left, right) => right.count - left.count || left.code.localeCompare(right.code))
}

export function tagsIn(rows: Posting[], labels: Record<string, string>): Tag[] {
  return ranked(counted(rows, (row) => row.resumeCode ?? ''), labels)
}

export function countriesIn(rows: Posting[]): Tag[] {
  return ranked(counted(rows, (row) => countryOf(row.location)), {})
}

function Line({ term, children }: { term: string; children: string }) {
  if (!children) return null
  return (
    <p className="flex items-baseline gap-2 text-meta">
      <span className="label w-[4.5rem] shrink-0 text-muted">{term}</span>
      <span className="min-w-0 flex-1 text-ink">{children}</span>
    </p>
  )
}

function pay(posting: Posting): string {
  if (!posting.statedPay) return ''
  return `${posting.statedPay.currency} ${posting.statedPay.amount} per ${posting.statedPay.period}`
}

function verdict(posting: Posting): string {
  const word = VERDICT_WORDS[posting.status] ?? posting.status
  return posting.score === null ? word : `${word}, scored ${Math.round(posting.score)}`
}

export function byHand(posting: Posting): boolean {
  return posting.stage === 'hand'
}

function cvLine(posting: Posting): string {
  const tag = resumeTag(posting.resumeCode, posting.resumeLang)
  if (!tag) {
    return byHand(posting) ? 'none armed when you opened it' : ''
  }
  const who = byHand(posting) ? 'you chose it' : 'the agent assigned it'
  const fit = posting.resumeFit ? FIT_WORDS[posting.resumeFit] ?? '' : ''
  return [`${tag}, ${who}`, fit].filter(Boolean).join(', ')
}

function origin(posting: Posting): string {
  return [
    posting.cycleId ? `round ${posting.cycleId}` : '',
    posting.queryLabel ? `the ${posting.queryLabel} query` : '',
  ]
    .filter(Boolean)
    .join(', ')
}

function Chips({
  term,
  chosen,
  chips,
  onChoose,
}: {
  term: string
  chosen: string
  chips: Tag[]
  onChoose(value: string): void
}) {
  if (chips.length === 0) return null

  return (
    <div className="mt-2 flex flex-wrap items-center gap-1.5">
      <span className="label mr-1 w-8 shrink-0 text-muted">{term}</span>
      <button
        type="button"
        aria-pressed={chosen === ''}
        onClick={() => onChoose('')}
        className={cn(
          'label h-6 border border-hairline px-2',
          chosen === '' ? 'bg-active text-onactive' : 'text-muted hover:bg-hover hover:text-ink',
        )}
      >
        any
      </button>
      {chips.map((entry) => (
        <button
          key={entry.code}
          type="button"
          aria-pressed={chosen === entry.code}
          onClick={() => onChoose(chosen === entry.code ? '' : entry.code)}
          className={cn(
            'label flex h-6 items-center gap-1.5 border border-hairline px-2',
            chosen === entry.code
              ? 'bg-active text-onactive'
              : 'text-muted hover:bg-hover hover:text-ink',
          )}
        >
          <span>{entry.label}</span>
          <span className={cn('grid-num text-micro', chosen === entry.code ? 'text-onactive/70' : '')}>
            {entry.count}
          </span>
        </button>
      ))}
    </div>
  )
}

function OutcomeRecorder({
  posting,
  onRecord,
}: {
  posting: Posting
  onRecord(id: string, outcome: string, note: string): Promise<boolean>
}) {
  const [note, setNote] = useState(posting.outcomeNote ?? '')

  return (
    <div className="mt-2 border-t border-hairline pt-2">
      <div className="flex flex-wrap items-center gap-2">
        <span className="label w-[4.5rem] shrink-0 text-muted">outcome</span>
        <div role="radiogroup" aria-label="what came of this application" className="flex">
          {OUTCOMES.map((outcome: Outcome, index) => {
            const active = posting.outcome === outcome
            return (
              <button
                key={outcome}
                type="button"
                role="radio"
                aria-checked={active}
                onClick={() => void onRecord(posting.id, active ? '' : outcome, note)}
                className={cn(
                  'label h-6 border border-hairline px-2',
                  index > 0 ? 'border-l-0' : '',
                  active ? 'bg-active text-onactive' : 'text-muted hover:bg-hover hover:text-ink',
                )}
              >
                {outcome}
              </button>
            )
          })}
        </div>
      </div>
      <div className="mt-1.5 flex items-center gap-2">
        <span className="label w-[4.5rem] shrink-0 text-muted">note</span>
        <input
          value={note}
          onChange={(event) => setNote(event.target.value)}
          onKeyDown={(event) => {
            if (event.key === 'Enter' && posting.outcome) {
              event.preventDefault()
              void onRecord(posting.id, posting.outcome, note)
              return
            }
            blurOnEscape(event)
          }}
          placeholder="what actually happened, in your own words"
          spellCheck={false}
          className={cn(INPUT, 'flex-1')}
        />
        <Action
          onClick={() => posting.outcome && void onRecord(posting.id, posting.outcome, note)}
          disabled={!posting.outcome}
        >
          save
        </Action>
      </div>
      <p className="mt-1.5 text-meta text-muted">
        this is what the reflector reads. it never re-opens a verdict, it writes the lesson the next
        round carries.
      </p>
    </div>
  )
}

function Detail({
  posting,
  onRecord,
}: {
  posting: Posting
  onRecord(id: string, outcome: string, note: string): Promise<boolean>
}) {
  const focus = posting.tailoredResume?.focus ?? ''

  return (
    <div className="mt-2 space-y-0.5 border-l border-hairline pt-0.5 pb-0.5 pl-3">
      <Line term="verdict">{verdict(posting)}</Line>
      <Line term="reason">{posting.verdictReason ?? posting.triageReason ?? ''}</Line>
      <Line term="read">{STAGE_WORDS[posting.stage] ?? ''}</Line>
      <Line term="cv">{cvLine(posting)}</Line>
      {focus ? <Line term="tailor">{focus}</Line> : null}
      <Line term="posting">
        {[
          posting.seniority && posting.seniority !== 'unknown' ? posting.seniority : '',
          posting.workplace && posting.workplace !== 'unclear' ? posting.workplace : '',
          posting.contractType && posting.contractType !== 'unclear' ? posting.contractType : '',
          posting.postingLang ? `written in ${posting.postingLang}` : '',
          posting.agency ? 'hired through an agency' : '',
          pay(posting),
        ]
          .filter(Boolean)
          .join(', ')}
      </Line>
      <Line term="found">{byHand(posting) ? 'you, in the linkedin feed' : origin(posting)}</Line>
      <Line term="stale">
        {posting.stale ? 'judged before your settings or cvs changed, so this verdict may not hold' : ''}
      </Line>
      {posting.status === 'applied' || byHand(posting) ? (
        <OutcomeRecorder posting={posting} onRecord={onRecord} />
      ) : null}
    </div>
  )
}

interface PostingsViewProps {
  jobs: Resource<Posting[]>
  rows: Posting[]
  filter: PostingFilter
  tag: string
  tags: Tag[]
  country: string
  countries: Tag[]
  cursor: number
  openedId: string | null
  reflection: string | null
  busy: string | null
  onFilter(filter: PostingFilter): void
  onTag(tag: string): void
  onCountry(country: string): void
  onCursor(index: number): void
  onOpen(id: string | null): void
  onVisit(posting: Posting): void
  onRecord(id: string, outcome: string, note: string): Promise<boolean>
  onReflect(): void
}

export function PostingsView({
  jobs,
  rows,
  filter,
  tag,
  tags,
  country,
  countries,
  cursor,
  openedId,
  reflection,
  busy,
  onFilter,
  onTag,
  onCountry,
  onCursor,
  onOpen,
  onVisit,
  onRecord,
  onReflect,
}: PostingsViewProps) {
  const recorded = (jobs.value ?? []).filter((row) => row.outcome).length

  return (
    <div className="w-full max-w-[46rem]">
      <p className="max-w-[42rem] text-body text-muted">
        every posting the agent judged and every one you opened yourself, oldest first, so the one
        that has waited longest sits at the top and nothing rots at the bottom. it opens on the
        inbox, the ones worth your time; the lists after it are where the rest went, and all is the
        proof that nothing was thrown away silently. press enter to open a posting on linkedin and d
        for the reason, which cv is on it and whether the agent assigned that cv or you chose it.
        recording what an application produced is the only thing that teaches the next round.
      </p>

      {jobs.failure ? <Note term="error">{jobs.failure.message}</Note> : null}

      <div className="mt-4 flex flex-wrap items-center gap-3">
        <div className="flex items-stretch border border-hairline">
          {POSTING_FILTERS.map((key, index) => (
            <button
              key={key}
              type="button"
              aria-pressed={key === filter}
              onClick={() => onFilter(key)}
              className={cn(
                'label h-6 px-2',
                index > 0 ? 'border-l border-hairline' : '',
                key === filter ? 'bg-active text-onactive' : 'text-muted hover:bg-hover hover:text-ink',
              )}
            >
              {key}
            </button>
          ))}
        </div>
        <span className="grid-num text-meta text-muted">
          {`${rows.length} shown, ${recorded} with an outcome`}
        </span>
        {!jobs.live && jobs.value ? <span className="label text-muted">cached</span> : null}
        <Action onClick={onReflect} disabled={busy === 'reflecting'}>
          {busy === 'reflecting' ? 'reading the outcomes' : 'draw lessons from the outcomes'}
        </Action>
      </div>

      <Chips term="cv" chosen={tag} chips={tags} onChoose={onTag} />
      <Chips term="place" chosen={country} chips={countries} onChoose={onCountry} />

      {reflection ? <Note term="reflector">{reflection}</Note> : null}

      {rows.length === 0 ? (
        <p className="mt-6 text-meta text-muted">
          {jobs.value
            ? 'nothing in this list. a round brings postings in and screening decides where they land.'
            : 'the agent has not answered, so no posting is listed. this is silence, not an empty list.'}
        </p>
      ) : (
        <ul className="mt-4 -mx-2 divide-y divide-hairline border-y border-hairline">
          {rows.map((posting, index) => {
            const open = posting.id === openedId
            return (
              <li
                key={posting.id}
                className={cn('px-2 py-2', index === cursor ? 'bg-select' : 'hover:bg-hover')}
                onMouseDown={() => onCursor(index)}
              >
                <button
                  type="button"
                  onClick={() => onOpen(open ? null : posting.id)}
                  aria-expanded={open}
                  className="flex w-full items-baseline gap-2 text-left"
                >
                  <span className="min-w-0 flex-1 truncate text-row text-ink">
                    {posting.title || 'untitled posting'}
                  </span>
                  <span className="label shrink-0 text-muted">{posting.status}</span>
                  {byHand(posting) ? <span className="label shrink-0 text-ink">by hand</span> : null}
                  {posting.outcome ? (
                    <span className="label shrink-0 text-ink">{posting.outcome}</span>
                  ) : null}
                  <span className="grid-num w-7 shrink-0 text-right text-meta text-muted">
                    {posting.score === null ? '–' : Math.round(posting.score)}
                  </span>
                  <span className="grid-num w-8 shrink-0 text-right text-micro text-muted">
                    {relativeAge(posting.listedAt)}
                  </span>
                </button>
                <div className="flex items-baseline gap-1.5 text-meta text-muted">
                  <span className="max-w-[14rem] truncate text-ink">{posting.company}</span>
                  {posting.location ? (
                    <span className="max-w-[12rem] truncate">{posting.location}</span>
                  ) : null}
                  {!posting.easyApply ? <span className="label">manual</span> : null}
                  {posting.url ? (
                    <button
                      type="button"
                      onClick={() => onVisit(posting)}
                      className="label ml-auto shrink-0 text-muted hover:text-ink"
                    >
                      open
                    </button>
                  ) : null}
                </div>
                {open ? <Detail posting={posting} onRecord={onRecord} /> : null}
              </li>
            )
          })}
        </ul>
      )}
    </div>
  )
}
