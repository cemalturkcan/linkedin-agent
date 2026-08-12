import type { PlanState } from '@/lib/agent/types'

export const DEFAULT_MINUTES = 10
export const MIN_MINUTES = 0.5
export const MAX_MINUTES = 720

export interface Pace {
  run: boolean
  minutes: number
  why: string
}

export function minutesOf(value: number): number {
  if (!Number.isFinite(value) || value <= 0) return DEFAULT_MINUTES
  return Math.min(MAX_MINUTES, Math.max(MIN_MINUTES, value))
}

export function paceFrom(state: PlanState | null, now: number): Pace {
  if (!state) {
    return {
      run: false,
      minutes: DEFAULT_MINUTES,
      why: 'the agent is not answering, so nothing is planned until it does',
    }
  }

  const gap = minutesOf(state.cycleMinutes)
  if (!state.autoCycle) {
    return { run: false, minutes: gap, why: 'rounds only run when you ask, autoCycle is off' }
  }
  if (state.paused) {
    return { run: false, minutes: gap, why: 'the agent is paused' }
  }

  const due = Date.parse(state.nextRunAt ?? '')
  if (Number.isFinite(due) && due > now) {
    return {
      run: false,
      minutes: minutesOf((due - now) / 60_000),
      why: 'the agent says the next round is not due yet',
    }
  }
  return { run: true, minutes: gap, why: 'the agent says a round is due' }
}

export function nextWakeAfter(nextRunAt: string | null, cycleMinutes: number, now: number): number {
  const due = Date.parse(nextRunAt ?? '')
  if (Number.isFinite(due) && due > now) return minutesOf((due - now) / 60_000)
  return minutesOf(cycleMinutes)
}
