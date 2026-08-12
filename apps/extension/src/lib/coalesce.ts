export const BURST_MS = 400

export interface Coalesced {
  (): void
  cancel(): void
}

export function coalesce(run: () => void, waitMs = BURST_MS): Coalesced {
  let timer: ReturnType<typeof setTimeout> | null = null

  const fire = () => {
    timer = null
    run()
  }

  const call = (() => {
    if (timer) clearTimeout(timer)
    timer = setTimeout(fire, waitMs)
  }) as Coalesced

  call.cancel = () => {
    if (timer) clearTimeout(timer)
    timer = null
  }
  return call
}
