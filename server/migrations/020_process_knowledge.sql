CREATE EXTENSION IF NOT EXISTS pg_trgm;

ALTER TABLE project_attributions DROP CONSTRAINT IF EXISTS project_attributions_method_check;
ALTER TABLE project_attributions ADD CONSTRAINT project_attributions_method_check CHECK (method IN ('exact_binding','remote_fingerprint','safe_label','authorized_root','superseded_event_inheritance','session_correlation','time_correlation','manual','unresolved','ambiguous_exact','ambiguous_superseded','ambiguous_label','ambiguous_session','ambiguous_time'));

CREATE OR REPLACE VIEW current_event_projects AS
SELECT
    e.tenant_id,
    e.event_id,
    COALESCE(pa.logical_project_id, inherited_pa.logical_project_id) AS logical_project_id,
    COALESCE(pa.project_location_id, inherited_pa.project_location_id) AS project_location_id,
    COALESCE(
        lp.display_name,
        inherited_lp.display_name,
        NULLIF(e.payload->>'project_label', ''),
        NULLIF(p.name, e.project_id),
        CASE
            WHEN e.source = 'core.ai.codex' THEN '未归属 · Codex'
            WHEN e.source = 'core.ai.claude_code' THEN '未归属 · Claude Code'
            WHEN e.source = 'core.ai.cursor' THEN '未归属 · Cursor'
            WHEN e.source = 'core.ai.github_copilot' THEN '未归属 · GitHub Copilot'
            ELSE '未归属项目'
        END
    ) AS project_name,
    COALESCE(pa.method, CASE WHEN inherited_pa.logical_project_id IS NOT NULL THEN 'superseded_event_inheritance' END, 'unresolved') AS attribution_method,
    COALESCE(pa.confidence, CASE WHEN inherited_pa.logical_project_id IS NOT NULL THEN 'high' END, 'low') AS attribution_confidence,
    CASE
        WHEN pa.event_id IS NOT NULL THEN pa.needs_review
        WHEN inherited_pa.logical_project_id IS NOT NULL THEN FALSE
        ELSE TRUE
    END AS needs_review
FROM events e
JOIN projects p ON p.tenant_id=e.tenant_id AND p.id=e.project_id
LEFT JOIN project_attribution_state state ON state.tenant_id=e.tenant_id
LEFT JOIN project_attributions pa ON pa.tenant_id=e.tenant_id AND pa.event_id=e.event_id AND pa.rule_version=state.active_rule_version
LEFT JOIN logical_projects lp ON lp.tenant_id=pa.tenant_id AND lp.id=pa.logical_project_id
LEFT JOIN project_attributions inherited_pa ON inherited_pa.tenant_id=e.tenant_id AND inherited_pa.event_id=e.supersedes_event_id AND inherited_pa.rule_version=state.active_rule_version
LEFT JOIN logical_projects inherited_lp ON inherited_lp.tenant_id=inherited_pa.tenant_id AND inherited_lp.id=inherited_pa.logical_project_id;

CREATE TABLE IF NOT EXISTS process_knowledge_versions (
    tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    version INTEGER NOT NULL CHECK (version > 0),
    state TEXT NOT NULL CHECK (state IN ('building','completed','active','failed','rolled_back')),
    extractor_version TEXT NOT NULL,
    chunk_version TEXT NOT NULL,
    cleaning_rule_version INTEGER NOT NULL,
    attribution_rule_version INTEGER,
    created_by TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ,
    activated_at TIMESTAMPTZ,
    PRIMARY KEY (tenant_id, version)
);

CREATE TABLE IF NOT EXISTS process_knowledge_state (
    tenant_id TEXT PRIMARY KEY REFERENCES tenants(id) ON DELETE CASCADE,
    mode TEXT NOT NULL DEFAULT 'shadow' CHECK (mode IN ('shadow','canary','active')),
    active_version INTEGER,
    previous_version INTEGER,
    canary_percent INTEGER NOT NULL DEFAULT 0 CHECK (canary_percent BETWEEN 0 AND 100),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    FOREIGN KEY (tenant_id, active_version) REFERENCES process_knowledge_versions(tenant_id, version),
    FOREIGN KEY (tenant_id, previous_version) REFERENCES process_knowledge_versions(tenant_id, version)
);

