ALTER TABLE public_knowledge_jobs DROP CONSTRAINT IF EXISTS public_knowledge_jobs_mode_check;
ALTER TABLE public_knowledge_jobs ADD CONSTRAINT public_knowledge_jobs_mode_check
    CHECK (mode IN ('build_candidates','atomize','reindex','withdraw','rebuild'));

CREATE TABLE IF NOT EXISTS process_knowledge_claims (
    tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    claim_id TEXT NOT NULL,
    knowledge_id TEXT NOT NULL,
    revision INTEGER NOT NULL,
    sequence INTEGER NOT NULL CHECK (sequence >= 0),
    domain TEXT NOT NULL,
    entities TEXT[] NOT NULL DEFAULT '{}',
    problem TEXT NOT NULL,
    claim TEXT NOT NULL,
    applicability TEXT NOT NULL DEFAULT '',
    validation_state TEXT NOT NULL CHECK (validation_state IN ('unverified','partially_verified','verified','contradicted')),
    lifecycle_state TEXT NOT NULL DEFAULT 'candidate' CHECK (lifecycle_state IN ('candidate','confirmed','rejected','superseded')),
    evidence_ids TEXT[] NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (tenant_id, claim_id),
    FOREIGN KEY (tenant_id, knowledge_id, revision) REFERENCES process_knowledge_units(tenant_id, knowledge_id, revision) ON DELETE CASCADE,
    UNIQUE (tenant_id, knowledge_id, revision, sequence)
);

CREATE INDEX IF NOT EXISTS process_knowledge_claims_domain_idx
    ON process_knowledge_claims(tenant_id, domain, lifecycle_state, validation_state);
CREATE INDEX IF NOT EXISTS process_knowledge_claims_knowledge_idx
    ON process_knowledge_claims(tenant_id, knowledge_id, revision, sequence);
CREATE INDEX IF NOT EXISTS process_knowledge_claims_entities_idx
    ON process_knowledge_claims USING GIN(entities);

CREATE TABLE IF NOT EXISTS process_knowledge_claim_relations (
    tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    relation_id TEXT NOT NULL,
    from_claim_id TEXT NOT NULL,
    to_claim_id TEXT NOT NULL,
    relation_type TEXT NOT NULL CHECK (relation_type IN ('prerequisite','causes','alternative','validates','contradicts','revises','follows')),
    evidence_ids TEXT[] NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (tenant_id, relation_id),
    CHECK (from_claim_id <> to_claim_id),
    FOREIGN KEY (tenant_id, from_claim_id) REFERENCES process_knowledge_claims(tenant_id, claim_id) ON DELETE CASCADE,
    FOREIGN KEY (tenant_id, to_claim_id) REFERENCES process_knowledge_claims(tenant_id, claim_id) ON DELETE CASCADE
);

ALTER TABLE process_knowledge_claims ENABLE ROW LEVEL SECURITY;
ALTER TABLE process_knowledge_claim_relations ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_scope_process_knowledge_claims ON process_knowledge_claims USING (tenant_id=current_setting('aetheris.tenant_id', true));
CREATE POLICY tenant_scope_process_knowledge_claim_relations ON process_knowledge_claim_relations USING (tenant_id=current_setting('aetheris.tenant_id', true));
