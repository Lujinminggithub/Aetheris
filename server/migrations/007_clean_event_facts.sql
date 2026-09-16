CREATE TABLE IF NOT EXISTS clean_event_facts (
    tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    fact_id TEXT NOT NULL,
    rule_version INTEGER NOT NULL,
    subject_id TEXT NOT NULL,
    device_id TEXT NOT NULL,
    project_id TEXT NOT NULL,
    occurred_at TIMESTAMPTZ NOT NULL,
    fact_type TEXT NOT NULL CHECK (fact_type IN ('terminal_operation', 'ai_interaction', 'command_fragment')),
    actor_origin TEXT NOT NULL CHECK (actor_origin IN ('human', 'ai', 'unknown')),
    message_role TEXT NOT NULL DEFAULT 'unknown' CHECK (message_role IN ('user', 'assistant', 'system', 'tool', 'unknown')),
    ai_tool TEXT NOT NULL DEFAULT '',
    command_type TEXT NOT NULL DEFAULT '',
    command_summary TEXT NOT NULL DEFAULT '',
    command_hash TEXT NOT NULL DEFAULT '',
    command_text TEXT NOT NULL DEFAULT '',
    quality_state TEXT NOT NULL CHECK (quality_state IN ('accepted', 'merged', 'quarantined')),
    confidence TEXT NOT NULL CHECK (confidence IN ('high', 'medium', 'low')),
    merge_method TEXT NOT NULL DEFAULT 'none' CHECK (merge_method IN ('none', 'explicit_backtick_join', 'inferred_parameter_join', 'ai_terminal_hash_match')),
    reason_codes TEXT[] NOT NULL DEFAULT ARRAY[]::TEXT[],
    source_event_ids TEXT[] NOT NULL,
    canonical_event_id TEXT NOT NULL,
    excluded_from_effectiveness BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (tenant_id, fact_id, rule_version),
    FOREIGN KEY (tenant_id, subject_id) REFERENCES subjects(tenant_id, id),
    FOREIGN KEY (tenant_id, device_id) REFERENCES devices(tenant_id, id),
    FOREIGN KEY (tenant_id, project_id) REFERENCES projects(tenant_id, id),
    CHECK (actor_origin <> 'ai' OR command_text = '')
);

CREATE TABLE IF NOT EXISTS cleaning_jobs (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    date_from DATE NOT NULL,
    date_to DATE NOT NULL,
    rule_version INTEGER NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('queued', 'running', 'completed', 'failed')),
    processed_events INTEGER NOT NULL DEFAULT 0,
    fact_count INTEGER NOT NULL DEFAULT 0,
    merged_count INTEGER NOT NULL DEFAULT 0,
    quarantined_count INTEGER NOT NULL DEFAULT 0,
    error_code TEXT,
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS cleaning_jobs_active_range_idx ON cleaning_jobs(tenant_id, date_from, date_to, rule_version) WHERE status IN ('queued', 'running');
CREATE INDEX IF NOT EXISTS clean_event_facts_scope_time_idx ON clean_event_facts(tenant_id, device_id, project_id, occurred_at DESC, rule_version);
CREATE INDEX IF NOT EXISTS clean_event_facts_quality_idx ON clean_event_facts(tenant_id, fact_type, quality_state, rule_version, occurred_at DESC);
CREATE INDEX IF NOT EXISTS clean_event_facts_source_ids_idx ON clean_event_facts USING GIN(source_event_ids);

INSERT INTO permissions(id, description) VALUES ('cleaning:manage', '重算清洗事实') ON CONFLICT (id) DO NOTHING;
INSERT INTO access_role_permissions(role_id, permission_id) VALUES ('platform_admin', 'cleaning:manage'), ('tenant_admin', 'cleaning:manage') ON CONFLICT DO NOTHING;

ALTER TABLE clean_event_facts ENABLE ROW LEVEL SECURITY;
ALTER TABLE cleaning_jobs ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_scope_clean_event_facts ON clean_event_facts USING (tenant_id = current_setting('aetheris.tenant_id', true));
CREATE POLICY tenant_scope_cleaning_jobs ON cleaning_jobs USING (tenant_id = current_setting('aetheris.tenant_id', true));
