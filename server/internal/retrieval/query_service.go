package retrieval

import (
	"context"
	"fmt"
	"time"

	"github.com/aetheris-dev/aetheris/server/internal/auth"
)

type QueryFilters struct {
	From         string `json:"from"`
	To           string `json:"to"`
	DeviceID     string `json:"device_id,omitempty"`
	ProjectID    string `json:"project_id,omitempty"`
	ActivityType string `json:"activity_type,omitempty"`
}
type QueryInput struct {
	Question string       `json:"question"`
	Filters  QueryFilters `json:"filters"`
}
type Citation struct {
	Number           int       `json:"number"`
	DocumentID       string    `json:"document_id"`
	FactID           string    `json:"fact_id"`
	CanonicalEventID string    `json:"canonical_event_id"`
	SourceEventIDs   []string  `json:"source_event_ids"`
	ProjectID        string    `json:"project_id"`
	ProjectName      string    `json:"project_name"`
	ActivityType     string    `json:"activity_type"`
	Excerpt          string    `json:"excerpt"`
	Score            float64   `json:"score"`
	OccurredAt       time.Time `json:"occurred_at"`
}
type RetrievedDocument struct {
	DocumentID, FactID, CanonicalEventID, DeviceID, ProjectID, ProjectName, ActivityType, Excerpt string
	SourceEventIDs                                                                                []string
	Score                                                                                         float64
	OccurredAt                                                                                    time.Time
}
type QueryJob struct {
	ID              string       `json:"query_id"`
	TenantID        string       `json:"-"`
	ActorID         string       `json:"-"`
	Question        string       `json:"question"`
	Filters         QueryFilters `json:"filters"`
	Status          string       `json:"status"`
	Progress        int          `json:"progress"`
	Answer          string       `json:"answer"`
	AnswerMode      AnswerMode   `json:"answer_mode"`
	Confidence      string       `json:"confidence"`
	Details         string       `json:"details"`
	CitationNumbers []int        `json:"citation_numbers"`
	Citations       []Citation   `json:"citations"`
	ErrorCode       string       `json:"error_code,omitempty"`
	CreatedAt       time.Time    `json:"created_at"`
	CompletedAt     *time.Time   `json:"completed_at,omitempty"`
}

type QueryRepository interface {
	CreateQuery(context.Context, QueryJob) error
	UpdateStage(context.Context, string, string, int, string) error
	CompleteQuery(context.Context, string, string, []Citation) error
	LoadDocuments(context.Context, QueryFilter, []Hit) ([]RetrievedDocument, error)
	GetQuery(context.Context, string, string, string) (QueryJob, error)
	Status(context.Context, string, string) (IndexStatus, error)
}

type StructuredQueryRepository interface {
	CompleteStructuredQuery(context.Context, string, GeneratedAnswer, []Citation) error
}
type AnswerGenerator interface {
	Generate(context.Context, string, []Citation) (string, error)
}

type StructuredAnswerGenerator interface {
	GenerateAnswer(context.Context, string, AnswerMode, []Citation, string) (GeneratedAnswer, error)
}

type QueryService struct {
	repository QueryRepository
	embedder   EmbeddingClient
	vectors    VectorIndex
	generator  any
	location   *time.Location
	model      string
	gate       *WorkloadGate
}

func (service *QueryService) WithGate(gate *WorkloadGate) *QueryService {
	service.gate = gate
	return service
}

func NewQueryService(repository QueryRepository, embedder EmbeddingClient, vectors VectorIndex, generator any, location *time.Location, model ...string) *QueryService {
	selected := "embeddinggemma"
	if len(model) > 0 && model[0] != "" {
		selected = model[0]
	}
	return &QueryService{repository: repository, embedder: embedder, vectors: vectors, generator: generator, location: location, model: selected}
}

func ValidateQueryInput(input QueryInput, location *time.Location) error {
	if len([]rune(input.Question)) < 2 || len([]rune(input.Question)) > 1000 {
		return fmt.Errorf("问题长度必须为 2 到 1000 字")
	}
	from, err1 := time.ParseInLocation("2006-01-02", input.Filters.From, location)
	to, err2 := time.ParseInLocation("2006-01-02", input.Filters.To, location)
	if err1 != nil || err2 != nil || to.Before(from) || int(to.Sub(from).Hours()/24)+1 > 90 {
		return fmt.Errorf("日期范围必须为 1 到 90 天")
	}
	switch input.Filters.ActivityType {
	case "", "ai", "terminal", "ide", "delivery", "application", "browser", "version_control", "other":
	default:
		return fmt.Errorf("活动类型无效")
	}
	return nil
}

func (service *QueryService) Create(ctx context.Context, tenantID, actorID string, input QueryInput) (QueryJob, error) {
	if err := ValidateQueryInput(input, service.location); err != nil {
		return QueryJob{}, err
	}
	token, err := auth.RandomToken(12)
	if err != nil {
		return QueryJob{}, err
	}
	job := QueryJob{ID: "rag-" + token, TenantID: tenantID, ActorID: actorID, Question: input.Question, Filters: input.Filters, Status: "queued", Progress: 0, Citations: []Citation{}, CreatedAt: time.Now().UTC()}
	if err := service.repository.CreateQuery(ctx, job); err != nil {
		return QueryJob{}, err
	}
	go func() { _ = service.RunJob(context.Background(), job) }()
	return job, nil
}

func (service *QueryService) Get(ctx context.Context, tenantID, actorID, id string) (QueryJob, error) {
	return service.repository.GetQuery(ctx, tenantID, actorID, id)
}

