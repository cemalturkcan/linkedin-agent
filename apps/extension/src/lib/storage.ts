type Bag = Record<string, unknown>

const memory = new Map<string, unknown>()

function area(): chrome.storage.StorageArea | null {
  const runtime = (globalThis as { chrome?: typeof chrome }).chrome
  return runtime?.storage?.local ?? null
}

export async function readKeys(keys: string[]): Promise<Bag> {
  const local = area()
  if (!local) {
    const out: Bag = {}
    for (const key of keys) {
      if (memory.has(key)) out[key] = memory.get(key)
    }
    return out
  }
  try {
    return (await local.get(keys)) as Bag
  } catch {
    return {}
  }
}

export async function readKey<T>(key: string, fallback: T): Promise<T> {
  const bag = await readKeys([key])
  return (bag[key] as T | undefined) ?? fallback
}

export async function writeKeys(patch: Bag): Promise<void> {
  const local = area()
  if (!local) {
    for (const [key, value] of Object.entries(patch)) memory.set(key, value)
    return
  }
  try {
    await local.set(patch)
  } catch {
    for (const [key, value] of Object.entries(patch)) memory.set(key, value)
  }
}

export async function forgetKeys(keys: string[]): Promise<void> {
  const local = area()
  if (!local) {
    for (const key of keys) memory.delete(key)
    return
  }
  try {
    await local.remove(keys)
  } catch {
    for (const key of keys) memory.delete(key)
  }
}

export function resetMemoryStore(): void {
  memory.clear()
}

export function watchKey(key: string, listener: (value: unknown) => void): () => void {
  const runtime = (globalThis as { chrome?: typeof chrome }).chrome
  const changes = runtime?.storage?.onChanged
  if (!changes) return () => {}

  const handler = (patch: Record<string, chrome.storage.StorageChange>, area: string) => {
    if (area !== 'local' || !(key in patch)) return
    listener(patch[key]?.newValue)
  }
  changes.addListener(handler)
  return () => changes.removeListener(handler)
}
