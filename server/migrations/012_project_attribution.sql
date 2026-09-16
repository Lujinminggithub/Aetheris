CREATE TABLE IF NOT EXISTS project_attribution_versions (
    tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    rule_version INTEGER NOT NULL CHECK (rule_version > 0),
    state TEXT NOT NULL CHECK (state IN ('calculating', 'completed', 'active', 'failed', 'rolled_back')),
    created_by TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ,
    activated_at TIMESTAMPTZ,
    PRIMARY KEY (tenant_id, rule_version)
);

CREATE TABLE IF NOT EXISTS project_attribution_state (
    tenant_id TEXT PRIMARY KEY REFERENCES tenants(id) ON DELETE CASCADE,
    active_rule_version INTEGER CHECK (active_rule_version > 0),
    previous_rule_version INTEGER CHECK (previous_rule_version > 0),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    FOREIGN KEY (tenant_id, active_rule_version) REFERENCES project_attribution_versions(tenant_id, rule_version),
    FOREIGN KEY (tenant_id, previous_rule_version) REFERENCES project_attribution_versions(tenant_id, rule_version)
);

CREATE TABLE IF NOT EXISTS project_attributions (
    tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    event_id TEXT NOT NULL,
    rule_version INTEGER NOT NULL,
    logical_project_id TEXT,
    project_location_id TEXT,
    method TEXT NOT NULL CHECK (method IN ('exact_binding', 'remote_fingerprint', 'safe_label', 'authorized_root', 'session_correlation', 'time_correlation', 'manual', 'unresolved', 'ambiguous_exact', 'ambiguous_label', 'ambiguous_session', 'ambiguous_time')),
    confidence TEXT NOT NULL CHECK (confidence IN ('high', 'medium', 'low')),
    evidence JSONB NOT NULL DEFAULT '{}'::jsonb,
    needs_review BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (tenant_id, event_id, rule_version),
    FOREIGN KEY (tenant_id, event_id) REFERENCES events(tenant_id, event_id) ON DELETE CASCADE,
    FOREIGN KEY (tenant_id, rule_version) REFERENCES project_attribution_versions(tenant_id, rule_version) ON DELETE CASCADE,
    FOREIGN KEY (tenant_id, logical_project_id) REFERENCES logical_projects(tenant_id, id),
    FOREIGN KEY (tenant_id, project_location_id) REFERENCES project_locations(tenant_id, id)
);

CREATE INDEX IF NOT EXISTS project_attributions_project_idx
    ON project_attributions(tenant_id, rule_version, logical_project_id, needs_review);

CREATE TABLE IF NOT EXISTS project_backfill_jobs (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    mode TEXT NOT NULL CHECK (mode IN ('dry_run', 'apply')),
    rule_version INTEGER NOT NULL CHECK (rule_version > 0),
    state TEXT NOT NULL CHECK (state IN ('pending', 'running', 'completed', 'failed')),
    scanned_count BIGINT NOT NULL DEFAULT 0,
    assigned_count BIGINT NOT NULL DEFAULT 0,
    unresolved_count BIGINT NOT NULL DEFAULT 0,
    conflict_count BIGINT NOT NULL DEFAULT 0,
    last_event_id TEXT NOT NULL DEFAULT '',
    error_code TEXT NOT NULL DEFAULT '',
    created_by TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ,
    UNIQUE (tenant_id, id),
    FOREIGN KEY (tenant_id, rule_version) REFERENCES project_attribution_versions(tenant_id, rule_version)
);

CREATE INDEX IF NOT EXISTS project_backfill_jobs_tenant_time_idx
    ON project_backfill_jobs(tenant_id, created_at DESC);

CREATE OR REPLACE VIEW current_event_projects AS
SELECT
    e.tenant_id,
    e.event_id,
    pa.logical_project_id,
    pa.project_location_id,
    COALESCE(
        lp.display_name,
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
    COALESCE(pa.method, 'unresolved') AS attribution_method,
    COALESCE(pa.confidence, 'low') AS attribution_confidence,
    COALESCE(pa.needs_review, TRUE) AS needs_review
FROM events e
JOIN projects p ON p.tenant_id = e.tenant_id AND p.id = e.project_id
LEFT JOIN project_attribution_state state ON state.tenant_id = e.tenant_id
LEFT JOIN project_attributions pa
    ON pa.tenant_id = e.tenant_id
    AND pa.event_id = e.event_id
    AND pa.rule_version = state.active_rule_version
LEFT JOIN logical_projects lp
    ON lp.tenant_id = pa.tenant_id
    AND lp.id = pa.logical_project_id;

ALTER TABLE project_attribution_versions ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_scope_project_attribution_versions ON project_attribution_versions
    USING (tenant_id = current_setting('aetheris.tenant_id', true));

ALTER TABLE project_attribution_state ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_scope_project_attribution_state ON project_attribution_state
    USING (tenant_id = current_setting('aetheris.tenant_id', true));

ALTER TABLE project_attributions ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_scope_project_attributions ON project_attributions
    USING (tenant_id = current_setting('aetheris.tenant_id', true));

ALTER TABLE project_backfill_jobs ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_scope_project_backfill_jobs ON project_backfill_jobs
    USING (tenant_id = current_setting('aetheris.tenant_id', true));
