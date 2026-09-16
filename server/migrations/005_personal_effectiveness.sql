CREATE TABLE IF NOT EXISTS user_subject_links (
    tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    subject_id TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (tenant_id, user_id),
    UNIQUE (tenant_id, subject_id),
    FOREIGN KEY (tenant_id, subject_id) REFERENCES subjects(tenant_id, id)
);

CREATE TABLE IF NOT EXISTS subject_effectiveness_daily (
    tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    subject_id TEXT NOT NULL,
    local_date DATE NOT NULL,
    timezone TEXT NOT NULL,
    metric_definition_version INTEGER NOT NULL,
    active_window_minutes INTEGER NOT NULL DEFAULT 0,
    session_count INTEGER NOT NULL DEFAULT 0,
    total_session_minutes INTEGER NOT NULL DEFAULT 0,
    longest_session_minutes INTEGER NOT NULL DEFAULT 0,
    focus_block_count INTEGER NOT NULL DEFAULT 0,
    focus_block_minutes INTEGER NOT NULL DEFAULT 0,
    context_switch_count INTEGER NOT NULL DEFAULT 0,
    delivery_events INTEGER NOT NULL DEFAULT 0,
    coding_events INTEGER NOT NULL DEFAULT 0,
    terminal_events INTEGER NOT NULL DEFAULT 0,
    ai_collaboration_events INTEGER NOT NULL DEFAULT 0,
    other_events INTEGER NOT NULL DEFAULT 0,
    project_breakdown JSONB NOT NULL DEFAULT '{}'::jsonb,
    work_role_breakdown JSONB NOT NULL DEFAULT '{}'::jsonb,
    source_counts JSONB NOT NULL DEFAULT '{}'::jsonb,
    device_ids JSONB NOT NULL DEFAULT '[]'::jsonb,
    evidence_event_ids JSONB NOT NULL DEFAULT '[]'::jsonb,
    first_event_at TIMESTAMPTZ,
    last_event_at TIMESTAMPTZ,
    computed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (tenant_id, subject_id, local_date, timezone, metric_definition_version),
    FOREIGN KEY (tenant_id, subject_id) REFERENCES subjects(tenant_id, id),
    CHECK (active_window_minutes BETWEEN 0 AND 1440)
);

CREATE INDEX IF NOT EXISTS subject_effectiveness_daily_range_idx
    ON subject_effectiveness_daily(tenant_id, subject_id, local_date DESC);

CREATE TABLE IF NOT EXISTS effectiveness_recompute_jobs (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    requested_by TEXT,
    date_from DATE NOT NULL,
    date_to DATE NOT NULL,
    timezone TEXT NOT NULL,
    metric_definition_version INTEGER NOT NULL,
    status TEXT NOT NULL,
    error_code TEXT,
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (date_to >= date_from),
    CHECK (date_to - date_from <= 30)
);

INSERT INTO permissions(id, description) VALUES
    ('effectiveness:read', '读取可解释个人效能'),
    ('effectiveness:manage', '重算个人效能')
ON CONFLICT (id) DO NOTHING;

INSERT INTO access_role_permissions(role_id, permission_id) VALUES
    ('platform_admin', 'effectiveness:read'),
    ('platform_admin', 'effectiveness:manage'),
    ('tenant_admin', 'effectiveness:read'),
    ('tenant_admin', 'effectiveness:manage'),
    ('member', 'effectiveness:read')
ON CONFLICT DO NOTHING;

ALTER TABLE user_subject_links ENABLE ROW LEVEL SECURITY;
ALTER TABLE subject_effectiveness_daily ENABLE ROW LEVEL SECURITY;
ALTER TABLE effectiveness_recompute_jobs ENABLE ROW LEVEL SECURITY;

CREATE POLICY tenant_scope_user_subject_links ON user_subject_links
    USING (tenant_id = current_setting('aetheris.tenant_id', true));
CREATE POLICY tenant_scope_subject_effectiveness_daily ON subject_effectiveness_daily
    USING (tenant_id = current_setting('aetheris.tenant_id', true));
CREATE POLICY tenant_scope_effectiveness_recompute_jobs ON effectiveness_recompute_jobs
    USING (tenant_id = current_setting('aetheris.tenant_id', true));

