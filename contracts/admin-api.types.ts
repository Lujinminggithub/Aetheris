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

export type PublicKnowledgePublicationState = 'candidate' | 'pending_review' | 'published' | 'suspended' | 'withdrawn'
export type PublicKnowledgeValidationState = 'unverified' | 'source_confirmed' | 'evidence_verified' | 'cross_tenant_corroborated' | 'platform_certified' | 'contradicted'
export interface PublicKnowledgeSummary { candidate: number; pending_review: number; published: number; suspended: number; withdrawn: number; open_conflicts: number; active_jobs: number }
export interface PublicKnowledgeRevision {
  revision: number; problem_pattern: string; conclusion: string; rationale: string; applicability: string; caveats: string; alternatives: string;
  validation_state: PublicKnowledgeValidationState; anonymous_source_tenant_count: number; independent_session_count: number; canonical_hash: string;
}
export interface PublicKnowledgeReview { review_id: string; revision: number; action: string; actor_id: string; reason: string; created_at: string }
export interface PublicKnowledgePrivateEvidence { knowledge_id: string; revision: number; relation: string }
export interface PublicKnowledgeUnit {
  public_knowledge_id: string; canonical_topic: string; knowledge_type: string; publication_state: PublicKnowledgePublicationState;
  current_revision: number; domains: string[]; entities: string[]; scope_state: 'classified' | 'unclassified' | 'rejected'; current: PublicKnowledgeRevision; reviews?: PublicKnowledgeReview[]; private_evidence?: PublicKnowledgePrivateEvidence[];
}
export interface PublicKnowledgeReviewCommand { expected_revision: number; reason: string }
export interface ProcessKnowledgeClaim { claim_id: string; knowledge_id: string; revision: number; sequence: number; domain: string; entities: string[]; problem: string; claim: string; applicability: string; validation_state: 'unverified' | 'partially_verified' | 'verified' | 'contradicted'; lifecycle_state: 'candidate' | 'confirmed' | 'rejected' | 'superseded'; evidence_ids: string[] }
