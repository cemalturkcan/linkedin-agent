import { failure } from '@/lib/linkedin/failure'
import { HOST, isPlaceId } from '@/lib/linkedin/voyager'
import { readKey, writeKeys } from '@/lib/storage'

export const TYPEAHEAD_PATH = '/voyager/api/graphql'
export const TYPEAHEAD_QUERY_ID =
  'voyagerSearchDashReusableTypeahead.4c7caa85341b17b470153ad3d1a29caf'
export const GEO_SEARCH_TYPES =
  'List(POSTCODE_1,POSTCODE_2,POPULATED_PLACE,ADMIN_DIVISION_1,ADMIN_DIVISION_2,COUNTRY_REGION,MARKET_AREA,COUNTRY_CLUSTER)'

export const PLACES_KEY = 'placeCache'
export const PLACE_CACHE_MAX = 200
export const HIT_TTL_MS = 30 * 24 * 60 * 60 * 1000
export const MISS_TTL_MS = 60 * 60 * 1000

export const NO_PLACE = 'a query that is not worldwide has to name a place'

export function unknownPlace(place: string): string {
  return `linkedin's place lookup knows nowhere called ${place}`
}

const FOLD: Record<string, string> = {
  ı: 'i',
  İ: 'i',
  ş: 's',
  ğ: 'g',
  ü: 'u',
  ö: 'o',
  ç: 'c',
}
const DIACRITIC = /\p{Diacritic}/gu

export function fold(value: string): string {
  let folded = ''
  for (const char of String(value ?? '').toLowerCase()) folded += FOLD[char] ?? char
  return folded.normalize('NFKD').replace(DIACRITIC, '')
}

const PLACE_URN = /urn:li:(?:fsd?_)?geo(?:Region)?:(\d+)/
const TEXT_KEYS = ['text', 'title', 'subtext', 'defaultLocalizedName', 'displayName', 'name', 'label']
const URN_KEYS = ['targetUrn', 'geoUrn', 'entityUrn', 'urn', 'objectUrn', 'trackingUrn', '*geo', 'geo']

export interface PlaceHit {
  locationId: string
  label: string
}

export function typeaheadUrl(place: string): string {
  const keywords = String(place ?? '').trim()
  const variables =
    `(keywords:${keywords},query:(typeaheadFilterQuery:` +
    `(geoSearchTypes:${GEO_SEARCH_TYPES}),typeaheadUseCase:JOBS),type:GEO)`
  return `${HOST}${TYPEAHEAD_PATH}?variables=${variables}&queryId=${TYPEAHEAD_QUERY_ID}`
}

function idIn(value: unknown): string {
  if (typeof value !== 'string') return ''
  const found = PLACE_URN.exec(value)
  return found ? (found[1] as string) : ''
}

function idOf(node: Record<string, unknown>): string {
  for (const key of URN_KEYS) {
    const found = idIn(node[key])
    if (found) return found
  }
  const target = node.target
  if (target && typeof target === 'object') {
    for (const value of Object.values(target as Record<string, unknown>)) {
      const found =
        idIn(value) || (value && typeof value === 'object' ? idOf(value as Record<string, unknown>) : '')
      if (found) return found
    }
  }
  return ''
}

function labelOf(node: Record<string, unknown>): string {
  for (const key of TEXT_KEYS) {
    const value = node[key]
    if (typeof value === 'string' && value.trim()) return value.trim()
    if (value && typeof value === 'object') {
      const inner = (value as { text?: unknown }).text
      if (typeof inner === 'string' && inner.trim()) return inner.trim()
    }
  }
  return ''
}

