import type { ProfileState, ResumeState } from '@/lib/agent/types'
import { readKey, writeKeys } from '@/lib/storage'

export const PRESETS_KEY = 'resumePresets'
export const BROAD_ID = 'everything'

export interface Preset {
  id: string
  label: string
  aim: string
  code: string | null
  languages: string[]
  leads: string[]
}

export const BROAD_PRESET: Preset = {
  id: BROAD_ID,
  label: 'everything',
  aim: 'no cv armed, so only the agent’s own pick is attached',
  code: null,
  languages: [],
  leads: [],
}

function aimOf(resume: ResumeState): string {
  const profile = resume.profile
  if (!profile) return ''
  if (profile.summary) return profile.summary
  if (profile.coreStack.length > 0) return profile.coreStack.join(', ')
  return ''
}

function presetFor(resume: ResumeState): Preset | null {
  const profile = resume.profile
  if (!profile) return null

  const leads = profile.coreStack.filter(
    (entry) => typeof entry === 'string' && entry.trim() !== '',
  )
  if (!profile.targetRole && leads.length === 0) return null

  return {
    id: resume.code,
    label: profile.targetRole || resume.label,
    aim: aimOf(resume),
    code: resume.code,
    languages: resume.fileLanguages ?? [],
    leads,
  }
}

export function derivePresets(profile: ProfileState | null): Preset[] {
  const derived: Preset[] = []
  for (const resume of profile?.resumes ?? []) {
    const preset = presetFor(resume)
    if (preset) derived.push(preset)
  }
  return [...derived, BROAD_PRESET]
}

export function isDerived(presets: Preset[]): boolean {
  return presets.some((preset) => preset.id !== BROAD_ID)
}

export const TERM_CHIPS_MAX = 12
export const PLACE_CHIPS_MAX = 8

function collect(values: (string | null | undefined)[], limit: number): string[] {
  const kept: string[] = []
  const seen = new Set<string>()
  for (const value of values) {
    const term = String(value ?? '').trim()
    const key = term.toLowerCase()
    if (term === '' || seen.has(key)) continue
    seen.add(key)
    kept.push(term)
    if (kept.length >= limit) break
  }
  return kept
}

export function deriveTerms(profile: ProfileState | null): string[] {
  const words: string[] = []
  for (const resume of profile?.resumes ?? []) {
    if (resume.profile?.targetRole) words.push(resume.profile.targetRole)
  }
  for (const stack of profile?.candidate?.coreStack ?? []) words.push(stack)
  for (const resume of profile?.resumes ?? []) {
    for (const stack of resume.profile?.coreStack ?? []) words.push(stack)
  }
  return collect(words, TERM_CHIPS_MAX)
}

export function derivePlaceNames(profile: ProfileState | null): string[] {
  const names: string[] = []
  for (const place of profile?.candidate?.places ?? []) names.push(place.name)
  for (const resume of profile?.resumes ?? []) {
    for (const place of resume.profile?.places ?? []) names.push(place.name)
  }
  return collect(names, PLACE_CHIPS_MAX)
}

interface PresetCache {
  at: number
  presets: Preset[]
}

export async function cachePresets(presets: Preset[]): Promise<void> {
  if (!isDerived(presets)) return
  await writeKeys({ [PRESETS_KEY]: { at: Date.now(), presets } satisfies PresetCache })
}

export async function cachedPresets(): Promise<Preset[]> {
  const cache = await readKey<PresetCache | null>(PRESETS_KEY, null)
  if (!cache || !Array.isArray(cache.presets) || cache.presets.length === 0) return [BROAD_PRESET]
  return cache.presets
}

export function presetById(presets: Preset[], id: string | null): Preset | null {
  if (!id) return null
  return presets.find((preset) => preset.id === id) ?? null
}
