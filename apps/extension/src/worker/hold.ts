import type { HoldPort } from '@/lib/linkedin/round'

export const PULSE_MS = 20_000
export const WATCH_ALARM = 'round-watch'
export const WATCH_MINUTES = 1

export function workerHold(): HoldPort {
  let pulse: ReturnType<typeof setInterval> | null = null

  return {
    start() {
      if (pulse !== null) return
      pulse = setInterval(() => {
        void chrome.runtime.getPlatformInfo()
      }, PULSE_MS)
      void chrome.alarms.create(WATCH_ALARM, { periodInMinutes: WATCH_MINUTES })
    },
    stop() {
      if (pulse !== null) clearInterval(pulse)
      pulse = null
      void chrome.alarms.clear(WATCH_ALARM)
    },
  }
}
