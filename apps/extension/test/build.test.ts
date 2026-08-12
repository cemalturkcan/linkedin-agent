import { existsSync, readFileSync, readdirSync, statSync } from 'node:fs'
import { dirname, join, relative, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { expect, test } from 'bun:test'
import { capabilityManifest } from '@/lib/capabilities'
import { placeFromHash } from '@/desk/Desk'

const here = dirname(fileURLToPath(import.meta.url))
const root = resolve(here, '..')

function read(...parts: string[]): string {
  return readFileSync(join(root, ...parts), 'utf8')
}

function walk(dir: string, out: string[] = []): string[] {
  for (const entry of readdirSync(dir)) {
    const path = join(dir, entry)
    if (statSync(path).isDirectory()) walk(path, out)
    else out.push(path)
  }
  return out
}

test('the manifest parses and lets nothing outside the extension talk to it', () => {
  const manifest = JSON.parse(read('public', 'manifest.json')) as Record<string, unknown>

  expect(manifest.manifest_version).toBe(3)
  expect(manifest).not.toHaveProperty('externally_connectable')
  expect((manifest.side_panel as { default_path: string }).default_path).toBe('panel.html')
  expect((manifest.background as { service_worker: string }).service_worker).toBe(
    'service-worker.js',
  )
  expect(manifest.minimum_chrome_version).toBe('114')
})

test('the worker and the surfaces read one capability manifest', () => {
  const declared = JSON.parse(read('public', 'capabilities.json'))
  const worker = read('public', 'service-worker.js')

  expect(capabilityManifest()).toEqual(declared)
  expect(worker).toContain("chrome.runtime.getURL('capabilities.json')")
  expect(worker).not.toContain('titleMatch')
})

test('the worker and the content script carry no import', () => {
  for (const file of ['public/service-worker.js', 'public/content/linkedin.js']) {
    const source = read(...file.split('/'))
    expect(source).not.toMatch(/^\s*import\s/m)
    expect(source).not.toMatch(/\brequire\(/)
    expect(source.split('\n').length).toBeLessThan(140)
  }
})

test('the apply surfaces read through shadow roots and gate on the resume field itself', () => {
  const script = read('public', 'content', 'apply.js')
  const injected = read('src', 'worker', 'inject.ts')

  for (const source of [script, injected]) {
    expect(source).toContain('shadowRoot')
    expect(source).not.toContain('[data-test-modal]')
    expect(source).not.toContain('.artdeco-modal')
  }
  expect(script).toContain("root.querySelectorAll('input[type=\"file\"]')")
})

test('only an attach that already happened settles the loop, and arming a cv restarts it', () => {
  const script = read('public', 'content', 'apply.js')

  expect(script).toContain("const SETTLED = ['already-attached']")
  expect(script).not.toContain("'no-assignment'")
  expect(script).toContain("const ARMED_KEY = 'armedResume'")
  expect(script).toContain('chrome.storage.onChanged.addListener')
  expect(read('src', 'lib', 'armed.ts')).toContain("export const ARMED_KEY = 'armedResume'")
})

test('once the cv step has been seen the tab keeps asking linkedin whether it was applied to', () => {
  const script = read('public', 'content', 'apply.js')

  expect(script).toContain('owner.everOpen = true')
  expect(script).toContain("send({ type: 'apply:check', id: owner.id })")
  expect(script).toContain('Date.now() - owner.lastCheck >= CHECK_MS')
})

test('enter opens the posting on linkedin and the detail sits on its own key', () => {
  const desk = read('src', 'desk', 'Desk.tsx')

  expect(desk).toContain('Enter: () => openSelected()')
  expect(desk).toContain('d: () => showDetail()')
  expect(desk).toContain('if (posting) void state.actions.openPosting(posting)')
  expect(read('src', 'desk', 'useDeskState.ts')).toContain('queueOnOpen(posting.status)')
  expect(read('src', 'panel', 'useAgentState.ts')).toContain('queueOnOpen(from)')
  expect(desk).toContain("{ keys: ['↵'], label: 'open on linkedin' }")
})

test('the desk carries a log section and the worker writes to it', () => {
  const desk = read('src', 'desk', 'Desk.tsx')
  const worker = read('src', 'worker', 'main.ts')

  expect(desk).toContain("'log'")
  expect(desk).toContain("l: () => open('log')")
  expect(worker).toContain("note('attach'")
  expect(worker).toContain("note('round'")
})

test('no checkout of this repository names the person who wrote it', () => {
  const everything = [
    ...walk(join(root, 'src')),
    ...walk(join(root, 'public')),
    ...walk(join(root, 'test')),
    ...walk(join(root, 'scripts')),
  ].filter((path) => !path.endsWith('.png') && !path.endsWith('build.test.ts'))

  const hits: string[] = []
  for (const path of everything) {
    const source = readFileSync(path, 'utf8')
    const named = /(cemal|turkcan|t\u00fcrkcan)/i.exec(source)
    if (named) hits.push(`${relative(root, path)} names ${named[0]}`)
    const home = /\/(home|Users)\/[a-z][a-z0-9_-]+\//i.exec(source)
    if (home) hits.push(`${relative(root, path)} hardcodes ${home[0]}`)
  }

  expect(hits).toEqual([])
})

test('nothing personal is hardcoded in what ships', () => {
  const shipped = [...walk(join(root, 'src')), ...walk(join(root, 'public'))].filter(
    (path) => !path.endsWith('.png'),
  )

  const forbidden: [string, RegExp][] = [
    ['a person', /\b(cemal|turkcan|türkcan)\b/i],
    ['a city or country', /\b(istanbul|ist?anbul|ankara|izmir|türkiye|turkiye|turkey|amsterdam|berlin|london)\b/i],
    ['a linkedin geo id', /(?<![#\w])\d{6,}(?!\w)/],
    ['a stack vocabulary', /\b(golang|dotnet|csharp|kotlin|django|laravel|rails|nestjs|flutter|kubernetes|terraform|spring)\b/i],
    ['a resume code', /\bresumeCode\s*[=:]\s*['"][A-Z]{2}['"]/],
  ]

  const hits: string[] = []
  for (const path of shipped) {
    const source = readFileSync(path, 'utf8')
    for (const [what, pattern] of forbidden) {
      const found = pattern.exec(source)
      if (found) hits.push(`${relative(root, path)} holds ${what}: ${found[0]}`)
    }
  }

  expect(hits).toEqual([])
})

test('no title matcher survives anywhere in what ships', () => {
  const shipped = [...walk(join(root, 'src')), ...walk(join(root, 'public'))].filter(
    (path) => !path.endsWith('.png'),
  )

  const gone = /\b(compileMatcher|compileTitles|titleAny|titleAll|titleNone|titleMatch|TitleTest)\b/

  const hits: string[] = []
  for (const path of shipped) {
    const found = gone.exec(readFileSync(path, 'utf8'))
    if (found) hits.push(`${relative(root, path)} still carries ${found[0]}`)
  }

  expect(hits).toEqual([])
  expect(existsSync(join(root, 'src', 'lib', 'matcher.ts'))).toBe(false)
})

test('the two entry points exist and each mounts its own surface', () => {
  expect(read('panel.html')).toContain('/src/panel/main.tsx')
  expect(read('desk.html')).toContain('/src/desk/main.tsx')
})

test('the desk is a built surface, not a stub, and every panel link lands where the desk draws', () => {
  const desk = read('src', 'desk', 'Desk.tsx')
  const panel = read('src', 'panel', 'Panel.tsx')

  expect(desk).not.toContain('not built')
  for (const id of ['work', 'rounds', 'settings', 'setup', 'profile', 'model']) {
    expect(desk).toContain(`'${id}'`)
  }

  const hashes = [...new Set([...panel.matchAll(/openDesk\('([^']+)'\)/g)].map((found) => found[1] as string))]
  expect(hashes.slice().sort()).toEqual(['model', 'profile', 'settings', 'setup'])
  for (const hash of hashes) {
    expect(placeFromHash(hash, null)).not.toBeNull()
  }
})