CREATE TABLE IF NOT EXISTS process_sessions (
    tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    session_id TEXT NOT NULL,
    subject_id TEXT NOT NULL,
    device_id TEXT NOT NULL,
    logical_project_id TEXT NOT NULL,
    ai_tool TEXT NOT NULL,
    source_session_id TEXT NOT NULL,
    association_method TEXT NOT NULL CHECK (association_method IN ('exact','parent_chain','inferred_time','isolated')),
    association_confidence TEXT NOT NULL CHECK (association_confidence IN ('high','medium','low')),
    state TEXT NOT NULL CHECK (state IN ('open','idle','closed','stale','superseded')),
    started_at TIMESTAMPTZ NOT NULL,
    ended_at TIMESTAMPTZ NOT NULL,
    source_event_ids TEXT[] NOT NULL DEFAULT '{}',
    cleaning_rule_version INTEGER NOT NULL,
    attribution_rule_version INTEGER,
    revision INTEGER NOT NULL DEFAULT 1 CHECK (revision > 0),
    supersedes_revision INTEGER,
    dirty BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (tenant_id, session_id),
    FOREIGN KEY (tenant_id, logical_project_id) REFERENCES logical_projects(tenant_id, id)
);

CREATE INDEX IF NOT EXISTS process_sessions_scope_idx
    ON process_sessions(tenant_id, logical_project_id, state, ended_at DESC);
CREATE INDEX IF NOT EXISTS process_sessions_dirty_idx
    ON process_sessions(tenant_id, dirty, updated_at) WHERE dirty=TRUE;

