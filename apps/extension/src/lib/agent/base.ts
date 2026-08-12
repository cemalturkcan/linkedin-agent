import { readKey, writeKeys } from '@/lib/storage'

export const BASE_KEY = 'agentBase'
export const DEFAULT_BASE = 'http://127.0.0.1:8787'

const shape = /^https?:\/\/[^\s/]+$/

export function normaliseBase(value: string): string | null {
  const trimmed = String(value ?? '')
    .trim()
    .replace(/\/+$/, '')
  return shape.test(trimmed) ? trimmed : null
}

let cached: string | null = null

export async function agentBase(): Promise<string> {
  if (cached) return cached
  const stored = await readKey<string>(BASE_KEY, '')
  cached = normaliseBase(stored) ?? DEFAULT_BASE
  return cached
}

export async function setAgentBase(value: string): Promise<boolean> {
  const next = normaliseBase(value)
  if (!next) return false
  cached = next
  await writeKeys({ [BASE_KEY]: next })
  return true
}

export function forgetAgentBase(): void {
  cached = null
}
