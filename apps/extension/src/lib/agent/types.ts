export type Ring = 'city' | 'country' | 'region' | 'worldwide'

export type PostingStatus =
  | 'new'
  | 'reading'
  | 'inbox'
  | 'manual'
  | 'queue'
  | 'applied'
  | 'opened'
  | 'skipped'
  | 'duplicate'

export const HANDLED_STATUSES: PostingStatus[] = [
  'reading',
  'inbox',
  'manual',
  'queue',
  'applied',
  'opened',
  'skipped',
  'duplicate',
]

export type ListStatus = PostingStatus | 'all'

export type Transition = 'queue' | 'applied' | 'skip'

export type Outcome = 'no-response' | 'rejected' | 'recruiter' | 'interview' | 'offer'

export const OUTCOMES: Outcome[] = ['no-response', 'rejected', 'recruiter', 'interview', 'offer']

export type ResumeFit = 'strong' | 'partial' | 'none'

export interface Pay {
  amount: number
  currency: string
  period: string
}

export interface Tailored {
  needed: boolean
  focus: string
}

export interface Posting {
  id: string
  title: string
  company: string
  location: string
  url: string
  dupeOf: string | null
  description: string
  descriptionState: string
  listedAt: number | null
  easyApply: boolean
  status: PostingStatus
  stage: string
  score: number | null
  triageReason: string | null
  verdictReason: string | null
  seniority: string | null
  workplace: string | null
  contractType: string | null
  postingLang: string | null
  agency: boolean | null
  statedPay: Pay | null
  resumeCode: string | null
  resumeLang: string | null
  resumeFit: ResumeFit | null
  tailoredResume: Tailored | null
  stale: boolean
  outcome: Outcome | null
  outcomeNote: string | null
  cycleId: number | null
  queryLabel: string | null
  createdAt: string
  updatedAt: string
}

export interface PostingList {
  jobs: Posting[]
}

export interface Harvested {
  id: string
  title: string
  company: string
  location: string
  url: string
  description: string
  listedAt: number | null
  easyApply: boolean
}

export interface Ingested {
  received: number
  inserted: number
  duplicates: number
  collapsed: number
  judged: number
  newIds: string[]
  roundNew: number
  newJobTarget: number
  stop: boolean
  reason?: string
}

export interface OpenedRecord {
  received: number
  recorded: number
  collapsed: number
  held: number
  ids: string[]
}

export interface Query {
  label: string
  ring: Ring
  place: string
  keyword?: string
  range: string
  easyApply?: boolean
  remoteOnly?: boolean
  want: number
  reason: string
  relaxed?: string
}

export interface Pacing {
  newJobTarget: number
  maxWideningSteps: number
  roundDeadlineMs: number
  requestDelayMs: number
  requestTimeoutMs: number
  cycleMinutes: number
  maxPagesPerQuery: number
}

export interface Plan {
  ok: boolean
  cycleId: number
  startedAt: string
  deadlineAt: string
  nextRunAt: string | null
  rationale: string
  screenBudget: number
  queries: Query[]
  pacing: Pacing
  apply: { autoAttach: boolean; uploadFileName: string; uploadFileNameMode: string }
  knownIds: string[]
  resumed?: boolean
  rejected?: string[]
}

export interface Refusal {
  ok: false
  reason: string
  message: string
}

export interface Widening {
  ok: boolean
  cycleId: number
  query: Query | null
  relaxed: string
  reason: string
  stepsLeft: number
}

export interface RoundQuery {
  label: string
  query: Query
  widened: boolean
  pages: number
  received: number
  inserted: number
  duplicates: number
  skipped: boolean
  skipReason?: string
}

export interface Round {
  id: number
  startedAt: string
  finishedAt: string | null
  status: 'open' | 'done' | 'failed'
  rationale: string
  screenBudget: number
  received: number
  inserted: number
  duplicates: number
  wideningSteps: number
  modelCalls: number
  terminationReason: string | null
  error: string | null
  queries: RoundQuery[]
}

export interface PluginFetch {
  place: boolean
  rings: string[]
  ranges: string[]
  easyApply: boolean
  remoteOnly: boolean
  sorts: string[]
  keyword: boolean
  pageSize: number
  maxPagesPerQuery: number
}

export interface PluginCapabilities {
  fetch: PluginFetch
  seenSet: boolean
  autoAttach: boolean
}

export interface PluginState {
  everSeen: boolean
  connected: boolean
  id: string
  version: string | null
  firstSeen: string | null
  lastSeen: string | null
  hellos: number
  capabilities: PluginCapabilities | null
}

export interface Budget {
  used: number
  cap: number
  remaining: number
  day: string
}

export interface RoundBudget {
  cycleId: number
  open: boolean
  used: number
  cap: number
  remaining: number
}

export interface Note {
  id: number
  scope: string
  text: string
  evidence: string
  priority: string
  createdAt: string
  updatedAt: string
}

export interface PlanState {
  plugin: PluginState
  paused: boolean
  autoCycle: boolean
  cycleMinutes: number
  nextRunAt: string | null
  blocked: Refusal | null
  budget: Budget
  roundBudget: RoundBudget
  cycle: Round | null
  history: Round[]
  notes: Note[]
  knownIdCount: number
}

