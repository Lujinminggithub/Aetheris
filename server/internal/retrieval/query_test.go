package retrieval

import (
	"context"
	"testing"
	"time"
)

type fakeQueryRepository struct {
	stages    []string
	completed QueryJob
	documents []RetrievedDocument
}

func (repository *fakeQueryRepository) CreateQuery(context.Context, QueryJob) error { return nil }
func (repository *fakeQueryRepository) UpdateStage(_ context.Context, _ string, status string, _ int, _ string) error {
	repository.stages = append(repository.stages, status)
	return nil
}
func (repository *fakeQueryRepository) CompleteQuery(_ context.Context, _ string, answer string, citations []Citation) error {
	repository.completed.Answer, repository.completed.Citations = answer, citations
	repository.stages = append(repository.stages, "completed")
	return nil
}
func (repository *fakeQueryRepository) LoadDocuments(context.Context, QueryFilter, []Hit) ([]RetrievedDocument, error) {
	return repository.documents, nil
}
func (repository *fakeQueryRepository) GetQuery(context.Context, string, string, string) (QueryJob, error) {
	return repository.completed, nil
}
func (repository *fakeQueryRepository) Status(context.Context, string, string) (IndexStatus, error) {
	return IndexStatus{}, nil
}

type queryEmbedder struct{}

func (queryEmbedder) Embed(context.Context, []string) ([][]float32, error) {
	return [][]float32{{0.1, 0.2}}, nil
}

type queryVectors struct{}

func (queryVectors) EnsureCollection(context.Context, int) error { return nil }
func (queryVectors) Upsert(context.Context, []Point) error       { return nil }
func (queryVectors) Health(context.Context) error                { return nil }
func (queryVectors) Query(context.Context, []float32, QueryFilter, int) ([]Hit, error) {
	return []Hit{{DocumentID: "doc-allowed", Score: 0.9}, {DocumentID: "doc-denied", Score: 0.8}}, nil
}

type queryGenerator struct{ citations []Citation }

func (generator *queryGenerator) Generate(_ context.Context, _ string, citations []Citation) (string, error) {
	generator.citations = citations
	return "根据证据 [1] 已完成。", nil
}

type structuredGenerator struct {
	answers []GeneratedAnswer
	calls   int
}

func (generator *structuredGenerator) GenerateAnswer(_ context.Context, _ string, _ AnswerMode, _ []Citation, _ string) (GeneratedAnswer, error) {
	answer := generator.answers[generator.calls]
	generator.calls++
	return answer, nil
}

func TestQueryServiceUsesOnlyAuthorizedDocumentsAsCitations(t *testing.T) {
	repository := &fakeQueryRepository{documents: []RetrievedDocument{{DocumentID: "doc-allowed", FactID: "fact-1", CanonicalEventID: "event-1", SourceEventIDs: []string{"event-1"}, ProjectID: "project-1", ProjectName: "Codex", ActivityType: "ai", Excerpt: "安全证据", Score: 0.9, OccurredAt: time.Now()}}}
	generator := &queryGenerator{}
	service := NewQueryService(repository, queryEmbedder{}, queryVectors{}, generator, time.UTC)
	job := QueryJob{ID: "query-1", TenantID: "tenant-1", ActorID: "user-1", Question: "做了什么？", Filters: QueryFilters{From: "2026-09-01", To: "2026-09-07"}}

	if err := service.RunJob(context.Background(), job); err != nil {
		t.Fatal(err)
	}

	wantStages := []string{"embedding", "retrieving", "generating", "completed"}
	if len(repository.stages) != len(wantStages) {
		t.Fatalf("stages = %#v", repository.stages)
	}
	for index := range wantStages {
		if repository.stages[index] != wantStages[index] {
			t.Fatalf("stages = %#v", repository.stages)
		}
	}
	if len(generator.citations) != 1 || generator.citations[0].DocumentID != "doc-allowed" || len(repository.completed.Citations) != 1 {
		t.Fatalf("citations = %#v", generator.citations)
	}
}

