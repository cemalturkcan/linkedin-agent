import { expect, test } from 'bun:test'
import type { ResumeState } from '@/lib/agent/types'
import { closestResume, tokensOf } from '@/lib/linkedin/closest'

function indexed(code: string, core: string[], role = '', secondary: string[] = []): ResumeState {
  return {
    code,
    label: code.toLowerCase(),
    fileLanguages: ['en'],
    indexed: true,
    profile: {
      code,
      targetRole: role,
      seniorityClaimed: 'senior',
      coreStack: core,
      secondaryStack: secondary,
      domains: [],
      languages: [],
      yearsClaimed: 5,
      earliestStart: '',
      places: [],
      summary: '',
    },
  }
}

function bare(code: string): ResumeState {
  return { code, label: code.toLowerCase(), fileLanguages: ['en'], indexed: false, profile: null }
}

test('tokens survive linkedin spellings of stack words', () => {
  const tokens = tokensOf('Node.js, C++ and C# on .NET with Vue')
  expect(tokens.has('nodejs')).toBe(true)
  expect(tokens.has('c++')).toBe(true)
  expect(tokens.has('c#')).toBe(true)
  expect(tokens.has('vue')).toBe(true)
})

test('the resume whose stack the posting names is the closest', () => {
  const pick = closestResume(
    [indexed('GO', ['go', 'postgres']), indexed('JA', ['java', 'spring boot'])],
    { title: 'Senior Java Developer', body: 'we run java services on spring boot' },
  )
  expect(pick?.code).toBe('JA')
  expect(pick?.matched).toContain('java')
})

test('a multiword stack phrase needs most of its words, not an echo of one', () => {
  const pick = closestResume([indexed('JA', ['spring boot'])], {
    title: '',
    body: 'our services run on spring boot',
  })
  expect(pick?.code).toBe('JA')
  for (const body of ['we sell boots and nothing else', 'a walk in the spring sunshine']) {
    expect(closestResume([indexed('JA', ['spring boot'])], { title: '', body })).toBeNull()
  }
})

test('version numbers in a stack phrase do not stop the words from matching', () => {
  const pick = closestResume(
    [indexed('DO', ['C#', 'ASP.NET Core', '.NET 8/10'], 'Senior .NET Developer')],
    { title: 'Kartlı Sistemler .NET Yazılım Uzmanı', body: '' },
  )
  expect(pick?.code).toBe('DO')
  expect(pick?.matched).toContain('.NET 8/10')
})

test('a three word phrase still hits when two of its words are present', () => {
  const pick = closestResume([indexed('DO', ['Entity Framework Core'])], {
    title: '',
    body: 'we use entity framework for persistence',
  })
  expect(pick?.code).toBe('DO')
})

test('a tie between two resumes picks neither', () => {
  const pick = closestResume(
    [indexed('AA', ['react']), indexed('BB', ['react'])],
    { title: 'React Developer', body: '' },
  )
  expect(pick).toBeNull()
})

test('a faint echo below the floor is not a pick', () => {
  const pick = closestResume([indexed('GO', [], '', ['docker'])], {
    title: '',
    body: 'docker is mentioned once in passing',
  })
  expect(pick).toBeNull()
})

test('resumes the agent never indexed cannot be picked', () => {
  const pick = closestResume([bare('JA')], {
    title: 'Java Developer',
    body: 'java java java',
  })
  expect(pick).toBeNull()
})

test('a title hit outweighs the same hit buried in the body', () => {
  const pick = closestResume(
    [indexed('RE', ['react']), indexed('AN', ['angular'])],
    { title: 'React Developer', body: 'migrating away from angular' },
  )
  expect(pick?.code).toBe('RE')
})