CREATE TABLE IF NOT EXISTS process_turns (
    tenant_id TEXT NOT NULL,
    turn_id TEXT NOT NULL,
    session_id TEXT NOT NULL,
    sequence INTEGER NOT NULL CHECK (sequence >= 0),
    message_role TEXT NOT NULL CHECK (message_role IN ('user','assistant','system','tool','unknown')),
    statement_kind TEXT NOT NULL CHECK (statement_kind IN ('human_question','human_constraint','human_followup','human_confirmation','human_rejection','ai_exploration','ai_final_answer','tool_call','tool_result','code_change','test_result','build_result','runtime_validation','system_context')),
    safe_content TEXT NOT NULL,
    content_hash TEXT NOT NULL,
    source_event_ids TEXT[] NOT NULL DEFAULT '{}',
    classification_method TEXT NOT NULL,
    classification_version TEXT NOT NULL,
    occurred_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (tenant_id, turn_id),
    UNIQUE (tenant_id, session_id, sequence),
    FOREIGN KEY (tenant_id, session_id) REFERENCES process_sessions(tenant_id, session_id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS process_turns_session_time_idx
    ON process_turns(tenant_id, session_id, sequence, occurred_at);
CREATE INDEX IF NOT EXISTS process_turns_source_ids_idx
    ON process_turns USING GIN(source_event_ids);

CREATE TABLE IF NOT EXISTS process_knowledge_units (
    tenant_id TEXT NOT NULL,
    knowledge_id TEXT NOT NULL,
    revision INTEGER NOT NULL CHECK (revision > 0),
    version INTEGER NOT NULL CHECK (version > 0),
    logical_project_id TEXT NOT NULL,
    session_id TEXT NOT NULL,
    topic TEXT NOT NULL,
    knowledge_type TEXT NOT NULL CHECK (knowledge_type IN ('concept','implementation_pattern','failure_mode','decision','validation','troubleshooting')),
    problem TEXT NOT NULL,
    intent TEXT NOT NULL,
    constraints_text TEXT NOT NULL DEFAULT '',
    conclusion TEXT NOT NULL,
    rationale TEXT NOT NULL DEFAULT '',
    alternatives TEXT NOT NULL DEFAULT '',
    applicability TEXT NOT NULL DEFAULT '',
    caveats TEXT NOT NULL DEFAULT '',
    decision_state TEXT NOT NULL CHECK (decision_state IN ('proposed','accepted','rejected','superseded')),
    validation_state TEXT NOT NULL CHECK (validation_state IN ('unverified','partially_verified','verified','contradicted')),
    lifecycle_state TEXT NOT NULL CHECK (lifecycle_state IN ('candidate','active','stale','withdrawn')),
    extractor TEXT NOT NULL,
    extractor_version TEXT NOT NULL,
    supersedes_revision INTEGER,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (tenant_id, knowledge_id, revision),
    FOREIGN KEY (tenant_id, version) REFERENCES process_knowledge_versions(tenant_id, version) ON DELETE CASCADE,
    FOREIGN KEY (tenant_id, logical_project_id) REFERENCES logical_projects(tenant_id, id),
    FOREIGN KEY (tenant_id, session_id) REFERENCES process_sessions(tenant_id, session_id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS process_knowledge_units_scope_idx
    ON process_knowledge_units(tenant_id, version, logical_project_id, lifecycle_state, validation_state, updated_at DESC);
CREATE INDEX IF NOT EXISTS process_knowledge_units_topic_idx
    ON process_knowledge_units(tenant_id, version, topic, knowledge_type);

CREATE TABLE IF NOT EXISTS process_knowledge_evidence (
    tenant_id TEXT NOT NULL,
    knowledge_id TEXT NOT NULL,
    revision INTEGER NOT NULL,
    evidence_id TEXT NOT NULL,
    evidence_kind TEXT NOT NULL CHECK (evidence_kind IN ('question','constraint','answer','tool','code','test','build','runtime','confirmation','rejection')),
    event_id TEXT,
    fact_id TEXT,
    turn_id TEXT,
    supports_section TEXT NOT NULL CHECK (supports_section IN ('problem','intent','constraints','conclusion','rationale','alternatives','applicability','caveats','validation')),
    relation TEXT NOT NULL CHECK (relation IN ('supports','contradicts','supersedes','context')),
    reason_code TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (tenant_id, knowledge_id, revision, evidence_id),
    FOREIGN KEY (tenant_id, knowledge_id, revision) REFERENCES process_knowledge_units(tenant_id, knowledge_id, revision) ON DELETE CASCADE,
    FOREIGN KEY (tenant_id, turn_id) REFERENCES process_turns(tenant_id, turn_id),
    FOREIGN KEY (tenant_id, event_id) REFERENCES events(tenant_id, event_id)
);

CREATE INDEX IF NOT EXISTS process_knowledge_evidence_event_idx
    ON process_knowledge_evidence(tenant_id, event_id, relation);
CREATE INDEX IF NOT EXISTS process_knowledge_evidence_turn_idx
    ON process_knowledge_evidence(tenant_id, turn_id, relation);

CREATE TABLE IF NOT EXISTS process_knowledge_chunks (
    tenant_id TEXT NOT NULL,
    chunk_id TEXT NOT NULL,
    knowledge_id TEXT NOT NULL,
    revision INTEGER NOT NULL,
    version INTEGER NOT NULL,
    logical_project_id TEXT NOT NULL,
    session_id TEXT NOT NULL,
    chunk_index INTEGER NOT NULL CHECK (chunk_index >= 0),
    topic TEXT NOT NULL,
    knowledge_type TEXT NOT NULL,
    decision_state TEXT NOT NULL,
    validation_state TEXT NOT NULL,
    content TEXT NOT NULL,
    search_text TEXT NOT NULL,
    content_hash TEXT NOT NULL,
    embedding_model TEXT NOT NULL,
    vector_key TEXT NOT NULL,
    index_status TEXT NOT NULL DEFAULT 'pending' CHECK (index_status IN ('pending','indexing','indexed','failed')),
    error_code TEXT,
    occurred_at TIMESTAMPTZ NOT NULL,
    indexed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (tenant_id, chunk_id),
    UNIQUE (tenant_id, knowledge_id, revision, chunk_index),
    FOREIGN KEY (tenant_id, knowledge_id, revision) REFERENCES process_knowledge_units(tenant_id, knowledge_id, revision) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS process_knowledge_chunks_scope_idx
    ON process_knowledge_chunks(tenant_id, version, logical_project_id, index_status, occurred_at DESC);
CREATE INDEX IF NOT EXISTS process_knowledge_chunks_pending_idx
    ON process_knowledge_chunks(tenant_id, index_status, updated_at) WHERE index_status<>'indexed';
CREATE INDEX IF NOT EXISTS process_knowledge_chunks_search_idx
    ON process_knowledge_chunks USING GIN (search_text gin_trgm_ops);

CREATE TABLE IF NOT EXISTS process_knowledge_jobs (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    mode TEXT NOT NULL CHECK (mode IN ('dry_run','apply')),
    version INTEGER NOT NULL CHECK (version > 0),
    logical_project_id TEXT,
    state TEXT NOT NULL CHECK (state IN ('pending','running','completed','failed')),
    last_session_id TEXT NOT NULL DEFAULT '',
    scanned_count BIGINT NOT NULL DEFAULT 0,
    candidate_count BIGINT NOT NULL DEFAULT 0,
    verified_count BIGINT NOT NULL DEFAULT 0,
    conflict_count BIGINT NOT NULL DEFAULT 0,
    failed_count BIGINT NOT NULL DEFAULT 0,
    error_code TEXT NOT NULL DEFAULT '',
    created_by TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ,
    FOREIGN KEY (tenant_id, version) REFERENCES process_knowledge_versions(tenant_id, version),
    FOREIGN KEY (tenant_id, logical_project_id) REFERENCES logical_projects(tenant_id, id)
);

CREATE INDEX IF NOT EXISTS process_knowledge_jobs_tenant_time_idx
    ON process_knowledge_jobs(tenant_id, created_at DESC);

INSERT INTO permissions(id, description) VALUES
    ('process_knowledge:read', '读取过程知识和证据'),
    ('process_knowledge:manage', '回填、激活和回滚过程知识')
ON CONFLICT (id) DO NOTHING;

INSERT INTO access_role_permissions(role_id, permission_id) VALUES
    ('platform_admin','process_knowledge:read'),
    ('platform_admin','process_knowledge:manage'),
    ('tenant_admin','process_knowledge:read'),
    ('tenant_admin','process_knowledge:manage'),
    ('analyst','process_knowledge:read'),
    ('member','process_knowledge:read')
ON CONFLICT DO NOTHING;

ALTER TABLE process_knowledge_versions ENABLE ROW LEVEL SECURITY;
ALTER TABLE process_knowledge_state ENABLE ROW LEVEL SECURITY;
ALTER TABLE process_sessions ENABLE ROW LEVEL SECURITY;
ALTER TABLE process_turns ENABLE ROW LEVEL SECURITY;
ALTER TABLE process_knowledge_units ENABLE ROW LEVEL SECURITY;
ALTER TABLE process_knowledge_evidence ENABLE ROW LEVEL SECURITY;
ALTER TABLE process_knowledge_chunks ENABLE ROW LEVEL SECURITY;
ALTER TABLE process_knowledge_jobs ENABLE ROW LEVEL SECURITY;

CREATE POLICY tenant_scope_process_knowledge_versions ON process_knowledge_versions USING (tenant_id=current_setting('aetheris.tenant_id', true));
CREATE POLICY tenant_scope_process_knowledge_state ON process_knowledge_state USING (tenant_id=current_setting('aetheris.tenant_id', true));
CREATE POLICY tenant_scope_process_sessions ON process_sessions USING (tenant_id=current_setting('aetheris.tenant_id', true));
CREATE POLICY tenant_scope_process_turns ON process_turns USING (tenant_id=current_setting('aetheris.tenant_id', true));
CREATE POLICY tenant_scope_process_knowledge_units ON process_knowledge_units USING (tenant_id=current_setting('aetheris.tenant_id', true));
CREATE POLICY tenant_scope_process_knowledge_evidence ON process_knowledge_evidence USING (tenant_id=current_setting('aetheris.tenant_id', true));
CREATE POLICY tenant_scope_process_knowledge_chunks ON process_knowledge_chunks USING (tenant_id=current_setting('aetheris.tenant_id', true));
CREATE POLICY tenant_scope_process_knowledge_jobs ON process_knowledge_jobs USING (tenant_id=current_setting('aetheris.tenant_id', true));
