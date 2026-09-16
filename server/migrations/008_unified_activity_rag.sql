ALTER TABLE clean_event_facts DROP CONSTRAINT IF EXISTS clean_event_facts_fact_type_check;
ALTER TABLE clean_event_facts ADD CONSTRAINT clean_event_facts_fact_type_check
    CHECK (fact_type IN ('terminal_operation', 'ai_interaction', 'activity', 'command_fragment'));

ALTER TABLE clean_event_facts ADD COLUMN IF NOT EXISTS event_type TEXT NOT NULL DEFAULT '';
ALTER TABLE clean_event_facts ADD COLUMN IF NOT EXISTS source TEXT NOT NULL DEFAULT '';
ALTER TABLE clean_event_facts ADD COLUMN IF NOT EXISTS activity_type TEXT NOT NULL DEFAULT 'other';
ALTER TABLE clean_event_facts DROP CONSTRAINT IF EXISTS clean_event_facts_activity_type_check;
ALTER TABLE clean_event_facts ADD CONSTRAINT clean_event_facts_activity_type_check
    CHECK (activity_type IN ('ai', 'terminal', 'ide', 'browser', 'version_control', 'other'));

UPDATE clean_event_facts f SET
    event_type = e.event_type,
    source = e.source,
    activity_type = CASE
        WHEN e.event_type LIKE 'ai.%' OR e.source LIKE 'core.ai.%' THEN 'ai'
        WHEN e.event_type = 'terminal.command' THEN 'terminal'
        WHEN e.event_type = 'ide.activity' OR e.source LIKE '%vscode%' OR e.source LIKE '%visual_studio%' THEN 'ide'
        WHEN e.event_type LIKE 'browser.%' OR e.source LIKE 'core.browser%' THEN 'browser'
        WHEN e.event_type LIKE 'git.%' OR e.event_type LIKE 'svn.%' THEN 'version_control'
        ELSE 'other' END
FROM events e
WHERE e.tenant_id=f.tenant_id AND e.event_id=f.canonical_event_id AND f.event_type='';

CREATE INDEX IF NOT EXISTS clean_event_facts_activity_idx
    ON clean_event_facts(tenant_id, rule_version, activity_type, occurred_at DESC)
    WHERE quality_state IN ('accepted','merged') AND excluded_from_effectiveness=FALSE;

CREATE TABLE IF NOT EXISTS retrieval_documents (
    tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    document_id TEXT NOT NULL,
    fact_id TEXT NOT NULL,
    rule_version INTEGER NOT NULL,
    subject_id TEXT NOT NULL,
    device_id TEXT NOT NULL,
    project_id TEXT NOT NULL,
    activity_type TEXT NOT NULL,
    occurred_at TIMESTAMPTZ NOT NULL,
    content TEXT NOT NULL,
    content_hash TEXT NOT NULL,
    embedding_model TEXT NOT NULL,
    vector_key TEXT NOT NULL DEFAULT '',
    index_status TEXT NOT NULL DEFAULT 'pending' CHECK (index_status IN ('pending','indexing','indexed','failed')),
    error_code TEXT,
    indexed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (tenant_id, document_id),
    UNIQUE (tenant_id, fact_id, rule_version),
    FOREIGN KEY (tenant_id, fact_id, rule_version) REFERENCES clean_event_facts(tenant_id, fact_id, rule_version) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS retrieval_query_jobs (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    actor_id TEXT NOT NULL,
    question TEXT NOT NULL,
    filters JSONB NOT NULL DEFAULT '{}'::jsonb,
    status TEXT NOT NULL CHECK (status IN ('queued','embedding','retrieving','generating','completed','failed')),
    progress INTEGER NOT NULL DEFAULT 0 CHECK (progress BETWEEN 0 AND 100),
    answer TEXT NOT NULL DEFAULT '',
    citations JSONB NOT NULL DEFAULT '[]'::jsonb,
    error_code TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS retrieval_documents_pending_idx ON retrieval_documents(tenant_id, index_status, occurred_at) WHERE index_status <> 'indexed';
CREATE INDEX IF NOT EXISTS retrieval_documents_scope_idx ON retrieval_documents(tenant_id, device_id, project_id, activity_type, occurred_at DESC);
CREATE INDEX IF NOT EXISTS retrieval_query_jobs_actor_idx ON retrieval_query_jobs(tenant_id, actor_id, created_at DESC);

INSERT INTO permissions(id, description) VALUES
    ('retrieval:query', '使用智能查询'),
    ('retrieval:manage', '管理检索索引')
ON CONFLICT (id) DO NOTHING;
INSERT INTO access_role_permissions(role_id, permission_id) VALUES
    ('platform_admin','retrieval:query'), ('platform_admin','retrieval:manage'),
    ('tenant_admin','retrieval:query'), ('tenant_admin','retrieval:manage'),
    ('analyst','retrieval:query'), ('member','retrieval:query')
ON CONFLICT DO NOTHING;

ALTER TABLE retrieval_documents ENABLE ROW LEVEL SECURITY;
ALTER TABLE retrieval_query_jobs ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_scope_retrieval_documents ON retrieval_documents USING (tenant_id = current_setting('aetheris.tenant_id', true));
CREATE POLICY tenant_scope_retrieval_query_jobs ON retrieval_query_jobs USING (tenant_id = current_setting('aetheris.tenant_id', true));
