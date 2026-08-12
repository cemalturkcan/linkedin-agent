export type FailureKind =
  | 'rate-limited'
  | 'challenge'
  | 'signed-out'
  | 'timeout'
  | 'network'
  | 'http'
  | 'unknown-place'
  | 'config'

export const SIGNED_OUT = 'linkedin signed this session out, open linkedin.com and sign in again'
export const RATE_LIMITED = 'linkedin is rate limiting this session'
export const CHALLENGE = 'linkedin wants a security check, open linkedin.com in a tab and clear it'
export const NO_SESSION = 'no linkedin session cookie in this browser, sign in at linkedin.com'
export const BAD_LOCATION = 'the place lookup answered with something that is not a linkedin place id'
export const NOT_A_FEED = 'linkedin answered with something that is not a job feed'
export const TIMED_OUT = 'linkedin did not answer in time'

export class LinkedInFailure extends Error {
  readonly kind: FailureKind

  constructor(kind: FailureKind, message: string) {
    super(message)
    this.name = 'LinkedInFailure'
    this.kind = kind
  }
}

export function failure(kind: FailureKind, message: string): LinkedInFailure {
  return new LinkedInFailure(kind, message)
}

export function kindOf(error: unknown): FailureKind | null {
  return error instanceof LinkedInFailure ? error.kind : null
}

export function backsOff(error: unknown): boolean {
  const kind = kindOf(error)
  return kind === 'rate-limited' || kind === 'challenge'
}

export function endsSession(error: unknown): boolean {
  return kindOf(error) === 'signed-out'
}

export function sentenceOf(error: unknown): string {
  if (error instanceof Error && error.message) return error.message
  return 'linkedin failed in a way this build has no sentence for'
}
