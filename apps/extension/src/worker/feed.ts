import type { AgentClient } from '@/lib/agent/client'
import { backsOff, endsSession, kindOf, sentenceOf } from '@/lib/linkedin/failure'
import { lookupPlaces, rememberPlace, type Lookup, type PlaceHit } from '@/lib/linkedin/geo'
import { searchFor, type FeedKnobs, type FeedPage } from '@/lib/linkedin/feed'
import { clockAt, type BackoffPort, type LinkedInPort } from '@/lib/linkedin/round'

export interface FeedPorts {
  agent: AgentClient
  linkedin: LinkedInPort
  lookup: Lookup
  backoff: BackoffPort
  timeoutMs: number
}

export interface Blocked {
  ok: false
  message: string
}

export type FeedAnswer = ({ ok: true } & FeedPage) | Blocked

export interface PlacesAnswer {
  ok: boolean
  hits?: PlaceHit[]
  message?: string
}

async function held(ports: FeedPorts): Promise<Blocked | null> {
  const until = await ports.backoff.held()
  if (until <= 0) return null
  return {
    ok: false,
    message: `linkedin asked this session to slow down, so the feed is off until ${clockAt(until)}`,
  }
}

async function blocked(ports: FeedPorts, error: unknown): Promise<Blocked> {
  if (kindOf(error) === null) throw error
  const message = sentenceOf(error)
  if (backsOff(error)) {
    const until = await ports.backoff.escalate()
    return { ok: false, message: `${message}, holding off until ${clockAt(until)}` }
  }
  if (endsSession(error)) {
    ports.linkedin.forgetSession()
    return { ok: false, message }
  }
  return { ok: false, message }
}

export async function feedPlaces(ports: FeedPorts, name: string): Promise<PlacesAnswer> {
  const waiting = await held(ports)
  if (waiting) return { ok: false, message: waiting.message }
  try {
    return { ok: true, hits: await lookupPlaces(name, ports.lookup, ports.timeoutMs) }
  } catch (error) {
    return { ok: false, message: (await blocked(ports, error)).message }
  }
}

export async function rememberChosenPlace(
  ports: FeedPorts,
  hit: PlaceHit,
  asked: string,
): Promise<void> {
  await rememberPlace(asked || hit.label, hit)
  await ports.agent.savePlaces([
    {
      name: hit.label,
      asked: asked || hit.label,
      ring: '',
      locationId: hit.locationId,
      source: 'hand',
    },
  ])
}

export async function feedPage(
  ports: FeedPorts,
  knobs: FeedKnobs,
  start: number,
): Promise<FeedAnswer> {
  const waiting = await held(ports)
  if (waiting) return waiting

  const wanted = { ...knobs }
  if (!wanted.locationId && wanted.place) {
    try {
      const place = await ports.linkedin.place(wanted.place, ports.timeoutMs)
      wanted.locationId = place.locationId
      wanted.place = place.label || wanted.place
    } catch (error) {
      if (kindOf(error) === 'unknown-place') return { ok: false, message: sentenceOf(error) }
      return blocked(ports, error)
    }
  }

  try {
    const page = await ports.linkedin.search(searchFor(wanted), start, ports.timeoutMs)
    await ports.backoff.clear()
    return { ok: true, cards: page.jobs, total: page.total, start }
  } catch (error) {
    return blocked(ports, error)
  }
}
