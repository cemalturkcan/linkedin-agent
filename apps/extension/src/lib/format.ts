export function relativeAge(value: number | string | null, now = Date.now()): string {
  if (value === null || value === undefined || value === '') return ''
  const ms = typeof value === 'number' ? value : Date.parse(value)
  if (!Number.isFinite(ms)) return ''

  const seconds = Math.max(0, Math.round((now - ms) / 1000))
  if (seconds < 60) return 'now'
  const minutes = Math.round(seconds / 60)
  if (minutes < 60) return `${minutes}m`
  const hours = Math.round(minutes / 60)
  if (hours < 24) return `${hours}h`
  const days = Math.round(hours / 24)
  if (days < 7) return `${days}d`
  const weeks = Math.round(days / 7)
  if (weeks < 5) return `${weeks}w`
  return `${Math.round(days / 30)}mo`
}

export function resumeTag(code: string | null, lang: string | null): string {
  if (!code) return ''
  return lang ? `${code}·${lang.toUpperCase()}` : code
}

export function duration(ms: number): string {
  if (!Number.isFinite(ms) || ms < 0) return ''
  if (ms < 1000) return `${Math.round(ms)}ms`
  const seconds = ms / 1000
  if (seconds < 10) return `${seconds.toFixed(1)}s`
  if (seconds < 60) return `${Math.round(seconds)}s`
  const minutes = Math.floor(seconds / 60)
  return `${minutes}m ${String(Math.round(seconds - minutes * 60)).padStart(2, '0')}s`
}

export function plural(count: number, one: string, many: string): string {
  return `${count} ${count === 1 ? one : many}`
}

export function spend(used: number, cap: number, span: string): string {
  if (cap > 0) return `${used} of ${cap} model calls ${span}`
  return `${plural(used, 'model call', 'model calls')} ${span}, no cap`
}
