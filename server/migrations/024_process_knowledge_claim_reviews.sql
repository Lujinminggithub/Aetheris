CREATE TABLE IF NOT EXISTS process_knowledge_claim_reviews (
    tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    review_id TEXT NOT NULL,
    claim_id TEXT NOT NULL,
    action TEXT NOT NULL CHECK (action IN ('confirm','reject')),
    actor_id TEXT NOT NULL REFERENCES users(id),
    reason TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (tenant_id, review_id),
    FOREIGN KEY (tenant_id, claim_id) REFERENCES process_knowledge_claims(tenant_id, claim_id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS process_knowledge_claim_reviews_claim_idx
    ON process_knowledge_claim_reviews(tenant_id,claim_id,created_at DESC);

ALTER TABLE process_knowledge_claim_reviews ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_scope_process_knowledge_claim_reviews ON process_knowledge_claim_reviews USING (tenant_id=current_setting('aetheris.tenant_id', true));
