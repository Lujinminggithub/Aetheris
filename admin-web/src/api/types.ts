export type WorkRole = '研发' | '测试' | '产品'

export interface Session { user_id: string; username: string; access_role: string; tenant_id?: string; expires_at?: string }
export interface Device { id: string; name: string; subject_id: string; status?: string; last_seen_at?: string }
export interface Subject { id: string; name: string; device_count?: number }
export interface RoleAssignment { id?: string; subject_id: string; project_id?: string; role: WorkRole; expires_at?: string }
export interface Event { id?: string; event_id: string; subject_id: string; device_id: string; work_role?: WorkRole; occurred_at: string; event_type: string; summary?: string; payload?: Record<string, unknown> }
export interface AuditLog { id: string; actor: string; action: string; resource: string; result: string; created_at: string }
export interface ApiError { error: string; message: string; request_id?: string }
export interface ActivityItem { id?: string; event_id?: string; subject_id?: string; device_id?: string; event_type: string; occurred_at: string; summary?: string }
export interface AdminSummary { total_subjects: number; total_devices: number; events_today: number; active_devices: number; activity: ActivityItem[] }
export interface ProviderHealth { name: string; provider: string; status: 'healthy' | 'degraded' | 'down' | string; latency_ms?: number; checked_at?: string; message?: string }
export interface BrowserCapturePolicy { enabled: boolean; allowed_domains: string[]; revision: number; updated_at?: string }
export interface ApplicationCapturePolicy { enabled: boolean; revision: number; updated_at?: string }
export interface EventDetail extends Event { payload?: Record<string, unknown>; created_at?: string; tombstoned_at?: string }
export type AIMessageRole = 'user' | 'assistant' | 'ai_tool' | 'system' | 'tool' | 'unknown'
export interface AIDeviceFacet { device_id: string; device_name: string; event_count: number; last_interaction_at: string }
export interface AIProjectFacet { project_id: string; project_name: string; event_count: number; last_interaction_at: string }
export interface AIInteraction {
  event_id: string; device_id: string; device_name: string; project_id: string; project_name: string;
  message_role: AIMessageRole; interaction_kind?: 'message' | 'tool_call'; actor_origin?: 'human' | 'ai' | 'unknown';
  tool: string; occurred_at: string; content_preview: string; command_type?: string; quality_state?: string; source_event_ids?: string[];
}
export interface AIInteractionsResult {
  devices: AIDeviceFacet[]; projects: AIProjectFacet[]; role_counts: Record<AIMessageRole, number>;
  interactions: AIInteraction[]; total: number; limit: number; offset: number;
}
export interface AIInteractionsFilter { from: string; to: string; device_id?: string; project_id?: string; message_role?: AIMessageRole; limit?: number; offset?: number }
export type ActivityType = 'ai' | 'terminal' | 'ide' | 'delivery' | 'application' | 'browser' | 'version_control' | 'other'
export interface ActivityFacet { id: string; label: string; count: number; last_activity_at: string }
export interface Activity {
  fact_id: string; canonical_event_id: string; source_event_ids: string[];
  device_id: string; device_name: string; project_id: string; project_name: string;
  activity_type: ActivityType; event_type: string; source: string; actor_origin: string;
  message_role: string; tool?: string; preview: string; occurred_at: string;
}
export interface ActivitiesResult { devices: ActivityFacet[]; projects: ActivityFacet[]; activity_counts: Record<string, number>; activities: Activity[]; total: number; limit: number; offset: number }
export interface ActivitiesFilter { from: string; to: string; device_id?: string; project_id?: string; activity_type?: ActivityType; message_role?: string; limit?: number; offset?: number }
export interface RAGStatus { documents: number; indexed: number; pending: number; failed: number; embedding_model: string; vector_status: string; last_indexed_at?: string }
export interface RAGFilters { from: string; to: string; device_id?: string; project_id?: string; activity_type?: ActivityType }
export interface RAGCitation { number: number; document_id: string; fact_id: string; canonical_event_id: string; source_event_ids: string[]; project_id: string; project_name: string; activity_type: ActivityType; excerpt: string; score: number; occurred_at: string }
export interface RAGQueryJob { query_id: string; question: string; filters: RAGFilters; status: 'queued' | 'embedding' | 'retrieving' | 'generating' | 'completed' | 'failed'; progress: number; answer: string; answer_mode?: 'direct' | 'numeric' | 'reason' | 'procedure' | 'analysis'; confidence?: 'high' | 'medium' | 'low'; details?: string; citation_numbers?: number[]; citations: RAGCitation[]; error_code?: string; created_at: string; completed_at?: string }
export interface DataQualitySummary { raw_events: number; clean_facts: number; merged_facts: number; command_fragments: number; quarantined_facts: number; excluded_facts: number; rule_version: number }
export interface DataQualityFact { fact_id: string; fact_type: string; actor_origin: string; project_id: string; project_name: string; device_id: string; device_name: string; occurred_at: string; quality_state: string; confidence: string; merge_method: string; reason_codes: string[]; source_event_ids: string[]; command_display: string; excluded_from_effectiveness: boolean }
export interface DataQualityFactPage { facts: DataQualityFact[]; total: number; limit: number; offset: number }
export interface EpisodeEvidence { event_id: string; section: string; reason: string }
export interface EpisodeAction { event_id: string; actor: string; action_type: string; summary: string; occurred_at: string }
export interface EpisodeValidation { event_id: string; validation_type: string; result: string; summary: string; occurred_at: string }
export interface WorkEpisode { episode_id: string; subject_id: string; device_id: string; project_id: string; project_name?: string; session_id: string; started_at: string; ended_at: string; title: string; objective: string; context: string; outcome: string; confidence: string; needs_review: boolean; actions: EpisodeAction[]; validations: EpisodeValidation[]; evidence: EpisodeEvidence[] }
export interface WorkEpisodesResult { episodes: WorkEpisode[]; count: number; limit: number; offset: number }
export interface AdapterHealthSnapshot { device_id: string; adapter_id: string; state: string; capability_version: string; detected_format?: string; last_scan_at?: string; last_success_at?: string; last_event_at?: string; discovered: number; parsed: number; skipped: number; failed: number; lag_seconds: number; error_code?: string; error_stage?: string; component_state?: string; component_version?: string; protocol_version?: number; vscode_version?: string; last_component_heartbeat_at?: string; pending_events?: number; sent_events?: number; dropped_events?: number }
export interface AdapterHealthResult { items: AdapterHealthSnapshot[]; count: number }
export interface ProjectLocation { id: string; logical_project_id: string; device_id: string; local_project_id: string; display_name: string; workspace_kind: 'primary' | 'clone' | 'worktree' | 'non_vcs'; worktree_name?: string; active: boolean; key_version: number; metadata_revision: number }
export interface LogicalProject { id: string; display_name: string; vcs: 'git' | 'svn' | 'none'; status: 'active' | 'archived' | 'needs_review'; metadata_revision: number; location_count: number; locations: ProjectLocation[] }
export interface EffectivenessBreakdown { event_count: number; active_window_minutes: number }
export interface EffectivenessTotals {
  active_days: number; active_window_minutes: number; session_count: number; average_session_minutes: number;
  longest_session_minutes: number; focus_block_count: number; focus_block_minutes: number; context_switch_count: number;
  delivery_events: number; coding_events: number; terminal_events: number; ai_collaboration_events: number; browser_events: number; other_events: number;
}
export interface EffectivenessDaily {
  local_date: string; active_window_minutes: number; session_count: number; delivery_events: number; coding_events: number;
  terminal_events: number; ai_collaboration_events: number; browser_events: number; other_events: number;
}
export interface EffectivenessTrend { current: number; previous: number; delta: number; percent_change?: number | null }
export interface EffectivenessCoverage { covered_days: number; period_days: number; coverage_ratio: number; source_count: number; device_count: number; last_event_at?: string; insufficient: boolean }
export interface EffectivenessSubject { subject_id: string; display_name: string; active_days: number; active_window_minutes: number; event_count: number; coverage_ratio: number }
export interface EffectivenessReport {
  subject_id: string; from: string; to: string; timezone: string; metric_definition_version: number; totals: EffectivenessTotals;
  daily: EffectivenessDaily[]; project_breakdown: Record<string, EffectivenessBreakdown>; work_role_breakdown: Record<string, EffectivenessBreakdown>;
  activity_breakdown: Record<string, number>; trends: Record<string, EffectivenessTrend>; coverage: EffectivenessCoverage;
  evidence_event_ids: string[]; definitions: Record<string, string>;
}
