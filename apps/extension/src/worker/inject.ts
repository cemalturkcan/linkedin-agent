export interface DocumentOption {
  text: string
  selected: boolean
}

export interface FormReading {
  fileInput: boolean
  options: DocumentOption[]
}

export interface InjectionResult {
  ok: boolean
  reason: string
  matches?: number
}

export function readForm(): FormReading {
  const roots: (Document | ShadowRoot)[] = [document]
  const walk = (root: Document | ShadowRoot) => {
    for (const element of root.querySelectorAll('*')) {
      const shadow = (element as HTMLElement).shadowRoot
      if (!shadow) continue
      roots.push(shadow)
      walk(shadow)
    }
  }
  walk(document)

  const optionSelector = 'input[type="radio"], [role="radio"]'
  const reading: FormReading = { fileInput: false, options: [] }
  const seen: string[] = []

  for (const root of roots) {
    if (root.querySelector('input[type="file"]')) reading.fileInput = true

    for (const control of root.querySelectorAll(optionSelector)) {
      let container: Element | null = control
      for (let step = 0; step < 6 && container; step += 1) {
        const text = (container.textContent || '').trim()
        if (text.length > 3) break
        container = container.parentElement
      }
      const text = ((container || control).textContent || '').replace(/\s+/g, ' ').trim()
      if (!text || text.length > 400 || seen.includes(text)) continue
      seen.push(text)
      reading.options.push({
        text,
        selected:
          (control as HTMLInputElement).checked === true ||
          control.getAttribute('aria-checked') === 'true',
      })
    }
  }
  return reading
}

export function selectDocument(wanted: string): InjectionResult {
  const roots: (Document | ShadowRoot)[] = [document]
  const walk = (root: Document | ShadowRoot) => {
    for (const element of root.querySelectorAll('*')) {
      const shadow = (element as HTMLElement).shadowRoot
      if (!shadow) continue
      roots.push(shadow)
      walk(shadow)
    }
  }
  walk(document)

  const found: Element[] = []
  for (const root of roots) {
    for (const control of root.querySelectorAll('input[type="radio"], [role="radio"]')) {
      let container: Element | null = control
      for (let step = 0; step < 6 && container; step += 1) {
        const text = (container.textContent || '').trim()
        if (text.length > 3) break
        container = container.parentElement
      }
      const text = ((container || control).textContent || '').replace(/\s+/g, ' ').toLowerCase()
      if (text.includes(wanted.toLowerCase())) found.push(control)
    }
  }

  if (found.length === 0) return { ok: false, reason: 'not-listed', matches: 0 }
  if (found.length > 1) return { ok: false, reason: 'ambiguous', matches: found.length }

  const control = found[0] as HTMLElement
  if ((control as HTMLInputElement).checked === true) {
    return { ok: true, reason: 'already-attached', matches: 1 }
  }
  control.click()
  control.dispatchEvent(new Event('input', { bubbles: true, composed: true }))
  control.dispatchEvent(new Event('change', { bubbles: true, composed: true }))
  return { ok: true, reason: 'selected', matches: 1 }
}

export async function attachResume(base64: string, fileName: string): Promise<InjectionResult> {
  const collect = (): HTMLInputElement[] => {
    const found: HTMLInputElement[] = []
    const walk = (root: Document | ShadowRoot) => {
      for (const input of root.querySelectorAll('input[type="file"]')) {
        found.push(input as HTMLInputElement)
      }
      for (const element of root.querySelectorAll('*')) {
        const shadow = (element as HTMLElement).shadowRoot
        if (shadow) walk(shadow)
      }
    }
    walk(document)
    return found
  }

  try {
    const bytes = Uint8Array.from(atob(base64), (character) => character.charCodeAt(0))
    const file = new File([bytes], fileName, { type: 'application/pdf' })

    let inputs = collect()
    const deadline = Date.now() + 3000
    while (inputs.length === 0 && Date.now() < deadline) {
      await new Promise((resolve) => setTimeout(resolve, 250))
      inputs = collect()
    }

    inputs = inputs.filter(
      (input) =>
        !input.disabled &&
        (!input.accept || /pdf|msword|officedocument|octet-stream|\*/i.test(input.accept)),
    )
    if (inputs.length === 0) return { ok: false, reason: 'no-input', matches: 0 }

    const score = (input: HTMLInputElement): number => {
      let points = 0
      const meta = `${input.accept || ''} ${input.id || ''} ${input.name || ''} ${input.className || ''}`
      if (/pdf/i.test(input.accept || '')) points += 2
      if (/resume|upload|document|cv/i.test(meta)) points += 3
      return points
    }
    inputs.sort((left, right) => score(right) - score(left))
    const target = inputs[0] as HTMLInputElement
    if (score(target) < 3) return { ok: false, reason: 'no-resume-field', matches: inputs.length }

    const transfer = new DataTransfer()
    transfer.items.add(file)
    target.files = transfer.files
    target.dispatchEvent(new Event('input', { bubbles: true, composed: true }))
    target.dispatchEvent(new Event('change', { bubbles: true, composed: true }))
    return { ok: true, reason: 'uploaded', matches: inputs.length }
  } catch (error) {
    return { ok: false, reason: (error as Error).message, matches: 0 }
  }
}
