import { useState } from 'react'
import { agent } from '@/lib/agent/client'
import type { PromptPreview as Preview } from '@/lib/agent/types'
import { cn } from '@/lib/utils'
import { Action, Note, Output } from '@/desk/components/Fields'

export function PromptPreview() {
  const [preview, setPreview] = useState<Preview | null>(null)
  const [shown, setShown] = useState('')
  const [reading, setReading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  async function toggle() {
    if (preview) {
      setPreview(null)
      return
    }
    setReading(true)
    setError(null)
    const result = await agent.promptPreview()
    setReading(false)
    if (!result.ok) {
      setError(result.failure.message)
      return
    }
    setPreview(result.value)
    setShown(result.value.prompts[0]?.id ?? '')
  }

  const draft = preview?.prompts.find((prompt) => prompt.id === shown) ?? null

  return (
    <div className="mt-3">
      <Action onClick={() => void toggle()} disabled={reading} pressed={preview !== null}>
        {reading ? 'reading' : preview ? 'hide the prompts' : 'preview the prompts'}
      </Action>

      {error ? <Note term="error">{error}</Note> : null}

      {preview && !preview.ready ? (
        <Note term="standby">
          {preview.missing || 'the agent cannot assemble a prompt yet'}
        </Note>
      ) : null}

      {preview && preview.prompts.length > 0 ? (
        <div className="mt-3">
          <div
            role="tablist"
            aria-label="assembled prompts"
            className="flex items-stretch border border-hairline"
          >
            {preview.prompts.map((prompt, index) => {
              const active = prompt.id === shown
              return (
                <button
                  key={prompt.id}
                  type="button"
                  role="tab"
                  aria-selected={active}
                  onClick={() => setShown(prompt.id)}
                  className={cn(
                    'label h-6 flex-1 px-2',
                    index > 0 ? 'border-l border-hairline' : '',
                    active ? 'bg-active text-onactive' : 'text-muted hover:bg-hover hover:text-ink',
                  )}
                >
                  {prompt.label}
                </button>
              )
            })}
          </div>

          {draft?.unavailable ? (
            <Note term="standby">{draft.unavailable}</Note>
          ) : (
            <Output label="exactly what the model sees" text={draft?.system ?? ''} height="max-h-[30rem]" />
          )}
        </div>
      ) : null}
    </div>
  )
}
