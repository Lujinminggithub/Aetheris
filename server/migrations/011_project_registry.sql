CREATE TABLE IF NOT EXISTS logical_projects (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    display_name TEXT NOT NULL CHECK (length(display_name) BETWEEN 1 AND 255),
    vcs TEXT NOT NULL CHECK (vcs IN ('git', 'svn', 'none')),
    remote_fingerprint TEXT,
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'archived', 'needs_review')),
    metadata_revision BIGINT NOT NULL DEFAULT 0 CHECK (metadata_revision >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (tenant_id, id)
);

CREATE UNIQUE INDEX IF NOT EXISTS logical_projects_remote_fingerprint_idx
    ON logical_projects(tenant_id, vcs, remote_fingerprint)
    WHERE remote_fingerprint IS NOT NULL AND remote_fingerprint <> '';

CREATE TABLE IF NOT EXISTS project_locations (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    logical_project_id TEXT NOT NULL,
    device_id TEXT NOT NULL,
    local_project_id TEXT NOT NULL,
    display_name TEXT NOT NULL CHECK (length(display_name) BETWEEN 1 AND 255),
    root_fingerprint TEXT NOT NULL,
    workspace_kind TEXT NOT NULL CHECK (workspace_kind IN ('primary', 'clone', 'worktree', 'non_vcs')),
    worktree_name TEXT NOT NULL DEFAULT '',
    active BOOLEAN NOT NULL DEFAULT TRUE,
    key_version INTEGER NOT NULL CHECK (key_version > 0),
    metadata_revision BIGINT NOT NULL DEFAULT 0 CHECK (metadata_revision >= 0),
    first_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (tenant_id, id),
    UNIQUE (tenant_id, device_id, local_project_id),
    FOREIGN KEY (tenant_id, logical_project_id) REFERENCES logical_projects(tenant_id, id) ON DELETE CASCADE,
    FOREIGN KEY (tenant_id, device_id) REFERENCES devices(tenant_id, id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS project_locations_logical_project_idx
    ON project_locations(tenant_id, logical_project_id, active, last_seen_at DESC);

CREATE INDEX IF NOT EXISTS project_locations_legacy_project_idx
    ON project_locations(tenant_id, device_id, local_project_id);

CREATE TABLE IF NOT EXISTS project_registry_state (
    tenant_id TEXT PRIMARY KEY REFERENCES tenants(id) ON DELETE CASCADE,
    revision BIGINT NOT NULL DEFAULT 0 CHECK (revision >= 0),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE logical_projects ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_scope_logical_projects ON logical_projects
    USING (tenant_id = current_setting('aetheris.tenant_id', true));

ALTER TABLE project_locations ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_scope_project_locations ON project_locations
    USING (tenant_id = current_setting('aetheris.tenant_id', true));

ALTER TABLE project_registry_state ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_scope_project_registry_state ON project_registry_state
    USING (tenant_id = current_setting('aetheris.tenant_id', true));

INSERT INTO permissions(id, description)
VALUES ('projects:register', '设备注册安全项目身份')
ON CONFLICT (id) DO NOTHING;

INSERT INTO access_role_permissions(role_id, permission_id)
VALUES ('device_ingest', 'projects:register')
ON CONFLICT DO NOTHING;
