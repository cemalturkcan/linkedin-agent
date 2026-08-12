import { expect, test } from 'bun:test'
import type { PlanState } from '@/lib/agent/types'
import { DEFAULT_MINUTES, minutesOf, nextWakeAfter, paceFrom } from '@/lib/linkedin/pace'

const NOW = 1_770_000_000_000

function state(overrides: Partial<PlanState>): PlanState {
  return {
    plugin: {
      everSeen: true,
      connected: true,
      id: 'a'.repeat(32),
      version: '0.1.0',
      firstSeen: null,
      lastSeen: null,
      hellos: 1,
      capabilities: null,
    },
    paused: false,
    autoCycle: true,
    cycleMinutes: 10,
    nextRunAt: null,
    blocked: null,
    budget: { used: 0, cap: 60, remaining: 60, day: '2026-08-11' },
    roundBudget: { cycleId: 0, open: false, used: 0, cap: 0, remaining: 0 },
    cycle: null,
    history: [],
    notes: [],
    knownIdCount: 0,
    ...overrides,
  }
}

test('autoCycle off means no round fires on a wake', () => {
  const pace = paceFrom(state({ autoCycle: false, cycleMinutes: 45 }), NOW)
  expect(pace.run).toBe(false)
  expect(pace.minutes).toBe(45)
  expect(pace.why).toBe('rounds only run when you ask, autoCycle is off')
})

test('the pause switch stops the wake as well', () => {
  const pace = paceFrom(state({ paused: true }), NOW)
  expect(pace.run).toBe(false)
  expect(pace.why).toBe('the agent is paused')
})

test('a round due in the future waits for the agent own clock, not a constant here', () => {
  const pace = paceFrom(state({ nextRunAt: new Date(NOW + 7 * 60_000).toISOString() }), NOW)
  expect(pace.run).toBe(false)
  expect(pace.minutes).toBe(7)
  expect(pace.why).toBe('the agent says the next round is not due yet')
})

test('a round already due runs, and the gap comes from the agent', () => {
  const pace = paceFrom(
    state({ cycleMinutes: 30, nextRunAt: new Date(NOW - 60_000).toISOString() }),
    NOW,
  )
  expect(pace.run).toBe(true)
  expect(pace.minutes).toBe(30)
})

test('with no nextRunAt at all the agent still decides by autoCycle', () => {
  expect(paceFrom(state({ nextRunAt: null }), NOW).run).toBe(true)
  expect(paceFrom(state({ nextRunAt: 'not a time' }), NOW).run).toBe(true)
})

test('an agent that is not answering stops rounds rather than guessing a pace', () => {
  const pace = paceFrom(null, NOW)
  expect(pace.run).toBe(false)
  expect(pace.minutes).toBe(DEFAULT_MINUTES)
  expect(pace.why).toBe('the agent is not answering, so nothing is planned until it does')
})

test('the next wake follows the round own nextRunAt when there is one', () => {
  expect(nextWakeAfter(new Date(NOW + 12 * 60_000).toISOString(), 10, NOW)).toBe(12)
  expect(nextWakeAfter(new Date(NOW - 60_000).toISOString(), 10, NOW)).toBe(10)
  expect(nextWakeAfter(null, 10, NOW)).toBe(10)
})

test('an alarm period stays inside what chrome accepts', () => {
  expect(minutesOf(0)).toBe(DEFAULT_MINUTES)
  expect(minutesOf(-4)).toBe(DEFAULT_MINUTES)
  expect(minutesOf(0.1)).toBe(0.5)
  expect(minutesOf(5_000)).toBe(720)
})
