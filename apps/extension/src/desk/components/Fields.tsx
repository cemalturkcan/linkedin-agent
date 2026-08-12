import { useEffect, useRef, useState, type KeyboardEvent, type ReactNode } from 'react'
import { cn } from '@/lib/utils'

export const INPUT =
  'h-6 min-w-0 border border-hairline bg-ground px-1.5 text-body text-ink outline-none placeholder:text-muted/50 focus-visible:border-ink'

export const SELECT =
  'h-6 max-w-[13rem] border border-hairline bg-ground px-1 text-meta text-ink outline-none focus-visible:border-ink'

export const TEXTAREA =
  'min-h-[4.5rem] w-full resize-y border border-hairline bg-ground px-2 py-1.5 text-body text-ink outline-none placeholder:text-muted/50 focus-visible:border-ink'

export function blurOnEscape(event: KeyboardEvent<HTMLElement>) {
  if (event.key !== 'Escape') return
  event.stopPropagation()
  event.currentTarget.blur()
}

export function Action({
  children,
  onClick,
  disabled,
  pressed,
}: {
  children: ReactNode
  onClick(): void
  disabled?: boolean
  pressed?: boolean
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      onKeyDown={blurOnEscape}
      disabled={disabled}
      aria-pressed={pressed}
      className={cn(
        'label h-6 border px-2',
        pressed ? 'border-ink bg-select text-ink' : 'border-hairline text-ink hover:bg-hover',
        disabled ? 'border-hairline text-muted hover:bg-transparent' : '',
      )}
    >
      {children}
    </button>
  )
}

export function Quiet({
  children,
  onClick,
  disabled,
}: {
  children: ReactNode
  onClick(): void
  disabled?: boolean
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      onKeyDown={blurOnEscape}
      disabled={disabled}
      className="label text-muted hover:text-ink disabled:text-muted/50"
    >
      {children}
    </button>
  )
}

export function Remove({ label, onClick }: { label: string; onClick(): void }) {
  return (
    <button
      type="button"
      onClick={onClick}
      onKeyDown={blurOnEscape}
      aria-label={label}
      className="flex size-4 shrink-0 items-center justify-center text-meta leading-none text-muted hover:text-ink"
    >
      ×
    </button>
  )
}

export function Group({
  title,
  lede,
  children,
}: {
  title: string
  lede: string
  children: ReactNode
}) {
  return (
    <section className="mt-8 border-t border-hairline pt-5 first:mt-0 first:border-t-0 first:pt-0">
      <h2 className="label text-ink">{title}</h2>
      <p className="mt-1.5 max-w-[42rem] text-meta text-muted">{lede}</p>
      <div className="mt-4">{children}</div>
    </section>
  )
}

export function Section({
  title,
  hint,
  children,
}: {
  title: string
  hint?: string
  children: ReactNode
}) {
  return (
    <section className="mt-5 first:mt-0">
      <h3 className="label text-muted">{title}</h3>
      {hint ? <p className="mt-1 max-w-[42rem] text-meta text-muted">{hint}</p> : null}
      <div className="mt-2">{children}</div>
    </section>
  )
}

export function Row({
  label,
  htmlFor,
  hint,
  children,
}: {
  label: string
  htmlFor?: string
  hint?: string
  children: ReactNode
}) {
  return (
    <div className="flex flex-wrap items-baseline gap-x-3 gap-y-1 py-1">
      <label className="label w-[8.5rem] shrink-0 text-muted" htmlFor={htmlFor}>
        {label}
      </label>
      <div className="min-w-0 flex-1">
        {children}
        {hint ? <p className="mt-1 max-w-[38rem] text-meta text-muted">{hint}</p> : null}
      </div>
    </div>
  )
}

export function Facts({ items }: { items: { term: string; value: string }[] }) {
  const shown = items.filter((item) => item.value)
  if (shown.length === 0) return null

  return (
    <dl className="divide-y divide-hairline border border-hairline">
      {shown.map((item) => (
        <div key={item.term} className="flex items-baseline gap-3 px-2 py-1">
          <dt className="label w-[5.5rem] shrink-0 text-muted">{item.term}</dt>
          <dd className="min-w-0 flex-1 text-body text-ink">{item.value}</dd>
        </div>
      ))}
    </dl>
  )
}

export function Note({ term, children }: { term: string; children: ReactNode }) {
  return (
    <p className="mt-3 flex items-start gap-3 border border-hairline bg-accent px-2 py-1.5">
      <span className="label shrink-0 pt-px text-ink">{term}</span>
      <span className="min-w-0 flex-1 text-meta text-ink">{children}</span>
    </p>
  )
}

export function Switch({
  checked,
  label,
  onChange,
}: {
  checked: boolean
  label: string
  onChange(value: boolean): void
}) {
  return (
    <button
      type="button"
      role="switch"
      aria-checked={checked}
      onClick={() => onChange(!checked)}
      onKeyDown={blurOnEscape}
      className="flex items-center gap-2 text-left"
    >
      <span
        aria-hidden
        className={cn(
          'flex size-3.5 shrink-0 items-center justify-center border',
          checked ? 'border-ink bg-ink' : 'border-hairline bg-ground',
        )}
      >
        {checked ? <span className="block size-1.5 bg-ground" /> : null}
      </span>
      <span className="text-body text-ink">{label}</span>
    </button>
  )
}

