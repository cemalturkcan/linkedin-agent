import { existsSync, readFileSync } from 'node:fs'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { expect, test } from 'bun:test'
import { capabilityManifest } from '@/lib/capabilities'
import { MAX_PAGES_PER_QUERY, PAGE_SIZE, RANGES, RINGS, SORTS } from '@/lib/linkedin/voyager'

const root = resolve(dirname(fileURLToPath(import.meta.url)), '..')

function read(...parts: string[]): string {
  return readFileSync(join(root, ...parts), 'utf8')
}

test('the manifest names files that exist, and the worker loads the executor beside it', () => {
  const manifest = JSON.parse(read('public', 'manifest.json')) as {
    background: { service_worker: string }
    content_scripts: { js: string[] }[]
  }

  expect(existsSync(join(root, 'public', manifest.background.service_worker))).toBe(true)
  for (const script of manifest.content_scripts.flatMap((entry) => entry.js)) {
    expect(existsSync(join(root, 'public', script))).toBe(true)
  }
  expect(read('public', 'service-worker.js')).toContain("importScripts('executor.js')")
  expect(read('package.json')).toContain('--outfile=dist/executor.js')
})

test('the capability manifest is what this code can actually run', () => {
  const declared = capabilityManifest()

  expect(declared.fetch.place).toBe(true)
  expect(declared.fetch.rings).toEqual([...RINGS])
  expect(declared.fetch.ranges).toEqual([...RANGES])
  expect(declared.fetch.sorts).toEqual([...SORTS])
  expect(declared.fetch.easyApply).toBe(true)
  expect(declared.fetch.remoteOnly).toBe(true)
  expect(declared.fetch.keyword).toBe(true)
  expect(declared.fetch.pageSize).toBe(PAGE_SIZE)
  expect(declared.fetch.maxPagesPerQuery).toBe(MAX_PAGES_PER_QUERY)
  expect(declared.seenSet).toBe(true)
  expect(declared.autoAttach).toBe(true)
})

test('the apply content script never advances the form', () => {
  const source = read('public', 'content', 'apply.js')

  for (const forbidden of [/\bsubmit\b/i, /\bnext\b/i, /\breview\b/i, /\.value\s*=/, /\.click\(/]) {
    expect(source).not.toMatch(forbidden)
  }
  expect(source.split('\n').length).toBeLessThan(200)
  expect(source).not.toMatch(/^\s*import\s/m)
})

test('nothing in the executor clicks its way through an application', () => {
  const source = read('src', 'worker', 'inject.ts')

  expect(source).not.toMatch(/\bsubmit\b/i)
  expect(source).not.toMatch(/\breview\b/i)
  expect(source.match(/\.click\(\)/g)?.length).toBe(1)
  expect(source).toContain('input[type="radio"]')
})

test('a content script stops itself when the extension context dies', () => {
  for (const file of ['public/content/linkedin.js', 'public/content/apply.js']) {
    const source = readFileSync(join(root, file), 'utf8')
    expect(source).toContain('shutdown')
    expect(source).toContain('chrome.runtime && chrome.runtime.id')
    const intervals = source.match(/setInterval\(/g) ?? []
    const cleared = source.match(/clearInterval\(/g) ?? []
    expect(cleared.length).toBeGreaterThan(0)
    expect(intervals.length).toBeGreaterThan(0)
  }
})

test('the worker re-injects the content scripts into tabs that are already open', () => {
  const source = readFileSync(join(root, 'src/worker/main.ts'), 'utf8')
  expect(source).toContain('reinjectOpenTabs')
  expect(source).toContain('content_scripts')
})
