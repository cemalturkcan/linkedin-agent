import { expect, test } from 'bun:test'
import { coalesce } from '@/lib/coalesce'

const tick = (ms: number) => new Promise((resolve) => setTimeout(resolve, ms))

test('a burst of events costs one call, not one call each', async () => {
  let calls = 0
  const load = coalesce(() => { calls += 1 }, 20)

  for (let index = 0; index < 100; index += 1) load()
  expect(calls).toBe(0)

  await tick(40)
  expect(calls).toBe(1)
})

test('events that are not a burst still each get their call', async () => {
  let calls = 0
  const load = coalesce(() => { calls += 1 }, 20)

  load()
  await tick(40)
  load()
  await tick(40)

  expect(calls).toBe(2)
})

test('a pending call is dropped when the surface goes away', async () => {
  let calls = 0
  const load = coalesce(() => { calls += 1 }, 20)

  load()
  load.cancel()
  await tick(40)

  expect(calls).toBe(0)
})
