function runtime(): typeof chrome | null {
  return (globalThis as { chrome?: typeof chrome }).chrome ?? null
}

export function openTab(url: string, active = true): void {
  const api = runtime()
  if (api?.tabs?.create) {
    void api.tabs.create({ url, active })
    return
  }
  globalThis.open?.(url, '_blank')
}

export function deskUrl(hash = ''): string {
  const api = runtime()
  const path = `desk.html${hash ? `#${hash}` : ''}`
  return api?.runtime?.getURL ? api.runtime.getURL(path) : path
}

export function openDesk(hash = ''): void {
  openTab(deskUrl(hash))
}
