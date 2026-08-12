import { readFileSync } from 'node:fs'
import { expect, test } from 'bun:test'
import { renderToStaticMarkup } from 'react-dom/server'
import { createElement } from 'react'
import type { Failure } from '@/lib/agent/client'
import type {
  CredentialStatus,
  Posting,
  ProfileState,
  SettingsDocument,
  SetupState,
  TraceList,
} from '@/lib/agent/types'
import { landingPlace, placeFromHash } from '@/desk/Desk'
import {
  ANY_TAG,
  PostingsView,
  countriesIn,
  countryOf,
  filterPostings,
  tagsIn,
} from '@/desk/components/PostingsView'
import { ProfileView } from '@/desk/components/ProfileView'
import { SettingsView } from '@/desk/components/SettingsView'
import { SetupView } from '@/desk/components/SetupView'
import { TraceView } from '@/desk/components/TraceView'
import { outageLine, pending, type Resource } from '@/desk/useDeskState'

const OUTAGE: Failure = {
  kind: 'unreachable',
  status: 0,
  reason: 'unreachable',
  message: 'the agent is not answering on http://127.0.0.1:8787, start it',
}

function held<T>(value: T): Resource<T> {
  return { value, live: true, at: 1, failure: null }
}

function cached<T>(value: T): Resource<T> {
  return { value, live: false, at: 1, failure: OUTAGE }
}

function silent<T>(): Resource<T> {
  return { ...pending<T>(), failure: OUTAGE }
}

