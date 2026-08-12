import { useEffect, useMemo, useState } from 'react'
import type { Place, ProfileState, Settings, SettingsDocument } from '@/lib/agent/types'
import { cn } from '@/lib/utils'
import {
  Action,
  Choice,
  Facts,
  Group,
  INPUT,
  NumberField,
  Note,
  Picks,
  Quiet,
  Row,
  Section,
  Switch,
  TEXTAREA,
  TagEditor,
  blurOnEscape,
} from '@/desk/components/Fields'
import { PLACE_ID_IS_THE_AGENT_S, PlaceEditor } from '@/desk/components/PlaceEditor'
import { consequenceOf, UPLOAD_NAME_IS_PUBLIC } from '@/lib/linkedin/resume'
import { PromptPreview } from '@/desk/components/PromptPreview'
import type { Resource } from '@/desk/useDeskState'

const SCOPE_WORDS: Record<string, string> = {
  local: 'anchored in the same city',
  country: 'anchored anywhere in the same country',
  region: 'anchored anywhere in the same region',
  global: 'anchored anywhere in the world',
}

const WORLDWIDE_HINT =
  'a worldwide feed carries far more than anyone can take, so the planner reaches it last, after the places your cvs point at.'

function derivedPlaces(profile: ProfileState | null): Place[] {
  const seen = new Map<string, Place>()
  for (const place of profile?.candidate?.places ?? []) {
    if (place.name) seen.set(place.name.toLowerCase(), place)
  }
  for (const resume of profile?.resumes ?? []) {
    for (const place of resume.profile?.places ?? []) {
      if (place.name && !seen.has(place.name.toLowerCase())) seen.set(place.name.toLowerCase(), place)
    }
  }
  return [...seen.values()]
}

function reachLines(settings: Settings): { term: string; value: string }[] {
  const { workplace, relocation, places } = settings.locations
  const taken = [
    workplace.onsite ? 'on site' : '',
    workplace.hybrid ? 'hybrid' : '',
    workplace.remote ? 'remote' : '',
  ].filter(Boolean)

  return [
    {
      term: 'i take',
      value:
        taken.length > 0
          ? taken.join(', ')
          : 'nothing: every workplace type is off, so a round keeps nothing',
    },
    {
      term: 'remote',
      value: workplace.remote
        ? (SCOPE_WORDS[workplace.scope] ?? workplace.scope)
        : 'not taken, so the scope below is unused',
    },
    {
      term: 'places',
      value:
        places.length > 0
          ? places.map((place) => place.name).join(', ')
          : 'none of your own, so the planner starts where your cvs point and widens ring by ring',
    },
    {
      term: 'moving',
      value: relocation.open
        ? relocation.targets.length > 0
          ? `open, to ${relocation.targets.join(', ')}`
          : 'open, anywhere the role is worth it'
        : 'staying put',
    },
  ]
}

interface SettingsViewProps {
  settings: Resource<SettingsDocument>
  profile: Resource<ProfileState>
  busy: string | null
  onSave(next: Settings): Promise<boolean>
}

