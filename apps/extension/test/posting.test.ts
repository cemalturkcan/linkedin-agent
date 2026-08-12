import { expect, test } from 'bun:test'
import {
  followFor,
  followKeyOf,
  NO_FOLLOW,
  postingIdFrom,
  showsPosting,
  type Following,
} from '@/lib/linkedin/posting'

const ON_A_POSTING: Following = { screen: 'posting', postingId: '4021998877' }

test('a job page is a posting, with or without the tracking a click adds', () => {
  expect(postingIdFrom('https://www.linkedin.com/jobs/view/4021998877/')).toBe('4021998877')
  expect(
    postingIdFrom(
      'https://www.linkedin.com/jobs/view/4021998877/?alternateChannel=search&refId=abc&trackingId=x%3D%3D',
    ),
  ).toBe('4021998877')
  expect(
    postingIdFrom(
      'https://www.linkedin.com/jobs/search-results/?currentJobId=4021998877&start=25',
    ),
  ).toBe('4021998877')
  expect(postingIdFrom('https://www.linkedin.com/jobs/collections/recommended/?currentJobId=4021998877')).toBe(
    '4021998877',
  )
})

test('everything else on and off linkedin is not a posting', () => {
  for (const url of [
    'https://www.linkedin.com/feed/',
    'https://www.linkedin.com/jobs/',
    'https://www.linkedin.com/jobs/search-results/?keywords=engineer',
    'https://www.linkedin.com/company/northwind/jobs/',
    'https://www.linkedin.com/in/someone/',
    'https://notlinkedin.com/jobs/view/4021998877/',
    'about:blank',
    'chrome://newtab/',
    '',
  ]) {
    expect(postingIdFrom(url)).toBe('')
  }
})

test('an id too short to be a posting is not one', () => {
  expect(postingIdFrom('https://www.linkedin.com/jobs/view/12/')).toBe('')
  expect(postingIdFrom('https://www.linkedin.com/jobs/search-results/?currentJobId=12')).toBe('')
})

test('the active tab decides the screen, both ways', () => {
  expect(followFor({ url: 'https://www.linkedin.com/jobs/view/4021998877/', status: 'complete' }, NO_FOLLOW))
    .toEqual(ON_A_POSTING)
  expect(followFor({ url: 'https://www.linkedin.com/feed/', status: 'complete' }, ON_A_POSTING))
    .toEqual(NO_FOLLOW)
  expect(followFor(null, ON_A_POSTING)).toEqual(NO_FOLLOW)
})

test('a tab still loading holds the screen it had, so nothing flickers', () => {
  expect(followFor({ url: 'about:blank', status: 'loading' }, ON_A_POSTING)).toEqual(ON_A_POSTING)
  expect(followFor({ url: '', status: 'loading' }, NO_FOLLOW)).toEqual(NO_FOLLOW)
  expect(
    followFor({ url: 'https://www.linkedin.com/jobs/view/4021998877/', status: 'loading' }, NO_FOLLOW),
  ).toEqual(ON_A_POSTING)
})

test('moving from one posting to another follows the new one', () => {
  expect(
    followFor({ url: 'https://www.linkedin.com/jobs/view/4111222333/', status: 'complete' }, ON_A_POSTING),
  ).toEqual({ screen: 'posting', postingId: '4111222333' })
})

test('automatic switching owns the screen until he picks one himself', () => {
  const key = followKeyOf(ON_A_POSTING)

  expect(showsPosting(ON_A_POSTING, '')).toBe(true)
  expect(showsPosting(ON_A_POSTING, key)).toBe(false)
  expect(showsPosting(NO_FOLLOW, '')).toBe(false)
})

test('a deliberate choice holds for that tab and lets go when the tab changes', () => {
  const first = followKeyOf(ON_A_POSTING)
  const second: Following = { screen: 'posting', postingId: '4111222333' }

  expect(showsPosting(second, first)).toBe(true)
  expect(followKeyOf(NO_FOLLOW)).not.toBe(first)
})
