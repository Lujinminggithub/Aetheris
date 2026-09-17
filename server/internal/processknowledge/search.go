package processknowledge

import (
	"context"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/aetheris-dev/aetheris/server/internal/retrieval"
)

type SearchCandidate struct {
	ChunkID, KnowledgeID, SessionID, LogicalProjectID string
	Topic, KnowledgeType, DecisionState               string
	ValidationState, Content, Applicability           string
	SourceEventIDs                                    []string
	OccurredAt                                        time.Time
	Rank                                              int
	Score                                             float64
}

func FuseCandidates(vector, keyword []SearchCandidate, limit int) []SearchCandidate {
	if limit < 1 {
		limit = 12
	}
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
			existing.Score += 1.0 / float64(60+rank)
			byChunk[item.ChunkID] = existing
		}
	}
	add(vector)
	add(keyword)
	items := make([]SearchCandidate, 0, len(byChunk))
	for _, item := range byChunk {
		switch item.ValidationState {
		case "verified":
			item.Score += 0.08
		case "partially_verified":
			item.Score += 0.03
		case "contradicted":
			item.Score -= 0.20
		}
		if item.DecisionState == "accepted" {
			item.Score += 0.03
		}
		if item.DecisionState == "rejected" {
			item.Score -= 0.10
		}
		items = append(items, item)
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Score == items[j].Score {
			return items[i].ChunkID < items[j].ChunkID
		}
		return items[i].Score > items[j].Score
	})
	result := []SearchCandidate{}
	knowledgeSeen := map[string]bool{}
	sessionCount := map[string]int{}
	for _, item := range items {
		if knowledgeSeen[item.KnowledgeID] || sessionCount[item.SessionID] >= 1 || item.ValidationState == "contradicted" {
			continue
		}
		knowledgeSeen[item.KnowledgeID] = true
		sessionCount[item.SessionID]++
		result = append(result, item)
		if len(result) == limit {
			break
		}
	}
	return result
}

var keywordPattern = regexp.MustCompile(`[\pL\pN_]+`)

func KeywordTerms(question string) []string {
	matches := keywordPattern.FindAllString(strings.ToLower(question), -1)
	result := []string{}
	seen := map[string]bool{}
	for _, item := range matches {
		if len([]rune(item)) < 2 || seen[item] {
			continue
		}
		seen[item] = true
		result = append(result, item)
	}
	return result
}

type SearchRepository interface {
	ActiveVersion(context.Context, string) (int, error)
	LoadVectorCandidates(context.Context, retrieval.KnowledgeQuery, []retrieval.Hit) ([]SearchCandidate, error)
	KeywordCandidates(context.Context, retrieval.KnowledgeQuery, []string, int) ([]SearchCandidate, error)
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
	version, err := searcher.repository.ActiveVersion(ctx, query.TenantID)
	if err != nil {
		return nil, err
	}
	embedded, err := searcher.embedder.Embed(ctx, []string{query.Question})
	if err != nil || len(embedded) != 1 {
		return nil, err
	}
	filter := retrieval.QueryFilter{TenantID: query.TenantID, ProjectID: query.LogicalProjectID, ActivityType: "process_knowledge", From: query.From, ToExclusive: query.ToExclusive, KnowledgeVersion: version}
	hits, err := searcher.vectors.Query(ctx, embedded[0], filter, 40)
	if err != nil {
		return nil, err
	}
	vector, err := searcher.repository.LoadVectorCandidates(ctx, query, hits)
	if err != nil {
		return nil, err
	}
	keyword, err := searcher.repository.KeywordCandidates(ctx, query, KeywordTerms(query.Question), 40)
	if err != nil {
		return nil, err
	}
	fused := FuseCandidates(vector, keyword, query.Limit)
	result := make([]retrieval.KnowledgeHit, len(fused))
	for index, item := range fused {
		result[index] = retrieval.KnowledgeHit{ChunkID: item.ChunkID, KnowledgeID: item.KnowledgeID, SessionID: item.SessionID, LogicalProjectID: item.LogicalProjectID, Topic: item.Topic, KnowledgeType: item.KnowledgeType, DecisionState: item.DecisionState, ValidationState: item.ValidationState, Content: item.Content, Applicability: item.Applicability, SourceEventIDs: item.SourceEventIDs, Score: item.Score, OccurredAt: item.OccurredAt}
	}
	return result, nil
}
