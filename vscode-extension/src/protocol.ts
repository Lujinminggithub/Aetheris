export const PROTOCOL_VERSION = 1 as const
export const PIPE_NAME = '\\\\.\\pipe\\Aetheris.VSCode.Bridge.v1'

export type BehaviorEventType =
  | 'ide.file_opened'
  | 'ide.file_edited'
  | 'ide.file_saved'
  | 'ide.file_closed'
  | 'ide.workspace_changed'
  | 'ide.extension_changed'

export interface BehaviorEvent {
  event_id: string
  event_type: BehaviorEventType
  occurred_at: string
  session_id?: string
  workspace_path?: string
  file_path?: string
  relative_path?: string
  file_name?: string
  extension?: string
  language_id?: string
  uri_scheme?: string
  read_only?: boolean
  change_count?: number
  inserted_chars?: number
  deleted_chars?: number
  undo_count?: number
  redo_count?: number
  flush_reason?: string
  edit_started_at?: string
  edit_ended_at?: string
  edit_duration_ms?: number
  had_pending_edit?: boolean
  workspace_kind?: string
  added_workspace_paths?: string[]
  removed_workspace_paths?: string[]
  extension_id?: string
  version?: string
  previous_version?: string
  change?: 'installed' | 'removed' | 'updated' | 'activation_changed'
  is_active?: boolean
  previous_is_active?: boolean
}

export interface BridgeEnvelope {
  version: typeof PROTOCOL_VERSION
  type: 'handshake' | 'heartbeat' | 'events'
  session_id: string
  events?: BehaviorEvent[]
  extension_id?: string
  extension_version?: string
  vscode_version?: string
  host_kind?: 'local' | 'remote'
  pending_events?: number
  sent_events?: number
  dropped_events?: number
  component_state?: 'active' | 'paused_by_user' | 'unsupported_remote_host'
}

export interface BridgeAck {
  version: typeof PROTOCOL_VERSION
  accepted_ids: string[]
  rejected: Array<{ event_id: string; reason_code: string }>
  control_state?: 'active' | 'paused_by_user'
  clear_cache?: boolean
}
