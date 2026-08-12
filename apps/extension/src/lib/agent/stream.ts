export const STATE_EVENTS = [
  'setup',
  'index',
  'cycle',
  'jobs',
  'screen',
  'plugin',
  'settings',
  'error',
] as const

export const DELTA_EVENTS = ['trace', 'trace-delta'] as const

export type StateEvent = (typeof STATE_EVENTS)[number]
export type DeltaEvent = (typeof DELTA_EVENTS)[number]
export type StreamStatus = 'connecting' | 'live' | 'down'

export type Payload = Record<string, unknown>

export interface StateFrame {
  name: StateEvent
  route: string
  data: Payload
}

export interface DeltaFrame {
  name: DeltaEvent
  data: Payload
}

export const ROUTE_FOR_STATE: Record<StateEvent, string> = {
  setup: '/api/setup',
  index: '/api/profile',
  cycle: '/api/plan',
  jobs: '/api/jobs',
  screen: '/api/jobs',
  plugin: '/api/plugin',
  settings: '/api/settings',
  error: '',
}

export interface EventSourceLike {
  addEventListener(type: string, listener: (event: MessageEvent<string>) => void): void
  close(): void
  onopen: ((event: Event) => void) | null
  onerror: ((event: Event) => void) | null
}

export interface StreamOptions {
  url: (lastEventId: string) => Promise<string>
  open?: (url: string) => EventSourceLike
  backoff?: number[]
  schedule?: (run: () => void, wait: number) => number
  unschedule?: (handle: number) => void
}

const DEFAULT_BACKOFF = [1_000, 2_000, 4_000, 8_000, 15_000, 30_000]

function parse(body: string): Payload | null {
  if (!body) return {}
  try {
    const value: unknown = JSON.parse(body)
    if (!value || typeof value !== 'object' || Array.isArray(value)) return null
    return value as Payload
  } catch {
    return null
  }
}

function carriesData(event: Event): event is MessageEvent<string> {
  return typeof (event as MessageEvent).data === 'string'
}

export class AgentStream {
  private readonly options: StreamOptions
  private readonly backoff: number[]
  private readonly stateListeners = new Set<(frame: StateFrame) => void>()
  private readonly deltaListeners = new Set<(frame: DeltaFrame) => void>()
  private readonly statusListeners = new Set<(status: StreamStatus) => void>()

  private source: EventSourceLike | null = null
  private timer: number | null = null
  private attempt = 0
  private lastEventId = ''
  private current: StreamStatus = 'down'
  private running = false

  constructor(options: StreamOptions) {
    this.options = options
    this.backoff = options.backoff ?? DEFAULT_BACKOFF
  }

  status(): StreamStatus {
    return this.current
  }

  cursor(): string {
    return this.lastEventId
  }

  onState(listener: (frame: StateFrame) => void): () => void {
    this.stateListeners.add(listener)
    return () => this.stateListeners.delete(listener)
  }

  onDelta(listener: (frame: DeltaFrame) => void): () => void {
    this.deltaListeners.add(listener)
    return () => this.deltaListeners.delete(listener)
  }

  onStatus(listener: (status: StreamStatus) => void): () => void {
    this.statusListeners.add(listener)
    return () => this.statusListeners.delete(listener)
  }

  start(): void {
    if (this.running) return
    this.running = true
    void this.connect()
  }

  stop(): void {
    this.running = false
    this.clearTimer()
    this.drop()
    this.announce('down')
  }

  retryNow(): void {
    if (!this.running) return this.start()
    this.clearTimer()
    this.attempt = 0
    this.drop()
    void this.connect()
  }

  private announce(next: StreamStatus): void {
    if (this.current === next) return
    this.current = next
    for (const listener of [...this.statusListeners]) listener(next)
  }

  private clearTimer(): void {
    if (this.timer === null) return
    const unschedule = this.options.unschedule ?? clearTimeout
    unschedule(this.timer)
    this.timer = null
  }

  private drop(): void {
    if (!this.source) return
    const source = this.source
    this.source = null
    source.close()
  }

  private later(): void {
    if (!this.running || this.timer !== null) return
    const wait = this.backoff[Math.min(this.attempt, this.backoff.length - 1)] ?? 30_000
    this.attempt += 1
    const schedule = this.options.schedule ?? ((run, ms) => setTimeout(run, ms) as unknown as number)
    this.timer = schedule(() => {
      this.timer = null
      void this.connect()
    }, wait)
  }

  private async connect(): Promise<void> {
    if (!this.running || this.source) return
    if (this.current !== 'live') this.announce('connecting')

    const factory = this.options.open ?? ((url: string) => new EventSource(url) as EventSourceLike)
    let source: EventSourceLike
    try {
      source = factory(await this.options.url(this.lastEventId))
    } catch {
      this.announce('down')
      this.later()
      return
    }
    if (!this.running) {
      source.close()
      return
    }
    this.source = source

    const seen = (event: MessageEvent<string>) => {
      if (event.lastEventId) this.lastEventId = event.lastEventId
      this.attempt = 0
      this.announce('live')
    }

    for (const name of STATE_EVENTS) {
      source.addEventListener(name, (event) => {
        if (!carriesData(event)) return
        seen(event)
        const data = parse(event.data)
        if (!data) return
        const frame: StateFrame = { name, route: ROUTE_FOR_STATE[name], data }
        for (const listener of [...this.stateListeners]) listener(frame)
      })
    }

    for (const name of DELTA_EVENTS) {
      source.addEventListener(name, (event) => {
        if (!carriesData(event)) return
        seen(event)
        const data = parse(event.data)
        if (!data) return
        for (const listener of [...this.deltaListeners]) listener({ name, data })
      })
    }

    source.onopen = () => {
      this.attempt = 0
      this.announce('live')
    }

    source.onerror = () => {
      if (this.source !== source) return
      this.drop()
      this.announce('down')
      this.later()
    }
  }
}