export function SettingsView({ settings, profile, busy, onSave }: SettingsViewProps) {
  const document = settings.value
  const [draft, setDraft] = useState<Settings | null>(document?.settings ?? null)
  const [savedAt, setSavedAt] = useState<number | null>(null)

  useEffect(() => {
    setDraft(document?.settings ?? null)
  }, [document])

  const derived = useMemo(() => derivedPlaces(profile.value), [profile.value])

  if (!draft || !document) {
    return (
      <div className="w-full max-w-[46rem]">
        <p className="text-body text-muted">
          {settings.failure
            ? settings.failure.message
            : 'settings are read from the agent, and it has not answered yet.'}
        </p>
      </div>
    )
  }

  const enums = document.enums
  const saved = document.settings
  const dirty = JSON.stringify(draft) !== JSON.stringify(saved)
  const ladder = enums.seniorityLadder

  function edit(patch: Partial<Settings>) {
    setDraft((previous) => (previous ? { ...previous, ...patch } : previous))
  }

  function editGroup<K extends keyof Settings>(key: K, patch: Partial<Settings[K]>) {
    setDraft((previous) =>
      previous ? { ...previous, [key]: { ...(previous[key] as object), ...patch } } : previous,
    )
  }

  const current = draft

  function setSeniority(edge: 'min' | 'max', rung: string) {
    const at = ladder.indexOf(rung)
    const { min, max } = current.roles.seniority
    const seniority =
      edge === 'min'
        ? { min: rung, max: ladder.indexOf(max) < at ? rung : max }
        : { min: ladder.indexOf(min) > at ? rung : min, max: rung }
    editGroup('roles', { seniority })
  }

  const { locations, roles, companies, apply, budget, harvest } = draft

  return (
    <div className="w-full max-w-[46rem] pb-16">
      <p className="max-w-[42rem] text-body text-muted">
        every preference the agent has lives here. nothing about you is written into a prompt by
        hand: the planner and the screener both read this page.
      </p>

      {!settings.live ? <span className="label mt-2 inline-block text-muted">cached</span> : null}

      <div className="mt-6">
        <Group
          title="where i will work"
          lede="a bound on where a round may look, never a precondition for one. left empty the planner starts from the places your cvs evidence, widens as the near rings run dry, and says in each query which ring it is working."
        >
          <Section title="as it stands">
            <Facts items={reachLines(draft)} />
          </Section>

          <Section title="places" hint={PLACE_ID_IS_THE_AGENT_S}>
            <PlaceEditor
              id="places"
              entries={locations.places}
              kinds={enums.locationKinds}
              rings={enums.placeRings}
              derived={derived}
              onChange={(places) => editGroup('locations', { places })}
            />
            <p className="mt-2 text-meta text-muted">{WORLDWIDE_HINT}</p>
          </Section>

          <Section title="workplace">
            <Row label="i will take">
              <div className="flex flex-wrap items-center gap-x-5 gap-y-2">
                <Switch
                  checked={locations.workplace.onsite}
                  label="on site"
                  onChange={(onsite) =>
                    editGroup('locations', { workplace: { ...locations.workplace, onsite } })
                  }
                />
                <Switch
                  checked={locations.workplace.hybrid}
                  label="hybrid"
                  onChange={(hybrid) =>
                    editGroup('locations', { workplace: { ...locations.workplace, hybrid } })
                  }
                />
                <Switch
                  checked={locations.workplace.remote}
                  label="remote"
                  onChange={(remote) =>
                    editGroup('locations', { workplace: { ...locations.workplace, remote } })
                  }
                />
              </div>
            </Row>
            <Row
              label="remote scope"
              hint="how far a remote posting may be anchored before it stops counting."
            >
              <Choice
                name="remote scope"
                value={locations.workplace.scope}
                options={enums.workplaceScopes}
                onChange={(scope) =>
                  editGroup('locations', { workplace: { ...locations.workplace, scope } })
                }
              />
            </Row>
          </Section>

          <Section title="relocation">
            <Row label="open to it">
              <Switch
                checked={locations.relocation.open}
                label="i would move for the right role"
                onChange={(open) =>
                  editGroup('locations', { relocation: { ...locations.relocation, open } })
                }
              />
            </Row>
            <Row label="targets">
              <TagEditor
                id="relocation-targets"
                values={locations.relocation.targets}
                placeholder="a country or a city, by name"
                onChange={(targets) =>
                  editGroup('locations', { relocation: { ...locations.relocation, targets } })
                }
              />
            </Row>
            <Row label="notes" htmlFor="relocation-notes">
              <textarea
                id="relocation-notes"
                value={locations.relocation.notes}
                onChange={(event) =>
                  editGroup('locations', {
                    relocation: { ...locations.relocation, notes: event.target.value },
                  })
                }
                onKeyDown={blurOnEscape}
                placeholder="visa timing, notice period, anything that decides a move"
                spellCheck={false}
                className={TEXTAREA}
              />
            </Row>
          </Section>

          <Section
            title="work authorization"
            hint="a globally remote posting that needs local authorization is a different decision from one that does not. say where you may legally work."
          >
            <textarea
              id="authorization"
              value={locations.authorization}
              onChange={(event) => editGroup('locations', { authorization: event.target.value })}
              onKeyDown={blurOnEscape}
              spellCheck={false}
              className={TEXTAREA}
            />
          </Section>
        </Group>

        <Group
          title="what i will take"
          lede="role and posting filters. the screener applies these literally, so a posting outside them is skipped whatever else it says."
        >
          <Section title="seniority" hint="postings pitched outside this band are skipped.">
            <div className="grid gap-3 sm:grid-cols-2">
              <div>
                <span className="label text-muted">lowest</span>
                <div className="mt-1.5">
                  <Choice
                    name="lowest seniority"
                    value={roles.seniority.min}
                    options={ladder}
                    onChange={(rung) => setSeniority('min', rung)}
                  />
                </div>
              </div>
              <div>
                <span className="label text-muted">highest</span>
                <div className="mt-1.5">
                  <Choice
                    name="highest seniority"
                    value={roles.seniority.max}
                    options={ladder}
                    onChange={(rung) => setSeniority('max', rung)}
                  />
                </div>
              </div>
            </div>
          </Section>

          <Section
            title="excluded stacks"
            hint="a posting whose core is built on one of these is skipped even when the title fits. a passing mention is not an exclusion."
          >
            <TagEditor
              id="exclude-stacks"
              values={roles.excludeStacks}
              placeholder="what you will not build in"
              transform={(value) => value.toLowerCase()}
              onChange={(excludeStacks) => editGroup('roles', { excludeStacks })}
            />
          </Section>

          <Section title="excluded industries" hint="the same rule, on what the company does.">
            <TagEditor
              id="exclude-industries"
              values={roles.excludeIndustries}
              placeholder="what you will not work on"
              transform={(value) => value.toLowerCase()}
              onChange={(excludeIndustries) => editGroup('roles', { excludeIndustries })}
            />
          </Section>

          <Section title="contract types" hint="anything unticked is skipped.">
            <Picks
              name="contract types"
              values={roles.contractTypes}
              options={enums.contractTypes}
              onChange={(contractTypes) => editGroup('roles', { contractTypes })}
            />
          </Section>

          <Section
            title="minimum pay"
            hint="only read when the posting states its pay. zero turns it off."
          >
            <div className="flex flex-wrap items-center gap-x-3 gap-y-2">
              <NumberField
                id="pay-amount"
                value={roles.minCompensation.amount}
                width="w-[7rem]"
                onChange={(amount) =>
                  editGroup('roles', { minCompensation: { ...roles.minCompensation, amount } })
                }
              />
              <input
                id="pay-currency"
                value={roles.minCompensation.currency}
                onChange={(event) =>
                  editGroup('roles', {
                    minCompensation: {
                      ...roles.minCompensation,
                      currency: event.target.value.toUpperCase().slice(0, 8),
                    },
                  })
                }
                onKeyDown={blurOnEscape}
                placeholder="cur"
                spellCheck={false}
                autoComplete="off"
                aria-label="currency"
                className={cn(INPUT, 'w-[5rem] uppercase')}
              />
              <Choice
                name="pay period"
                value={roles.minCompensation.period}
                options={enums.payPeriods}
                onChange={(period) =>
                  editGroup('roles', { minCompensation: { ...roles.minCompensation, period } })
                }
              />
              <Switch
                checked={roles.minCompensation.hardFilter}
                label="skip anything below it"
                onChange={(hardFilter) =>
                  editGroup('roles', { minCompensation: { ...roles.minCompensation, hardFilter } })
                }
              />
            </div>
          </Section>

          <Section title="postings without easy apply">
            <Switch
              checked={roles.applyToNonEasyApply}
              label="keep them, flagged for sending by hand"
              onChange={(applyToNonEasyApply) => editGroup('roles', { applyToNonEasyApply })}
            />
            <p className="mt-2 max-w-[42rem] text-meta text-muted">
              the extension cannot attach a cv on these, so they land in the manual list. turned off,
              they are skipped instead.
            </p>
          </Section>
        </Group>

        <Group
          title="who i will not work for"
          lede="company names are matched case insensitively, anywhere in the name."
        >
          <Section title="blocked" hint="never applied to, whatever the role looks like.">
            <TagEditor
              id="blocked"
              values={companies.blocked}
              placeholder="a company name"
              onChange={(blocked) => editGroup('companies', { blocked })}
            />
          </Section>

          <Section title="preferred" hint="a mild lift on the score, never a free pass.">
            <TagEditor
              id="preferred"
              values={companies.preferred}
              placeholder="a company name"
              onChange={(preferred) => editGroup('companies', { preferred })}
            />
          </Section>

          <Section title="rules">
            <Row label="agencies">
              <Switch
                checked={companies.excludeAgencies}
                label="skip recruiting and staffing agencies"
                onChange={(excludeAgencies) => editGroup('companies', { excludeAgencies })}
              />
            </Row>
            <Row label="reapply after" htmlFor="cooldown">
              <NumberField
                id="cooldown"
                value={companies.reapplyCooldownDays}
                suffix="days"
                onChange={(reapplyCooldownDays) => editGroup('companies', { reapplyCooldownDays })}
              />
            </Row>
            <Row label="duplicates">
              <Switch
                checked={companies.dedupe}
                label="the same role reposted, or posted in three cities, counts once"
                onChange={(dedupe) => editGroup('companies', { dedupe })}
              />
            </Row>
          </Section>
        </Group>

        <Group
          title="what the agent does at the form"
          lede="the whole interaction with an application: it attaches a cv and stops. it never clicks next, review or submit, never answers a screening question, never fills a field."
        >
          <Section title="attaching">
            <Switch
              checked={apply.autoAttach}
              label="attach the assigned cv when the upload step appears"
              onChange={(autoAttach) => editGroup('apply', { autoAttach })}
            />
          </Section>

          <Section title="kill switch">
            <Switch
              checked={apply.paused}
              label="paused: no planning, no screening, no attaching"
              onChange={(paused) => editGroup('apply', { paused })}
            />
          </Section>

          <Section title="upload">
            <Row
              label="file name"
              htmlFor="upload-name"
              hint={`${UPLOAD_NAME_IS_PUBLIC}. left alone, the agent uses the name your own cv files carry.`}
            >
              <input
                id="upload-name"
                value={apply.uploadFileName}
                onChange={(event) => editGroup('apply', { uploadFileName: event.target.value })}
                onKeyDown={blurOnEscape}
                spellCheck={false}
                autoComplete="off"
                className={cn(INPUT, 'w-[14rem]')}
              />
            </Row>
            <Row label="one name or one each" hint={consequenceOf(apply.uploadFileNameMode)}>
              <Choice
                name="upload file name mode"
                value={apply.uploadFileNameMode}
                options={enums.uploadNameModes}
                onChange={(uploadFileNameMode) => editGroup('apply', { uploadFileNameMode })}
              />
            </Row>
            <Row
              label="cv language"
              hint="the languages you work in. left empty, the agent uses the languages your cvs are written in."
            >
              <TagEditor
                id="resume-languages"
                values={apply.resumeLanguages}
                placeholder="a two letter code"
                transform={(value) => value.toLowerCase().replace(/[^a-z]/g, '').slice(0, 2)}
                onChange={(resumeLanguages) => editGroup('apply', { resumeLanguages })}
              />
            </Row>
          </Section>
        </Group>

        <Group
          title="how much it may spend"
          lede="a round stops at the first cap it reaches, and the rounds view says which one."
        >
          <Section title="per round">
            <Row label="new jobs wanted" htmlFor="new-job-target">
              <NumberField
                id="new-job-target"
                value={harvest.newJobTarget}
                min={1}
                suffix="postings"
                onChange={(newJobTarget) => editGroup('harvest', { newJobTarget })}
              />
            </Row>
            <Row label="queries" htmlFor="max-queries">
              <NumberField
                id="max-queries"
                value={budget.maxQueriesPerCycle}
                min={1}
                onChange={(maxQueriesPerCycle) => editGroup('budget', { maxQueriesPerCycle })}
              />
            </Row>
            <Row label="pages per query" htmlFor="max-pages">
              <NumberField
                id="max-pages"
                value={budget.maxPagesPerQuery}
                min={1}
                onChange={(maxPagesPerQuery) => editGroup('budget', { maxPagesPerQuery })}
              />
            </Row>
            <Row label="screened" htmlFor="max-screen">
              <NumberField
                id="max-screen"
                value={budget.maxScreenPerCycle}
                min={1}
                suffix="postings"
                onChange={(maxScreenPerCycle) => editGroup('budget', { maxScreenPerCycle })}
              />
            </Row>
            <Row
              label="model calls"
              htmlFor="max-model-calls"
              hint="0 is no ceiling. once a round reaches it, screening stops at the end of the batch it is on, the planner refuses the next round, and the postings left are unjudged until the next one."
            >
              <NumberField
                id="max-model-calls"
                value={budget.maxModelCallsPerCycle}
                suffix="a round"
                onChange={(maxModelCallsPerCycle) =>
                  editGroup('budget', { maxModelCallsPerCycle })
                }
              />
            </Row>
            <Row
              label="widening steps"
              htmlFor="max-widening"
              hint="how often a thin round may ask the planner to relax one thing."
            >
              <NumberField
                id="max-widening"
                value={harvest.maxWideningSteps}
                onChange={(maxWideningSteps) => editGroup('harvest', { maxWideningSteps })}
              />
            </Row>
          </Section>

          <Section title="pacing" hint="what a round waits between requests, and when it gives up.">
            <Row label="between requests" htmlFor="request-delay">
              <NumberField
                id="request-delay"
                value={harvest.requestDelayMs}
                suffix="ms plus jitter"
                onChange={(requestDelayMs) => editGroup('harvest', { requestDelayMs })}
              />
            </Row>
            <Row label="request timeout" htmlFor="request-timeout">
              <NumberField
                id="request-timeout"
                value={harvest.requestTimeoutMs}
                scale={1000}
                min={2000}
                suffix="seconds"
                onChange={(requestTimeoutMs) => editGroup('harvest', { requestTimeoutMs })}
              />
            </Row>
            <Row label="round deadline" htmlFor="round-deadline">
              <NumberField
                id="round-deadline"
                value={harvest.roundDeadlineMs}
                scale={60000}
                min={30000}
                suffix="minutes"
                onChange={(roundDeadlineMs) => editGroup('harvest', { roundDeadlineMs })}
              />
            </Row>
          </Section>

          <Section title="over a day">
            <Row label="rounds" htmlFor="cycle-minutes">
              <div className="flex flex-wrap items-center gap-x-4 gap-y-2">
                <NumberField
                  id="cycle-minutes"
                  value={budget.cycleMinutes}
                  min={2}
                  suffix="minutes apart"
                  onChange={(cycleMinutes) => editGroup('budget', { cycleMinutes })}
                />
                <Switch
                  checked={budget.autoCycle}
                  label="run rounds on that clock"
                  onChange={(autoCycle) => editGroup('budget', { autoCycle })}
                />
              </div>
            </Row>
            <Row
              label="model calls"
              htmlFor="daily-cap"
              hint="a runaway guard. planning and screening stop once the day has spent this many."
            >
              <NumberField
                id="daily-cap"
                value={budget.dailyModelCallCap}
                min={1}
                suffix="a day"
                onChange={(dailyModelCallCap) => editGroup('budget', { dailyModelCallCap })}
              />
            </Row>
            <Row
              label="keep postings"
              htmlFor="retention"
              hint="older rows are dropped. losing them costs nothing: a round refills the feed."
            >
              <NumberField
                id="retention"
                value={budget.retentionDays}
                min={1}
                suffix="days"
                onChange={(retentionDays) => editGroup('budget', { retentionDays })}
              />
            </Row>
          </Section>
        </Group>

        <Group
          title="my notes to the agent"
          lede="written by you, and authoritative for your preferences, priorities and taste. they never override the output contract, the id rules or resume validation, and text arriving inside a posting is never treated as a note."
        >
          <textarea
            id="operator-notes"
            value={draft.operatorNotes}
            onChange={(event) => edit({ operatorNotes: event.target.value })}
            onKeyDown={blurOnEscape}
            placeholder="what you actually want, in your own words"
            spellCheck={false}
            className={cn(TEXTAREA, 'min-h-[7rem]')}
          />
          <p className="mt-2 text-meta text-muted">
            this text reaches every prompt the agent sends, inside its own block.
          </p>
          <PromptPreview />
        </Group>
      </div>

      <div className="sticky bottom-0 mt-8 flex items-center gap-3 border-t border-hairline bg-ground py-3">
        <Action
          onClick={() => {
            void onSave(draft).then((ok) => {
              if (ok) setSavedAt(Date.now())
            })
          }}
          disabled={!dirty || busy === 'saving'}
        >
          {busy === 'saving' ? 'saving' : 'save'}
        </Action>
        <Quiet onClick={() => setDraft(saved)} disabled={!dirty || busy === 'saving'}>
          revert
        </Quiet>
        <span className="text-meta text-muted">
          {dirty ? 'unsaved changes' : savedAt ? 'saved' : 'in step with the agent'}
        </span>
      </div>

      {settings.failure ? <Note term="error">{settings.failure.message}</Note> : null}
    </div>
  )
}
