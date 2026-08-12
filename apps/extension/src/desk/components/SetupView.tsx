import { useState } from 'react'
import type { CredentialStatus, PluginState, ProfileState, SetupState } from '@/lib/agent/types'
import { relativeAge } from '@/lib/format'
import { cn } from '@/lib/utils'
import {
  Action,
  Choice,
  Note,
  Quiet,
  TEXTAREA,
  blurOnEscape,
  INPUT,
} from '@/desk/components/Fields'
import type { Resource } from '@/desk/useDeskState'

const CREDENTIAL_FILE = '~/.claude/.credentials.json'

const CLIPBOARDS = ['wayland', 'x11', 'macos', 'windows', 'plain'] as const

type Clipboard = (typeof CLIPBOARDS)[number]

const ONE_LINERS: Record<Clipboard, string> = {
  wayland: `cat ${CREDENTIAL_FILE} | wl-copy`,
  x11: `cat ${CREDENTIAL_FILE} | xclip -selection clipboard`,
  macos: `cat ${CREDENTIAL_FILE} | pbcopy`,
  windows: `type %USERPROFILE%\\.claude\\.credentials.json | clip`,
  plain: `cat ${CREDENTIAL_FILE}`,
}

function guessClipboard(): Clipboard {
  const agent = typeof navigator === 'undefined' ? '' : navigator.userAgent
  if (/mac/i.test(agent)) return 'macos'
  if (/win/i.test(agent)) return 'windows'
  if (/linux|x11/i.test(agent)) return 'wayland'
  return 'plain'
}

type LinkState = 'done' | 'running' | 'waiting' | 'missing'

const WORDS: Record<LinkState, string> = {
  done: 'done',
  running: 'running',
  waiting: 'waiting',
  missing: 'missing',
}

interface Link {
  key: string
  title: string
  state: LinkState
  detail: string
}

function ago(value: number | string | null): string {
  const age = relativeAge(value)
  if (!age) return ''
  return age === 'now' ? 'just now' : `${age} ago`
}

function expiry(minutes: number | null): string {
  if (minutes === null) return 'no expiry recorded'
  if (minutes <= 0) return 'expired, and the next call refreshes it'
  if (minutes < 90) return `${Math.round(minutes)} minutes left on this token`
  return `${Math.round(minutes / 60)} hours left on this token`
}

function pdfCount(count: number): string {
  return `${count} ${count === 1 ? 'pdf' : 'pdfs'}`
}

interface SetupViewProps {
  setup: Resource<SetupState>
  credentials: Resource<CredentialStatus>
  profile: Resource<ProfileState>
  plugin: PluginState | null
  busy: string | null
  onFolder(dir: string): Promise<boolean>
  onPaste(value: string): Promise<boolean>
  onClear(): Promise<void>
  onIndex(force: boolean): Promise<void>
}

