CREATE TABLE IF NOT EXISTS event_blobs (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    event_id TEXT NOT NULL,
    provider TEXT NOT NULL,
    object_key TEXT NOT NULL,
    content_hash TEXT NOT NULL,
    size_bytes BIGINT NOT NULL,
    retention_until TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS event_embeddings (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    event_id TEXT NOT NULL,
    provider TEXT NOT NULL,
    model_version TEXT NOT NULL,
    vector_key TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS model_runs (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    actor_id TEXT,
    provider TEXT NOT NULL,
    model TEXT NOT NULL,
    task TEXT NOT NULL,
    input_event_ids JSONB NOT NULL DEFAULT '[]'::jsonb,
    status TEXT NOT NULL,
    latency_ms BIGINT,
    error_code TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS audit_logs (
    id BIGSERIAL PRIMARY KEY,
    tenant_id TEXT REFERENCES tenants(id),
    actor_type TEXT NOT NULL,
    actor_id TEXT NOT NULL,
    action TEXT NOT NULL,
    resource_type TEXT NOT NULL,
    resource_id TEXT NOT NULL,
    scope JSONB NOT NULL DEFAULT '{}'::jsonb,
    outcome TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS api_sessions (
    id BIGSERIAL PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL UNIQUE,
    csrf_hash TEXT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS audit_logs_tenant_time_idx ON audit_logs(tenant_id, created_at DESC);
CREATE INDEX IF NOT EXISTS model_runs_tenant_time_idx ON model_runs(tenant_id, created_at DESC);

ALTER TABLE tenants ENABLE ROW LEVEL SECURITY;
ALTER TABLE subjects ENABLE ROW LEVEL SECURITY;
ALTER TABLE devices ENABLE ROW LEVEL SECURITY;
ALTER TABLE work_roles ENABLE ROW LEVEL SECURITY;
ALTER TABLE work_role_assignments ENABLE ROW LEVEL SECURITY;
ALTER TABLE projects ENABLE ROW LEVEL SECURITY;
ALTER TABLE events ENABLE ROW LEVEL SECURITY;
ALTER TABLE event_tombstones ENABLE ROW LEVEL SECURITY;
ALTER TABLE event_blobs ENABLE ROW LEVEL SECURITY;
ALTER TABLE event_embeddings ENABLE ROW LEVEL SECURITY;
ALTER TABLE model_runs ENABLE ROW LEVEL SECURITY;
ALTER TABLE audit_logs ENABLE ROW LEVEL SECURITY;

CREATE POLICY tenant_scope_tenants ON tenants USING (id = current_setting('aetheris.tenant_id', true));
CREATE POLICY tenant_scope_subjects ON subjects USING (tenant_id = current_setting('aetheris.tenant_id', true));
CREATE POLICY tenant_scope_devices ON devices USING (tenant_id = current_setting('aetheris.tenant_id', true));
CREATE POLICY tenant_scope_work_roles ON work_roles USING (tenant_id = current_setting('aetheris.tenant_id', true));
CREATE POLICY tenant_scope_work_role_assignments ON work_role_assignments USING (tenant_id = current_setting('aetheris.tenant_id', true));
CREATE POLICY tenant_scope_projects ON projects USING (tenant_id = current_setting('aetheris.tenant_id', true));
CREATE POLICY tenant_scope_events ON events USING (tenant_id = current_setting('aetheris.tenant_id', true));
CREATE POLICY tenant_scope_event_tombstones ON event_tombstones USING (tenant_id = current_setting('aetheris.tenant_id', true));
CREATE POLICY tenant_scope_event_blobs ON event_blobs USING (tenant_id = current_setting('aetheris.tenant_id', true));
CREATE POLICY tenant_scope_event_embeddings ON event_embeddings USING (tenant_id = current_setting('aetheris.tenant_id', true));
CREATE POLICY tenant_scope_model_runs ON model_runs USING (tenant_id = current_setting('aetheris.tenant_id', true));
CREATE POLICY tenant_scope_audit_logs ON audit_logs USING (tenant_id = current_setting('aetheris.tenant_id', true));

