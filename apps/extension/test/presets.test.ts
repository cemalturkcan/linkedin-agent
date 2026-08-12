import { beforeEach, expect, test } from 'bun:test'
import type { ProfileState } from '@/lib/agent/types'
import {
  BROAD_ID,
  cachePresets,
  cachedPresets,
  derivePresets,
  isDerived,
} from '@/lib/presets'
import { resetMemoryStore } from '@/lib/storage'

function profile(): ProfileState {
  return {
    resumeDir: '/somewhere/cv',
    candidate: null,
    indexedAt: '2026-08-11T10:00:00Z',
    indexState: 'current',
    indexing: false,
    progress: { phase: 'idle', done: 0, total: 0 },
    resumes: [
      {
        code: 'AA',
        label: 'aa',
        fileLanguages: ['en', 'tr'],
        indexed: true,
        profile: {
          code: 'AA',
          targetRole: 'Backend Engineer',
          seniorityClaimed: 'senior',
          coreStack: ['Kotlin', 'PostgreSQL'],
          secondaryStack: ['Redis'],
          domains: ['payments'],
          languages: ['en', 'tr'],
          yearsClaimed: 6,
          earliestStart: '2019-01',
          places: [],
          summary: 'payment services and their data stores',
        },
      },
      {
        code: 'BB',
        label: 'bb',
        fileLanguages: ['en'],
        indexed: true,
        profile: {
          code: 'BB',
          targetRole: 'Mobil Geliştirici',
          seniorityClaimed: 'mid',
          coreStack: ['Yazılım', 'Swift'],
          secondaryStack: [],
          domains: [],
          languages: ['tr'],
          yearsClaimed: 4,
          earliestStart: '2021-03',
          places: [],
          summary: '',
        },
      },
      {
        code: 'CC',
        label: 'cc',
        fileLanguages: ['en'],
        indexed: false,
        profile: null,
      },
    ],
  }
}

beforeEach(() => {
  resetMemoryStore()
})

test('one preset per indexed variant, labelled with what it is aimed at', () => {
  const presets = derivePresets(profile())

  expect(presets.map((preset) => preset.id)).toEqual(['AA', 'BB', BROAD_ID])
  expect(presets[0]?.label).toBe('Backend Engineer')
  expect(presets[0]?.code).toBe('AA')
  expect(presets[0]?.languages).toEqual(['en', 'tr'])
  expect(presets[1]?.label).toBe('Mobil Geliştirici')
})

test('a preset says what its variant leads with, and filters nothing', () => {
  const presets = derivePresets(profile())

  expect(presets[0]?.leads).toEqual(['Kotlin', 'PostgreSQL'])
  expect(presets[1]?.leads).toEqual(['Yazılım', 'Swift'])
  for (const preset of presets) {
    expect(preset).not.toHaveProperty('terms')
    expect(preset).not.toHaveProperty('matcher')
  }
})

test('a variant with a role but nothing indexed under it still arms its cv', () => {
  const bare = profile()
  const second = bare.resumes[1]
  if (second?.profile) second.profile.coreStack = []
  const presets = derivePresets(bare)

  expect(presets[1]?.id).toBe('BB')
  expect(presets[1]?.leads).toEqual([])
  expect(presets[1]?.code).toBe('BB')
})

test('nothing derived falls back to one broad preset that arms nothing', () => {
  const presets = derivePresets(null)

  expect(presets.length).toBe(1)
  expect(presets[0]?.id).toBe(BROAD_ID)
  expect(presets[0]?.code).toBeNull()
  expect(presets[0]?.leads).toEqual([])
  expect(isDerived(presets)).toBe(false)
})

test('derived presets survive the agent going away', async () => {
  await cachePresets(derivePresets(profile()))
  const cached = await cachedPresets()

  expect(cached.map((preset) => preset.id)).toEqual(['AA', 'BB', BROAD_ID])
  expect(isDerived(cached)).toBe(true)
})

test('a cache is never written from the fallback alone', async () => {
  await cachePresets(derivePresets(null))
  const cached = await cachedPresets()

  expect(cached.length).toBe(1)
  expect(cached[0]?.id).toBe(BROAD_ID)
})