export function SetupView({
  setup,
  credentials,
  profile,
  plugin,
  busy,
  onFolder,
  onPaste,
  onClear,
  onIndex,
}: SetupViewProps) {
  const [path, setPath] = useState('')
  const state = setup.value
  const status = credentials.value ?? state?.credentials ?? null
  const derived = profile.value

  const loaded = status?.loaded === true
  const resumes = state?.resumes ?? []
  const folderDone = resumes.length > 0
  const indexing = derived?.indexing ?? state?.indexing ?? false
  const indexState = derived?.indexState ?? state?.indexState ?? 'never'

  const links: Link[] = [
    {
      key: 'credentials',
      title: 'claude credentials',
      state: loaded ? 'done' : 'missing',
      detail: loaded
        ? `answered by ${status?.where ?? 'a source that did not name itself'}`
        : 'nothing signs in, so no cv is read and no posting is screened',
    },
    {
      key: 'folder',
      title: 'cv folder',
      state: folderDone ? 'done' : 'missing',
      detail: folderDone
        ? `${state?.resumeDir ?? ''}, ${pdfCount(resumes.length)}`
        : 'the agent reads your cvs off disk, and it has no folder to read',
    },
    {
      key: 'index',
      title: 'indexing',
      state: !folderDone ? 'waiting' : indexing ? 'running' : indexState === 'current' ? 'done' : 'missing',
      detail: !folderDone
        ? 'waits on the cv folder'
        : indexing
          ? 'reading the cvs now'
          : indexState === 'current'
            ? `${resumes.length} read ${ago(derived?.indexedAt ?? null)}`.trim()
            : indexState === 'stale'
              ? 'a cv changed since the last read'
              : 'these cvs have never been read',
    },
    {
      key: 'plugin',
      title: 'this extension',
      state: plugin?.connected ? 'done' : plugin?.everSeen ? 'waiting' : 'missing',
      detail: plugin?.connected
        ? `checked in ${ago(plugin.lastSeen)}, version ${plugin.version ?? 'unknown'}`
        : plugin?.everSeen
          ? `quiet since ${ago(plugin.lastSeen)}`
          : 'no extension has checked in, so nothing is fetched and nothing is attached',
    },
  ]

  const short = links.filter((link) => link.state !== 'done')
  const missing = state?.missing ?? []

  return (
    <div className="w-full max-w-[46rem]">
      <div className="flex items-baseline justify-between gap-3">
        <p className="max-w-[42rem] text-body text-muted">
          four links. a round runs when every one of them holds, and the list says which one is
          short.
        </p>
        {!setup.live && state ? <span className="label shrink-0 text-muted">cached</span> : null}
      </div>

      {!state ? (
        <Note term="standby">
          {setup.failure
            ? `${setup.failure.message}. none of the four links is known, so none of them is drawn.`
            : 'the setup chain is read from the agent, and it has not answered yet.'}
        </Note>
      ) : null}

      {state?.error ? <Note term="error">{state.error}</Note> : null}

      {!state ? null : (
        <>
      <ol className="mt-4 divide-y divide-hairline border-y border-hairline">
        {links.map((link, index) => (
          <li key={link.key} className="flex items-baseline gap-3 py-1.5">
            <span className="grid-num w-3 shrink-0 text-center text-micro text-muted">
              {index + 1}
            </span>
            <span className="w-[9rem] shrink-0 text-row text-ink">{link.title}</span>
            <span
              className={cn(
                'label w-[3.75rem] shrink-0',
                link.state === 'done' ? 'text-muted' : 'text-ink',
              )}
            >
              {WORDS[link.state]}
            </span>
            <span className="min-w-0 flex-1 text-meta text-muted">{link.detail}</span>
          </li>
        ))}
      </ol>

      <p className="mt-2 text-meta text-muted">
        {short.length === 0
          ? 'nothing is short, so a round can plan, fetch, screen and hand you the inbox.'
          : `still short: ${(missing.length > 0 ? missing : short.map((link) => link.title)).join('; ')}.`}
      </p>

      {loaded ? (
        <LoadedCredentials status={status} busy={busy} onPaste={onPaste} onClear={onClear} />
      ) : (
        <PasteCredentials
          error={status?.error ?? null}
          busy={busy}
          onPaste={onPaste}
        />
      )}

      <section className="mt-8 border-t border-hairline pt-5">
        <h2 className="label text-ink">cv folder</h2>
        <p className="mt-1.5 max-w-[42rem] text-meta text-muted">
          one sub-folder per role, or a flat folder of pdfs. choosing one starts the read by itself.
        </p>

        {state?.resumeDir ? (
          <p className="mt-2 flex items-baseline gap-2 text-meta">
            <span className="label shrink-0 text-muted">current</span>
            <span className="min-w-0 flex-1 truncate text-ink">{state.resumeDir}</span>
          </p>
        ) : null}

        {(state?.candidates ?? []).length > 0 ? (
          <ul className="mt-2 divide-y divide-hairline border border-hairline">
            {(state?.candidates ?? []).map((candidate) => (
              <li key={candidate.dir} className="flex items-center gap-3 px-2 py-1">
                <div className="min-w-0 flex-1">
                  <div className="truncate text-body text-ink">{candidate.dir}</div>
                  {candidate.sample ? (
                    <div className="truncate text-meta text-muted">{candidate.sample}</div>
                  ) : null}
                </div>
                <span className="grid-num shrink-0 text-meta text-muted">{candidate.count}</span>
                <Action
                  onClick={() => void onFolder(candidate.dir)}
                  disabled={busy === 'scanning'}
                >
                  use
                </Action>
              </li>
            ))}
          </ul>
        ) : (
          <p className="mt-2 text-meta text-muted">
            nothing found automatically, so name the folder yourself.
          </p>
        )}

        <form
          className="mt-3 flex items-center gap-2"
          onSubmit={(event) => {
            event.preventDefault()
            if (path.trim()) void onFolder(path.trim())
          }}
        >
          <label className="label shrink-0 text-muted" htmlFor="resume-dir">
            another folder
          </label>
          <input
            id="resume-dir"
            value={path}
            onChange={(event) => setPath(event.target.value)}
            onKeyDown={blurOnEscape}
            placeholder="the folder holding your cv pdfs"
            spellCheck={false}
            autoComplete="off"
            className={cn(INPUT, 'flex-1')}
          />
          <Action
            onClick={() => {
              if (path.trim()) void onFolder(path.trim())
            }}
            disabled={path.trim() === '' || busy === 'scanning'}
          >
            {busy === 'scanning' ? 'scanning' : 'scan'}
          </Action>
        </form>

        <div className="mt-4 flex flex-wrap items-center gap-3">
          <Action
            onClick={() => void onIndex(indexState === 'current')}
            disabled={!folderDone || indexing || busy === 'indexing'}
          >
            {indexing || busy === 'indexing'
              ? 'reading'
              : indexState === 'never'
                ? 'read the cvs'
                : 'read them again'}
          </Action>
          {indexing ? (
            <span className="grid-num text-meta text-muted">
              {`${derived?.progress.phase ?? 'starting'} ${derived?.progress.done ?? 0} of ${derived?.progress.total ?? 0}`}
            </span>
          ) : null}
        </div>
      </section>
        </>
      )}
    </div>
  )
}

