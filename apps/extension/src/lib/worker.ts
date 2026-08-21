import type { PickPreview } from '@/lib/armed'
import type { FeedKnobs } from '@/lib/linkedin/feed'
import type { PlaceHit } from '@/lib/linkedin/geo'
import type { RoundReport, RoundUpdate } from '@/lib/linkedin/round'
import type { Card } from '@/lib/linkedin/voyager'

export const NO_WORKER =
  'the extension worker is not answering, so a round cannot be run from here'

function runtime(): typeof chrome | null {
  return (globalThis as { chrome?: typeof chrome }).chrome ?? null
}

async function ask<T>(message: Record<string, unknown>): Promise<T | null> {
  const api = runtime()
  if (!api?.runtime?.sendMessage) return null
  try {
    return (await api.runtime.sendMessage(message)) as T
  } catch {
    return null
  }
}

export interface RoundAnswer {
  ok: boolean
  report: RoundReport | null
  reason?: string
}

export function announceNow(): Promise<{ ok: boolean } | null> {
  return ask<{ ok: boolean }>({ type: 'hello' })
}

export function runRoundNow(): Promise<RoundAnswer | null> {
  return ask<RoundAnswer>({ type: 'round:run' })
}

export interface PickAnswer {
  ok: boolean
  preview: PickPreview | null
}

export function previewPick(id: string): Promise<PickAnswer | null> {
  return ask<PickAnswer>({ type: 'apply:pick', id })
}

export interface LastRoundAnswer {
  ok: boolean
  report: RoundReport | null
  running: boolean
}

export function lastRoundSeen(): Promise<LastRoundAnswer | null> {
  return ask<LastRoundAnswer>({ type: 'round:last' })
}

export interface FeedPageAnswer {
  ok: boolean
  cards?: Card[]
  total?: number
  start?: number
  message?: string
}

export interface FeedPlacesAnswer {
  ok: boolean
  hits?: PlaceHit[]
  message?: string
}

export function feedPageNow(knobs: FeedKnobs, start: number): Promise<FeedPageAnswer | null> {
  return ask<FeedPageAnswer>({ type: 'feed:page', knobs, start })
}

export function feedPlacesNow(name: string): Promise<FeedPlacesAnswer | null> {
  return ask<FeedPlacesAnswer>({ type: 'feed:places', name })
}

export function rememberPlaceNow(hit: PlaceHit, asked: string): Promise<{ ok: boolean } | null> {
  return ask<{ ok: boolean }>({ type: 'feed:place', hit, asked })
}

export function recordOpenedNow(
  cards: Card[],
  resumeCode: string,
  resumeLang: string,
): Promise<{ ok: boolean } | null> {
  return ask<{ ok: boolean }>({ type: 'feed:opened', cards, resumeCode, resumeLang })
}

export function reportUnseen(unseen: number): Promise<{ ok: boolean } | null> {
  return ask<{ ok: boolean }>({ type: 'feed:unseen', unseen })
}

export function watchRound(listener: (update: RoundUpdate) => void): () => void {
  const api = runtime()
  const messages = api?.runtime?.onMessage
  if (!messages) return () => {}

  const handler = (message: unknown) => {
    if (!message || typeof message !== 'object') return
    const framed = message as { type?: string; update?: RoundUpdate }
    if (framed.type !== 'round:update' || !framed.update) return
    listener(framed.update)
  }
  messages.addListener(handler)
  return () => messages.removeListener(handler)
}