export interface Place {
  name: string
  ring: string
}

export interface PlaceResolution {
  name: string
  asked: string
  ring: string
  locationId: string
  source: string
}

export interface ResumeProfile {
  code: string
  targetRole: string
  seniorityClaimed: string
  coreStack: string[]
  secondaryStack: string[]
  domains: string[]
  languages: string[]
  yearsClaimed: number
  earliestStart: string
  places: Place[]
  summary: string
}

export interface CandidateProfile {
  yearsExperience: number
  seniorityBand: string
  coreStack: string[]
  secondaryStack: string[]
  domains: string[]
  workLanguages: string[]
  places: Place[]
  headline: string
}

export interface ResumeState {
  code: string
  label: string
  fileLanguages: string[]
  indexed: boolean
  profile: ResumeProfile | null
}

export interface IndexProgress {
  phase: string
  done: number
  total: number
}

export interface ProfileState {
  resumeDir: string
  candidate: CandidateProfile | null
  indexedAt: string
  resumes: ResumeState[]
  indexState: 'never' | 'stale' | 'current'
  indexing: boolean
  progress: IndexProgress
  error?: string
}

export interface Resume {
  code: string
  label: string
  role: string
  languages: string[]
}

export interface ResumeList {
  resumeDir: string
  resumes: Resume[]
}

export interface CredentialStatus {
  loaded: boolean
  source: string
  where: string
  stored: boolean
  expiresInMinutes: number | null
  refreshable: boolean
  error: string | null
}

export interface FolderCandidate {
  dir: string
  count: number
  sample: string
}

export interface SetupState {
  configured: boolean
  ready: boolean
  missing: string[]
  indexState: string
  indexing: boolean
  resumeDir: string
  candidates: FolderCandidate[]
  resumes: Resume[]
  credentials: CredentialStatus
  progress: IndexProgress
  error?: string
}

export interface Settings {
  locations: {
    places: { name: string; ring: string; kind: string }[]
    relocation: { open: boolean; targets: string[]; notes: string }
    workplace: { onsite: boolean; hybrid: boolean; remote: boolean; scope: string }
    authorization: string
  }
  roles: {
    seniority: { min: string; max: string }
    postingLanguages: string[]
    excludeStacks: string[]
    excludeIndustries: string[]
    contractTypes: string[]
    minCompensation: { amount: number; currency: string; period: string; hardFilter: boolean }
    applyToNonEasyApply: boolean
  }
  companies: {
    blocked: string[]
    preferred: string[]
    excludeAgencies: boolean
    reapplyCooldownDays: number
    dedupe: boolean
  }
  apply: {
    autoAttach: boolean
    uploadFileName: string
    uploadFileNameMode: string
    resumeLanguages: string[]
    paused: boolean
  }
  budget: {
    maxQueriesPerCycle: number
    maxScreenPerCycle: number
    maxModelCallsPerCycle: number
    maxPagesPerQuery: number
    cycleMinutes: number
    autoCycle: boolean
    dailyModelCallCap: number
    retentionDays: number
  }
  harvest: {
    newJobTarget: number
    maxWideningSteps: number
    roundDeadlineMs: number
    requestDelayMs: number
    requestTimeoutMs: number
  }
  operatorNotes: string
}

export interface Enums {
  seniorityLadder: string[]
  locationKinds: string[]
  placeRings: string[]
  workplaceScopes: string[]
  contractTypes: string[]
  payPeriods: string[]
  uploadNameModes: string[]
}

export interface SettingsDocument {
  settings: Settings
  enums: Enums
}

export interface ScreenResult {
  paused: boolean
  triaged: number
  kept: number
  droppedAtTriage: number
  gated: number
  screened: number
  picked: number
  manual: number
  skipped: number
  flagged: number
  corrected: number
  nudges: number
  awaitingDescription: number
  screenKey: string
  verdicts: unknown[]
  stopped?: string
  error?: string
}

export interface Staleness {
  screenKey: string
  stale: number
}

export interface Trace {
  id: number
  purpose: string
  model: string
  toolName: string
  state: string
  startedAt: string
  finishedAt: string | null
  durationMs: number | null
  inputTokens: number
  outputTokens: number
  cacheReadTokens: number
  cacheWriteTokens: number
  error: string | null
  system?: string
  user?: string
  output?: string
}

export interface TraceList {
  running: Trace | null
  traces: Trace[]
}

export interface PromptDraft {
  id: string
  label: string
  system: string
  unavailable: string | null
}

export interface PromptPreview {
  ready: boolean
  missing?: string
  prompts: PromptDraft[]
}

export interface Lesson {
  text: string
  evidence: string
  scope: string
}

export interface Reflection {
  ran: boolean
  reason?: string
  read: number
  written: number
  retired: number
  lessons: Lesson[]
}

export interface IndexResult {
  resumeDir: string
  indexed: number
  cached: number
  calls: number
  model: string
}

export interface Health {
  ok: boolean
}

export interface Hello {
  id: string
  version: string
  capabilities: PluginCapabilities
}
