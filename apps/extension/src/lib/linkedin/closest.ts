import type { ResumeState } from '@/lib/agent/types'

export const CLOSEST_FLOOR = 3

const CORE_WEIGHT = 3
const ROLE_WEIGHT = 2
const EXTRA_WEIGHT = 1

const SPLIT = /[^\p{L}\p{N}+#]+/u

export interface PostingText {
  title: string
  body: string
}

export interface ClosestPick {
  code: string
  matched: string[]
}

export function tokensOf(value: string): Set<string> {
  const cleaned = String(value ?? '')
    .toLowerCase()
    .replace(/\./g, '')
  const tokens = new Set<string>()
  for (const word of cleaned.split(SPLIT)) {
    if (word.length >= 2 && !/^\d+$/.test(word)) tokens.add(word)
  }
  return tokens
}

function phraseIn(phrase: string, tokens: Set<string>): boolean {
  const parts = [...tokensOf(phrase)]
  if (parts.length === 0) return false
  let matched = 0
  for (const part of parts) {
    if (tokens.has(part)) matched += 1
  }
  return matched * 2 > parts.length
}

function scoreOf(
  resume: ResumeState,
  title: Set<string>,
  body: Set<string>,
): { score: number; matched: string[] } {
  const profile = resume.profile
  if (!profile) return { score: 0, matched: [] }

  let score = 0
  const matched: string[] = []
  const weigh = (phrases: string[], weight: number) => {
    for (const phrase of phrases ?? []) {
      const inTitle = phraseIn(phrase, title)
      if (!inTitle && !phraseIn(phrase, body)) continue
      score += weight * (inTitle ? 2 : 1)
      matched.push(phrase)
    }
  }
  weigh(profile.coreStack, CORE_WEIGHT)
  weigh([profile.targetRole], ROLE_WEIGHT)
  weigh(profile.secondaryStack, EXTRA_WEIGHT)
  weigh(profile.domains, EXTRA_WEIGHT)
  return { score, matched }
}

export function closestResume(resumes: ResumeState[], text: PostingText): ClosestPick | null {
  const title = tokensOf(text.title)
  const body = tokensOf(text.body)

  let best: { code: string; score: number; matched: string[] } | null = null
  let contested = false
  for (const resume of resumes ?? []) {
    const { score, matched } = scoreOf(resume, title, body)
    if (score < CLOSEST_FLOOR) continue
    if (best && score === best.score) {
      contested = true
      continue
    }
    if (!best || score > best.score) {
      best = { code: resume.code, score, matched }
      contested = false
    }
  }
  if (!best || contested) return null
  return { code: best.code, matched: best.matched }
}
