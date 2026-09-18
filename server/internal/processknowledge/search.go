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
		if strings.Contains(item.Content, "结论：") {
			item.Score += 0.012
		}
		contentLength := len([]rune(strings.TrimSpace(item.Content)))
		if contentLength < 80 {
			item.Score -= 0.012
		} else if contentLength > 300 {
			item.Score += 0.002
		}
		switch item.ValidationState {
		case "verified":
			item.Score += 0.008
		case "partially_verified":
			item.Score += 0.003
		case "contradicted":
			item.Score -= 0.20
		}
		if item.DecisionState == "accepted" {
			item.Score += 0.003
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
	contentSeen := map[string]bool{}
	sessionCount := map[string]int{}
	for _, item := range items {
		contentKey := strings.ToLower(strings.Join(strings.Fields(item.Content), " "))
		if knowledgeSeen[item.KnowledgeID] || (contentKey != "" && contentSeen[contentKey]) || sessionCount[item.SessionID] >= 3 || item.ValidationState == "contradicted" {
			continue
		}
		knowledgeSeen[item.KnowledgeID] = true
		contentSeen[contentKey] = true
		sessionCount[item.SessionID]++
		result = append(result, item)
		if len(result) == limit {
			break
		}
	}
	return result
}

var asciiKeywordPattern = regexp.MustCompile(`[a-z][a-z0-9_]*`)
var cjkKeywordPattern = regexp.MustCompile(`[\p{Han}]+`)
var weightedDomainTerms = map[string]bool{"dlp": true, "edr": true, "ocr": true, "wfp": true, "vpn": true, "hips": true, "etw": true, "yara": true, "sigma": true}

func KeywordTerms(question string) []string {
	normalized := strings.ToLower(question)
	matches := append(asciiKeywordPattern.FindAllString(normalized, -1), cjkKeywordPattern.FindAllString(normalized, -1)...)
	result := []string{}
	seen := map[string]bool{}
	for _, item := range matches {
		if len([]rune(item)) < 2 || seen[item] {
			continue
		}
		seen[item] = true
		result = append(result, item)
		if weightedDomainTerms[item] {
			result = append(result, item, item, item)
		}
	}
	return result
}

func FilterDomainCandidates(candidates []SearchCandidate, terms []string, limit int) []SearchCandidate {
	domains := map[string]bool{}
	for _, term := range terms {
		if weightedDomainTerms[term] {
			domains[term] = true
		}
	}
	if len(domains) == 0 {
		return limitCandidates(candidates, limit)
	}
	topicMatches := make([]SearchCandidate, 0, len(candidates))
	contentMatches := make([]SearchCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		topic := strings.ToLower(candidate.Topic)
		content := strings.ToLower(candidate.Content)
		for domain := range domains {
			if strings.Contains(topic, domain) {
				topicMatches = append(topicMatches, candidate)
				break
			}
			if strings.Contains(content, domain) {
				contentMatches = append(contentMatches, candidate)
				break
			}
		}
	}
	if len(topicMatches) > 0 {
		return limitCandidates(topicMatches, limit)
	}
	if len(contentMatches) > 0 {
		return limitCandidates(contentMatches, limit)
	}
	return limitCandidates(candidates, limit)
}

func limitCandidates(candidates []SearchCandidate, limit int) []SearchCandidate {
	if limit > 0 && len(candidates) > limit {
		return candidates[:limit]
	}
	return candidates
}

func SelectAutomaticProject(candidates []SearchCandidate) string {
	scores := map[string]float64{}
	for index, candidate := range candidates {
		if candidate.LogicalProjectID == "" {
			continue
		}
		rank := candidate.Rank
		if rank < 1 {
			rank = index + 1
		}
		scores[candidate.LogicalProjectID] += 1.0 / float64(10+rank)
	}
	selected := ""
	best := 0.0
	for projectID, score := range scores {
		if score > best || (score == best && (selected == "" || projectID < selected)) {
			selected, best = projectID, score
		}
	}
	return selected
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
	terms := KeywordTerms(query.Question)
	if query.AutoScope && query.LogicalProjectID == "" {
		globalKeyword, keywordErr := searcher.repository.KeywordCandidates(ctx, query, terms, 80)
		if keywordErr != nil {
			return nil, keywordErr
		}
		query.LogicalProjectID = SelectAutomaticProject(globalKeyword)
		if query.LogicalProjectID == "" {
			globalFilter := retrieval.QueryFilter{TenantID: query.TenantID, ActivityType: "process_knowledge", From: query.From, ToExclusive: query.ToExclusive, KnowledgeVersion: version}
			globalHits, vectorErr := searcher.vectors.Query(ctx, embedded[0], globalFilter, 40)
			if vectorErr != nil {
				return nil, vectorErr
			}
			globalVector, loadErr := searcher.repository.LoadVectorCandidates(ctx, query, globalHits)
			if loadErr != nil {
				return nil, loadErr
			}
			query.LogicalProjectID = SelectAutomaticProject(globalVector)
		}
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
	keyword, err := searcher.repository.KeywordCandidates(ctx, query, terms, 40)
	if err != nil {
		return nil, err
	}
	fused := FuseCandidates(vector, keyword, query.Limit*3)
	fused = FilterDomainCandidates(fused, terms, query.Limit)
	result := make([]retrieval.KnowledgeHit, len(fused))
	for index, item := range fused {
		result[index] = retrieval.KnowledgeHit{ChunkID: item.ChunkID, KnowledgeID: item.KnowledgeID, SessionID: item.SessionID, LogicalProjectID: item.LogicalProjectID, Topic: item.Topic, KnowledgeType: item.KnowledgeType, DecisionState: item.DecisionState, ValidationState: item.ValidationState, Content: item.Content, Applicability: item.Applicability, SourceEventIDs: item.SourceEventIDs, Score: item.Score, OccurredAt: item.OccurredAt}
	}
	return result, nil
}
