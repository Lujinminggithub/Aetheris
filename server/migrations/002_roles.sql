CREATE TABLE IF NOT EXISTS access_roles (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    description TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS permissions (
    id TEXT PRIMARY KEY,
    description TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS access_role_permissions (
    role_id TEXT NOT NULL REFERENCES access_roles(id) ON DELETE CASCADE,
    permission_id TEXT NOT NULL REFERENCES permissions(id) ON DELETE CASCADE,
    PRIMARY KEY (role_id, permission_id)
);

CREATE TABLE IF NOT EXISTS memberships (
    tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role_id TEXT NOT NULL REFERENCES access_roles(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (tenant_id, user_id, role_id)
);

CREATE TABLE IF NOT EXISTS projects (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    root_hint TEXT NOT NULL DEFAULT '',
    active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (tenant_id, id)
);

CREATE TABLE IF NOT EXISTS project_memberships (
    tenant_id TEXT NOT NULL,
    project_id TEXT NOT NULL,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role_id TEXT NOT NULL REFERENCES access_roles(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (tenant_id, project_id, user_id, role_id),
    FOREIGN KEY (tenant_id, project_id) REFERENCES projects(tenant_id, id)
);

CREATE TABLE IF NOT EXISTS work_roles (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    code TEXT NOT NULL,
    display_name TEXT NOT NULL,
    version INTEGER NOT NULL DEFAULT 1,
    active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (tenant_id, id),
    UNIQUE (tenant_id, code, version)
);

CREATE TABLE IF NOT EXISTS work_role_assignments (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    work_role_id TEXT NOT NULL,
    subject_id TEXT,
    device_id TEXT,
    project_id TEXT,
    source TEXT NOT NULL,
    confirmation_state TEXT NOT NULL DEFAULT 'confirmed',
    valid_from TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    valid_to TIMESTAMPTZ,
    created_by TEXT REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (subject_id IS NOT NULL OR device_id IS NOT NULL OR project_id IS NULL),
    FOREIGN KEY (tenant_id, work_role_id) REFERENCES work_roles(tenant_id, id),
    FOREIGN KEY (tenant_id, subject_id) REFERENCES subjects(tenant_id, id),
    FOREIGN KEY (tenant_id, device_id) REFERENCES devices(tenant_id, id),
    FOREIGN KEY (tenant_id, project_id) REFERENCES projects(tenant_id, id)
);

CREATE INDEX IF NOT EXISTS work_role_assignment_lookup_idx
    ON work_role_assignments(tenant_id, subject_id, device_id, project_id, valid_from, valid_to);

INSERT INTO access_roles(id, name) VALUES
    ('platform_admin', 'platform_admin'), ('tenant_admin', 'tenant_admin'), ('analyst', 'analyst'),
    ('reviewer', 'reviewer'), ('member', 'member'), ('device_ingest', 'device_ingest')
ON CONFLICT (id) DO NOTHING;

INSERT INTO permissions(id) VALUES
    ('events:ingest'), ('events:read'), ('events:export'), ('events:delete'),
    ('devices:register'), ('devices:manage'), ('projects:manage'), ('users:manage'),
    ('audit:read'), ('models:invoke'), ('models:manage')
ON CONFLICT (id) DO NOTHING;

INSERT INTO access_role_permissions(role_id, permission_id)
SELECT 'platform_admin', id FROM permissions
ON CONFLICT DO NOTHING;
INSERT INTO access_role_permissions(role_id, permission_id)
SELECT 'tenant_admin', id FROM permissions WHERE id IN ('events:ingest','events:read','events:export','events:delete','devices:register','devices:manage','projects:manage','users:manage','audit:read','models:invoke','models:manage')
ON CONFLICT DO NOTHING;
INSERT INTO access_role_permissions(role_id, permission_id) VALUES
    ('analyst','events:read'), ('analyst','events:export'), ('analyst','models:invoke'),
    ('reviewer','events:read'), ('reviewer','events:delete'), ('reviewer','audit:read'),
    ('member','events:read'), ('device_ingest','events:ingest'), ('device_ingest','devices:register')
ON CONFLICT DO NOTHING;