export function parseHits(document: unknown): PlaceHit[] {
  if (!document || typeof document !== 'object') return []

  const order: string[] = []
  const labels = new Map<string, string>()
  const seen = new Set<string>()

  const remember = (id: string, label: string) => {
    if (!id) return
    if (!seen.has(id)) {
      seen.add(id)
      order.push(id)
    }
    if (label && !labels.has(id)) labels.set(id, label)
  }

  const walk = (node: unknown, depth: number) => {
    if (!node || typeof node !== 'object' || depth > 8) return
    if (Array.isArray(node)) {
      for (const item of node) {
        if (typeof item === 'string') remember(idIn(item), '')
        else walk(item, depth + 1)
      }
      return
    }
    const record = node as Record<string, unknown>
    remember(idOf(record), labelOf(record))
    for (const value of Object.values(record)) {
      if (value && typeof value === 'object') walk(value, depth + 1)
    }
  }

  walk(document, 0)
  return order
    .filter((id) => labels.has(id))
    .map((id) => ({ locationId: id, label: labels.get(id) as string }))
}

export function pickHit(hits: PlaceHit[], place: string): PlaceHit | null {
  const wanted = fold(place).trim()
  const exact = hits.find((hit) => {
    const label = fold(hit.label)
    return label === wanted || (label.split(',')[0] ?? '').trim() === wanted
  })
  return exact ?? hits[0] ?? null
}

export const placeKey = (place: string): string => fold(place).trim()

interface CacheEntry {
  locationId: string
  label: string
  until: number
}

type PlaceCache = Record<string, CacheEntry>

async function readCache(): Promise<PlaceCache> {
  const stored = await readKey<unknown>(PLACES_KEY, {})
  if (!stored || typeof stored !== 'object' || Array.isArray(stored)) return {}
  return stored as PlaceCache
}

async function writeCache(key: string, hit: PlaceHit | null, ttl: number, now: number): Promise<void> {
  const cache = await readCache()
  cache[key] = {
    locationId: hit?.locationId ?? '',
    label: hit?.label ?? '',
    until: now + ttl,
  }
  const keys = Object.keys(cache)
  if (keys.length > PLACE_CACHE_MAX) {
    keys.sort((left, right) => (cache[left]?.until ?? 0) - (cache[right]?.until ?? 0))
    for (const stale of keys.slice(0, keys.length - PLACE_CACHE_MAX)) delete cache[stale]
  }
  await writeKeys({ [PLACES_KEY]: cache })
}

export async function rememberPlace(place: string, hit: PlaceHit, now = Date.now()): Promise<void> {
  const key = placeKey(place)
  if (!key || !isPlaceId(hit.locationId)) return
  await writeCache(key, { locationId: hit.locationId, label: hit.label || place }, HIT_TTL_MS, now)
}

export async function forgetPlaces(): Promise<void> {
  await writeKeys({ [PLACES_KEY]: {} })
}

export interface ResolvedPlace extends PlaceHit {
  cached: boolean
}

export interface Lookup {
  json(url: string, timeoutMs?: number): Promise<unknown>
}

export async function lookupPlaces(
  place: string,
  lookup: Lookup,
  timeoutMs?: number,
): Promise<PlaceHit[]> {
  const name = String(place ?? '').trim()
  if (!name) return []
  return parseHits(await lookup.json(typeaheadUrl(name), timeoutMs))
}

export async function resolvePlace(
  place: string,
  lookup: Lookup,
  options: { timeoutMs?: number; now?: () => number } = {},
): Promise<ResolvedPlace> {
  const name = String(place ?? '').trim()
  if (!name) throw failure('unknown-place', NO_PLACE)

  const now = options.now ?? Date.now
  const key = placeKey(name)
  const entry = (await readCache())[key]
  if (entry && Number(entry.until) > now()) {
    if (!entry.locationId) throw failure('unknown-place', unknownPlace(name))
    return { locationId: entry.locationId, label: entry.label || name, cached: true }
  }

  const found = pickHit(await lookupPlaces(name, lookup, options.timeoutMs), name)
  if (!found || !isPlaceId(found.locationId)) {
    await writeCache(key, null, MISS_TTL_MS, now())
    throw failure('unknown-place', unknownPlace(name))
  }

  await writeCache(key, found, HIT_TTL_MS, now())
  return { ...found, cached: false }
}
