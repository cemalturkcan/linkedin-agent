import { expect, test } from 'bun:test'
import search from './fixtures/voyager-search.json'
import posting from './fixtures/voyager-posting.json'
import { LinkedInFailure } from '@/lib/linkedin/failure'
import {
  ACCEPT,
  buildQuery,
  classify,
  csrfFrom,
  headers,
  parseCards,
  parsePosting,
  parseTotal,
  postingUrl,
  RESTLI_VERSION,
  searchUrl,
  Voyager,
} from '@/lib/linkedin/voyager'

const CITY = { locationId: '102135806', range: 'r86400' }

test('a city query carries a location clause and worldwide carries none', () => {
  expect(buildQuery(CITY)).toBe(
    '(origin:JOB_SEARCH_PAGE_OTHER_ENTRY,locationUnion:(geoId:102135806),selectedFilters:(sortBy:List(DD),timePostedRange:List(r86400)),spellCorrectionEnabled:true)',
  )
  expect(buildQuery({ locationId: '', range: 'r86400' })).toBe(
    '(origin:JOB_SEARCH_PAGE_OTHER_ENTRY,selectedFilters:(sortBy:List(DD),timePostedRange:List(r86400)),spellCorrectionEnabled:true)',
  )
  expect(buildQuery({ locationId: '', range: 'r86400' })).not.toContain('locationUnion')
  expect(buildQuery({ locationId: '', range: 'r86400' })).not.toContain('geoId:)')
})

test('the filters the planner may ask for land in the query', () => {
  const both = buildQuery({ ...CITY, easyApply: true, remoteOnly: true })
  expect(both).toContain('applyWithLinkedin:List(true)')
  expect(both).toContain('workplaceType:List(2)')
  expect(buildQuery(CITY)).not.toContain('applyWithLinkedin')
  expect(buildQuery(CITY)).not.toContain('workplaceType')
})

test('the rest.li query keeps its punctuation literal and never carries a keyword', () => {
  const url = searchUrl(CITY, 50)
  const query = url.slice(url.indexOf('&query=') + '&query='.length, url.indexOf('&start='))

  expect(query.startsWith('(')).toBe(true)
  expect(query.endsWith(')')).toBe(true)
  expect(query).not.toContain('%28')
  expect(query).not.toContain('%29')
  expect(query).not.toContain('%3A')
  expect(query).not.toContain('%2C')
  expect(url).not.toContain('keywords')
  expect(url).toContain('&start=50')
  expect(url).toContain('&count=25')
  expect(url).toContain('q=jobSearch')
  expect(url.startsWith('https://www.linkedin.com/voyager/api/voyagerJobsDashJobCards?')).toBe(true)
})

test('sorting is newest first and nothing else is offered', () => {
  expect(buildQuery(CITY)).toContain('sortBy:List(DD)')
})

test('a location that is not a place id is refused before a request is spent', () => {
  expect(() => buildQuery({ locationId: 'Rotterdam', range: 'r86400' })).toThrow(LinkedInFailure)
  expect(() => buildQuery({ locationId: '12', range: 'r86400' })).toThrow(/not a linkedin place id/)
  expect(() => buildQuery({ locationId: '', range: 'r999' })).toThrow(/not a time window/)
})

test('the posting url escapes only the id', () => {
  expect(postingUrl('4051234567')).toBe(
    'https://www.linkedin.com/voyager/api/jobs/jobPostings/4051234567?decorationId=com.linkedin.voyager.deco.jobs.web.shared.WebFullJobPosting-65',
  )
  expect(postingUrl('a b/c')).toContain('/jobPostings/a%20b%2Fc?')
})

test('the csrf token is the session cookie with its quotes stripped', () => {
  expect(csrfFrom('"ajax:1234567890"')).toBe('ajax:1234567890')
  expect(() => csrfFrom('')).toThrow(/sign in again/)
  expect(() => csrfFrom(null)).toThrow(LinkedInFailure)
  expect(headers('ajax:1')).toEqual({
    'csrf-token': 'ajax:1',
    accept: ACCEPT,
    'x-restli-protocol-version': RESTLI_VERSION,
  })
})

test('every failure linkedin can answer with is told apart', () => {
  const kinds: [number, string, string][] = [
    [429, 'https://www.linkedin.com/voyager/api/x', 'rate-limited'],
    [999, 'https://www.linkedin.com/voyager/api/x', 'rate-limited'],
    [200, 'https://www.linkedin.com/checkpoint/challenge', 'challenge'],
    [200, 'https://www.linkedin.com/authwall', 'challenge'],
    [401, 'https://www.linkedin.com/voyager/api/x', 'signed-out'],
    [403, 'https://www.linkedin.com/voyager/api/x', 'signed-out'],
    [500, 'https://www.linkedin.com/voyager/api/x', 'http'],
  ]
  for (const [status, url, kind] of kinds) {
    let caught: unknown = null
    try {
      classify({ status, url, type: 'application/json' })
    } catch (error) {
      caught = error
    }
    expect((caught as LinkedInFailure).kind).toBe(kind as never)
  }
  expect(() => classify({ status: 200, url: 'https://www.linkedin.com/x', type: 'text/html' })).toThrow(
    /sign in again/,
  )
  expect(() =>
    classify({ status: 200, url: 'https://www.linkedin.com/x', type: 'application/json' }),
  ).not.toThrow()
})

