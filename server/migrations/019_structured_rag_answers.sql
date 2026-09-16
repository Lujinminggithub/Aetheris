ALTER TABLE retrieval_query_jobs
    ADD COLUMN IF NOT EXISTS answer_mode TEXT NOT NULL DEFAULT 'direct',
    ADD COLUMN IF NOT EXISTS answer_confidence TEXT NOT NULL DEFAULT 'low',
    ADD COLUMN IF NOT EXISTS answer_details TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS citation_numbers JSONB NOT NULL DEFAULT '[]'::jsonb;

ALTER TABLE retrieval_query_jobs DROP CONSTRAINT IF EXISTS retrieval_query_jobs_answer_mode_check;
ALTER TABLE retrieval_query_jobs ADD CONSTRAINT retrieval_query_jobs_answer_mode_check
    CHECK (answer_mode IN ('direct', 'numeric', 'reason', 'procedure', 'analysis'));
ALTER TABLE retrieval_query_jobs DROP CONSTRAINT IF EXISTS retrieval_query_jobs_answer_confidence_check;
ALTER TABLE retrieval_query_jobs ADD CONSTRAINT retrieval_query_jobs_answer_confidence_check
    CHECK (answer_confidence IN ('high', 'medium', 'low'));
