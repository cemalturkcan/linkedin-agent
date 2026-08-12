import { useEffect, useRef, useState, type KeyboardEvent } from 'react'
import { agent } from '@/lib/agent/client'
import type { Place } from '@/lib/agent/types'
import { cn } from '@/lib/utils'
import { Action, INPUT, Remove, SELECT, blurOnEscape } from '@/desk/components/Fields'

export const PLACE_ID_IS_THE_AGENT_S = 'a place is a name here. the extension asks linkedin what it calls that place when a round runs, and a name linkedin does not know skips its query and says so.'

const UNCHECKED = 'not checked yet'

interface Chosen {
  name: string
  ring: string
  kind: string
}

function same(one: string, other: string): boolean {
  return one.trim().toLowerCase() === other.trim().toLowerCase()
}

function useResolved(term: string): Place[] {
  const [found, setFound] = useState<Place[]>([])

  useEffect(() => {
    let cancelled = false
    const timer = setTimeout(() => {
      void agent.places(term).then((result) => {
        if (cancelled) return
        setFound(result.ok ? (result.value.places ?? []) : [])
      })
    }, 160)
    return () => {
      cancelled = true
      clearTimeout(timer)
    }
  }, [term])

  return found
}

interface PlaceEditorProps {
  id: string
  entries: Chosen[]
  kinds: string[]
  rings: string[]
  derived: Place[]
  onChange(entries: Chosen[]): void
}

export function PlaceEditor({ id, entries, kinds, rings, derived, onChange }: PlaceEditorProps) {
  const placeRings = rings.filter((ring) => ring !== 'worldwide')
  const [term, setTerm] = useState('')
  const [cursor, setCursor] = useState(-1)
  const box = useRef<HTMLInputElement>(null)
  const resolved = useResolved(term)

  const held = (name: string) => entries.some((entry) => same(entry.name, name))
  const offered = derived.filter((place) => !held(place.name))
  const listed = resolved.filter((place) => !held(place.name))
  const open = term.trim() !== '' && listed.length > 0

  function take(place: { name: string; ring: string }) {
    const name = place.name.trim()
    if (!name || held(name)) return
    onChange([
      ...entries,
      { name, ring: place.ring || (placeRings[0] ?? ''), kind: kinds[0] ?? '' },
    ])
    setTerm('')
    setCursor(-1)
    box.current?.focus()
  }

  function onKeyDown(event: KeyboardEvent<HTMLInputElement>) {
    if (open && (event.key === 'ArrowDown' || event.key === 'ArrowUp')) {
      event.preventDefault()
      const step = event.key === 'ArrowDown' ? 1 : -1
      setCursor((previous) => Math.min(listed.length - 1, Math.max(-1, previous + step)))
      return
    }
    if (event.key === 'Enter') {
      event.preventDefault()
      take(cursor >= 0 && listed[cursor] ? listed[cursor] : { name: term, ring: '' })
      return
    }
    if (event.key === 'Escape' && open) {
      event.stopPropagation()
      setCursor(-1)
      setTerm('')
      return
    }
    blurOnEscape(event)
  }

  return (
    <div>
      {entries.length > 0 ? (
        <ul className="mb-2 divide-y divide-hairline border border-hairline">
          {entries.map((entry) => (
            <li key={entry.name} className="flex flex-wrap items-center gap-x-3 gap-y-1 px-2 py-1">
              <span className="min-w-0 flex-1 truncate text-body text-ink">{entry.name}</span>
              <select
                value={entry.ring}
                aria-label={`how far ${entry.name} reaches`}
                onChange={(event) =>
                  onChange(
                    entries.map((item) =>
                      item.name === entry.name ? { ...item, ring: event.target.value } : item,
                    ),
                  )
                }
                onKeyDown={blurOnEscape}
                className={cn(SELECT, 'shrink-0')}
              >
                {!entry.ring ? <option value="">{UNCHECKED}</option> : null}
                {placeRings.map((ring) => (
                  <option key={ring} value={ring}>
                    {ring}
                  </option>
                ))}
              </select>
              <select
                value={entry.kind}
                aria-label={`what ${entry.name} means for you`}
                onChange={(event) =>
                  onChange(
                    entries.map((item) =>
                      item.name === entry.name ? { ...item, kind: event.target.value } : item,
                    ),
                  )
                }
                onKeyDown={blurOnEscape}
                className={cn(SELECT, 'shrink-0')}
              >
                {kinds.map((kind) => (
                  <option key={kind} value={kind}>
                    {kind}
                  </option>
                ))}
              </select>
              <Remove
                label={`remove ${entry.name}`}
                onClick={() => onChange(entries.filter((item) => item.name !== entry.name))}
              />
            </li>
          ))}
        </ul>
      ) : null}

      {offered.length > 0 ? (
        <div className="mb-2 flex flex-wrap items-center gap-1">
          <span className="label mr-0.5 text-muted">from your cvs</span>
          {offered.map((place) => (
            <button
              key={place.name}
              type="button"
              onClick={() => take(place)}
              onKeyDown={blurOnEscape}
              className="flex h-5 items-center gap-1 border border-hairline px-1.5 text-meta text-ink hover:bg-raised"
            >
              {place.name}
            </button>
          ))}
        </div>
      ) : null}

      <div className="relative">
        <div className="flex items-center gap-2">
          <input
            id={id}
            ref={box}
            value={term}
            onChange={(event) => {
              setTerm(event.target.value)
              setCursor(-1)
            }}
            onKeyDown={onKeyDown}
            placeholder="a city, a country, a region, by name"
            spellCheck={false}
            autoComplete="off"
            role="combobox"
            aria-expanded={open}
            aria-controls={`${id}-resolved`}
            aria-label="add a place by name"
            className={cn(INPUT, 'flex-1')}
          />
          <Action onClick={() => take({ name: term, ring: '' })} disabled={term.trim() === ''}>
            add
          </Action>
        </div>

        {open ? (
          <ul
            id={`${id}-resolved`}
            role="listbox"
            aria-label="places already resolved"
            className="absolute inset-x-0 top-[calc(100%+2px)] z-10 divide-y divide-hairline border border-hairline bg-ground"
          >
            {listed.map((place, index) => (
              <li
                key={`${place.name}-${place.ring}`}
                role="option"
                aria-selected={index === cursor}
                onMouseDown={(event) => event.preventDefault()}
                onClick={() => take(place)}
                className={cn(
                  'flex select-none items-baseline gap-3 px-2 py-1',
                  index === cursor ? 'bg-select' : 'hover:bg-hover',
                )}
              >
                <span className="min-w-0 flex-1 truncate text-body text-ink">{place.name}</span>
                <span className="label shrink-0 text-muted">{place.ring || UNCHECKED}</span>
              </li>
            ))}
          </ul>
        ) : null}
      </div>
    </div>
  )
}
