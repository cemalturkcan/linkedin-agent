import declared from '../../public/capabilities.json'
import type { Hello, PluginCapabilities } from '@/lib/agent/types'

export const EXECUTOR_MISSING =
  'the extension that checked in cannot fetch from linkedin, so a planned round would have nothing to run'

export function capabilityManifest(): PluginCapabilities {
  return declared as PluginCapabilities
}

export function canFetch(capabilities: PluginCapabilities | null): boolean {
  return Boolean(capabilities && capabilities.fetch.rings.length > 0)
}

export function hello(id: string, version: string): Hello {
  return { id, version, capabilities: capabilityManifest() }
}
