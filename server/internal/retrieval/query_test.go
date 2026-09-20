package retrieval

import (
	"context"
	"errors"
	"strings"
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
func (repository *fakeQueryRepository) CompleteStructuredQuery(_ context.Context, _ string, answer GeneratedAnswer, citations []Citation) error {
	repository.completed.Answer = answer.Answer
	repository.completed.AnswerMode = answer.Mode
	repository.completed.Confidence = answer.Confidence
	repository.completed.Details = answer.Details
	repository.completed.CitationNumbers = answer.CitationNumbers
	repository.completed.Citations = citations
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
func (queryVectors) Delete(context.Context, []string) error      { return nil }
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

type failingStructuredGenerator struct{ calls int }

func (generator *failingStructuredGenerator) GenerateAnswer(context.Context, string, AnswerMode, []Citation, string) (GeneratedAnswer, error) {
	generator.calls++
	return GeneratedAnswer{}, errors.New("model unavailable")
}

type fakeKnowledgeSearcher struct {
	queries []KnowledgeQuery
	hits    []KnowledgeHit
}

func (searcher *fakeKnowledgeSearcher) Search(_ context.Context, query KnowledgeQuery) ([]KnowledgeHit, error) {
	searcher.queries = append(searcher.queries, query)
	return searcher.hits, nil
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
	if err := ValidateQueryInput(QueryInput{Question: "分析 EDR", Filters: QueryFilters{From: "2026-09-01", To: "2026-09-07", KnowledgeScope: "project_process", ScopeMode: "manual"}}, time.UTC); err == nil {
		t.Fatal("process knowledge query without explicit project scope was accepted")
	}
	if err := ValidateQueryInput(QueryInput{Question: "分析 EDR", Filters: QueryFilters{KnowledgeScope: "project_process", ScopeMode: "auto"}}, time.UTC); err != nil {
		t.Fatalf("automatic scope without manual filters rejected: %v", err)
	}
}

func TestQueryServiceAutomaticScopeSearchesFullRetentionWithoutProject(t *testing.T) {
	repository := &fakeQueryRepository{}
	searcher := &fakeKnowledgeSearcher{hits: []KnowledgeHit{{ChunkID: "chunk-1", KnowledgeID: "knowledge-1", LogicalProjectID: "logical-safe", Topic: "DLP", ValidationState: "unverified", Content: "Windows DLP 使用 OCR 和规则检测。", OccurredAt: time.Now()}}}
	generator := &structuredGenerator{answers: []GeneratedAnswer{{Answer: "Windows DLP 采用内容采集、识别、规则检测和阻断闭环。", Mode: AnalysisMode, Confidence: "low", Details: "客户端采集文件、剪贴板与截图内容，文本直接解析，图片经 OCR 后进入规则引擎，再按策略执行记录、告警或阻断，并保留可追溯证据。", CitationNumbers: []int{1}}}}
	service := NewQueryService(repository, queryEmbedder{}, queryVectors{}, generator, time.UTC).WithKnowledge(searcher)
	job := QueryJob{ID: "auto", TenantID: "tenant", ActorID: "user", Question: "如何在 Windows 实现 DLP", Filters: QueryFilters{KnowledgeScope: "project_process", ScopeMode: "auto"}}
	if err := service.RunJob(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	if len(searcher.queries) != 1 || !searcher.queries[0].AutoScope || searcher.queries[0].LogicalProjectID != "" || searcher.queries[0].From.Year() > 1970 {
		t.Fatalf("automatic query=%+v", searcher.queries)
	}
}

func TestQueryServiceFallsBackToUnverifiedKnowledgeInsteadOfEmptyAnswer(t *testing.T) {
	repository := &fakeQueryRepository{}
	searcher := &fakeKnowledgeSearcher{hits: []KnowledgeHit{{ChunkID: "chunk-1", KnowledgeID: "knowledge-1", LogicalProjectID: "logical-safe", Topic: "DLP", ValidationState: "unverified", Content: "结论：Windows DLP 将文本解析和 OCR 结果送入规则引擎，并按策略记录或阻断。", Applicability: "Windows 终端"}}}
	generator := &failingStructuredGenerator{}
	service := NewQueryService(repository, queryEmbedder{}, queryVectors{}, generator, time.UTC).WithKnowledge(searcher)
	job := QueryJob{ID: "fallback-unverified", TenantID: "tenant", ActorID: "user", Question: "详细分析 Windows DLP", Filters: QueryFilters{KnowledgeScope: "project_process", ScopeMode: "auto"}}
	if err := service.RunJob(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	if repository.completed.Confidence != "low" || len(repository.completed.Citations) != 1 || !strings.Contains(repository.completed.Details, "OCR") {
		t.Fatalf("completed=%+v", repository.completed)
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

func TestQueryServiceUsesProcessKnowledgeWithLogicalProject(t *testing.T) {
	repository := &fakeQueryRepository{}
	searcher := &fakeKnowledgeSearcher{hits: []KnowledgeHit{{
		ChunkID: "chunk-1", KnowledgeID: "knowledge-1", SessionID: "session-safe", LogicalProjectID: "logical-safe",
		Topic: "Windows EDR", KnowledgeType: "implementation_pattern", DecisionState: "accepted", ValidationState: "verified",
		Content: "内核采集保持轻量，复杂分析放在用户态。", Applicability: "Windows 高 IRQL 路径", SourceEventIDs: []string{"event-answer"}, Score: 0.9, OccurredAt: time.Now(),
	}}}
	generator := &structuredGenerator{answers: []GeneratedAnswer{{Answer: "Windows EDR 采用内核采集与用户态分析协作。", Mode: AnalysisMode, Confidence: "high", Details: "内核热路径只采集进程、文件、注册表和网络事件，用户态代理完成规则关联、进程树分析、告警聚合与响应执行，并通过管理端下发策略和保留可追溯证据。", CitationNumbers: []int{1}}}}
	service := NewQueryService(repository, queryEmbedder{}, queryVectors{}, generator, time.UTC).WithKnowledge(searcher)
	job := QueryJob{ID: "query-knowledge", TenantID: "tenant-1", ActorID: "user-1", Question: "详细分析 Windows 如何实现 EDR", Filters: QueryFilters{From: "2026-09-01", To: "2026-09-17", LogicalProjectID: "logical-safe", KnowledgeScope: "project_process"}}
	if err := service.RunJob(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	if len(searcher.queries) != 1 || searcher.queries[0].LogicalProjectID != "logical-safe" {
		t.Fatalf("queries=%+v", searcher.queries)
	}
	if len(repository.completed.Citations) != 1 || repository.completed.Citations[0].KnowledgeID != "knowledge-1" || repository.completed.Citations[0].ValidationState != "verified" {
		t.Fatalf("citations=%+v", repository.completed.Citations)
	}
}

func TestQueryServiceFallsBackToVerifiedKnowledgeWhenModelFails(t *testing.T) {
	repository := &fakeQueryRepository{}
	searcher := &fakeKnowledgeSearcher{hits: []KnowledgeHit{{ChunkID: "chunk-1", KnowledgeID: "knowledge-1", SessionID: "session", LogicalProjectID: "logical-safe", Topic: "Windows EDR", DecisionState: "accepted", ValidationState: "verified", Content: "内核采集保持轻量，复杂检测放在用户态。", Applicability: "Windows 高 IRQL 路径", SourceEventIDs: []string{"event-1"}}}}
	generator := &failingStructuredGenerator{}
	service := NewQueryService(repository, queryEmbedder{}, queryVectors{}, generator, time.UTC).WithKnowledge(searcher)
	job := QueryJob{ID: "fallback", TenantID: "tenant", ActorID: "user", Question: "详细分析 EDR", Filters: QueryFilters{From: "2026-09-01", To: "2026-09-17", LogicalProjectID: "logical-safe", KnowledgeScope: "project_process"}}
	if err := service.RunJob(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(repository.completed.Answer, "基于已验证过程知识") || len(repository.completed.Citations) != 1 || generator.calls != 2 {
		t.Fatalf("answer=%q citations=%+v calls=%d", repository.completed.Answer, repository.completed.Citations, generator.calls)
	}
}
