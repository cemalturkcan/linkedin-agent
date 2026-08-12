import {
  BAD_LOCATION,
  CHALLENGE,
  NOT_A_FEED,
  RATE_LIMITED,
  SIGNED_OUT,
  TIMED_OUT,
  failure,
} from '@/lib/linkedin/failure'

export const HOST = 'https://www.linkedin.com'
export const SEARCH_PATH = '/voyager/api/voyagerJobsDashJobCards'
export const POSTING_PATH = '/voyager/api/jobs/jobPostings'

const SEARCH_DECORATION =
  'com.linkedin.voyager.dash.deco.jobs.search.JobSearchCardsCollection-220'
const POSTING_DECORATION = 'com.linkedin.voyager.deco.jobs.web.shared.WebFullJobPosting-65'

export const PAGE_SIZE = 25
export const MAX_PAGES_PER_QUERY = 8
export const DEFAULT_TIMEOUT_MS = 120_000

export const RINGS = ['city', 'country', 'region', 'worldwide'] as const
export const WORLDWIDE = 'worldwide'
export const RANGES = ['r3600', 'r21600', 'r86400', 'r604800'] as const
export const SORTS = ['DD'] as const

export const ACCEPT = 'application/vnd.linkedin.normalized+json+2.1'
export const RESTLI_VERSION = '2.0.0'