export function Choice<T extends string>({
  name,
  value,
  options,
  onChange,
}: {
  name: string
  value: T
  options: readonly T[]
  onChange(value: T): void
}) {
  return (
    <div role="radiogroup" aria-label={name} className="flex items-stretch border border-hairline">
      {options.map((option, index) => {
        const active = option === value
        return (
          <button
            key={option}
            type="button"
            role="radio"
            aria-checked={active}
            onClick={() => onChange(option)}
            onKeyDown={blurOnEscape}
            className={cn(
              'label h-6 flex-1 px-2',
              index > 0 ? 'border-l border-hairline' : '',
              active ? 'bg-active text-onactive' : 'text-muted hover:bg-hover hover:text-ink',
            )}
          >
            {option}
          </button>
        )
      })}
    </div>
  )
}

export function Picks<T extends string>({
  name,
  values,
  options,
  onChange,
}: {
  name: string
  values: T[]
  options: readonly T[]
  onChange(values: T[]): void
}) {
  return (
    <div role="group" aria-label={name} className="flex items-stretch border border-hairline">
      {options.map((option, index) => {
        const active = values.includes(option)
        return (
          <button
            key={option}
            type="button"
            role="checkbox"
            aria-checked={active}
            onClick={() =>
              onChange(active ? values.filter((item) => item !== option) : [...values, option])
            }
            onKeyDown={blurOnEscape}
            className={cn(
              'label h-6 flex-1 px-2',
              index > 0 ? 'border-l border-hairline' : '',
              active ? 'bg-active text-onactive' : 'text-muted hover:bg-hover hover:text-ink',
            )}
          >
            {option}
          </button>
        )
      })}
    </div>
  )
}

export function TagEditor({
  id,
  values,
  placeholder,
  transform,
  onChange,
}: {
  id: string
  values: string[] | null | undefined
  placeholder: string
  transform?(value: string): string
  onChange(values: string[]): void
}) {
  const [entry, setEntry] = useState('')
  const kept = Array.isArray(values) ? values : []

  function add() {
    const raw = entry.trim()
    if (!raw) return
    const next = transform ? transform(raw) : raw
    setEntry('')
    if (!next || kept.includes(next)) return
    onChange([...kept, next])
  }

  function onKeyDown(event: KeyboardEvent<HTMLInputElement>) {
    if (event.key === 'Enter') {
      event.preventDefault()
      add()
      return
    }
    if (event.key === 'Backspace' && entry === '' && kept.length > 0) {
      event.preventDefault()
      onChange(kept.slice(0, -1))
      return
    }
    blurOnEscape(event)
  }

  return (
    <div>
      {kept.length > 0 ? (
        <div className="mb-1.5 flex flex-wrap gap-1">
          {kept.map((value) => (
            <span
              key={value}
              className="flex h-5 items-center gap-1 border border-hairline pr-0.5 pl-1.5"
            >
              <span className="text-meta text-ink">{value}</span>
              <Remove label={`remove ${value}`} onClick={() => onChange(kept.filter((item) => item !== value))} />
            </span>
          ))}
        </div>
      ) : null}

      <div className="flex items-center gap-2">
        <input
          id={id}
          value={entry}
          onChange={(event) => setEntry(event.target.value)}
          onKeyDown={onKeyDown}
          placeholder={placeholder}
          spellCheck={false}
          autoComplete="off"
          className={cn(INPUT, 'flex-1')}
        />
        <Action onClick={add} disabled={entry.trim() === ''}>
          add
        </Action>
      </div>
    </div>
  )
}

export function NumberField({
  id,
  value,
  onChange,
  suffix,
  scale = 1,
  min = 0,
  width = 'w-[5.5rem]',
}: {
  id: string
  value: number
  onChange(value: number): void
  suffix?: string
  scale?: number
  min?: number
  width?: string
}) {
  const shown = value / scale
  const [text, setText] = useState(String(shown))

  useEffect(() => {
    setText(String(value / scale))
  }, [value, scale])

  function commit() {
    const parsed = Number(text.replace(',', '.'))
    if (!Number.isFinite(parsed)) {
      setText(String(shown))
      return
    }
    const next = Math.max(min, Math.round(parsed * scale))
    setText(String(next / scale))
    if (next !== value) onChange(next)
  }

  return (
    <span className="flex items-center gap-1.5">
      <input
        id={id}
        value={text}
        inputMode="numeric"
        onChange={(event) => setText(event.target.value)}
        onBlur={commit}
        onKeyDown={(event) => {
          if (event.key === 'Enter') {
            event.preventDefault()
            commit()
            event.currentTarget.blur()
            return
          }
          blurOnEscape(event)
        }}
        spellCheck={false}
        autoComplete="off"
        className={cn(INPUT, 'grid-num text-right', width)}
      />
      {suffix ? <span className="label text-muted">{suffix}</span> : null}
    </span>
  )
}

export function Output({
  label,
  text,
  follow = false,
  height = 'max-h-[22rem]',
}: {
  label: string
  text: string
  follow?: boolean
  height?: string
}) {
  const box = useRef<HTMLPreElement>(null)
  const pinned = useRef(true)

  useEffect(() => {
    const node = box.current
    if (!follow || !node || !pinned.current) return
    node.scrollTop = node.scrollHeight
  }, [text, follow])

  return (
    <div className="mt-2">
      <div className="flex items-baseline justify-between gap-3">
        <h3 className="label text-muted">{label}</h3>
        <span className="grid-num text-micro text-muted">{text.length} chars</span>
      </div>
      <pre
        ref={box}
        tabIndex={0}
        aria-label={label}
        onScroll={(event) => {
          const node = event.currentTarget
          pinned.current = node.scrollHeight - node.scrollTop - node.clientHeight < 24
        }}
        className={cn(
          'mt-1 w-full max-w-full overflow-auto overscroll-contain whitespace-pre-wrap break-words border border-hairline bg-raised px-2 py-1.5 text-meta text-ink',
          height,
        )}
      >
        {text}
      </pre>
    </div>
  )
}
