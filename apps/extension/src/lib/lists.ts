import type { Posting } from '@/lib/agent/types'
import { hideHandled } from '@/lib/handled'

export type ListKey = 'inbox' | 'manual' | 'queue' | 'applied'

export const LISTS: ListKey[] = ['inbox', 'manual', 'queue', 'applied']

const PROPOSING: ListKey[] = ['inbox', 'manual']

const QUEUES_ON_OPEN = ['inbox', 'manual']

export function queueOnOpen(status: string): boolean {
  return QUEUES_ON_OPEN.includes(status)
}

export function postingAge(posting: Posting): number {
  if (posting.listedAt !== null && Number.isFinite(posting.listedAt)) return posting.listedAt
  const arrived = Date.parse(posting.createdAt)
  return Number.isFinite(arrived) ? arrived : 0
}

export function oldestFirst(rows: Posting[]): Posting[] {
  return [...rows].sort(
    (left, right) => postingAge(left) - postingAge(right) || left.id.localeCompare(right.id),
  )
}

export function groupLists(rows: Posting[], opened: Set<string>): Record<ListKey, Posting[]> {
  const grouped: Record<ListKey, Posting[]> = { inbox: [], manual: [], queue: [], applied: [] }
  for (const list of LISTS) {
    const owned = rows.filter((row) => row.status === list)
    grouped[list] = oldestFirst(PROPOSING.includes(list) ? hideHandled(owned, opened) : owned)
  }
  return grouped
}

export function countLists(lists: Record<ListKey, Posting[]>): Record<ListKey, number> {
  return {
    inbox: lists.inbox.length,
    manual: lists.manual.length,
    queue: lists.queue.length,
    applied: lists.applied.length,
  }
}
