CREATE TABLE IF NOT EXISTS events (
    event_id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    subject_id TEXT NOT NULL,
    device_id TEXT NOT NULL,
    project_id TEXT NOT NULL,
    session_id TEXT NOT NULL,
    correlation_id TEXT NOT NULL,
    event_type TEXT NOT NULL,
    schema_version INTEGER NOT NULL,
    source TEXT NOT NULL,
    source_version TEXT NOT NULL,
    occurred_at TIMESTAMPTZ NOT NULL,
    ingested_at TIMESTAMPTZ NOT NULL,
    payload JSONB NOT NULL,
    content_hash TEXT NOT NULL,
    content_refs JSONB NOT NULL DEFAULT '[]'::jsonb,
    provenance JSONB NOT NULL DEFAULT '{}'::jsonb,
    redaction_report JSONB NOT NULL DEFAULT '{}'::jsonb,
    processing_grants TEXT[] NOT NULL DEFAULT ARRAY[]::TEXT[],
    crypto_mode TEXT NOT NULL DEFAULT 'device-redacted',
    key_version TEXT,
    work_role_id TEXT,
    work_role_code TEXT,
    work_role_version INTEGER,
    role_source TEXT,
    supersedes_event_id TEXT,
    UNIQUE (tenant_id, event_id),
    FOREIGN KEY (tenant_id, subject_id) REFERENCES subjects(tenant_id, id),
    FOREIGN KEY (tenant_id, device_id) REFERENCES devices(tenant_id, id),
    FOREIGN KEY (tenant_id, project_id) REFERENCES projects(tenant_id, id)
);

CREATE TABLE IF NOT EXISTS event_tombstones (
    event_id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    reason TEXT NOT NULL,
    deleted_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_by TEXT
);

CREATE INDEX IF NOT EXISTS events_scope_time_idx ON events(tenant_id, project_id, occurred_at DESC);
CREATE INDEX IF NOT EXISTS events_role_time_idx ON events(tenant_id, work_role_code, occurred_at DESC);
CREATE INDEX IF NOT EXISTS events_device_time_idx ON events(tenant_id, device_id, occurred_at DESC);

