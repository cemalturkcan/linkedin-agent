export const POSTING_ID = /^\d{5,}$/
export const VIEW_PATH = /\/jobs\/view\/(\d{5,})/

export type Screen = 'lists' | 'posting'

export interface TabState {
  url?: string
  status?: string
}

export interface Following {
  screen: Screen
  postingId: string
}

export const NO_FOLLOW: Following = { screen: 'lists', postingId: '' }

export function postingIdFrom(href: string): string {
  let url: URL
  try {
    url = new URL(String(href ?? ''))
  } catch {
    return ''
  }
  if (!/(^|\.)linkedin\.com$/i.test(url.hostname)) return ''
  const current = url.searchParams.get('currentJobId')
  if (current && POSTING_ID.test(current)) return current
  const found = VIEW_PATH.exec(url.pathname)
  return found ? (found[1] as string) : ''
}

export function followFor(tab: TabState | null | undefined, held: Following): Following {
  const postingId = postingIdFrom(tab?.url ?? '')
  if (postingId) return { screen: 'posting', postingId }
  if (tab?.status === 'loading') return held
  return NO_FOLLOW
}

export function followKeyOf(following: Following): string {
  return `${following.screen}:${following.postingId}`
}

export function showsPosting(following: Following, overridden: string): boolean {
  return following.screen === 'posting' && overridden !== followKeyOf(following)
}
