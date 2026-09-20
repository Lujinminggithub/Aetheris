package retrieval

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

type CompositeKnowledgeSearcher struct {
	private KnowledgeSearcher
	public  KnowledgeSearcher
	mode    string
}

func NewCompositeKnowledgeSearcher(private, public KnowledgeSearcher) *CompositeKnowledgeSearcher {
	return &CompositeKnowledgeSearcher{private: private, public: public, mode: "active"}
}

func (searcher *CompositeKnowledgeSearcher) WithMode(mode string) *CompositeKnowledgeSearcher {
	if mode == "shadow" || mode == "active" {
		searcher.mode = mode
	}
	return searcher
}

type knowledgeSearchResult struct {
	hits []KnowledgeHit
	err  error
}

func (searcher *CompositeKnowledgeSearcher) Search(ctx context.Context, query KnowledgeQuery) ([]KnowledgeHit, error) {
	results := make(chan knowledgeSearchResult, 2)
	count := 0
	for _, source := range []KnowledgeSearcher{searcher.private, searcher.public} {
		if source == nil {
			continue
		}
		count++
		go func(current KnowledgeSearcher) {
			hits, err := current.Search(ctx, query)
			results <- knowledgeSearchResult{hits: hits, err: err}
		}(source)
	}
	if count == 0 {
		return nil, fmt.Errorf("knowledge search unavailable")
	}
	combined := []KnowledgeHit{}
	failures := 0
	for index := 0; index < count; index++ {
		result := <-results
		if result.err != nil {
			failures++
			continue
		}
		combined = append(combined, result.hits...)
	}
	if failures == count {
		return nil, fmt.Errorf("private and public knowledge search unavailable")
	}
	for index := range combined {
		if combined[index].SourceScope == "" {
			combined[index].SourceScope = "tenant_private"
		}
	}
	if searcher.mode == "shadow" {
		privateOnly := combined[:0]
		for _, hit := range combined {
			if hit.SourceScope != "platform_public" {
				privateOnly = append(privateOnly, hit)
			}
		}
		combined = privateOnly
	}
	sort.SliceStable(combined, func(i, j int) bool {
		left, right := knowledgePriority(combined[i]), knowledgePriority(combined[j])
		if left == right {
			if combined[i].Score == combined[j].Score {
				return combined[i].ChunkID < combined[j].ChunkID
			}
			return combined[i].Score > combined[j].Score
		}
		return left > right
	})
	limit := query.Limit
	if limit < 1 {
		limit = 12
	}
	result := make([]KnowledgeHit, 0, limit)
	seenKnowledge, seenContent := map[string]bool{}, map[string]bool{}
	for _, hit := range combined {
		knowledgeKey := hit.SourceScope + ":" + hit.KnowledgeID
		contentKey := strings.ToLower(strings.Join(strings.Fields(hit.Content), " "))
		if seenKnowledge[knowledgeKey] || (contentKey != "" && seenContent[contentKey]) {
			continue
		}
		seenKnowledge[knowledgeKey] = true
		seenContent[contentKey] = true
		result = append(result, hit)
		if len(result) == limit {
			break
		}
	}
	return result, nil
}

func knowledgePriority(hit KnowledgeHit) int {
	if hit.SourceScope == "tenant_private" && hit.ValidationState == "verified" {
		return 400
	}
	if hit.SourceScope == "platform_public" && hit.ValidationState == "platform_certified" {
		return 300
	}
	if hit.SourceScope == "tenant_private" && hit.DecisionState == "accepted" {
		return 200
	}
	if hit.SourceScope == "tenant_private" {
		return 100
	}
	return 0
}
