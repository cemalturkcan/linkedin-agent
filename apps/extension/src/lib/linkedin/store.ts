import { loadHandled } from '@/lib/handled'
import type { BackoffPort, OpenRound, OpenRoundPort, RoundReport, SeenPort } from '@/lib/linkedin/round'
import { forgetKeys, readKey, writeKeys } from '@/lib/storage'

export const HARVESTED_KEY = 'harvested'
export const DESCRIBED_KEY = 'described'
export const BACKOFF_KEY = 'linkedinBackoff'
export const OPEN_ROUND_KEY = 'openRound'
export const LAST_ROUND_KEY = 'lastRound'

export const HARVESTED_MAX = 4_000
export const DESCRIBED_MAX = 2_000

export const BACKOFF_STEP_MS = 5 * 60_000
export const BACKOFF_MAX_MS = 6 * 60 * 60_000
export const BACKOFF_STEPS = 8

async function readIds(key: string): Promise<Set<string>> {
  const stored = await readKey<unknown>(key, [])
  if (!Array.isArray(stored)) return new Set()
  return new Set(stored.filter((id): id is string => typeof id === 'string' && id !== ''))
}

async function rememberIds(key: string, ids: string[], cap: number): Promise<void> {
  const wanted = ids.filter((id) => typeof id === 'string' && id !== '')
  if (wanted.length === 0) return
  const known = await readIds(key)
  for (const id of wanted) {
    known.delete(id)
    known.add(id)
  }
  await writeKeys({ [key]: [...known].slice(-cap) })
}

export function seenStore(): SeenPort {
  return {
    async load() {
      const [handled, harvested] = await Promise.all([loadHandled(), readIds(HARVESTED_KEY)])
      for (const id of handled) harvested.add(id)
      return harvested
    },
    remember: (ids) => rememberIds(HARVESTED_KEY, ids, HARVESTED_MAX),
    described: () => readIds(DESCRIBED_KEY),
    rememberDescribed: (ids) => rememberIds(DESCRIBED_KEY, ids, DESCRIBED_MAX),
  }
}

interface Backoff {
  step: number
  until: number
}

export function backoffStore(now: () => number = Date.now, random: () => number = Math.random): BackoffPort {
  return {
    async held() {
      const stored = await readKey<Backoff | null>(BACKOFF_KEY, null)
      const until = Number(stored?.until ?? 0)
      return until > now() ? until : 0
    },
    async escalate() {
      const stored = await readKey<Backoff | null>(BACKOFF_KEY, null)
      const step = Math.min(Number(stored?.step ?? 0) + 1, BACKOFF_STEPS)
      const wait = Math.min(BACKOFF_STEP_MS * 2 ** (step - 1), BACKOFF_MAX_MS)
      const until = now() + wait + Math.floor(random() * Math.min(wait, 60_000))
      await writeKeys({ [BACKOFF_KEY]: { step, until } satisfies Backoff })
      return until
    },
    async clear() {
      await forgetKeys([BACKOFF_KEY])
    },
  }
}

export function openRoundStore(): OpenRoundPort {
  return {
    async read() {
      const stored = await readKey<OpenRound | null>(OPEN_ROUND_KEY, null)
      if (!stored || !Number.isFinite(Number(stored.cycleId)) || Number(stored.cycleId) <= 0) {
        return null
      }
      return {
        cycleId: Number(stored.cycleId),
        startedAt: Number(stored.startedAt) || 0,
        deadlineAt: Number(stored.deadlineAt) || 0,
      }
    },
    async write(round) {
      await writeKeys({ [OPEN_ROUND_KEY]: round })
    },
    async clear() {
      await forgetKeys([OPEN_ROUND_KEY])
    },
  }
}

export async function rememberRound(report: RoundReport): Promise<void> {
  await writeKeys({ [LAST_ROUND_KEY]: report })
}

export async function lastRound(): Promise<RoundReport | null> {
  return readKey<RoundReport | null>(LAST_ROUND_KEY, null)
}
