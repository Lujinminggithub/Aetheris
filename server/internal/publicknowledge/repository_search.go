package publicknowledge

import (
	"context"
	"strings"

	"github.com/aetheris-dev/aetheris/server/internal/retrieval"
)

func (repository *Repository) LoadPublicVectorCandidates(ctx context.Context, hits []retrieval.Hit) ([]SearchCandidate, error) {
	if len(hits) == 0 {
		return nil, nil
	}
	ids := make([]string, len(hits))
	byID := make(map[string]retrieval.Hit, len(hits))
	for index, hit := range hits {
		ids[index], byID[hit.DocumentID] = hit.DocumentID, hit
	}
	rows, err := repository.pool.Query(ctx, `SELECT c.chunk_id,c.public_knowledge_id,c.canonical_topic,c.knowledge_type,c.domains,c.entities,c.validation_state,c.content,r.applicability,c.revision,r.anonymous_source_tenant_count
        FROM public_knowledge_chunks c
        JOIN public_knowledge_units u ON u.public_knowledge_id=c.public_knowledge_id AND u.current_revision=c.revision
        JOIN public_knowledge_revisions r ON r.public_knowledge_id=c.public_knowledge_id AND r.revision=c.revision
		WHERE c.chunk_id=ANY($1) AND c.index_status='indexed' AND u.publication_state='published' AND u.scope_state='classified' AND cardinality(u.domains)>0 AND r.validation_state='platform_certified'`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]SearchCandidate, 0, len(hits))
	for rows.Next() {
		var item SearchCandidate
		if err = rows.Scan(&item.ChunkID, &item.PublicKnowledgeID, &item.CanonicalTopic, &item.KnowledgeType, &item.Domains, &item.Entities, &item.ValidationState, &item.Content, &item.Applicability, &item.Revision, &item.AnonymousSourceTenantCount); err != nil {
			return nil, err
		}
		for rank, id := range ids {
			if id == item.ChunkID {
				item.Rank = rank + 1
				item.Score = byID[id].Score
				break
			}
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (repository *Repository) PublicKeywordCandidates(ctx context.Context, terms []string, limit int) ([]SearchCandidate, error) {
	if len(terms) == 0 {
		return nil, nil
	}
	question := strings.Join(terms, " ")
	rows, err := repository.pool.Query(ctx, `SELECT c.chunk_id,c.public_knowledge_id,c.canonical_topic,c.knowledge_type,c.domains,c.entities,c.validation_state,c.content,r.applicability,c.revision,r.anonymous_source_tenant_count,
        similarity(c.search_text,$2) score,
        COALESCE((SELECT SUM((length(lower(c.search_text))-length(replace(lower(c.search_text),lower(term),'')))/GREATEST(length(term),1)) FROM unnest($1::text[]) term WHERE term<>''),0) exact_hits
        FROM public_knowledge_chunks c
        JOIN public_knowledge_units u ON u.public_knowledge_id=c.public_knowledge_id AND u.current_revision=c.revision
        JOIN public_knowledge_revisions r ON r.public_knowledge_id=c.public_knowledge_id AND r.revision=c.revision
		WHERE c.index_status='indexed' AND u.publication_state='published' AND u.scope_state='classified' AND cardinality(u.domains)>0 AND r.validation_state='platform_certified'
          AND (EXISTS (SELECT 1 FROM unnest($1::text[]) term WHERE term<>'' AND strpos(lower(c.search_text),lower(term))>0) OR c.search_text % $2)
        ORDER BY exact_hits DESC,score DESC,c.chunk_id LIMIT $3`, terms, question, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]SearchCandidate, 0)
	for rows.Next() {
		var item SearchCandidate
		var exactHits int
		if err = rows.Scan(&item.ChunkID, &item.PublicKnowledgeID, &item.CanonicalTopic, &item.KnowledgeType, &item.Domains, &item.Entities, &item.ValidationState, &item.Content, &item.Applicability, &item.Revision, &item.AnonymousSourceTenantCount, &item.Score, &exactHits); err != nil {
			return nil, err
		}
		item.Rank = len(items) + 1
		items = append(items, item)
	}
	return items, rows.Err()
}
