import { useEffect, useRef } from 'react'

export type HotkeyHandler = (event: KeyboardEvent) => void

function isTypingTarget(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) return false
  if (target.isContentEditable) return true
  return /^(input|textarea|select)$/i.test(target.tagName)
}

function keyOf(event: KeyboardEvent): string {
  const key = event.key.length === 1 ? event.key.toLowerCase() : event.key
  const shifted = event.shiftKey && /^[a-z]$/.test(key)
  return shifted ? `shift+${key}` : key
}

export function useHotkeys(handlers: Record<string, HotkeyHandler | undefined>): void {
  const latest = useRef(handlers)

  useEffect(() => {
    latest.current = handlers
  })

  useEffect(() => {
    function onKeyDown(event: KeyboardEvent) {
      if (event.metaKey || event.ctrlKey || event.altKey) return
      if (event.isComposing) return
      if (isTypingTarget(event.target)) return

      const handler = latest.current[keyOf(event)]
      if (!handler) return

      event.preventDefault()
      handler(event)
    }

    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [])
}
