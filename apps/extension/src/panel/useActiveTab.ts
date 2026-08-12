import { useEffect, useRef, useState } from 'react'
import { followFor, NO_FOLLOW, type Following } from '@/lib/linkedin/posting'

function runtime(): typeof chrome | null {
  return (globalThis as { chrome?: typeof chrome }).chrome ?? null
}

export function useActiveTab(): Following {
  const [following, setFollowing] = useState<Following>(NO_FOLLOW)
  const held = useRef<Following>(NO_FOLLOW)

  useEffect(() => {
    const api = runtime()
    const tabs = api?.tabs
    if (!tabs?.query) return

    let cancelled = false

    const settle = (tab: chrome.tabs.Tab | null) => {
      if (cancelled) return
      const next = followFor(tab, held.current)
      held.current = next
      setFollowing((previous) =>
        previous.screen === next.screen && previous.postingId === next.postingId ? previous : next,
      )
    }

    const readActive = () => {
      tabs
        .query({ active: true, lastFocusedWindow: true })
        .then((found) => settle(found[0] ?? null))
        .catch(() => settle(null))
    }

    const onActivated = (info: chrome.tabs.OnActivatedInfo) => {
      tabs
        .get(info.tabId)
        .then((tab) => settle(tab))
        .catch(() => settle(null))
    }

    const onUpdated = (_tabId: number, _change: chrome.tabs.OnUpdatedInfo, tab: chrome.tabs.Tab) => {
      if (tab.active) settle(tab)
    }

    const onRemoved = () => readActive()

    readActive()
    tabs.onActivated?.addListener(onActivated)
    tabs.onUpdated?.addListener(onUpdated)
    tabs.onRemoved?.addListener(onRemoved)
    api?.windows?.onFocusChanged?.addListener(readActive)

    return () => {
      cancelled = true
      tabs.onActivated?.removeListener(onActivated)
      tabs.onUpdated?.removeListener(onUpdated)
      tabs.onRemoved?.removeListener(onRemoved)
      api?.windows?.onFocusChanged?.removeListener(readActive)
    }
  }, [])

  return following
}
