ALTER TABLE public_knowledge_units
    ADD COLUMN IF NOT EXISTS domains TEXT[] NOT NULL DEFAULT '{}',
    ADD COLUMN IF NOT EXISTS entities TEXT[] NOT NULL DEFAULT '{}',
    ADD COLUMN IF NOT EXISTS scope_state TEXT NOT NULL DEFAULT 'unclassified';

ALTER TABLE public_knowledge_units DROP CONSTRAINT IF EXISTS public_knowledge_units_scope_state_check;
ALTER TABLE public_knowledge_units ADD CONSTRAINT public_knowledge_units_scope_state_check
    CHECK (scope_state IN ('classified','unclassified','rejected'));

ALTER TABLE public_knowledge_chunks
    ADD COLUMN IF NOT EXISTS domains TEXT[] NOT NULL DEFAULT '{}',
    ADD COLUMN IF NOT EXISTS entities TEXT[] NOT NULL DEFAULT '{}';

UPDATE public_knowledge_units
SET domains = CASE
        WHEN lower(canonical_topic) LIKE '%dlp%' OR canonical_topic LIKE '%数据防泄漏%' THEN ARRAY['dlp']
        WHEN lower(canonical_topic) LIKE '%edr%' OR canonical_topic LIKE '%端点检测%' THEN ARRAY['edr']
        WHEN lower(canonical_topic) LIKE '%rag%' OR canonical_topic LIKE '%检索增强%' THEN ARRAY['rag']
        ELSE '{}'
    END,
    scope_state = CASE
        WHEN lower(canonical_topic) LIKE '%dlp%' OR canonical_topic LIKE '%数据防泄漏%'
          OR lower(canonical_topic) LIKE '%edr%' OR canonical_topic LIKE '%端点检测%'
          OR lower(canonical_topic) LIKE '%rag%' OR canonical_topic LIKE '%检索增强%'
        THEN 'classified'
        ELSE 'rejected'
    END
WHERE scope_state='unclassified';

UPDATE public_knowledge_units u
SET publication_state='withdrawn',scope_state='rejected',updated_at=NOW()
FROM public_knowledge_revisions r
WHERE r.public_knowledge_id=u.public_knowledge_id AND r.revision=u.current_revision
  AND (
      u.canonical_topic='研发过程知识'
      OR r.problem_pattern ILIKE '%委派子代理%'
      OR r.problem_pattern ILIKE '%继续规格实施%'
      OR r.problem_pattern ILIKE '%推荐方案实施%'
      OR r.conclusion ILIKE '%Agent message from%'
      OR r.conclusion ILIKE '%Message Type: FINAL_ANSWER%'
  );

UPDATE public_knowledge_chunks c
SET index_status='withdrawn',error_code='',updated_at=NOW()
FROM public_knowledge_units u
WHERE u.public_knowledge_id=c.public_knowledge_id
  AND (u.scope_state='rejected' OR u.publication_state='withdrawn')
  AND c.index_status<>'withdrawn';

UPDATE public_knowledge_chunks c
SET domains=u.domains,entities=u.entities
FROM public_knowledge_units u
WHERE u.public_knowledge_id=c.public_knowledge_id;

CREATE INDEX IF NOT EXISTS public_knowledge_units_domains_idx
    ON public_knowledge_units USING GIN(domains);
CREATE INDEX IF NOT EXISTS public_knowledge_units_entities_idx
    ON public_knowledge_units USING GIN(entities);
CREATE INDEX IF NOT EXISTS public_knowledge_units_scope_state_idx
    ON public_knowledge_units(scope_state,publication_state,updated_at DESC);

DROP VIEW IF EXISTS published_public_knowledge;
CREATE VIEW published_public_knowledge AS
SELECT
    u.public_knowledge_id,
    u.canonical_topic,
    u.knowledge_type,
    u.domains,
    u.entities,
    u.scope_state,
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
WHERE u.publication_state='published'
  AND u.scope_state='classified'
  AND cardinality(u.domains)>0
  AND r.validation_state='platform_certified';
