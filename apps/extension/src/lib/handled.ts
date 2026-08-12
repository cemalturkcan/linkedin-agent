import type { Posting } from '@/lib/agent/types'
import { HANDLED_STATUSES } from '@/lib/agent/types'
import { readKey, writeKeys } from '@/lib/storage'

export const HANDLED_KEY = 'handled'
export const OPENED_KEY = 'opened'
export const HANDLED_MAX = 4_000

async function readIds(key: string): Promise<Set<string>> {
  const stored = await readKey<unknown>(key, [])
  if (!Array.isArray(stored)) return new Set()
  return new Set(stored.filter((id): id is string => typeof id === 'string' && id !== ''))
}

async function addIds(key: string, ids: string[], known: Set<string>): Promise<Set<string>> {
  for (const id of ids) {
    if (typeof id !== 'string' || id === '') continue
    known.delete(id)
    known.add(id)
  }
  const kept = [...known].slice(-HANDLED_MAX)
  await writeKeys({ [key]: kept })
  return new Set(kept)
}

export function handledIn(postings: Posting[]): Set<string> {
  const handled = new Set<string>()
  for (const posting of postings) {
    if (HANDLED_STATUSES.includes(posting.status)) handled.add(posting.id)
  }
  return handled
}

export function knownIn(postings: Posting[]): Set<string> {
  return new Set(postings.map((posting) => posting.id))
}

export async function loadHandled(): Promise<Set<string>> {
  return readIds(HANDLED_KEY)
}

export async function loadOpened(): Promise<Set<string>> {
  return readIds(OPENED_KEY)
}

export async function cacheHandled(handled: Set<string>): Promise<Set<string>> {
  return addIds(HANDLED_KEY, [...handled], await loadHandled())
}

export async function markOpened(ids: string[]): Promise<Set<string>> {
  await addIds(OPENED_KEY, ids, await loadOpened())
  return addIds(HANDLED_KEY, ids, await loadHandled())
}

export function hideHandled<T extends { id: string }>(rows: T[], handled: Set<string>): T[] {
  return rows.filter((row) => !handled.has(row.id))
}