function text(node: Parameters<typeof renderToStaticMarkup>[0]): string {
  return renderToStaticMarkup(node)
    .replace(/<[^>]+>/g, ' ')
    .replace(/&#x27;/g, "'")
    .replace(/\s+/g, ' ')
    .trim()
}

function posting(patch: Partial<Posting>): Posting {
  return {
    id: 'p1',
    title: 'a role',
    company: 'a company',
    location: 'somewhere',
    url: 'https://www.linkedin.com/jobs/view/p1/',
    dupeOf: null,
    description: '',
    descriptionState: 'none',
    listedAt: null,
    easyApply: true,
    status: 'applied',
    stage: 'deep',
    score: 80,
    triageReason: null,
    verdictReason: 'the core work matches',
    seniority: 'senior',
    workplace: 'remote',
    contractType: 'full-time',
    postingLang: 'en',
    agency: false,
    statedPay: null,
    resumeCode: 'XX',
    resumeLang: 'en',
    resumeFit: 'partial',
    tailoredResume: { needed: true, focus: 'lead with the work this posting is built on' },
    stale: false,
    outcome: null,
    outcomeNote: null,
    cycleId: null,
    queryLabel: 'a query',
    createdAt: '',
    updatedAt: '',
    ...patch,
  }
}

const NO_PROFILE: Resource<ProfileState> = pending<ProfileState>()

test('the outage banner says the page is showing the last thing the agent said', () => {
  expect(outageLine(OUTAGE.message)).toBe(
    'the agent is not answering on http://127.0.0.1:8787, start it. everything below is the last thing it said, not what is true now.',
  )
})

test('a cached setup chain is marked cached, and an unknown one is not drawn at all', () => {
  const state: SetupState = {
    configured: true,
    ready: false,
    missing: ['a folder holding your cv pdfs'],
    indexState: 'never',
    indexing: false,
    resumeDir: '/somewhere',
    candidates: [],
    resumes: [{ code: 'XX', label: 'a variant', role: 'a role', languages: ['en'] }],
    credentials: {
      loaded: true,
      source: 'file',
      where: 'a source',
      stored: false,
      expiresInMinutes: 60,
      refreshable: true,
      error: null,
    },
    progress: { phase: 'idle', done: 0, total: 0 },
  }

  const drawn = text(
    createElement(SetupView, {
      setup: cached(state),
      credentials: pending<CredentialStatus>(),
      profile: NO_PROFILE,
      plugin: null,
      busy: null,
      onFolder: async () => false,
      onPaste: async () => false,
      onClear: async () => {},
      onIndex: async () => {},
    }),
  )
  expect(drawn).toContain('cached')
  expect(drawn).toContain('claude credentials')

  const blank = text(
    createElement(SetupView, {
      setup: silent<SetupState>(),
      credentials: pending<CredentialStatus>(),
      profile: NO_PROFILE,
      plugin: null,
      busy: null,
      onFolder: async () => false,
      onPaste: async () => false,
      onClear: async () => {},
      onIndex: async () => {},
    }),
  )
  expect(blank).toContain('none of the four links is known')
  expect(blank).not.toContain('cached')
  expect(blank).not.toContain('claude credentials done')
})

test('the three index states are three different sentences, and neither is an error', () => {
  const base: ProfileState = {
    resumeDir: '/somewhere',
    candidate: null,
    indexedAt: '',
    resumes: [],
    indexState: 'never',
    indexing: false,
    progress: { phase: 'idle', done: 0, total: 0 },
  }

  const never = text(
    createElement(ProfileView, {
      profile: held(base),
      busy: null,
      onIndex: async () => {},
    }),
  )
  const stale = text(
    createElement(ProfileView, {
      profile: held<ProfileState>({ ...base, indexState: 'stale' }),
      busy: null,
      onIndex: async () => {},
    }),
  )
  const running = text(
    createElement(ProfileView, {
      profile: held({
        ...base,
        indexing: true,
        progress: { phase: 'profiling', done: 2, total: 5 },
      }),
      busy: null,
      onIndex: async () => {},
    }),
  )

  expect(never).toContain('have never been read')
  expect(stale).toContain('changed since the last read')
  expect(running).toContain('reading them now')
  for (const drawn of [never, stale, running]) {
    expect(drawn).not.toContain('error')
  }
})

test('an unanswered trace list reads as silence, not as idle', () => {
  const answered = text(
    createElement(TraceView, {
      traces: held<TraceList>({ running: null, traces: [] }),
      running: null,
      cursor: 0,
      openedId: null,
      onOpen: () => {},
      onCursor: () => {},
      read: async () => ({ ok: false as const, failure: OUTAGE }),
    }),
  )
  const silentList = text(
    createElement(TraceView, {
      traces: silent<TraceList>(),
      running: null,
      cursor: 0,
      openedId: null,
      onOpen: () => {},
      onCursor: () => {},
      read: async () => ({ ok: false as const, failure: OUTAGE }),
    }),
  )

  expect(answered).toContain('idle')
  expect(answered).toContain('nothing recorded yet')
  expect(silentList).toContain('a call could be running and this page would not know')
  expect(silentList).toContain('this is silence, not an empty list')
})

test('the postings view carries the detail the panel truncates', () => {
  const rows = [posting({})]
  const drawn = text(
    createElement(PostingsView, {
      jobs: held(rows),
      rows,
      filter: 'applied',
      tag: '',
      tags: [],
      country: '',
      countries: [],
      cursor: 0,
      openedId: 'p1',
      reflection: null,
      busy: null,
      onFilter: () => {},
      onTag: () => {},
      onCountry: () => {},
      onCursor: () => {},
      onOpen: () => {},
      onVisit: () => {},
      onRecord: async () => false,
      onReflect: () => {},
    }),
  )

  expect(drawn).toContain('the core work matches')
  expect(drawn).toContain('lead with the work this posting is built on')
  expect(drawn).toContain('partial, a variant overlaps but leads elsewhere')
  expect(drawn).toContain('the a query query')
  expect(drawn).toContain('outcome')
  expect(drawn).toContain('no-response')
})

test('an opened posting names the verdict, the score and the round that found it', () => {
  const rows = [posting({ status: 'applied', score: 88, cycleId: 4 })]
  const drawn = text(
    createElement(PostingsView, {
      jobs: held(rows),
      rows,
      filter: 'all',
      tag: '',
      tags: [],
      country: '',
      countries: [],
      cursor: 0,
      openedId: 'p1',
      reflection: null,
      busy: null,
      onFilter: () => {},
      onTag: () => {},
      onCountry: () => {},
      onCursor: () => {},
      onOpen: () => {},
      onVisit: () => {},
      onRecord: async () => false,
      onReflect: () => {},
    }),
  )

  expect(drawn).toContain('apply, and you sent it, scored 88')
  expect(drawn).toContain('round 4, the a query query')

  const skipped = text(
    createElement(PostingsView, {
      jobs: held([posting({ status: 'skipped', score: 12 })]),
      rows: [posting({ status: 'skipped', score: 12 })],
      filter: 'all',
      tag: '',
      tags: [],
      country: '',
      countries: [],
      cursor: 0,
      openedId: 'p1',
      reflection: null,
      busy: null,
      onFilter: () => {},
      onTag: () => {},
      onCountry: () => {},
      onCursor: () => {},
      onOpen: () => {},
      onVisit: () => {},
      onRecord: async () => false,
      onReflect: () => {},
    }),
  )
  expect(skipped).toContain('skip, scored 12')
})

test('the work list draws oldest first, so the longest wait is the first row', () => {
  const hour = 60 * 60 * 1000
  const now = Date.parse('2026-01-01T12:00:00.000Z')
  const rows = filterPostings(
    [
      posting({ id: 'newest', title: 'newest posting', listedAt: now - hour }),
      posting({ id: 'oldest', title: 'oldest posting', listedAt: now - 50 * hour }),
      posting({ id: 'middle', title: 'middle posting', listedAt: now - 8 * hour }),
    ],
    'all',
  )

  expect(rows.map((row) => row.id)).toEqual(['oldest', 'middle', 'newest'])

  const drawn = text(
    createElement(PostingsView, {
      jobs: held(rows),
      rows,
      filter: 'all',
      tag: '',
      tags: [],
      country: '',
      countries: [],
      cursor: 0,
      openedId: null,
      reflection: null,
      busy: null,
      onFilter: () => {},
      onTag: () => {},
      onCountry: () => {},
      onCursor: () => {},
      onOpen: () => {},
      onVisit: () => {},
      onRecord: async () => false,
      onReflect: () => {},
    }),
  )

  expect(drawn.indexOf('oldest posting')).toBeLessThan(drawn.indexOf('middle posting'))
  expect(drawn.indexOf('middle posting')).toBeLessThan(drawn.indexOf('newest posting'))
})

test('the desk lands on the work when the chain holds, and on setup when a link is short', () => {
  function chain(ready: boolean): SetupState {
    return {
      configured: ready,
      ready,
      missing: ready ? [] : ['a folder holding your cv pdfs'],
      indexState: ready ? 'current' : 'never',
      indexing: false,
      resumeDir: ready ? '/somewhere' : '',
      candidates: [],
      resumes: [],
      credentials: {
        loaded: ready,
        source: 'file',
        where: 'a source',
        stored: false,
        expiresInMinutes: 60,
        refreshable: true,
        error: null,
      },
      progress: { phase: 'idle', done: 0, total: 0 },
    }
  }

  expect(landingPlace(held(chain(true)))).toEqual({ view: 'work', section: 'settings' })
  expect(landingPlace(held(chain(false)))).toEqual({ view: 'settings', section: 'setup' })
  expect(landingPlace(silent<SetupState>())).toEqual({ view: 'settings', section: 'setup' })
  expect(landingPlace(pending<SetupState>())).toBeNull()
})

test('a hash names a view or a section, and anything else leaves the desk where it stands', () => {
  expect(placeFromHash('#work', null)).toEqual({ view: 'work', section: 'settings' })
  expect(placeFromHash('#rounds', null)).toEqual({ view: 'rounds', section: 'settings' })
  expect(placeFromHash('#settings', null)).toEqual({ view: 'settings', section: 'settings' })
  expect(placeFromHash('#setup', null)).toEqual({ view: 'settings', section: 'setup' })
  expect(placeFromHash('#profile', null)).toEqual({ view: 'settings', section: 'profile' })
  expect(placeFromHash('#model', null)).toEqual({ view: 'settings', section: 'model' })
  expect(placeFromHash('#work', { view: 'settings', section: 'model' })).toEqual({
    view: 'work',
    section: 'model',
  })
  expect(placeFromHash('#nothing', null)).toBeNull()
})

test('the posting filter is the status filter and all keeps everything', () => {
  const rows = [
    posting({ id: 'a', status: 'applied' }),
    posting({ id: 'b', status: 'skipped' }),
    posting({ id: 'c', status: 'inbox' }),
  ]
  expect(filterPostings(rows, 'applied').map((row) => row.id)).toEqual(['a'])
  expect(filterPostings(rows, 'skipped').map((row) => row.id)).toEqual(['b'])
  expect(filterPostings(rows, 'all')).toHaveLength(3)
})

test('the work view can be narrowed to the cv the agent picked', () => {
  const rows = [
    posting({ id: 'a', status: 'inbox', resumeCode: 'JA', listedAt: 1 }),
    posting({ id: 'b', status: 'inbox', resumeCode: 'FR', listedAt: 2 }),
    posting({ id: 'c', status: 'inbox', resumeCode: 'JA', listedAt: 3 }),
    posting({ id: 'd', status: 'skipped', resumeCode: 'JA', listedAt: 4 }),
    posting({ id: 'e', status: 'inbox', resumeCode: null, listedAt: 5 }),
  ]

  expect(filterPostings(rows, 'inbox', 'JA').map((row) => row.id)).toEqual(['a', 'c'])
  expect(filterPostings(rows, 'inbox', ANY_TAG).map((row) => row.id)).toEqual(['a', 'b', 'c', 'e'])
  expect(filterPostings(rows, 'all', 'JA').map((row) => row.id)).toEqual(['a', 'c', 'd'])

  const inbox = filterPostings(rows, 'inbox')
  expect(tagsIn(inbox, { JA: 'Java', FR: 'Frontend' })).toEqual([
    { code: 'JA', label: 'Java', count: 2 },
    { code: 'FR', label: 'Frontend', count: 1 },
  ])
  expect(tagsIn(inbox, {})[0]?.label).toBe('JA')
})

test('the panel sends a fresh install to setup instead of drawing empty lists', () => {
  const panel = readFileSync(
    new URL('../src/panel/Panel.tsx', import.meta.url).pathname,
    'utf8',
  )

  expect(panel).toContain('!state.setup.value.configured')
  expect(panel).toContain("openDesk('setup')")
  expect(panel).toContain('nothing is set up yet')
})

test('a posting is filed under the country its location ends with', () => {
  expect(countryOf('Üsküdar, Istanbul, Türkiye (Hybrid)')).toBe('Türkiye')
  expect(countryOf('Istanbul, Türkiye (Remote)')).toBe('Türkiye')
  expect(countryOf('Türkiye (Remote)')).toBe('Türkiye')
  expect(countryOf('Berlin, Germany')).toBe('Germany')
  expect(countryOf('European Union (Remote)')).toBe('European Union')
  expect(countryOf('')).toBe('')
  expect(countryOf(null)).toBe('')
})

test('the work view can be narrowed to one country, and the counts follow', () => {
  const rows = [
    posting({ id: 'a', status: 'inbox', resumeCode: 'JA', location: 'Istanbul, Türkiye (Hybrid)', listedAt: 1 }),
    posting({ id: 'b', status: 'inbox', resumeCode: 'JA', location: 'Berlin, Germany (Remote)', listedAt: 2 }),
    posting({ id: 'c', status: 'inbox', resumeCode: 'FR', location: 'Ankara, Türkiye (On-site)', listedAt: 3 }),
    posting({ id: 'd', status: 'skipped', resumeCode: 'JA', location: 'Istanbul, Türkiye', listedAt: 4 }),
  ]
  const inbox = filterPostings(rows, 'inbox')

  expect(countriesIn(inbox)).toEqual([
    { code: 'Türkiye', label: 'Türkiye', count: 2 },
    { code: 'Germany', label: 'Germany', count: 1 },
  ])
  expect(filterPostings(rows, 'inbox', ANY_TAG, 'Türkiye').map((row) => row.id)).toEqual(['a', 'c'])
  expect(filterPostings(rows, 'inbox', 'JA', 'Türkiye').map((row) => row.id)).toEqual(['a'])
  expect(filterPostings(rows, 'all', ANY_TAG, 'Türkiye').map((row) => row.id)).toEqual(['a', 'c', 'd'])

  const inTurkiye = filterPostings(inbox, 'all', ANY_TAG, 'Türkiye')
  expect(tagsIn(inTurkiye, {}).map((entry) => entry.code)).toEqual(['FR', 'JA'])
})

test('a settings field the api has never heard of does not blank the screen', () => {
  const older = {
    settings: {
      locations: {
        places: [],
        relocation: { open: true, targets: [], notes: '' },
        workplace: { onsite: true, hybrid: true, remote: true, scope: 'global' },
        authorization: '',
      },
      roles: {
        seniority: { min: 'junior', max: 'staff' },
        excludeStacks: [],
        excludeIndustries: [],
        contractTypes: ['full-time'],
        minCompensation: { amount: 0, currency: '', period: 'year', hardFilter: false },
        applyToNonEasyApply: false,
      },
      companies: { blocked: [], preferred: [], excludeAgencies: false, reapplyCooldownDays: 90, dedupe: true },
      apply: {
        autoAttach: true,
        uploadFileName: 'my-cv.pdf',
        uploadFileNameMode: 'one-name',
        resumeLanguages: ['en'],
        paused: false,
      },
      budget: {
        maxQueriesPerCycle: 8,
        maxScreenPerCycle: 200,
        maxModelCallsPerCycle: 0,
        maxPagesPerQuery: 15,
        cycleMinutes: 60,
        autoCycle: true,
        dailyModelCallCap: 0,
        retentionDays: 30,
      },
      harvest: {
        newJobTarget: 100,
        maxWideningSteps: 2,
        roundDeadlineMs: 3600000,
        requestDelayMs: 900,
        requestTimeoutMs: 120000,
      },
      operatorNotes: '',
    },
    enums: {
      seniorityLadder: ['junior', 'mid', 'senior', 'staff'],
      locationKinds: ['commute'],
      placeRings: ['city', 'country', 'region', 'worldwide'],
      workplaceScopes: ['local', 'country', 'region', 'global'],
      contractTypes: ['full-time'],
      payPeriods: ['year'],
      uploadNameModes: ['one-name', 'per-variant'],
    },
  } as unknown as SettingsDocument

  const drawn = text(
    createElement(SettingsView, {
      settings: held(older),
      profile: pending<ProfileState>(),
      busy: null,
      onSave: async () => true,
    }),
  )

  expect(drawn).toContain('posting language')
})
