import type { EventSourceLike } from '../src/lib/agent/stream'

export class FetchEventSource implements EventSourceLike {
  onopen: ((event: Event) => void) | null = null
  onerror: ((event: Event) => void) | null = null

  private readonly listeners = new Map<string, (event: MessageEvent<string>) => void>()
  private readonly controller = new AbortController()
  private closed = false

  constructor(readonly url: string) {
    void this.pump()
  }

  addEventListener(type: string, listener: (event: MessageEvent<string>) => void): void {
    this.listeners.set(type, listener)
  }

  close(): void {
    this.closed = true
    this.controller.abort()
  }

  private dispatch(name: string, data: string, id: string): void {
    const listener = this.listeners.get(name)
    if (!listener) return
    listener({ data, lastEventId: id } as MessageEvent<string>)
  }

  private async pump(): Promise<void> {
    let response: Response
    try {
      response = await fetch(this.url, {
        headers: { accept: 'text/event-stream' },
        signal: this.controller.signal,
      })
    } catch {
      if (!this.closed) this.onerror?.(new Event('error'))
      return
    }
    if (!response.ok || !response.body) {
      if (!this.closed) this.onerror?.(new Event('error'))
      return
    }
    this.onopen?.(new Event('open'))

    const reader = response.body.getReader()
    const decoder = new TextDecoder()
    let buffer = ''
    let name = 'message'
    let data: string[] = []
    let id = ''

    try {
      for (;;) {
        const { done, value } = await reader.read()
        if (done) break
        buffer += decoder.decode(value, { stream: true })

        let split = buffer.indexOf('\n')
        while (split !== -1) {
          const line = buffer.slice(0, split).replace(/\r$/, '')
          buffer = buffer.slice(split + 1)
          split = buffer.indexOf('\n')

          if (line === '') {
            if (data.length > 0) this.dispatch(name, data.join('\n'), id)
            name = 'message'
            data = []
            continue
          }
          if (line.startsWith(':')) continue

          const colon = line.indexOf(':')
          const field = colon === -1 ? line : line.slice(0, colon)
          const rest = colon === -1 ? '' : line.slice(colon + 1).replace(/^ /, '')
          if (field === 'event') name = rest
          if (field === 'data') data.push(rest)
          if (field === 'id') id = rest
        }
      }
    } catch {
      this.closed = this.closed || false
    }

    if (!this.closed) this.onerror?.(new Event('error'))
  }
}
