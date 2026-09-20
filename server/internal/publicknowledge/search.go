package publicknowledge

import (
	"context"
	"regexp"
	"sort"
	"strings"

	"github.com/aetheris-dev/aetheris/server/internal/retrieval"
)

type SearchCandidate struct {
	ChunkID, PublicKnowledgeID, CanonicalTopic, KnowledgeType string
	ValidationState, Content, Applicability                   string
	Revision, AnonymousSourceTenantCount, Rank                int
	Score                                                     float64
}

type SearchRepository interface {
	LoadPublicVectorCandidates(context.Context, []retrieval.Hit) ([]SearchCandidate, error)
	PublicKeywordCandidates(context.Context, []string, int) ([]SearchCandidate, error)
}

type Searcher struct {
	repository SearchRepository
	embedder   retrieval.EmbeddingClient
	vectors    retrieval.VectorIndex
}

func NewSearcher(repository SearchRepository, embedder retrieval.EmbeddingClient, vectors retrieval.VectorIndex) *Searcher {
	return &Searcher{repository: repository, embedder: embedder, vectors: vectors}
}

func (searcher *Searcher) Search(ctx context.Context, query retrieval.KnowledgeQuery) ([]retrieval.KnowledgeHit, error) {
	if query.Limit < 1 {
		query.Limit = 12
	}
	embedded, err := searcher.embedder.Embed(ctx, []string{query.Question})
	if err != nil || len(embedded) != 1 {
		return nil, err
	}
	hits, err := searcher.vectors.Query(ctx, embedded[0], retrieval.QueryFilter{Public: true, ActivityType: "public_knowledge"}, query.Limit*4)
	if err != nil {
		return nil, err
	}
	vector, err := searcher.repository.LoadPublicVectorCandidates(ctx, hits)
	if err != nil {
		return nil, err
	}
	keyword, err := searcher.repository.PublicKeywordCandidates(ctx, publicKeywordTerms(query.Question), query.Limit*4)
	if err != nil {
		return nil, err
	}
	candidates := fusePublicCandidates(vector, keyword, query.Limit)
	result := make([]retrieval.KnowledgeHit, len(candidates))
	for index, item := range candidates {
		result[index] = retrieval.KnowledgeHit{
			ChunkID: item.ChunkID, KnowledgeID: item.PublicKnowledgeID, PublicKnowledgeID: item.PublicKnowledgeID,
			SourceScope: "platform_public", Topic: item.CanonicalTopic, KnowledgeType: item.KnowledgeType,
			DecisionState: "accepted", ValidationState: item.ValidationState, Content: item.Content,
			Applicability: item.Applicability, Revision: item.Revision,
			AnonymousSourceTenantCount: item.AnonymousSourceTenantCount, Score: item.Score,
		}
	}
	return result, nil
}

func fusePublicCandidates(vector, keyword []SearchCandidate, limit int) []SearchCandidate {
	byChunk := map[string]SearchCandidate{}
	add := func(items []SearchCandidate) {
		for index, item := range items {
			rank := item.Rank
			if rank < 1 {
				rank = index + 1
			}
			existing := byChunk[item.ChunkID]
			if existing.ChunkID == "" {
				existing = item
			}
			existing.Score += 1 / float64(60+rank)
			byChunk[item.ChunkID] = existing
		}
	}
	add(vector)
	add(keyword)
	items := make([]SearchCandidate, 0, len(byChunk))
	for _, item := range byChunk {
		if item.ValidationState == string(PlatformCertified) {
			item.Score += 0.01
			items = append(items, item)
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Score == items[j].Score {
			return items[i].ChunkID < items[j].ChunkID
		}
		return items[i].Score > items[j].Score
	})
	result := make([]SearchCandidate, 0, limit)
	seen := map[string]bool{}
	for _, item := range items {
		if seen[item.PublicKnowledgeID] {
			continue
		}
		seen[item.PublicKnowledgeID] = true
		result = append(result, item)
		if len(result) == limit {
			break
		}
	}
	return result
}

var publicASCIIKeyword = regexp.MustCompile(`[a-z][a-z0-9_]*`)
var publicCJKKeyword = regexp.MustCompile(`[\p{Han}]+`)

func publicKeywordTerms(question string) []string {
	value := strings.ToLower(question)
	matches := append(publicASCIIKeyword.FindAllString(value, -1), publicCJKKeyword.FindAllString(value, -1)...)
	result, seen := []string{}, map[string]bool{}
	for _, match := range matches {
		if len([]rune(match)) >= 2 && !seen[match] {
			seen[match] = true
			result = append(result, match)
		}
	}
	return result
}