type RetrievalStatus struct {
	Documents      int        `json:"documents"`
	Indexed        int        `json:"indexed"`
	Pending        int        `json:"pending"`
	Failed         int        `json:"failed"`
	EmbeddingModel string     `json:"embedding_model"`
	VectorStatus   string     `json:"vector_status"`
	LastIndexedAt  *time.Time `json:"last_indexed_at,omitempty"`
}

func (service *QueryService) Status(ctx context.Context, tenantID string) (RetrievalStatus, error) {
	state, err := service.repository.Status(ctx, tenantID, service.model)
	if err != nil {
		return RetrievalStatus{}, err
	}
	vectorStatus := "ready"
	if err := service.vectors.Health(ctx); err != nil {
		vectorStatus = "unavailable"
	}
	return RetrievalStatus{Documents: state.Documents, Indexed: state.Indexed, Pending: state.Pending, Failed: state.Failed, EmbeddingModel: service.model, VectorStatus: vectorStatus, LastIndexedAt: state.LastIndexedAt}, nil
}

func (service *QueryService) RunJob(ctx context.Context, job QueryJob) error {
	if service.gate != nil {
		service.gate.BeginQuery()
		defer service.gate.EndQuery()
	}
	fail := func(code string, err error) error {
		_ = service.repository.UpdateStage(ctx, job.ID, "failed", 100, code)
		return err
	}
	if err := service.repository.UpdateStage(ctx, job.ID, "embedding", 15, ""); err != nil {
		return err
	}
	vectors, err := service.embedder.Embed(ctx, []string{job.Question})
	if err != nil || len(vectors) != 1 || len(vectors[0]) == 0 {
		if err == nil {
			err = fmt.Errorf("invalid query embedding")
		}
		return fail("query_embedding_failed", err)
	}
	if err := service.repository.UpdateStage(ctx, job.ID, "retrieving", 40, ""); err != nil {
		return err
	}
	from, _ := time.ParseInLocation("2006-01-02", job.Filters.From, service.location)
	to, _ := time.ParseInLocation("2006-01-02", job.Filters.To, service.location)
	filter := QueryFilter{TenantID: job.TenantID, DeviceID: job.Filters.DeviceID, ProjectID: job.Filters.ProjectID, ActivityType: job.Filters.ActivityType, From: from, ToExclusive: to.AddDate(0, 0, 1)}
	hits, err := service.vectors.Query(ctx, vectors[0], filter, 12)
	if err != nil {
		return fail("vector_query_failed", err)
	}
	documents, err := service.repository.LoadDocuments(ctx, filter, hits)
	if err != nil {
		return fail("document_scope_failed", err)
	}
	if len(documents) > 6 {
		documents = documents[:6]
	}
	citations := make([]Citation, len(documents))
	for index, document := range documents {
		citations[index] = Citation{Number: index + 1, DocumentID: document.DocumentID, FactID: document.FactID, CanonicalEventID: document.CanonicalEventID, SourceEventIDs: document.SourceEventIDs, ProjectID: document.ProjectID, ProjectName: document.ProjectName, ActivityType: document.ActivityType, Excerpt: document.Excerpt, Score: document.Score, OccurredAt: document.OccurredAt}
	}
	if err := service.repository.UpdateStage(ctx, job.ID, "generating", 70, ""); err != nil {
		return err
	}
	generated := GeneratedAnswer{Answer: "当前范围没有检索到可引用的活动事实。", Mode: ClassifyAnswerMode(job.Question), Confidence: "low", Details: "", CitationNumbers: []int{}}
	selectedCitations := citations
	if len(citations) > 0 {
		generated = service.generateAnswer(ctx, job.Question, citations)
		selectedCitations = selectCitations(generated.CitationNumbers, citations)
	}
	if structured, ok := service.repository.(StructuredQueryRepository); ok {
		return structured.CompleteStructuredQuery(ctx, job.ID, generated, selectedCitations)
	}
	return service.repository.CompleteQuery(ctx, job.ID, generated.Answer, selectedCitations)
}

func (service *QueryService) generateAnswer(ctx context.Context, question string, citations []Citation) GeneratedAnswer {
	mode := ClassifyAnswerMode(question)
	correction := ""
	for attempt := 0; attempt < 2; attempt++ {
		var generated GeneratedAnswer
		var err error
		if structured, ok := service.generator.(StructuredAnswerGenerator); ok {
			generated, err = structured.GenerateAnswer(ctx, question, mode, citations, correction)
		} else if legacy, ok := service.generator.(AnswerGenerator); ok {
			var raw string
			raw, err = legacy.Generate(ctx, question, citations)
			generated = GeneratedAnswer{Answer: raw, Mode: mode, Confidence: "medium", CitationNumbers: citationNumbers(citations)}
		} else {
			err = fmt.Errorf("回答生成器不可用")
		}
		if err == nil && ValidateGeneratedAnswer(generated, citations) == nil {
			selected := selectCitations(generated.CitationNumbers, citations)
			if len(selected) == 0 && len(citations) > 0 && generated.Mode != AnalysisMode {
				generated.CitationNumbers = citationNumbers(citations[:1])
			}
			return generated
		}
		correction = "answer_validation_failed"
	}
	return GeneratedAnswer{Answer: "当前数据不足以确认。", Mode: mode, Confidence: "low", Details: "", CitationNumbers: []int{}}
}

func citationNumbers(citations []Citation) []int {
	result := make([]int, len(citations))
	for index, citation := range citations {
		result[index] = citation.Number
	}
	return result
}

func selectCitations(numbers []int, citations []Citation) []Citation {
	selected := make([]Citation, 0, len(numbers))
	byNumber := map[int]Citation{}
	for _, citation := range citations {
		byNumber[citation.Number] = citation
	}
	for _, number := range numbers {
		if citation, ok := byNumber[number]; ok {
			selected = append(selected, citation)
		}
	}
	return selected
}
