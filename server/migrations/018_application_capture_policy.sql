CREATE TABLE IF NOT EXISTS application_capture_policies (
    tenant_id TEXT PRIMARY KEY REFERENCES tenants(id) ON DELETE CASCADE,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    revision BIGINT NOT NULL DEFAULT 0 CHECK (revision >= 0),
    updated_by TEXT REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE application_capture_policies ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_scope_application_capture_policies ON application_capture_policies
    USING (tenant_id = current_setting('aetheris.tenant_id', true));

ALTER TABLE clean_event_facts DROP CONSTRAINT IF EXISTS clean_event_facts_activity_type_check;
ALTER TABLE clean_event_facts ADD CONSTRAINT clean_event_facts_activity_type_check
    CHECK (activity_type IN ('ai', 'terminal', 'ide', 'delivery', 'application', 'browser', 'version_control', 'other'));
