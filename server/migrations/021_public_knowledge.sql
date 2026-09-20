ALTER TABLE process_turns DROP CONSTRAINT IF EXISTS process_turns_statement_kind_check;
ALTER TABLE process_turns ADD CONSTRAINT process_turns_statement_kind_check CHECK (statement_kind IN (
    'human_question','human_constraint','human_followup','human_confirmation','human_rejection',
    'ai_exploration','ai_final_answer','search_query','search_result','reasoning_summary',
    'tool_call','tool_result','code_change','test_result','build_result','runtime_validation','system_context'
));

ALTER TABLE process_knowledge_evidence DROP CONSTRAINT IF EXISTS process_knowledge_evidence_evidence_kind_check;
ALTER TABLE process_knowledge_evidence ADD CONSTRAINT process_knowledge_evidence_evidence_kind_check CHECK (evidence_kind IN (
    'question','constraint','answer','tool','tool_result','search','external_source','analysis',
    'code','test','build','runtime','confirmation','rejection'
));

CREATE TABLE IF NOT EXISTS public_knowledge_units (
    public_knowledge_id TEXT PRIMARY KEY,
    canonical_topic TEXT NOT NULL,
    knowledge_type TEXT NOT NULL CHECK (knowledge_type IN ('concept','implementation_pattern','failure_mode','decision','validation','troubleshooting')),
    publication_state TEXT NOT NULL DEFAULT 'candidate' CHECK (publication_state IN ('candidate','pending_review','published','suspended','withdrawn')),
    current_revision INTEGER NOT NULL DEFAULT 0 CHECK (current_revision >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS public_knowledge_revisions (
    public_knowledge_id TEXT NOT NULL REFERENCES public_knowledge_units(public_knowledge_id) ON DELETE CASCADE,
    revision INTEGER NOT NULL CHECK (revision > 0),
    problem_pattern TEXT NOT NULL,
    conclusion TEXT NOT NULL,
    rationale TEXT NOT NULL DEFAULT '',
    applicability TEXT NOT NULL DEFAULT '',
    caveats TEXT NOT NULL DEFAULT '',
    alternatives TEXT NOT NULL DEFAULT '',
    validation_state TEXT NOT NULL CHECK (validation_state IN ('unverified','source_confirmed','evidence_verified','cross_tenant_corroborated','platform_certified','contradicted')),
    anonymous_source_tenant_count INTEGER NOT NULL DEFAULT 0 CHECK (anonymous_source_tenant_count >= 0),
    independent_session_count INTEGER NOT NULL DEFAULT 0 CHECK (independent_session_count >= 0),
    canonical_hash TEXT NOT NULL,
    extractor_version TEXT NOT NULL,
    redaction_version TEXT NOT NULL,
    review_policy_version TEXT NOT NULL,
    redaction_report JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (public_knowledge_id, revision)
);

CREATE INDEX IF NOT EXISTS public_knowledge_revision_hash_idx
    ON public_knowledge_revisions(canonical_hash, validation_state, created_at DESC);

CREATE TABLE IF NOT EXISTS public_knowledge_sources (
    source_id TEXT PRIMARY KEY,
    public_knowledge_id TEXT NOT NULL,
    public_revision INTEGER NOT NULL,
    source_tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    source_knowledge_id TEXT NOT NULL,
    source_revision INTEGER NOT NULL,
    relation TEXT NOT NULL CHECK (relation IN ('supports','contradicts','supersedes','context')),
    independence_group TEXT NOT NULL,
    source_content_hash TEXT NOT NULL,
    external_source_hash TEXT NOT NULL DEFAULT '',
    import_batch_id TEXT NOT NULL DEFAULT '',
    redaction_state TEXT NOT NULL CHECK (redaction_state IN ('passed','blocked','needs_review')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (public_knowledge_id, public_revision, source_tenant_id, source_knowledge_id, source_revision, relation),
    FOREIGN KEY (public_knowledge_id, public_revision) REFERENCES public_knowledge_revisions(public_knowledge_id, revision) ON DELETE CASCADE,
    FOREIGN KEY (source_tenant_id, source_knowledge_id, source_revision) REFERENCES process_knowledge_units(tenant_id, knowledge_id, revision) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS public_knowledge_sources_private_idx
    ON public_knowledge_sources(source_tenant_id, source_knowledge_id, source_revision);
CREATE INDEX IF NOT EXISTS public_knowledge_sources_independence_idx
    ON public_knowledge_sources(public_knowledge_id, public_revision, independence_group, relation);

CREATE TABLE IF NOT EXISTS public_knowledge_reviews (
    review_id TEXT PRIMARY KEY,
    public_knowledge_id TEXT NOT NULL,
    revision INTEGER NOT NULL,
    action TEXT NOT NULL CHECK (action IN ('confirm','verify','certify','reject','request_changes','suspend','withdraw','republish')),
    actor_id TEXT NOT NULL REFERENCES users(id),
    actor_tenant_id TEXT REFERENCES tenants(id),
    reason TEXT NOT NULL,
    validation_snapshot TEXT NOT NULL,
    publication_snapshot TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    FOREIGN KEY (public_knowledge_id, revision) REFERENCES public_knowledge_revisions(public_knowledge_id, revision) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS public_knowledge_reviews_unit_idx
    ON public_knowledge_reviews(public_knowledge_id, revision, created_at DESC);

CREATE TABLE IF NOT EXISTS public_knowledge_conflicts (
    conflict_id TEXT PRIMARY KEY,
    left_public_knowledge_id TEXT NOT NULL,
    left_revision INTEGER NOT NULL,
    right_public_knowledge_id TEXT NOT NULL,
    right_revision INTEGER NOT NULL,
    conflict_kind TEXT NOT NULL CHECK (conflict_kind IN ('conclusion','applicability','version','source_withdrawal')),
    safe_summary TEXT NOT NULL,
    state TEXT NOT NULL DEFAULT 'open' CHECK (state IN ('open','resolved','dismissed')),
    resolution TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    resolved_at TIMESTAMPTZ,
    FOREIGN KEY (left_public_knowledge_id, left_revision) REFERENCES public_knowledge_revisions(public_knowledge_id, revision),
    FOREIGN KEY (right_public_knowledge_id, right_revision) REFERENCES public_knowledge_revisions(public_knowledge_id, revision),
    CHECK (left_public_knowledge_id <> right_public_knowledge_id OR left_revision <> right_revision)
);

CREATE INDEX IF NOT EXISTS public_knowledge_conflicts_state_idx
    ON public_knowledge_conflicts(state, created_at DESC);

CREATE TABLE IF NOT EXISTS public_knowledge_chunks (
    chunk_id TEXT PRIMARY KEY,
    public_knowledge_id TEXT NOT NULL,
    revision INTEGER NOT NULL,
    chunk_index INTEGER NOT NULL CHECK (chunk_index >= 0),
    canonical_topic TEXT NOT NULL,
    knowledge_type TEXT NOT NULL,
    validation_state TEXT NOT NULL,
    content TEXT NOT NULL,
    search_text TEXT NOT NULL,
    content_hash TEXT NOT NULL,
    embedding_model TEXT NOT NULL,
    vector_key TEXT NOT NULL UNIQUE,
    index_status TEXT NOT NULL DEFAULT 'pending' CHECK (index_status IN ('pending','indexing','indexed','failed','withdrawn')),
    error_code TEXT NOT NULL DEFAULT '',
    indexed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (public_knowledge_id, revision, chunk_index),
    FOREIGN KEY (public_knowledge_id, revision) REFERENCES public_knowledge_revisions(public_knowledge_id, revision) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS public_knowledge_chunks_pending_idx
    ON public_knowledge_chunks(index_status, updated_at) WHERE index_status IN ('pending','failed');
CREATE INDEX IF NOT EXISTS public_knowledge_chunks_search_idx
    ON public_knowledge_chunks USING GIN (search_text gin_trgm_ops);

CREATE TABLE IF NOT EXISTS public_knowledge_jobs (
    id TEXT PRIMARY KEY,
    mode TEXT NOT NULL CHECK (mode IN ('build_candidates','reindex','withdraw','rebuild')),
    state TEXT NOT NULL DEFAULT 'pending' CHECK (state IN ('pending','running','paused','completed','failed')),
    last_tenant_id TEXT NOT NULL DEFAULT '',
    last_knowledge_id TEXT NOT NULL DEFAULT '',
    scanned_count BIGINT NOT NULL DEFAULT 0,
    candidate_count BIGINT NOT NULL DEFAULT 0,
    conflict_count BIGINT NOT NULL DEFAULT 0,
    failed_count BIGINT NOT NULL DEFAULT 0,
    error_code TEXT NOT NULL DEFAULT '',
    created_by TEXT NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS public_knowledge_jobs_state_idx
    ON public_knowledge_jobs(state, created_at, id);

CREATE OR REPLACE VIEW published_public_knowledge AS
SELECT
    u.public_knowledge_id,
    u.canonical_topic,
    u.knowledge_type,
    u.current_revision AS revision,
    r.problem_pattern,
    r.conclusion,
    r.rationale,
    r.applicability,
    r.caveats,
    r.alternatives,
    r.validation_state,
    r.anonymous_source_tenant_count,
    r.independent_session_count,
    r.canonical_hash,
    u.updated_at
FROM public_knowledge_units u
JOIN public_knowledge_revisions r
  ON r.public_knowledge_id=u.public_knowledge_id AND r.revision=u.current_revision
WHERE u.publication_state='published' AND r.validation_state='platform_certified';

INSERT INTO permissions(id, description) VALUES
    ('knowledge:confirm_source', '确认本租户知识来源'),
    ('knowledge:verify', '验证知识证据'),
    ('knowledge:certify_public', '认证并发布平台公共知识'),
    ('knowledge:withdraw_public', '暂停或撤回平台公共知识'),
    ('knowledge:diagnose', '查看公共知识治理状态')
ON CONFLICT (id) DO NOTHING;

INSERT INTO access_role_permissions(role_id, permission_id) VALUES
    ('platform_admin','knowledge:confirm_source'),
    ('platform_admin','knowledge:verify'),
    ('platform_admin','knowledge:certify_public'),
    ('platform_admin','knowledge:withdraw_public'),
    ('platform_admin','knowledge:diagnose'),
    ('tenant_admin','knowledge:confirm_source'),
    ('tenant_admin','knowledge:verify'),
    ('tenant_admin','knowledge:diagnose'),
    ('analyst','knowledge:confirm_source'),
    ('analyst','knowledge:verify'),
    ('analyst','knowledge:diagnose')
ON CONFLICT DO NOTHING;

ALTER TABLE public_knowledge_sources ENABLE ROW LEVEL SECURITY;
CREATE POLICY public_knowledge_sources_own_tenant ON public_knowledge_sources
    USING (source_tenant_id=current_setting('aetheris.tenant_id', true));