function LoadedCredentials({
  status,
  busy,
  onPaste,
  onClear,
}: {
  status: CredentialStatus | null
  busy: string | null
  onPaste(value: string): Promise<boolean>
  onClear(): Promise<void>
}) {
  const [replacing, setReplacing] = useState(false)

  return (
    <section className="mt-8 border-t border-hairline pt-5">
      <h2 className="label text-ink">claude credentials</h2>
      <div className="mt-2 flex flex-wrap items-baseline gap-x-3 gap-y-1 border border-hairline bg-accent px-2 py-1.5">
        <span className="label shrink-0 text-ink">loaded</span>
        <span className="min-w-0 flex-1 text-meta text-ink">
          {status?.where ?? 'a source that did not name itself'}
        </span>
        <span className="grid-num shrink-0 text-meta text-muted">
          {expiry(status?.expiresInMinutes ?? null)}
        </span>
      </div>

      {status && !status.refreshable ? (
        <Note term="standby">
          this credential carries no refresh token, so it stops working when it expires and has to be
          pasted again
        </Note>
      ) : null}

      <div className="mt-3 flex flex-wrap items-center gap-3">
        <Action onClick={() => setReplacing((open) => !open)} pressed={replacing}>
          {replacing ? 'keep what answered' : 'replace'}
        </Action>
        {status?.stored ? (
          <Quiet onClick={() => void onClear()}>clear the pasted one</Quiet>
        ) : null}
        {status?.source !== 'store' && status?.stored ? (
          <span className="text-meta text-muted">
            a pasted credential is held here too, and takes over when the one above goes away
          </span>
        ) : null}
      </div>

      {replacing ? (
        <PasteBox busy={busy} onPaste={async (value) => {
          const ok = await onPaste(value)
          if (ok) setReplacing(false)
          return ok
        }} />
      ) : null}
    </section>
  )
}

function PasteCredentials({
  error,
  busy,
  onPaste,
}: {
  error: string | null
  busy: string | null
  onPaste(value: string): Promise<boolean>
}) {
  return (
    <section className="mt-8 border-t border-hairline pt-5">
      <h2 className="label text-ink">claude credentials</h2>
      <p className="mt-1.5 max-w-[42rem] text-meta text-muted">
        the agent signs in as claude code and takes the first of three: an environment variable
        carrying the credentials, credentials pasted here, then a claude code login on this machine.
        none of them answered.
      </p>
      {error ? <Note term="error">{error}</Note> : null}
      <PasteBox busy={busy} onPaste={onPaste} />
    </section>
  )
}

function PasteBox({
  busy,
  onPaste,
}: {
  busy: string | null
  onPaste(value: string): Promise<boolean>
}) {
  const [value, setValue] = useState('')
  const [clipboard, setClipboard] = useState<Clipboard>(guessClipboard)
  const [copied, setCopied] = useState(false)

  const line = ONE_LINERS[clipboard]

  async function copy() {
    try {
      await navigator.clipboard.writeText(line)
      setCopied(true)
      window.setTimeout(() => setCopied(false), 1400)
    } catch {
      setCopied(false)
    }
  }

  return (
    <div className="mt-4 border-t border-hairline pt-3">
      <span className="label text-muted">get them onto your clipboard</span>
      <p className="mt-1 max-w-[42rem] text-meta text-muted">
        the machine holding the credentials is often not the machine reading this. copy the line for
        that machine, run it there, then paste the result below.
      </p>

      <div className="mt-2 max-w-[26rem]">
        <Choice<Clipboard>
          name="clipboard tool"
          value={clipboard}
          options={CLIPBOARDS}
          onChange={setClipboard}
        />
      </div>

      <div className="mt-2 flex items-center gap-2">
        <code className="min-w-0 flex-1 overflow-x-auto whitespace-pre border border-hairline bg-raised px-2 py-1 text-meta text-ink">
          {line}
        </code>
        <Action onClick={() => void copy()}>{copied ? 'copied' : 'copy'}</Action>
      </div>

      <form
        className="mt-4"
        onSubmit={(event) => {
          event.preventDefault()
          if (value.trim()) void onPaste(value).then((ok) => ok && setValue(''))
        }}
      >
        <label className="label text-muted" htmlFor="credentials-paste">
          paste them
        </label>
        <textarea
          id="credentials-paste"
          value={value}
          onChange={(event) => setValue(event.target.value)}
          onKeyDown={blurOnEscape}
          spellCheck={false}
          autoComplete="off"
          className={cn(TEXTAREA, 'mt-1.5')}
        />
        <div className="mt-2 flex flex-wrap items-center gap-3">
          <Action
            onClick={() => {
              if (value.trim()) void onPaste(value).then((ok) => ok && setValue(''))
            }}
            disabled={value.trim() === '' || busy === 'checking'}
          >
            {busy === 'checking' ? 'checking' : 'use these'}
          </Action>
          <span className="text-meta text-muted">
            held at 0600 under the agent's data folder. it is never sent back out, so this box stays
            empty once it takes.
          </span>
        </div>
      </form>
    </div>
  )
}
