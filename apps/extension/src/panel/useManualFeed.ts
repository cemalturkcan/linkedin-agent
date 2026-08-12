import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import {
  knobsKey,
  mergePages,
  morePages,
  NO_KNOBS,
  rowsFor,
  unseenCount,
  type FeedKnobs,
  type FeedRow,
} from '@/lib/linkedin/feed'
import type { PlaceHit } from '@/lib/linkedin/geo'
import type { Card } from '@/lib/linkedin/voyager'
import { PAGE_SIZE } from '@/lib/linkedin/voyager'
import { readKey, writeKeys } from '@/lib/storage'
import {
  feedPageNow,
  feedPlacesNow,
  NO_WORKER,
  recordOpenedNow,
  rememberPlaceNow,
  reportUnseen,
} from '@/lib/worker'

export const KNOBS_KEY = 'feedKnobs'

export const NO_SUCH_PLACE = 'linkedin does not know a place by that name'

export function opensWith(active: boolean, restored: boolean, everAsked: boolean): boolean {
  return active && restored && !everAsked
}

export interface ManualFeed {
  knobs: FeedKnobs
  rows: FeedRow[]
  unseen: number
  loading: boolean
  more: boolean
  notice: string | null
  hits: PlaceHit[]
  searching: boolean
  setKnobs(knobs: FeedKnobs): void
  reload(): Promise<void>
  loadMore(): Promise<void>
  suggest(name: string): Promise<void>
  choose(hit: PlaceHit, asked: string): Promise<void>
  chooseByName(name: string): Promise<void>
  setKeyword(keyword: string): void
  clearPlace(): void
  open(card: Card): Promise<void>
}

interface Loaded {
  key: string
  cards: Card[]
  start: number
  more: boolean
}

const NOTHING: Loaded = { key: '', cards: [], start: 0, more: false }

export function useManualFeed(
  handled: Set<string>,
  known: Set<string>,
  armed: { code: string; languages: string[] } | null,
  onOpened: (id: string) => Promise<void>,
  active = false,
): ManualFeed {
  const [knobs, setKnobs] = useState<FeedKnobs>(NO_KNOBS)
  const [loaded, setLoaded] = useState<Loaded>(NOTHING)
  const [loading, setLoading] = useState(false)
  const [notice, setNotice] = useState<string | null>(null)
  const [hits, setHits] = useState<PlaceHit[]>([])
  const [searching, setSearching] = useState(false)
  const [restored, setRestored] = useState(false)
  const inFlight = useRef(false)
  const everAsked = useRef(false)

  useEffect(() => {
    let cancelled = false
    void (async () => {
      const stored = await readKey<FeedKnobs | null>(KNOBS_KEY, null)
      if (cancelled) return
      if (stored && typeof stored === 'object') setKnobs({ ...NO_KNOBS, ...stored })
      setRestored(true)
    })()
    return () => {
      cancelled = true
    }
  }, [])

  const remember = useCallback((wanted: FeedKnobs) => {
    setKnobs(wanted)
    void writeKeys({ [KNOBS_KEY]: wanted })
  }, [])

  const fetchPage = useCallback(
    async (wanted: FeedKnobs, start: number) => {
      if (inFlight.current) return
      inFlight.current = true
      setLoading(true)
      const answer = await feedPageNow(wanted, start)
      inFlight.current = false
      setLoading(false)

      if (!answer) {
        setNotice(NO_WORKER)
        return
      }
      if (!answer.ok || !answer.cards) {
        setNotice(answer.message ?? 'linkedin did not answer the feed')
        return
      }
      setNotice(null)
      const key = knobsKey(wanted)
      setLoaded((previous) => {
        const held = previous.key === key && start > 0 ? previous.cards : []
        const cards = mergePages(held, answer.cards ?? [])
        return {
          key,
          cards,
          start,
          more: morePages(cards, {
            cards: answer.cards ?? [],
            total: answer.total ?? 0,
            start,
          }),
        }
      })
    },
    [],
  )

  useEffect(() => {
    if (!opensWith(active, restored, everAsked.current)) return
    everAsked.current = true
    void fetchPage(knobs, 0)
  }, [active, restored, knobs, fetchPage])

  const rows = useMemo(
    () => rowsFor(loaded.cards, handled, known),
    [handled, known, loaded.cards],
  )
  const unseen = useMemo(() => unseenCount(rows), [rows])

  useEffect(() => {
    void reportUnseen(unseen)
  }, [unseen])

  return {
    knobs,
    rows,
    unseen,
    loading,
    more: loaded.more,
    notice,
    hits,
    searching,
    setKnobs: remember,
    async reload() {
      everAsked.current = true
      setLoaded(NOTHING)
      await fetchPage(knobs, 0)
    },
    async loadMore() {
      await fetchPage(knobs, loaded.start + PAGE_SIZE)
    },
    async suggest(name) {
      const wanted = name.trim()
      if (wanted === '') {
        setHits([])
        return
      }
      setSearching(true)
      const answer = await feedPlacesNow(wanted)
      setSearching(false)
      if (!answer) {
        setNotice(NO_WORKER)
        return
      }
      if (!answer.ok) {
        setNotice(answer.message ?? 'linkedin did not answer the place lookup')
        setHits([])
        return
      }
      setNotice(null)
      setHits(answer.hits ?? [])
    },
    async choose(hit, asked) {
      setHits([])
      everAsked.current = true
      remember({ ...knobs, place: hit.label, locationId: hit.locationId })
      await rememberPlaceNow(hit, asked)
      setLoaded(NOTHING)
      await fetchPage({ ...knobs, place: hit.label, locationId: hit.locationId }, 0)
    },
    async chooseByName(name) {
      const wanted = name.trim()
      if (wanted === '') return
      setSearching(true)
      const answer = await feedPlacesNow(wanted)
      setSearching(false)
      if (!answer) {
        setNotice(NO_WORKER)
        return
      }
      if (!answer.ok) {
        setNotice(answer.message ?? 'linkedin did not answer the place lookup')
        return
      }
      const hit = (answer.hits ?? [])[0]
      if (!hit) {
        setNotice(`${NO_SUCH_PLACE}: ${wanted}`)
        return
      }
      setNotice(null)
      setHits([])
      everAsked.current = true
      remember({ ...knobs, place: hit.label, locationId: hit.locationId })
      await rememberPlaceNow(hit, wanted)
      setLoaded(NOTHING)
      await fetchPage({ ...knobs, place: hit.label, locationId: hit.locationId }, 0)
    },
    setKeyword(keyword) {
      remember({ ...knobs, keyword })
    },
    clearPlace() {
      setHits([])
      remember({ ...knobs, place: '', locationId: '' })
      setLoaded(NOTHING)
    },
    async open(card) {
      const answered = await recordOpenedNow(
        [card],
        armed?.code ?? '',
        armed?.languages[0] ?? '',
      )
      if (!answered?.ok) {
        setNotice('the posting opened, but the agent did not record it as handled')
      }
      await onOpened(card.id)
    },
  }
}
