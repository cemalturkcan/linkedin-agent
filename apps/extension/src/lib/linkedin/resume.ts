import type { Resume } from '@/lib/agent/types'

export const DEFAULT_UPLOAD_NAME = 'resume.pdf'

export const ONE_NAME = 'one-name'
export const PER_VARIANT = 'per-variant'
export const UPLOAD_NAME_MODES = [ONE_NAME, PER_VARIANT] as const
export type UploadNameMode = (typeof UPLOAD_NAME_MODES)[number]

export const ONE_NAME_CONSEQUENCE =
  'every variant attaches under the same name, so nothing tells the stored copies apart and each application uploads a fresh one'
export const PER_VARIANT_CONSEQUENCE =
  'each variant carries its own name, so a copy linkedin already holds can be selected instead of uploaded again'

export function modeOf(value: string | null | undefined): UploadNameMode {
  return value === PER_VARIANT ? PER_VARIANT : ONE_NAME
}

export function consequenceOf(mode: string | null | undefined): string {
  return modeOf(mode) === PER_VARIANT ? PER_VARIANT_CONSEQUENCE : ONE_NAME_CONSEQUENCE
}

export const UPLOAD_NAME_IS_PUBLIC = 'this is the file name an employer sees on the application'

const NOT_NAME = /[^a-z0-9]+/g
const EDGES = /^-+|-+$/g
const CONTROL = /\p{Cc}/gu
const PDF = /\.pdf$/i

function slug(value: string): string {
  return String(value ?? '')
    .toLowerCase()
    .replace(NOT_NAME, '-')
    .replace(EDGES, '')
}

export function stemOf(base: string): string {
  const named = String(base ?? '')
    .split(/[\\/]/)
    .pop()
    ?.replace(CONTROL, '')
    .trim()
  const stem = slug((named ?? '').replace(PDF, ''))
  return stem || slug(DEFAULT_UPLOAD_NAME.replace(PDF, ''))
}

export function uploadName(
  base: string,
  code: string,
  lang: string,
  mode: string | null | undefined = ONE_NAME,
): string {
  if (modeOf(mode) === ONE_NAME) return `${stemOf(base)}.pdf`
  const parts = [stemOf(base), slug(code), slug(lang)].filter((part) => part !== '')
  return `${parts.join('-')}.pdf`
}

export interface StoredResume {
  name: string
  selected?: boolean
}

const FILE_TOKEN = /[^\s\\/:*?"<>|]+\.pdf(?![\p{L}\p{N}])/iu

export function fileNameIn(text: string): string {
  const found = FILE_TOKEN.exec(String(text ?? ''))
  return found ? (found[0] as string) : ''
}

export function readStored(options: { text: string; selected: boolean }[]): StoredResume[] {
  const stored: StoredResume[] = []
  for (const option of options ?? []) {
    const name = fileNameIn(option.text)
    if (name) stored.push({ name, selected: option.selected === true })
  }
  return stored
}

export type Attachment =
  | { action: 'select'; index: number; name: string; attached: boolean; why: string }
  | { action: 'upload'; name: string; why: string }

function normalise(name: string): string {
  return String(name ?? '')
    .replace(CONTROL, '')
    .trim()
    .replace(/\s+/g, ' ')
    .toLowerCase()
}

export function decideAttachment(
  stored: StoredResume[],
  wanted: string,
  mode: string | null | undefined = ONE_NAME,
): Attachment {
  const target = normalise(wanted)
  if (!target) {
    return { action: 'upload', name: wanted, why: 'no file name was derived for this cv' }
  }
  if (modeOf(mode) === ONE_NAME) {
    return { action: 'upload', name: wanted, why: ONE_NAME_CONSEQUENCE }
  }

  const matches: number[] = []
  stored.forEach((entry, index) => {
    if (normalise(entry.name) === target) matches.push(index)
  })

  if (matches.length === 1) {
    const index = matches[0] as number
    const entry = stored[index] as StoredResume
    return {
      action: 'select',
      index,
      name: entry.name,
      attached: entry.selected === true,
      why: `linkedin already holds ${entry.name}, so it is selected rather than uploaded again`,
    }
  }
  if (matches.length > 1) {
    return {
      action: 'upload',
      name: wanted,
      why: `linkedin holds ${matches.length} files called ${wanted} and nothing tells them apart, so a fresh one is uploaded rather than guessing`,
    }
  }
  return {
    action: 'upload',
    name: wanted,
    why: `linkedin holds no file called ${wanted}`,
  }
}

export interface ResumeChoice {
  code: string
  lang: string
}

export function pickLanguage(available: string[], preferred: string[] = []): string {
  const languages = (available ?? []).filter((value) => typeof value === 'string' && value !== '')
  for (const candidate of [...preferred, 'en']) {
    if (candidate && languages.includes(candidate)) return candidate
  }
  return languages[0] ?? ''
}

export function chooseResume(
  resumes: Resume[],
  code: string,
  lang: string | null,
  preferred: string[] = [],
): ResumeChoice | null {
  const wanted = String(code ?? '').toUpperCase()
  const resume = resumes.find((entry) => String(entry.code).toUpperCase() === wanted)
  if (!resume) return null

  const chosen = pickLanguage(resume.languages, [lang ?? '', ...preferred])
  return chosen ? { code: resume.code, lang: chosen } : null
}
