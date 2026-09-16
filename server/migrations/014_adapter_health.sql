CREATE TABLE IF NOT EXISTS adapter_health_snapshots (
    tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    device_id TEXT NOT NULL,
    adapter_id TEXT NOT NULL,
    state TEXT NOT NULL CHECK (state IN ('active','idle','disabled','source_missing','permission_denied','format_changed','dependency_missing','error')),
    capability_version TEXT NOT NULL DEFAULT '1',
    detected_format TEXT NOT NULL DEFAULT '',
    last_scan_at TIMESTAMPTZ,
    last_success_at TIMESTAMPTZ,
    last_event_at TIMESTAMPTZ,
    discovered INTEGER NOT NULL DEFAULT 0 CHECK (discovered >= 0),
    parsed INTEGER NOT NULL DEFAULT 0 CHECK (parsed >= 0),
    skipped INTEGER NOT NULL DEFAULT 0 CHECK (skipped >= 0),
    failed INTEGER NOT NULL DEFAULT 0 CHECK (failed >= 0),
    lag_seconds BIGINT NOT NULL DEFAULT 0 CHECK (lag_seconds >= 0),
    error_code TEXT NOT NULL DEFAULT '',
    error_stage TEXT NOT NULL DEFAULT '',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (tenant_id, device_id, adapter_id),
    FOREIGN KEY (tenant_id, device_id) REFERENCES devices(tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS adapter_health_tenant_state_idx ON adapter_health_snapshots(tenant_id, state, updated_at DESC);
ALTER TABLE adapter_health_snapshots ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_scope_adapter_health_snapshots ON adapter_health_snapshots
    USING (tenant_id = current_setting('aetheris.tenant_id', true));

INSERT INTO permissions(id, description)
VALUES ('health:report', '设备上报适配器健康状态')
ON CONFLICT (id) DO NOTHING;
INSERT INTO access_role_permissions(role_id, permission_id)
VALUES ('device_ingest', 'health:report')
ON CONFLICT DO NOTHING;
