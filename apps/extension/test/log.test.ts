import { beforeEach, expect, test } from 'bun:test'
import { clearLog, LOG_MAX, logging, note, readLog, setLogging, settled } from '@/lib/log'
import { resetMemoryStore } from '@/lib/storage'

beforeEach(() => {
  resetMemoryStore()
})

test('recording is on until it is switched off', async () => {
  expect(await logging()).toBe(true)

  note('round', 'a round started')
  await settled()
  expect((await readLog()).map((entry) => entry.text)).toEqual(['a round started'])

  await setLogging(false)
  note('round', 'this one is not kept')
  await settled()
  expect((await readLog()).length).toBe(1)

  await setLogging(true)
  note('attach', 'uploaded resume.pdf')
  await settled()
  expect((await readLog()).map((entry) => entry.area)).toEqual(['round', 'attach'])
})

test('the log keeps the newest lines and nothing older', async () => {
  for (let index = 0; index < LOG_MAX + 25; index += 1) note('round', `line ${index}`)
  await settled()

  const rows = await readLog()
  expect(rows.length).toBe(LOG_MAX)
  expect(rows[0]?.text).toBe('line 25')
  expect(rows[rows.length - 1]?.text).toBe(`line ${LOG_MAX + 24}`)
})

test('clearing empties it without switching recording off', async () => {
  note('round', 'a round started')
  await settled()
  await clearLog()

  expect(await readLog()).toEqual([])
  expect(await logging()).toBe(true)
})
