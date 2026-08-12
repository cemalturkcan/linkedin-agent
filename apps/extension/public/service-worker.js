importScripts('executor.js')

const DEFAULT_BASE = 'http://127.0.0.1:8787'
const BASE_KEY = 'agentBase'
const HELLO_ALARM = 'hello'
const HELLO_MINUTES = 5
const HELLO_TIMEOUT_MS = 4000
const BASE_SHAPE = /^https?:\/\/[^\s/]+$/

async function agentBase() {
  try {
    const stored = await chrome.storage.local.get(BASE_KEY)
    const value = String(stored[BASE_KEY] || '')
      .trim()
      .replace(/\/+$/, '')
    return BASE_SHAPE.test(value) ? value : DEFAULT_BASE
  } catch {
    return DEFAULT_BASE
  }
}

async function capabilities() {
  const response = await fetch(chrome.runtime.getURL('capabilities.json'))
  return response.json()
}

async function announce() {
  const base = await agentBase()
  const { version } = chrome.runtime.getManifest()
  const controller = new AbortController()
  const timer = setTimeout(() => controller.abort(), HELLO_TIMEOUT_MS)
  try {
    const response = await fetch(`${base}/api/plugin/hello`, {
      method: 'POST',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({
        id: chrome.runtime.id,
        version,
        capabilities: await capabilities(),
      }),
      signal: controller.signal,
    })
    return response.ok
  } catch {
    return false
  } finally {
    clearTimeout(timer)
  }
}

function schedule() {
  chrome.alarms.create(HELLO_ALARM, { periodInMinutes: HELLO_MINUTES })
}

chrome.runtime.onInstalled.addListener(() => {
  chrome.sidePanel.setPanelBehavior({ openPanelOnActionClick: true }).catch(() => {})
  schedule()
  void announce()
})

chrome.runtime.onStartup.addListener(() => {
  schedule()
  void announce()
})

chrome.alarms.onAlarm.addListener((alarm) => {
  if (alarm.name === HELLO_ALARM) void announce()
})

chrome.runtime.onMessage.addListener((message, _sender, respond) => {
  if (!message || typeof message !== 'object') return false

  if (message.type === 'hello') {
    announce()
      .then((ok) => respond({ ok }))
      .catch(() => respond({ ok: false }))
    return true
  }

  return false
})
