import { beforeEach, expect, test } from 'bun:test'
import typeahead from './fixtures/typeahead.json'
import { LinkedInFailure } from '@/lib/linkedin/failure'
import {
  GEO_SEARCH_TYPES,
  TYPEAHEAD_QUERY_ID,
  forgetPlaces,
  parseHits,
  pickHit,
  rememberPlace,
  resolvePlace,
  typeaheadUrl,
} from '@/lib/linkedin/geo'
import { resetMemoryStore } from '@/lib/storage'

function lookup(answers: unknown[]): { json: (url: string) => Promise<unknown>; urls: string[] } {
  const urls: string[] = []
  let call = 0
  return {
    urls,
    async json(url: string) {
      urls.push(url)
      const answer = answers[Math.min(call, answers.length - 1)]
      call += 1
      if (answer instanceof Error) throw answer
      return answer
    },
  }
}

beforeEach(async () => {
  resetMemoryStore()
  await forgetPlaces()
})

test('the typeahead url is the graphql call linkedin itself makes', () => {
  const url = typeaheadUrl('Den Haag')
  expect(url).toBe(
    'https://www.linkedin.com/voyager/api/graphql?variables=' +
      `(keywords:Den Haag,query:(typeaheadFilterQuery:(geoSearchTypes:${GEO_SEARCH_TYPES}),` +
      `typeaheadUseCase:JOBS),type:GEO)&queryId=${TYPEAHEAD_QUERY_ID}`,
  )
  expect(url).toContain('/voyager/api/graphql')
  expect(url).toContain('typeaheadUseCase:JOBS')
  expect(url).toContain('POPULATED_PLACE')
  expect(url).not.toContain('hitsV2')
  expect(url).not.toContain('%28')
  expect(url).not.toContain('%2C')
})

test('both answer shapes parse, flat hits and normalized urn lists', () => {
  expect(parseHits(typeahead.flat)).toEqual([
    { locationId: '102135806', label: 'Rotterdam, South Holland, Netherlands' },
    { locationId: '105011081', label: 'Rotterdam, Missouri, United States' },
  ])
  expect(parseHits(typeahead.normalized)).toEqual([
    { locationId: '100565514', label: 'Munich, Bavaria, Germany' },
    { locationId: '101282230', label: 'Germany' },
  ])
  expect(parseHits(typeahead.empty)).toEqual([])
  expect(parseHits(null)).toEqual([])
})

test('the top hit wins unless a lower one is literally the name asked for', () => {
  const hits = parseHits(typeahead.normalized)
  expect(pickHit(hits, 'Munich')?.locationId).toBe('100565514')
  expect(pickHit(hits, 'Germany')?.locationId).toBe('101282230')
  expect(pickHit(hits, 'somewhere else')?.locationId).toBe('100565514')
  expect(pickHit([], 'Munich')).toBeNull()
})

test('a name resolves once and the second ask costs no request', async () => {
  const linkedin = lookup([typeahead.normalized])
  const first = await resolvePlace('Munich', linkedin)
  expect(first).toEqual({ locationId: '100565514', label: 'Munich, Bavaria, Germany', cached: false })

  const second = await resolvePlace('munich', linkedin)
  expect(second.cached).toBe(true)
  expect(second.locationId).toBe('100565514')
  expect(linkedin.urls.length).toBe(1)
})

test('a pick seeds the cache so the round that follows never asks', async () => {
  const linkedin = lookup([typeahead.empty])
  await rememberPlace('Munich', { locationId: '100565514', label: 'Munich, Bavaria, Germany' })

  const resolved = await resolvePlace('Munich', linkedin)
  expect(resolved.cached).toBe(true)
  expect(linkedin.urls.length).toBe(0)
})

test('a name linkedin does not know is an unknown place, and the miss is held briefly', async () => {
  const linkedin = lookup([typeahead.empty])
  await expect(resolvePlace('Atlantis', linkedin)).rejects.toThrow(/knows nowhere called Atlantis/)

  let kind = ''
  try {
    await resolvePlace('Atlantis', linkedin)
  } catch (error) {
    kind = (error as LinkedInFailure).kind
  }
  expect(kind).toBe('unknown-place')
  expect(linkedin.urls.length).toBe(1)
})

test('a query that is not worldwide has to name a place', async () => {
  const linkedin = lookup([typeahead.empty])
  await expect(resolvePlace('', linkedin)).rejects.toThrow(/has to name a place/)
  expect(linkedin.urls.length).toBe(0)
})

test('a linkedin failure during the lookup stays a linkedin failure', async () => {
  const rateLimited = new LinkedInFailure('rate-limited', 'linkedin is rate limiting this session')
  const linkedin = lookup([rateLimited])
  await expect(resolvePlace('Munich', linkedin)).rejects.toThrow(/rate limiting/)
})

test('a cache hit that is not a place id is never turned into a query', async () => {
  const linkedin = lookup([{ elements: [{ text: { text: 'Nowhere' }, targetUrn: 'urn:li:fs_geo:no' }] }])
  await expect(resolvePlace('Nowhere', linkedin)).rejects.toThrow(/knows nowhere/)
})
