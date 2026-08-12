import { readKey, writeKeys } from '@/lib/storage'

export const LOG_KEY = 'activityLog'
export const LOG_SWITCH_KEY = 'activityLogOn'
export const LOG_MAX = 400
export const LOG_TEXT_MAX = 400

export interface LogEntry {
  at: number
  area: string
  text: string
}

let pending: Promise<void> = Promise.resolve()

export async function logging(): Promise<boolean> {
  return (await readKey<boolean>(LOG_SWITCH_KEY, true)) !== false
}

export async function setLogging(on: boolean): Promise<void> {
  await writeKeys({ [LOG_SWITCH_KEY]: on })
}

export function note(area: string, text: string): void {
  const entry: LogEntry = {
    at: Date.now(),
    area,
    text: String(text ?? '').slice(0, LOG_TEXT_MAX),
  }
  pending = pending
    .then(async () => {
      if (!(await logging())) return
      const stored = await readKey<LogEntry[]>(LOG_KEY, [])
      const rows = Array.isArray(stored) ? stored : []
      rows.push(entry)
      await writeKeys({ [LOG_KEY]: rows.slice(-LOG_MAX) })
    })
    .catch(() => {})
}

export async function readLog(): Promise<LogEntry[]> {
  const stored = await readKey<LogEntry[]>(LOG_KEY, [])
  return Array.isArray(stored) ? stored : []
}

export async function clearLog(): Promise<void> {
  await writeKeys({ [LOG_KEY]: [] })
}

export async function settled(): Promise<void> {
  await pending
}
