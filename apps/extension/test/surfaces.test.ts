import { expect, test } from 'bun:test'
import { Desk } from '@/desk/Desk'
import { PostingsView } from '@/desk/components/PostingsView'
import { ProfileView } from '@/desk/components/ProfileView'
import { PlaceEditor } from '@/desk/components/PlaceEditor'
import { PromptPreview } from '@/desk/components/PromptPreview'
import { RoundsView } from '@/desk/components/RoundsView'
import { SettingsView } from '@/desk/components/SettingsView'
import { SetupView } from '@/desk/components/SetupView'
import { TraceView } from '@/desk/components/TraceView'
import { Feed } from '@/panel/components/Feed'
import { FeedKnobs } from '@/panel/components/FeedKnobs'
import { Header } from '@/panel/components/Header'
import { ManualFeed } from '@/panel/components/ManualFeed'
import { Legend } from '@/panel/components/Legend'
import { Notices } from '@/panel/components/Notices'
import { PostingRow } from '@/panel/components/PostingRow'
import { Presets } from '@/panel/components/Presets'
import { ResumeStep } from '@/panel/components/ResumeStep'
import { RoundPanel } from '@/panel/components/RoundPanel'
import { Panel } from '@/panel/Panel'

test('both surfaces and every panel part resolve without a cycle', () => {
  for (const part of [
    Panel,
    Desk,
    Header,
    Notices,
    Presets,
    RoundPanel,
    Feed,
    Legend,
    FeedKnobs,
    ManualFeed,
    ResumeStep,
  ]) {
    expect(part).toBeDefined()
    expect(typeof part).toBe('function')
  }
  expect(PostingRow).toBeDefined()
})

test('every desk view resolves without a cycle', () => {
  for (const part of [
    SetupView,
    ProfileView,
    SettingsView,
    RoundsView,
    TraceView,
    PostingsView,
    PlaceEditor,
    PromptPreview,
  ]) {
    expect(part).toBeDefined()
    expect(typeof part).toBe('function')
  }
})
