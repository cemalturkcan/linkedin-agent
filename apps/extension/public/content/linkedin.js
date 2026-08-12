let announced = ''
let timer = 0

function alive() {
  try {
    return Boolean(chrome.runtime && chrome.runtime.id)
  } catch {
    return false
  }
}

function shutdown() {
  if (timer) clearInterval(timer)
  timer = 0
  window.removeEventListener('popstate', report)
}

function report() {
  if (!alive()) {
    shutdown()
    return
  }
  const here = location.href
  if (here === announced) return
  announced = here
  try {
    chrome.runtime.sendMessage({ type: 'posting-opened', href: here }, () => chrome.runtime.lastError)
  } catch {
    announced = ''
    shutdown()
  }
}

report()
window.addEventListener('popstate', report)
timer = setInterval(report, 1500)
