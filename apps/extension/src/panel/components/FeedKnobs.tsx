import { useState } from 'react'
import { RANGE_WORDS, type FeedKnobs as Knobs } from '@/lib/linkedin/feed'
import type { PlaceHit } from '@/lib/linkedin/geo'
import { RANGES } from '@/lib/linkedin/voyager'
import { cn } from '@/lib/utils'

interface FeedKnobsProps {
  knobs: Knobs
  hits: PlaceHit[]
  searching: boolean
  terms: string[]
  places: string[]
  onKnobs(knobs: Knobs): void
  onSuggest(name: string): void
  onChoose(hit: PlaceHit, asked: string): void
  onChooseName(name: string): void
  onClearPlace(): void
}

const TOGGLE = 'label border px-1.5 py-1'

function chipClass(active: boolean): string {
  return cn(
    TOGGLE,
    active
      ? 'border-ink bg-select text-ink'
      : 'border-hairline text-muted hover:border-rule hover:text-ink',
  )
}

export function FeedKnobs({
  knobs,
  hits,
  searching,
  terms,
  places,
  onKnobs,
  onSuggest,
  onChoose,
  onChooseName,
  onClearPlace,
}: FeedKnobsProps) {
  const [typed, setTyped] = useState('')

  return (
    <section className="shrink-0 border-b border-hairline px-3 py-2">
      <div className="flex items-baseline gap-2">
        <h2 className="label shrink-0 text-muted">place</h2>
        {knobs.place ? (
          <>
            <span className="min-w-0 flex-1 truncate text-meta text-ink">{knobs.place}</span>
            <button
              type="button"
              onClick={onClearPlace}
              className="label shrink-0 text-muted hover:text-ink"
            >
              change
            </button>
          </>
        ) : (
          <input
            value={typed}
            onChange={(event) => {
              setTyped(event.target.value)
              onSuggest(event.target.value)
            }}
            placeholder="a city, a country, a region"
            spellCheck={false}
            aria-label="place"
            className="min-w-0 flex-1 border-b border-hairline bg-transparent text-meta text-ink outline-none placeholder:text-muted focus:border-rule"
          />
        )}
      </div>

      {!knobs.place && searching ? (
        <p className="mt-1 text-meta text-muted">asking linkedin what it knows by that name</p>
      ) : null}

      {!knobs.place && hits.length > 0 ? (
        <ul className="mt-1 divide-y divide-hairline border-y border-hairline">
          {hits.slice(0, 6).map((hit) => (
            <li key={hit.locationId}>
              <button
                type="button"
                onClick={() => {
                  onChoose(hit, typed)
                  setTyped('')
                }}
                className="w-full truncate py-1 text-left text-meta text-ink hover:bg-hover"
              >
                {hit.label}
              </button>
            </li>
          ))}
        </ul>
      ) : null}

      {!knobs.place && places.length > 0 ? (
        <div className="mt-1 flex flex-wrap gap-1">
          {places.map((name) => (
            <button
              key={name}
              type="button"
              onClick={() => onChooseName(name)}
              className={chipClass(false)}
            >
              {name}
            </button>
          ))}
        </div>
      ) : null}

      <div className="mt-1.5 flex items-baseline gap-2">
        <h2 className="label shrink-0 text-muted">term</h2>
        <input
          value={knobs.keyword}
          onChange={(event) => onKnobs({ ...knobs, keyword: event.target.value })}
          placeholder="what linkedin matches on, empty reads the raw feed"
          spellCheck={false}
          aria-label="search term"
          className="min-w-0 flex-1 border-b border-hairline bg-transparent text-meta text-ink outline-none placeholder:text-muted focus:border-rule"
        />
        {knobs.keyword ? (
          <button
            type="button"
            onClick={() => onKnobs({ ...knobs, keyword: '' })}
            className="label shrink-0 text-muted hover:text-ink"
          >
            clear
          </button>
        ) : null}
      </div>

      {terms.length > 0 ? (
        <div className="mt-1 flex flex-wrap gap-1">
          {terms.map((term) => {
            const active = knobs.keyword.trim().toLowerCase() === term.toLowerCase()
            return (
              <button
                key={term}
                type="button"
                aria-pressed={active}
                onClick={() => onKnobs({ ...knobs, keyword: active ? '' : term })}
                className={chipClass(active)}
              >
                {term}
              </button>
            )
          })}
        </div>
      ) : null}

      <div className="mt-1.5 flex flex-wrap gap-1">
        {RANGES.map((range) => {
          const active = knobs.range === range
          return (
            <button
              key={range}
              type="button"
              aria-pressed={active}
              onClick={() => onKnobs({ ...knobs, range })}
              className={cn(
                TOGGLE,
                active
                  ? 'border-ink bg-select text-ink'
                  : 'border-hairline text-muted hover:border-rule hover:text-ink',
              )}
            >
              {RANGE_WORDS[range] ?? range}
            </button>
          )
        })}
      </div>

      <div className="mt-1 flex flex-wrap gap-1">
        <button
          type="button"
          aria-pressed={knobs.easyApply}
          onClick={() => onKnobs({ ...knobs, easyApply: !knobs.easyApply })}
          className={cn(
            TOGGLE,
            knobs.easyApply
              ? 'border-ink bg-select text-ink'
              : 'border-hairline text-muted hover:border-rule hover:text-ink',
          )}
        >
          easy apply
        </button>
        <button
          type="button"
          aria-pressed={knobs.remoteOnly}
          onClick={() => onKnobs({ ...knobs, remoteOnly: !knobs.remoteOnly })}
          className={cn(
            TOGGLE,
            knobs.remoteOnly
              ? 'border-ink bg-select text-ink'
              : 'border-hairline text-muted hover:border-rule hover:text-ink',
          )}
        >
          remote
        </button>
      </div>
    </section>
  )
}