test('a captured search page parses into cards and drops what is not one', () => {
  const cards = parseCards(search)
  expect(cards.map((card) => card.id)).toEqual(['4051234567', '4051234568', '4051234571'])
  expect(parseTotal(search)).toBe(61)

  const [first, reposted, plain] = cards
  expect(first?.title).toBe('Backend Engineer')
  expect(first?.company).toBe('Northwind')
  expect(first?.easyApply).toBe(true)
  expect(first?.url).toBe('https://www.linkedin.com/jobs/view/4051234567/')
  expect(reposted?.reposted).toBe(true)
  expect(reposted?.listedAt).toBe(1769000000000)
  expect(plain?.title).toBe('Platform Engineer')
  expect(plain?.easyApply).toBe(false)
})

test('a posting keeps the longest body and reads linkedin own applied state', () => {
  const body = parsePosting('4051234567', posting)
  expect(body.id).toBe('4051234567')
  expect(body.title).toBe('Backend Engineer')
  expect(body.description.startsWith('We are hiring a backend engineer')).toBe(true)
  expect(body.applied).toBe(true)
  expect(body.closed).toBe(false)
  expect(parsePosting('1', { data: { jobState: 'CLOSED' } }).closed).toBe(true)
  expect(
    parsePosting('1', {
      included: [
        { $type: 'com.linkedin.voyager.entities.shared.JobApplyingInfo', applied: false, closed: true },
      ],
    }),
  ).toMatchObject({ applied: false, closed: true })
  expect(
    parsePosting('1', { included: [{ entityUrn: 'urn:li:fs_normalized_company:9', applied: true }] })
      .applied,
  ).toBe(false)
})

test('the client sends the session headers and classifies before parsing', async () => {
  const seen: { url: string; init?: RequestInit }[] = []
  const client = new Voyager({
    session: async () => '"ajax:99"',
    fetcher: (async (input: RequestInfo | URL, init?: RequestInit) => {
      seen.push({ url: String(input), init })
      return new Response(JSON.stringify(search), {
        status: 200,
        headers: { 'content-type': 'application/vnd.linkedin.normalized+json+2.1' },
      })
    }) as typeof fetch,
  })

  const page = await client.search(CITY, 0)
  expect(page.jobs.length).toBe(3)
  expect(page.total).toBe(61)

  const call = seen[0]
  expect(call?.init?.credentials).toBe('include')
  expect((call?.init?.headers as Record<string, string>)['csrf-token']).toBe('ajax:99')
  expect((call?.init?.headers as Record<string, string>).accept).toBe(ACCEPT)
  expect((call?.init?.headers as Record<string, string>)['x-restli-protocol-version']).toBe('2.0.0')
})

test('an authwall answering 200 with html reads as a lost session, not a feed', async () => {
  const client = new Voyager({
    session: async () => 'ajax:99',
    fetcher: (async (_input: RequestInfo | URL) =>
      new Response('<html></html>', {
        status: 200,
        headers: { 'content-type': 'text/html' },
      })) as typeof fetch,
  })
  await expect(client.search(CITY, 0)).rejects.toThrow(/sign in again/)
})

test('a timeout is a timeout, not a lost session', async () => {
  const client = new Voyager({
    session: async () => 'ajax:99',
    timeoutMs: 5,
    fetcher: ((_input: RequestInfo | URL, init?: RequestInit) =>
      new Promise((_resolve, reject) => {
        init?.signal?.addEventListener('abort', () => reject(new Error('aborted')))
      })) as typeof fetch,
  })
  await expect(client.search(CITY, 0)).rejects.toThrow(/did not answer in time/)
})

test('the default fetcher survives being called as a method', async () => {
  const real = globalThis.fetch
  let calls = 0
  globalThis.fetch = function (this: unknown) {
    if (this !== undefined && this !== globalThis) throw new TypeError('Illegal invocation')
    calls += 1
    return Promise.resolve(new Response('{}', { status: 200 }))
  } as unknown as typeof fetch

  try {
    const client = new Voyager({ session: async () => 'JSESSIONID="ajax:1234567890"' })
    const failed = await client
      .json('https://www.linkedin.com/voyager/api/probe')
      .then(() => null)
      .catch((error: Error) => error.message)
    expect(calls).toBe(1)
    expect(failed ?? '').not.toContain('Illegal invocation')
  } finally {
    globalThis.fetch = real
  }
})
