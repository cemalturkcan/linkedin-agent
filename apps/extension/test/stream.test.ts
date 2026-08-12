import { expect, test } from 'bun:test'
import { AgentStream, type EventSourceLike } from '@/lib/agent/stream'

class FakeSource implements EventSourceLike {
  readonly listeners = new Map<string, (event: MessageEvent<string>) => void>()
  onopen: ((event: Event) => void) | null = null
  onerror: ((event: Event) => void) | null = null
  closed = false

  constructor(readonly url: string) {}

  addEventListener(type: string, listener: (event: MessageEvent<string>) => void): void {
    this.listeners.set(type, listener)
  }

  close(): void {
    this.closed = true
  }

  send(name: string, data: unknown, id = ''): void {
    const listener = this.listeners.get(name)
    if (!listener) return
    listener({ data: JSON.stringify(data), lastEventId: id } as MessageEvent<string>)
  }

  fail(): void {
    this.onerror?.(new Event('error'))
  }
}

function harness() {
  const opened: FakeSource[] = []
  const waits: number[] = []
  const pending: (() => void)[] = []

  const stream = new AgentStream({
    url: async (cursor) => `http://agent/api/events${cursor ? `?lastEventId=${cursor}` : ''}`,
    open: (url) => {
      const source = new FakeSource(url)
      opened.push(source)
      return source
    },
    backoff: [10, 20, 40],
    schedule: (run, wait) => {
      waits.push(wait)
      pending.push(run)
      return pending.length
    },
    unschedule: () => {},
  })

  return { stream, opened, waits, run: () => pending.shift()?.() }
}

const settle = () => new Promise((resolve) => setTimeout(resolve, 0))

test('a state event names the route that owns the object', async () => {
  const { stream, opened } = harness()
  const frames: { name: string; route: string }[] = []
  stream.onState((frame) => frames.push({ name: frame.name, route: frame.route }))
  stream.start()
  await settle()

  opened[0]?.send('jobs', { moved: 1 }, '4')
  opened[0]?.send('cycle', { cycleId: 2 }, '5')
  opened[0]?.send('settings', {}, '6')

  expect(frames).toEqual([
    { name: 'jobs', route: '/api/jobs' },
    { name: 'cycle', route: '/api/plan' },
    { name: 'settings', route: '/api/settings' },
  ])
  expect(stream.cursor()).toBe('6')
})

test('a delta never asks anyone to refetch', async () => {
  const { stream, opened } = harness()
  const deltas: string[] = []
  const states: string[] = []
  stream.onDelta((frame) => deltas.push(frame.name))
  stream.onState((frame) => states.push(frame.name))
  stream.start()
  await settle()

  opened[0]?.send('trace-delta', { id: 3, text: 'partial' })
  opened[0]?.send('trace', { id: 3, state: 'ok' }, '9')

  expect(deltas).toEqual(['trace-delta', 'trace'])
  expect(states).toEqual([])
})

test('a dropped connection reconnects with backoff and loses nothing', async () => {
  const { stream, opened, waits, run } = harness()
  const status: string[] = []
  stream.onStatus((next) => status.push(next))
  stream.start()
  await settle()

  opened[0]?.send('jobs', {}, '12')
  opened[0]?.fail()
  expect(status).toEqual(['connecting', 'live', 'down'])
  expect(waits).toEqual([10])

  run()
  await settle()
  expect(opened[1]?.url).toBe('http://agent/api/events?lastEventId=12')

  opened[1]?.fail()
  run()
  await settle()
  expect(waits).toEqual([10, 20])
  expect(opened[2]?.url).toBe('http://agent/api/events?lastEventId=12')
})

test('stopping closes the socket and reports down', async () => {
  const { stream, opened } = harness()
  stream.start()
  await settle()
  stream.stop()

  expect(opened[0]?.closed).toBe(true)
  expect(stream.status()).toBe('down')
})

test('an unparsable body is dropped rather than breaking the stream', async () => {
  const { stream, opened } = harness()
  const frames: string[] = []
  stream.onState((frame) => frames.push(frame.name))
  stream.start()
  await settle()

  const listener = opened[0]?.listeners.get('jobs')
  listener?.({ data: 'not json', lastEventId: '3' } as MessageEvent<string>)
  opened[0]?.send('jobs', { ok: true }, '4')

  expect(frames).toEqual(['jobs'])
  expect(stream.cursor()).toBe('4')
})
