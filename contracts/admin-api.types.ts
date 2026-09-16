export interface Session { user_id: string; username: string; tenant_id: string; access_role: string; expires_at: string }
export interface Device { id: string; name: string; subject_id: string; status: string; last_seen_at: string }
export interface Subject { id: string; name: string; device_count: number }
export type WorkRole = '研发' | '测试' | '产品'
export interface RoleAssignment { id: string; subject_id: string; project_id?: string; role: WorkRole }
export interface Event { event_id: string; subject_id: string; device_id: string; event_type: string; work_role?: WorkRole; occurred_at: string }
export interface AuditLog { id: string; actor: string; action: string; resource: string; result: string; created_at: string }
export interface ProjectLocation { id: string; logical_project_id: string; device_id: string; local_project_id: string; display_name: string; workspace_kind: 'primary' | 'clone' | 'worktree' | 'non_vcs'; worktree_name?: string; active: boolean; key_version: number; metadata_revision: number }
export interface LogicalProject { id: string; display_name: string; vcs: 'git' | 'svn' | 'none'; status: 'active' | 'archived' | 'needs_review'; metadata_revision: number; location_count: number; locations: ProjectLocation[] }
export interface EffectivenessBreakdown { event_count: number; active_window_minutes: number }
export interface EffectivenessCoverage { covered_days: number; period_days: number; coverage_ratio: number; source_count: number; device_count: number; last_event_at?: string; insufficient: boolean }
export interface EffectivenessSubject { subject_id: string; display_name: string; active_days: number; active_window_minutes: number; event_count: number; coverage_ratio: number }
export interface EffectivenessReport {
  subject_id: string;
  from: string;
  to: string;
  timezone: string;
  metric_definition_version: number;
  totals: Record<string, number>;
  daily: Array<Record<string, number | string>>;
  project_breakdown: Record<string, EffectivenessBreakdown>;
  work_role_breakdown: Record<string, EffectivenessBreakdown>;
  activity_breakdown: Record<string, number>;
  coverage: EffectivenessCoverage;
  evidence_event_ids: string[];
  definitions: Record<string, string>;
}
