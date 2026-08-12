export const TAB_GAP_MS = 10_000
export const SWEEP_GAP_MS = 30 * 60_000
export const SWEEP_MAX = 40
export const SWEEP_STATUSES = ['inbox', 'manual', 'queue', 'opened']

export interface Waiting {
  id: string
  status: string
}

export function dueForCheck(
  checked: Record<string, number>,
  id: string,
  gapMs: number,
  now: number,
): boolean {
  return Number(checked?.[id] ?? 0) <= now - gapMs
}

export function toSweep(
  rows: Waiting[],
  checked: Record<string, number>,
  now: number,
): { take: Waiting[]; left: number } {
  const due = rows
    .filter((row) => SWEEP_STATUSES.includes(row.status))
    .filter((row) => dueForCheck(checked, row.id, SWEEP_GAP_MS, now))
  return { take: due.slice(0, SWEEP_MAX), left: Math.max(0, due.length - SWEEP_MAX) }
}
