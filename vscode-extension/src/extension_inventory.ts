import type { BehaviorEvent, BridgeEnvelope } from './protocol'
import { PROTOCOL_VERSION } from './protocol'

export interface ExtensionLike {
  id: string
  isActive: boolean
  packageJSON?: { version?: unknown; [key: string]: unknown }
  exports?: unknown
}

export interface ExtensionSnapshot {
  id: string
  version: string
  is_active: boolean
}

export interface HeartbeatOptions {
  sessionId: string
  extensionVersion: string
  vscodeVersion: string
  hostKind: 'local' | 'remote'
  pendingEvents: number
  sentEvents: number
  droppedEvents: number
  componentState?: 'active' | 'paused_by_user' | 'unsupported_remote_host'
}

export function snapshotExtensions(extensions: readonly ExtensionLike[]): ExtensionSnapshot[] {
  return extensions.map(extension => ({
    id: extension.id.slice(0, 256),
    version: String(extension.packageJSON?.version ?? '').slice(0, 128),
    is_active: Boolean(extension.isActive),
  })).sort((left, right) => left.id.localeCompare(right.id))
}

export function diffExtensions(
  previous: readonly ExtensionSnapshot[],
  current: readonly ExtensionSnapshot[],
  newId: () => string,
  now: () => Date,
): BehaviorEvent[] {
  const before = new Map(previous.map(item => [item.id, item]))
  const after = new Map(current.map(item => [item.id, item]))
  const events: BehaviorEvent[] = []
  for (const item of current.filter(item => !before.has(item.id))) {
    events.push(changeEvent(item.id, 'installed', newId, now, { version: item.version, is_active: item.is_active }))
  }
  for (const item of previous.filter(item => !after.has(item.id))) {
    events.push(changeEvent(item.id, 'removed', newId, now, { previous_version: item.version, previous_is_active: item.is_active }))
  }
  for (const item of current.filter(item => before.has(item.id))) {
    const old = before.get(item.id)!
    if (old.version !== item.version) {
      events.push(changeEvent(item.id, 'updated', newId, now, { version: item.version, previous_version: old.version, is_active: item.is_active }))
    } else if (old.is_active !== item.is_active) {
      events.push(changeEvent(item.id, 'activation_changed', newId, now, { version: item.version, is_active: item.is_active, previous_is_active: old.is_active }))
    }
  }
  return events
}

export function createComponentHeartbeat(options: HeartbeatOptions): BridgeEnvelope {
  return {
    version: PROTOCOL_VERSION,
    type: 'heartbeat',
    session_id: options.sessionId,
    extension_id: 'aetheris.aetheris-vscode',
    extension_version: options.extensionVersion,
    vscode_version: options.vscodeVersion,
    host_kind: options.hostKind,
    pending_events: Math.max(0, options.pendingEvents),
    sent_events: Math.max(0, options.sentEvents),
    dropped_events: Math.max(0, options.droppedEvents),
    component_state: options.componentState ?? 'active',
  }
}

function changeEvent(
  extensionId: string,
  change: NonNullable<BehaviorEvent['change']>,
  newId: () => string,
  now: () => Date,
  values: Partial<BehaviorEvent>,
): BehaviorEvent {
  return {
    event_id: newId(), event_type: 'ide.extension_changed', occurred_at: now().toISOString(),
    extension_id: extensionId.slice(0, 256), change, ...values,
  }
}
