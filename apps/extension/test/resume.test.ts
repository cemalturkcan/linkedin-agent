import { expect, test } from 'bun:test'
import {
  chooseResume,
  decideAttachment,
  fileNameIn,
  modeOf,
  ONE_NAME,
  ONE_NAME_CONSEQUENCE,
  PER_VARIANT,
  pickLanguage,
  readStored,
  uploadName,
} from '@/lib/linkedin/resume'

test('a variant gets one stable name derived from the resume itself', () => {
  expect(uploadName('resume.pdf', 'BE', 'en', PER_VARIANT)).toBe('resume-be-en.pdf')
  expect(uploadName('resume.pdf', 'BE', 'tr', PER_VARIANT)).toBe('resume-be-tr.pdf')
  expect(uploadName('resume.pdf', 'FE', 'en', PER_VARIANT)).toBe('resume-fe-en.pdf')
  expect(uploadName('resume.pdf', 'BE', 'en', PER_VARIANT)).toBe(
    uploadName('resume.pdf', 'be', 'EN', PER_VARIANT),
  )
})

test('one name means every variant attaches under the name the person chose', () => {
  expect(uploadName('my-cv.pdf', 'AN', 'en', ONE_NAME)).toBe('my-cv.pdf')
  expect(uploadName('my-cv.pdf', 'BE', 'tr', ONE_NAME)).toBe('my-cv.pdf')
  expect(uploadName('my-cv.pdf', 'FE', 'en', ONE_NAME)).toBe(
    uploadName('my-cv.pdf', 'GO', 'tr', ONE_NAME),
  )
})

test('one name is the default, and an unknown mode is read as one name', () => {
  expect(modeOf(undefined)).toBe(ONE_NAME)
  expect(modeOf('')).toBe(ONE_NAME)
  expect(modeOf('whatever')).toBe(ONE_NAME)
  expect(modeOf(PER_VARIANT)).toBe(PER_VARIANT)
  expect(uploadName('my-cv.pdf', 'AN', 'en')).toBe('my-cv.pdf')
})

test('the base name is the person own setting, and it is always a pdf', () => {
  expect(uploadName('Jan Novak CV', 'BE', 'en', PER_VARIANT)).toBe('jan-novak-cv-be-en.pdf')
  expect(uploadName('', 'BE', 'en', PER_VARIANT)).toBe('resume-be-en.pdf')
  expect(uploadName('../../etc/passwd', 'BE', 'en', PER_VARIANT)).toBe('passwd-be-en.pdf')
  expect(uploadName('cv.PDF', 'BE', 'en', PER_VARIANT)).toBe('cv-be-en.pdf')
  expect(uploadName('../../etc/passwd', 'BE', 'en', ONE_NAME)).toBe('passwd.pdf')
})

test('a file name is read out of what the card says, and only a pdf counts', () => {
  expect(fileNameIn('resume-be-en.pdf Uploaded 3/14/2026 Select')).toBe('resume-be-en.pdf')
  expect(fileNameIn('Resume-BE-EN.pdf')).toBe('Resume-BE-EN.pdf')
  expect(fileNameIn('no file here')).toBe('')
  expect(fileNameIn('notes.docx')).toBe('')
  expect(readStored([{ text: 'a.pdf x', selected: true }, { text: 'nothing', selected: false }])).toEqual([
    { name: 'a.pdf', selected: true },
  ])
})

test('the cards linkedin actually draws are read for their file names', () => {
  const drawn = [
    { text: 'Deselect resume resume.pdf 49 KB · Last used on 8/11/2026', selected: true },
    { text: 'Select resume my-cv.pdf 49 KB · Last used on 8/10/2026', selected: false },
  ]
  expect(readStored(drawn)).toEqual([
    { name: 'resume.pdf', selected: true },
    { name: 'my-cv.pdf', selected: false },
  ])
})

test('one name cannot tell the stored copies apart, so it uploads', () => {
  const stored = [{ name: 'my-cv.pdf', selected: false }]
  expect(decideAttachment(stored, 'my-cv.pdf', ONE_NAME)).toEqual({
    action: 'upload',
    name: 'my-cv.pdf',
    why: ONE_NAME_CONSEQUENCE,
  })
  expect(decideAttachment([{ name: 'my-cv.pdf', selected: true }], 'my-cv.pdf', ONE_NAME).action).toBe(
    'upload',
  )
})