func TestValidateQueryInputRejectsInvalidQuestionAndRange(t *testing.T) {
	if ValidateQueryInput(QueryInput{Question: "", Filters: QueryFilters{From: "2026-09-01", To: "2026-09-07"}}, time.UTC) == nil {
		t.Fatal("empty question accepted")
	}
	if ValidateQueryInput(QueryInput{Question: "test", Filters: QueryFilters{From: "2026-01-01", To: "2026-09-07"}}, time.UTC) == nil {
		t.Fatal("oversized range accepted")
	}
	if err := ValidateQueryInput(QueryInput{Question: "应用做了什么", Filters: QueryFilters{From: "2026-09-01", To: "2026-09-07", ActivityType: "application"}}, time.UTC); err != nil {
		t.Fatalf("application activity rejected: %v", err)
	}
}

func TestQueryServiceRetriesInvalidStructuredAnswerAndFallsBackSafely(t *testing.T) {
	repository := &fakeQueryRepository{documents: []RetrievedDocument{{DocumentID: "doc-1", FactID: "fact-1", CanonicalEventID: "event-1", SourceEventIDs: []string{"event-1"}, ProjectID: "project-1", ProjectName: "线路", ActivityType: "ai", Excerpt: "存在限制", Score: 0.9, OccurredAt: time.Now()}}}
	generator := &structuredGenerator{answers: []GeneratedAnswer{
		{Answer: "线路存在限制，Qdrant 向量索引尚未读取日志。", Mode: DirectMode, Confidence: "high", CitationNumbers: []int{1}},
		{Answer: "线路存在限制，Qdrant 向量索引尚未读取日志。", Mode: DirectMode, Confidence: "high", CitationNumbers: []int{1}},
	}}
	service := NewQueryService(repository, queryEmbedder{}, queryVectors{}, generator, time.UTC)
	job := QueryJob{ID: "query-invalid", TenantID: "tenant-1", ActorID: "user-1", Question: "线路有没有流量限制", Filters: QueryFilters{From: "2026-09-01", To: "2026-09-07"}}

	if err := service.RunJob(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	if generator.calls != 2 || repository.completed.Answer != "当前数据不足以确认。" || len(repository.completed.Citations) != 0 {
		t.Fatalf("calls=%d completed=%#v", generator.calls, repository.completed)
	}
}

func TestQueryServiceKeepsOnlyCitationsSelectedByStructuredAnswer(t *testing.T) {
	repository := &fakeQueryRepository{documents: []RetrievedDocument{
		{DocumentID: "doc-1", FactID: "fact-1", CanonicalEventID: "event-1", SourceEventIDs: []string{"event-1"}, ProjectID: "project-1", ProjectName: "线路", ActivityType: "ai", Excerpt: "存在限制", Score: 0.9, OccurredAt: time.Now()},
		{DocumentID: "doc-2", FactID: "fact-2", CanonicalEventID: "event-2", SourceEventIDs: []string{"event-2"}, ProjectID: "project-1", ProjectName: "线路", ActivityType: "ai", Excerpt: "其他证据", Score: 0.8, OccurredAt: time.Now()},
	}}
	generator := &structuredGenerator{answers: []GeneratedAnswer{{Answer: "线路存在流量限制。", Mode: DirectMode, Confidence: "high", CitationNumbers: []int{2}}}}
	service := NewQueryService(repository, queryEmbedder{}, queryVectors{}, generator, time.UTC)
	job := QueryJob{ID: "query-selected", TenantID: "tenant-1", ActorID: "user-1", Question: "线路有没有流量限制", Filters: QueryFilters{From: "2026-09-01", To: "2026-09-07"}}

	if err := service.RunJob(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	if len(repository.completed.Citations) != 1 || repository.completed.Citations[0].Number != 2 {
		t.Fatalf("citations=%#v", repository.completed.Citations)
	}
}
