import { expect, test } from 'bun:test'
import {
  dueForCheck,
  SWEEP_GAP_MS,
  SWEEP_MAX,
  SWEEP_STATUSES,
  TAB_GAP_MS,
  toSweep,
} from '@/lib/applied'

const NOW = 1_800_000_000_000

test('a check that found nothing blocks the next one for seconds, not hours', () => {
  const checked = { '1': NOW - 25_000 }

  expect(dueForCheck(checked, '1', TAB_GAP_MS, NOW)).toBe(true)
  expect(dueForCheck(checked, '1', SWEEP_GAP_MS, NOW)).toBe(false)
  expect(dueForCheck({ '1': NOW - 5_000 }, '1', TAB_GAP_MS, NOW)).toBe(false)
  expect(dueForCheck({}, '1', TAB_GAP_MS, NOW)).toBe(true)
})

test('the sweep looks at every posting the person could still apply to', () => {
  const rows = [
    { id: 'a', status: 'opened' },
    { id: 'b', status: 'skipped' },
    { id: 'c', status: 'queue' },
    { id: 'd', status: 'applied' },
    { id: 'e', status: 'manual' },
    { id: 'f', status: 'inbox' },
  ]

  expect(toSweep(rows, {}, NOW).take.map((row) => row.id)).toEqual(['a', 'c', 'e', 'f'])
  expect(SWEEP_STATUSES).toEqual(['inbox', 'manual', 'queue', 'opened'])
})

test('a sweep that cannot reach everything says how much it left behind', () => {
  const rows = Array.from({ length: SWEEP_MAX + 7 }, (_, index) => ({
    id: `p${index}`,
    status: 'opened',
  }))

  const { take, left } = toSweep(rows, {}, NOW)
  expect(take.length).toBe(SWEEP_MAX)
  expect(left).toBe(7)
})

test('a posting checked minutes ago waits for the next sweep', () => {
  const rows = [
    { id: 'a', status: 'opened' },
    { id: 'b', status: 'opened' },
  ]
  const checked = { a: NOW - 60_000 }

  expect(toSweep(rows, checked, NOW).take.map((row) => row.id)).toEqual(['b'])
})