const PLACE_ID = /^\d{3,}$/
const CHALLENGED = /\/checkpoint\/|\/authwall|\/uas\/login/
const NOT_A_TERM = /[^\p{L}\p{N}+#.\-/ ]+/gu

export const MAX_KEYWORD = 60

export interface SearchQuery {
  locationId: string
  keyword?: string
  range: string
  easyApply?: boolean
  remoteOnly?: boolean
}

export interface Card {
  id: string
  title: string
  company: string
  location: string
  url: string
  listedAt: number | null
  reposted: boolean
  easyApply: boolean
}

export interface SearchPage {
  jobs: Card[]
  total: number
}

export interface PostingBody {
  id: string
  description: string
  applied: boolean
  closed: boolean
}

export function isPlaceId(value: string): boolean {
  return PLACE_ID.test(String(value ?? '').trim())
}

export function locationOf(query: Pick<SearchQuery, 'locationId'>): string {
  const raw = String(query.locationId ?? '').trim()
  if (!raw) return ''
  if (!isPlaceId(raw)) throw failure('config', `${BAD_LOCATION}: ${raw}`)
  return raw
}

export function rangeOf(value: string): string {
  const wanted = String(value ?? '').trim()
  if (!(RANGES as readonly string[]).includes(wanted)) {
    throw failure('config', `${wanted || 'an empty time window'} is not a time window linkedin takes`)
  }
  return wanted
}

export function termOf(value: string | null | undefined): string {
  return String(value ?? '')
    .replace(NOT_A_TERM, ' ')
    .replace(/\s+/g, ' ')
    .trim()
    .slice(0, MAX_KEYWORD)
    .trim()
}

export function buildQuery(query: SearchQuery): string {
  const locationId = locationOf(query)
  const term = termOf(query.keyword)
  const filters = [`sortBy:List(${SORTS[0]})`, `timePostedRange:List(${rangeOf(query.range)})`]
  if (query.easyApply) filters.push('applyWithLinkedin:List(true)')
  if (query.remoteOnly) filters.push('workplaceType:List(2)')

  const parts = ['origin:JOB_SEARCH_PAGE_OTHER_ENTRY']
  if (term) parts.push(`keywords:${encodeURIComponent(term)}`)
  if (locationId) parts.push(`locationUnion:(geoId:${locationId})`)
  parts.push(`selectedFilters:(${filters.join(',')})`, 'spellCorrectionEnabled:true')
  return `(${parts.join(',')})`
}

export function searchUrl(query: SearchQuery, start: number): string {
  const offset = Math.max(0, Math.trunc(Number(start) || 0))
  return (
    `${HOST}${SEARCH_PATH}?decorationId=${SEARCH_DECORATION}` +
    `&count=${PAGE_SIZE}&q=jobSearch&query=${buildQuery(query)}&start=${offset}`
  )
}

export function postingUrl(id: string): string {
  return `${HOST}${POSTING_PATH}/${encodeURIComponent(id)}?decorationId=${POSTING_DECORATION}`
}

export function viewUrl(id: string): string {
  return `${HOST}/jobs/view/${encodeURIComponent(id)}/`
}

export function csrfFrom(cookie: string | null | undefined): string {
  const value = String(cookie ?? '').replace(/"/g, '').trim()
  if (!value) throw failure('signed-out', SIGNED_OUT)
  return value
}

export function headers(csrf: string): Record<string, string> {
  return {
    'csrf-token': csrf,
    accept: ACCEPT,
    'x-restli-protocol-version': RESTLI_VERSION,
  }
}

export function classify(response: { status: number; url?: string; type?: string }): void {
  if (response.status === 429 || response.status === 999) {
    throw failure('rate-limited', RATE_LIMITED)
  }
  if (CHALLENGED.test(response.url ?? '')) throw failure('challenge', CHALLENGE)
  if (response.status === 401 || response.status === 403) throw failure('signed-out', SIGNED_OUT)
  if (response.status < 200 || response.status >= 300) {
    throw failure('http', `linkedin answered ${response.status}`)
  }
  if (!/json/i.test(response.type ?? '')) throw failure('signed-out', SIGNED_OUT)
}

function postedAt(footer: Record<string, unknown>[], card: Record<string, unknown>): number | null {
  const dates: number[] = []
  for (const item of footer) {
    const at = Number(item?.timeAt)
    const type = String(item?.type ?? '')
    if (!Number.isFinite(at)) continue
    if (type === 'LISTED_DATE' || type === 'REPOSTED_DATE' || type === 'REPOSTED') dates.push(at)
  }
  const reposted = Number(card.repostedAt)
  if (Number.isFinite(reposted)) dates.push(reposted)
  return dates.length === 0 ? null : Math.min(...dates)
}

function text(value: unknown): string {
  if (typeof value === 'string') return value
  if (value && typeof value === 'object') {
    const inner = (value as { text?: unknown }).text
    if (typeof inner === 'string') return inner
  }
  return ''
}

export function parseCards(document: unknown): Card[] {
  if (!document || typeof document !== 'object') return []
  const included = (document as { included?: unknown }).included
  if (!Array.isArray(included)) return []

  const seen = new Set<string>()
  const cards: Card[] = []
  for (const entry of included) {
    if (!entry || typeof entry !== 'object') continue
    const card = entry as Record<string, unknown>
    const type = String(card.$type ?? '')
    if (!type.endsWith('JobPostingCard')) continue
    if (!card.jobPostingUrn) continue
    if (!String(card.entityUrn ?? '').includes('JOBS_SEARCH')) continue

    const id = String(card.jobPostingUrn).split(':').pop() ?? ''
    const title = (text(card.title) || String(card.jobPostingTitle ?? '')).trim()
    if (!id || !title || seen.has(id)) continue
    seen.add(id)

    const footer = Array.isArray(card.footerItems)
      ? (card.footerItems as Record<string, unknown>[])
      : []
    cards.push({
      id,
      title,
      company: text(card.primaryDescription).trim(),
      location: text(card.secondaryDescription).trim(),
      url: viewUrl(id),
      listedAt: postedAt(footer, card),
      reposted:
        card.repostedJob === true ||
        footer.some((item) => item?.type === 'REPOSTED_DATE' || item?.type === 'REPOSTED'),
      easyApply: footer.some((item) => item?.type === 'EASY_APPLY_TEXT'),
    })
  }
  return cards
}

export function parseTotal(document: unknown): number {
  const data = (document as { data?: { paging?: { total?: unknown } } })?.data
  const total = Number(data?.paging?.total)
  return Number.isFinite(total) && total > 0 ? Math.trunc(total) : 0
}

const APPLYING_INFO = /jobapplyinginfo/i

function isApplyingInfo(node: Record<string, unknown>): boolean {
  return (
    APPLYING_INFO.test(String(node.$type ?? '')) ||
    APPLYING_INFO.test(String(node.entityUrn ?? ''))
  )
}

export function parsePosting(id: string, document: unknown): PostingBody {
  const root = (document ?? {}) as { data?: unknown; included?: unknown }
  const nodes = [root.data, ...(Array.isArray(root.included) ? root.included : [])].filter(
    (node): node is Record<string, unknown> => Boolean(node) && typeof node === 'object',
  )

  let description = ''
  let applied = false
  let closed = false
  for (const node of nodes) {
    const body = text(node.description).trim()
    if (body.length > description.length) description = body
    if (isApplyingInfo(node)) {
      if (node.applied === true) applied = true
      if (node.closed === true) closed = true
    }
    if (node.jobState === 'CLOSED' || node.closedAt) closed = true
  }
  return { id: String(id), description, applied, closed }
}

export interface VoyagerOptions {
  session: () => Promise<string | null>
  fetcher?: typeof fetch
  timeoutMs?: number
}

export class Voyager {
  private readonly session: () => Promise<string | null>
  private readonly fetcher: typeof fetch
  private readonly timeoutMs: number
  private csrf: string | null = null

  constructor(options: VoyagerOptions) {
    this.session = options.session
    this.fetcher = options.fetcher ?? globalThis.fetch.bind(globalThis)
    this.timeoutMs = options.timeoutMs ?? DEFAULT_TIMEOUT_MS
  }

  forgetSession(): void {
    this.csrf = null
  }

  async token(): Promise<string> {
    if (this.csrf) return this.csrf
    this.csrf = csrfFrom(await this.session())
    return this.csrf
  }

  async json(url: string, timeoutMs?: number): Promise<unknown> {
    const csrf = await this.token()
    const controller = new AbortController()
    const timer = setTimeout(() => controller.abort(), timeoutMs ?? this.timeoutMs)

    let response: Response
    try {
      response = await this.fetcher(url, {
        credentials: 'include',
        redirect: 'follow',
        signal: controller.signal,
        headers: headers(csrf),
      })
    } catch (error) {
      if (controller.signal.aborted) throw failure('timeout', TIMED_OUT)
      throw failure('network', `linkedin is unreachable: ${(error as Error).message}`)
    } finally {
      clearTimeout(timer)
    }

    classify({
      status: response.status,
      url: response.url,
      type: response.headers.get('content-type') ?? '',
    })

    try {
      return await response.json()
    } catch {
      throw failure('http', NOT_A_FEED)
    }
  }

  async search(query: SearchQuery, start: number, timeoutMs?: number): Promise<SearchPage> {
    const document = await this.json(searchUrl(query, start), timeoutMs)
    return { jobs: parseCards(document), total: parseTotal(document) }
  }

  async posting(id: string, timeoutMs?: number): Promise<PostingBody> {
    return parsePosting(id, await this.json(postingUrl(id), timeoutMs))
  }
}