test('an exact match selects the file linkedin already holds and uploads nothing', () => {
  const decision = decideAttachment(
    [
      { name: 'resume-fe-en.pdf' },
      { name: 'resume-be-en.pdf' },
      { name: 'resume-be-tr.pdf' },
    ],
    'resume-be-en.pdf',
    PER_VARIANT,
  )
  expect(decision).toEqual({
    action: 'select',
    index: 1,
    name: 'resume-be-en.pdf',
    attached: false,
    why: 'linkedin already holds resume-be-en.pdf, so it is selected rather than uploaded again',
  })
})

test('a file already attached is recognised rather than replaced', () => {
  const decision = decideAttachment(
    [{ name: 'resume-be-en.pdf', selected: true }],
    'resume-be-en.pdf',
    PER_VARIANT,
  )
  expect(decision.action).toBe('select')
  if (decision.action === 'select') expect(decision.attached).toBe(true)
})

test('the wrong file is never chosen, however close its name is', () => {
  const stored = [
    { name: 'resume-be-tr.pdf' },
    { name: 'resume-be-en(1).pdf' },
    { name: 'resume-be-en 2.pdf' },
    { name: 'resume-be-en.pdf.pdf' },
    { name: 'my-resume-be-en.pdf' },
    { name: 'resume-be.pdf' },
  ]
  const decision = decideAttachment(stored, 'resume-be-en.pdf', PER_VARIANT)
  expect(decision.action).toBe('upload')
  expect(decision.why).toBe('linkedin holds no file called resume-be-en.pdf')
})

test('an ambiguous list uploads rather than guessing which copy is right', () => {
  const decision = decideAttachment(
    [{ name: 'resume-be-en.pdf' }, { name: 'resume-be-en.pdf' }],
    'resume-be-en.pdf',
    PER_VARIANT,
  )
  expect(decision).toEqual({
    action: 'upload',
    name: 'resume-be-en.pdf',
    why: 'linkedin holds 2 files called resume-be-en.pdf and nothing tells them apart, so a fresh one is uploaded rather than guessing',
  })
})

test('an empty store uploads once, and the next application selects that copy', () => {
  const first = decideAttachment([], 'resume-be-en.pdf', PER_VARIANT)
  expect(first.action).toBe('upload')

  const second = decideAttachment([{ name: 'resume-be-en.pdf' }], 'resume-be-en.pdf', PER_VARIANT)
  expect(second.action).toBe('select')
})

test('case and padding around a name do not make it a different file', () => {
  const decision = decideAttachment([{ name: '  Resume-BE-EN.pdf ' }], 'resume-be-en.pdf', PER_VARIANT)
  expect(decision.action).toBe('select')
})

test('a screening question is never mistaken for a stored cv', () => {
  const asked = [
    { text: 'How many years of experience do you have with Go? Yes No', selected: false },
    { text: 'Are you authorised to work in this country? Yes', selected: true },
    { text: 'Do you require sponsorship? No', selected: false },
  ]
  expect(readStored(asked)).toEqual([])
  expect(decideAttachment(readStored(asked), 'resume-be-en.pdf', PER_VARIANT).action).toBe('upload')
})

test('the language a variant is served in follows the person settings', () => {
  const resumes = [
    { code: 'BE', label: 'backend', role: 'backend', languages: ['en', 'tr'] },
    { code: 'FE', label: 'frontend', role: 'frontend', languages: ['tr'] },
  ]
  expect(chooseResume(resumes, 'BE', 'tr')).toEqual({ code: 'BE', lang: 'tr' })
  expect(chooseResume(resumes, 'BE', null)).toEqual({ code: 'BE', lang: 'en' })
  expect(chooseResume(resumes, 'be', null, ['tr'])).toEqual({ code: 'BE', lang: 'tr' })
  expect(chooseResume(resumes, 'FE', 'de')).toEqual({ code: 'FE', lang: 'tr' })
  expect(chooseResume(resumes, 'XX', 'en')).toBeNull()
  expect(pickLanguage([], ['tr'])).toBe('')
})
