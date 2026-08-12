const DOCUMENT_TYPES = /pdf|msword|officedocument|octet-stream|\*/i
const RESUME_FIELD = /resume|upload|document|cv/i
const VIEW_PATH = /\/jobs\/view\/(?:[^/]*-)?(\d{5,})/
const TICK_MS = 600
const RETRY_MS = 4000
const CHECK_MS = 20000
const SETTLED = ['already-attached']
const ARMED_KEY = 'armedResume'

let posting = null
let observers = []
let watched = new WeakSet()
let pending = null
let href = ''
let syncTimer = 0
let scheduleTimer = 0
let closed = false

function alive() {
  try {
    return Boolean(chrome.runtime && chrome.runtime.id)
  } catch {
    return false
  }
}

function shutdown() {
  if (closed) return
  closed = true
  if (syncTimer) clearInterval(syncTimer)
  if (scheduleTimer) clearInterval(scheduleTimer)
  if (pending) clearTimeout(pending)
  stop()
  syncTimer = 0
  scheduleTimer = 0
  pending = null
  window.removeEventListener('popstate', sync)
}

async function send(message) {
  if (!alive()) {
    shutdown()
    return null
  }
  try {
    return await chrome.runtime.sendMessage(message)
  } catch {
    return null
  }
}

function postingId() {
  const path = VIEW_PATH.exec(location.pathname)
  if (path) return path[1]
  const current = new URLSearchParams(location.search).get('currentJobId')
  return current && /^\d{5,}$/.test(current) ? current : null
}

function roots() {
  const found = [document]
  const walk = (root) => {
    for (const element of root.querySelectorAll('*')) {
      if (!element.shadowRoot) continue
      found.push(element.shadowRoot)
      walk(element.shadowRoot)
    }
  }
  walk(document)
  return found
}

function fileStep() {
  for (const root of roots()) {
    for (const input of root.querySelectorAll('input[type="file"]')) {
      if (input.disabled) continue
      const accept = input.accept || ''
      if (accept && !DOCUMENT_TYPES.test(accept)) continue
      const meta = `${accept} ${input.id || ''} ${input.name || ''} ${input.className || ''}`
      if (RESUME_FIELD.test(meta)) return true
    }
  }
  return false
}

async function tick() {
  pending = null
  const owner = posting
  if (!owner || !alive()) return

  const showing = fileStep()
  if (showing) owner.everOpen = true

  if (owner.everOpen && Date.now() - owner.lastCheck >= CHECK_MS) {
    owner.lastCheck = Date.now()
    const checked = await send({ type: 'apply:check', id: owner.id })
    if (posting !== owner) return
    if (checked && checked.moved) {
      stop()
      posting = null
      return
    }
  }
  if (!showing) return

  if (owner.done || Date.now() - owner.lastTry < RETRY_MS) return
  owner.lastTry = Date.now()

  const outcome = await send({ type: 'apply:step', id: owner.id })
  if (posting !== owner || !outcome) return
  if (outcome.ok) owner.done = true
  else if (SETTLED.includes(outcome.reason)) owner.done = true
}

function retry() {
  if (!posting || posting.done) return
  posting.lastTry = 0
  schedule()
}

function schedule() {
  if (pending || !posting) return
  pending = setTimeout(() => void tick(), TICK_MS)
}

function stop() {
  for (const observer of observers) observer.disconnect()
  observers = []
  watched = new WeakSet()
  if (pending) clearTimeout(pending)
  pending = null
}

function observeRoots() {
  if (!posting) return
  for (const root of roots()) {
    if (watched.has(root)) continue
    watched.add(root)
    const observer = new MutationObserver(() => schedule())
    observer.observe(root, { childList: true, subtree: true })
    observers.push(observer)
  }
}

function watch(id) {
  stop()
  posting = { id, done: false, everOpen: false, lastTry: 0, lastCheck: 0 }
  observeRoots()
  schedule()
}

function sync() {
  if (closed) return
  if (!alive()) {
    shutdown()
    return
  }
  if (location.href === href) return
  href = location.href
  const id = postingId()
  if (posting && posting.id === id) return
  stop()
  posting = null
  if (id) watch(id)
}

try {
  chrome.storage.onChanged.addListener((patch, area) => {
    if (closed || area !== 'local') return
    if (!(ARMED_KEY in patch)) return
    retry()
  })
} catch {
  shutdown()
}

window.addEventListener('popstate', sync)
syncTimer = setInterval(sync, 1000)
scheduleTimer = setInterval(() => {
  if (closed) return
  if (!alive()) {
    shutdown()
    return
  }
  observeRoots()
  schedule()
}, 2500)
sync()
