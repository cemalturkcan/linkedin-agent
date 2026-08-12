import type { ProfileState, ResumeState } from '@/lib/agent/types'
import { plural, relativeAge } from '@/lib/format'
import { Action, Facts, Note } from '@/desk/components/Fields'
import type { Resource } from '@/desk/useDeskState'

const INDEX_SENTENCE: Record<string, string> = {
  never:
    'these cvs have never been read. nothing about the person is derived yet, so screening has nothing to judge against.',
  stale:
    'a cv changed since the last read, so the profiles below describe files you no longer have. read them again and the screener judges what is on disk now.',
  current:
    'the profiles below were derived from the files on disk as they are now.',
}

const PHASE_SENTENCE: Record<string, string> = {
  starting: 'starting the read',
  extracting: 'pulling the text out of the pdfs',
  profiling: 'deriving one profile per variant',
  synthesising: 'folding the variants into one candidate',
  done: 'finished',
  failed: 'stopped',
  idle: 'idle',
}

function places(entries: { name: string; ring: string }[]): string {
  return entries.map((place) => (place.ring ? `${place.name} (${place.ring})` : place.name)).join(', ')
}

function Variant({ resume }: { resume: ResumeState }) {
  const derived = resume.profile

  return (
    <li className="py-2">
      <div className="flex items-baseline gap-2">
        <span className="label w-8 shrink-0 text-ink">{resume.code}</span>
        <span className="min-w-0 flex-1 text-row text-ink">
          {derived?.targetRole || resume.label}
        </span>
        {derived?.seniorityClaimed ? (
          <span className="label shrink-0 text-muted">{derived.seniorityClaimed}</span>
        ) : null}
        <span className="label shrink-0 text-muted">
          {resume.fileLanguages.join(' ') || 'no file'}
        </span>
      </div>

      {!derived ? (
        <p className="mt-1 pl-10 text-meta text-muted">
          not read yet, so nothing is derived from this variant and the screener will not pick it
          deliberately.
        </p>
      ) : (
        <div className="mt-1 pl-10">
          {derived.summary ? (
            <p className="max-w-[42rem] text-meta text-muted">{derived.summary}</p>
          ) : null}
          <dl className="mt-1 space-y-0.5">
            <Line term="leads with">{derived.coreStack.join(', ')}</Line>
            <Line term="also lists">{derived.secondaryStack.join(', ')}</Line>
            <Line term="domains">{derived.domains.join(', ')}</Line>
            <Line term="places">{places(derived.places)}</Line>
            <Line term="languages">{derived.languages.join(', ')}</Line>
            <Line term="years">
              {derived.yearsClaimed > 0
                ? `${plural(derived.yearsClaimed, 'year', 'years')}, earliest ${derived.earliestStart || 'not stated'}`
                : ''}
            </Line>
          </dl>
        </div>
      )}
    </li>
  )
}

function Line({ term, children }: { term: string; children: string }) {
  if (!children) return null
  return (
    <div className="flex items-baseline gap-2 text-meta">
      <dt className="label w-[4.5rem] shrink-0 text-muted">{term}</dt>
      <dd className="min-w-0 flex-1 text-ink">{children}</dd>
    </div>
  )
}

interface ProfileViewProps {
  profile: Resource<ProfileState>
  busy: string | null
  onIndex(force: boolean): Promise<void>
}

export function ProfileView({ profile, busy, onIndex }: ProfileViewProps) {
  const state = profile.value
  const candidate = state?.candidate ?? null
  const resumes = state?.resumes ?? []
  const indexState = state?.indexState ?? 'never'
  const indexing = state?.indexing ?? false
  const read = resumes.filter((resume) => resume.indexed).length
  const age = relativeAge(state?.indexedAt ?? null)

  return (
    <div className="w-full max-w-[46rem]">
      <p className="max-w-[42rem] text-body text-muted">
        the agent reads your cv pdfs once and derives everything below from them. the screener sees
        these derived profiles, never the cv text itself.
      </p>

      {state?.resumeDir ? (
        <p className="mt-2 flex items-baseline gap-2 text-meta">
          <span className="label shrink-0 text-muted">folder</span>
          <span className="min-w-0 flex-1 truncate text-ink">{state.resumeDir}</span>
        </p>
      ) : null}

      {state?.error ? <Note term="error">{state.error}</Note> : null}

      {!state ? (
        <Note term="standby">
          {profile.failure
            ? profile.failure.message
            : 'the profile is derived by the agent, and it has not answered yet.'}
        </Note>
      ) : null}

      <p className="mt-4 max-w-[42rem] text-body text-ink">
        {!state
          ? 'nothing is known about the index state, so nothing below is drawn.'
          : indexing
          ? `reading them now: ${PHASE_SENTENCE[state?.progress.phase ?? 'idle'] ?? state?.progress.phase}, ${state?.progress.done ?? 0} of ${state?.progress.total ?? 0}.`
          : INDEX_SENTENCE[indexState]}
      </p>

      <div className="mt-3 flex flex-wrap items-center gap-3">
        <Action
          onClick={() => void onIndex(indexState === 'current')}
          disabled={indexing || busy === 'indexing' || resumes.length === 0}
        >
          {indexing || busy === 'indexing'
            ? 'reading'
            : indexState === 'never'
              ? 'read the cvs'
              : 'read them again'}
        </Action>
        {!indexing && age ? (
          <span className="text-meta text-muted">{`read ${age === 'now' ? 'just now' : `${age} ago`}`}</span>
        ) : null}
        {!profile.live && state ? <span className="label text-muted">cached</span> : null}
      </div>

      {candidate ? (
        <section className="mt-6">
          <h2 className="label text-ink">the candidate</h2>
          {candidate.headline ? (
            <p className="mt-2 max-w-[42rem] text-body text-ink">{candidate.headline}</p>
          ) : null}
          <div className="mt-2">
            <Facts
              items={[
                {
                  term: 'experience',
                  value: [
                    candidate.yearsExperience > 0
                      ? plural(candidate.yearsExperience, 'year', 'years')
                      : '',
                    candidate.seniorityBand,
                  ]
                    .filter(Boolean)
                    .join(', '),
                },
                { term: 'core', value: candidate.coreStack.join(', ') },
                { term: 'secondary', value: candidate.secondaryStack.join(', ') },
                { term: 'domains', value: candidate.domains.join(', ') },
                { term: 'languages', value: candidate.workLanguages.join(', ') },
                { term: 'places', value: places(candidate.places) },
              ]}
            />
          </div>
        </section>
      ) : null}

      {resumes.length > 0 ? (
        <section className="mt-6">
          <div className="flex items-baseline justify-between">
            <h2 className="label text-ink">the variants</h2>
            <span className="grid-num text-meta text-muted">{`${read} of ${resumes.length} read`}</span>
          </div>
          <ul className="mt-2 divide-y divide-hairline border-y border-hairline">
            {resumes.map((resume) => (
              <Variant key={resume.code} resume={resume} />
            ))}
          </ul>
        </section>
      ) : state ? (
        <p className="mt-6 text-meta text-muted">
          no cv is on file, so there is no variant to describe. the setup view points the agent at
          the folder.
        </p>
      ) : null}
    </div>
  )
}
