package retrieval

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aetheris-dev/aetheris/server/internal/auth"
)

type QueryFilters struct {
	From             string `json:"from,omitempty"`
	To               string `json:"to,omitempty"`
	DeviceID         string `json:"device_id,omitempty"`
	ProjectID        string `json:"project_id,omitempty"`
	LogicalProjectID string `json:"logical_project_id,omitempty"`
	KnowledgeScope   string `json:"knowledge_scope,omitempty"`
	ScopeMode        string `json:"scope_mode,omitempty"`
	AllProjects      bool   `json:"all_projects,omitempty"`
	ActivityType     string `json:"activity_type,omitempty"`
}
type QueryInput struct {
	Question       string       `json:"question"`
	KnowledgeScope string       `json:"knowledge_scope,omitempty"`
	Filters        QueryFilters `json:"filters"`
}
type Citation struct {
	Number                     int       `json:"number"`
	DocumentID                 string    `json:"document_id"`
	FactID                     string    `json:"fact_id"`
	CanonicalEventID           string    `json:"canonical_event_id"`
	SourceEventIDs             []string  `json:"source_event_ids"`
	ProjectID                  string    `json:"project_id"`
	ProjectName                string    `json:"project_name"`
	ActivityType               string    `json:"activity_type"`
	Excerpt                    string    `json:"excerpt"`
	Score                      float64   `json:"score"`
	OccurredAt                 time.Time `json:"occurred_at"`
	KnowledgeID                string    `json:"knowledge_id,omitempty"`
	ChunkID                    string    `json:"chunk_id,omitempty"`
	SourceKind                 string    `json:"source_kind,omitempty"`
	DecisionState              string    `json:"decision_state,omitempty"`
	ValidationState            string    `json:"validation_state,omitempty"`
	Applicability              string    `json:"applicability,omitempty"`
	Topic                      string    `json:"topic,omitempty"`
	SourceScope                string    `json:"source_scope,omitempty"`
	PublicKnowledgeID          string    `json:"public_knowledge_id,omitempty"`
	Revision                   int       `json:"revision,omitempty"`
	AnonymousSourceTenantCount int       `json:"anonymous_source_tenant_count,omitempty"`
	Domains                    []string  `json:"domains,omitempty"`
	Entities                   []string  `json:"entities,omitempty"`
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
	knowledge  KnowledgeSearcher
}

func (service *QueryService) WithKnowledge(searcher KnowledgeSearcher) *QueryService {
	service.knowledge = searcher
	return service
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
	if input.Filters.From != "" || input.Filters.To != "" {
		from, err1 := time.ParseInLocation("2006-01-02", input.Filters.From, location)
		to, err2 := time.ParseInLocation("2006-01-02", input.Filters.To, location)
		if err1 != nil || err2 != nil || to.Before(from) || int(to.Sub(from).Hours()/24)+1 > 90 {
			return fmt.Errorf("日期范围必须同时填写且为 1 到 90 天")
		}
	}
	switch input.Filters.ActivityType {
	case "", "ai", "terminal", "ide", "delivery", "application", "browser", "version_control", "other":
	default:
		return fmt.Errorf("活动类型无效")
	}
	scope := input.KnowledgeScope
	if scope == "" {
		scope = input.Filters.KnowledgeScope
	}
	if scope != "" && scope != "project_process" && scope != "tenant_and_public" {
		return fmt.Errorf("知识范围无效")
	}
	scopeMode := input.Filters.ScopeMode
	if scopeMode == "" {
		scopeMode = "manual"
	}
	if scopeMode != "auto" && scopeMode != "manual" {
		return fmt.Errorf("检索范围模式无效")
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
	if input.KnowledgeScope != "" {
		input.Filters.KnowledgeScope = input.KnowledgeScope
	}
	if input.Filters.KnowledgeScope == "" {
		input.Filters.KnowledgeScope = "tenant_and_public"
	}
	input.Filters.LogicalProjectID = ""
	input.Filters.ProjectID = ""
	input.Filters.AllProjects = true
	input.Filters.ScopeMode = ""
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
	if err := service.repository.UpdateStage(ctx, job.ID, "retrieving", 40, ""); err != nil {
		return err
	}
	from, to := service.queryRange(job.Filters)
	citations := []Citation{}
	knowledgeScope := job.Filters.KnowledgeScope
	if knowledgeScope == "" && service.knowledge != nil {
		knowledgeScope = "tenant_and_public"
	}
	if (knowledgeScope == "project_process" || knowledgeScope == "tenant_and_public") && service.knowledge != nil {
		knowledgeHits, err := service.knowledge.Search(ctx, KnowledgeQuery{TenantID: job.TenantID, Question: job.Question, From: from, ToExclusive: to.AddDate(0, 0, 1), Limit: 12})
		if err != nil {
			return fail("knowledge_query_failed", err)
		}
		for index, hit := range knowledgeHits {
			canonical := ""
			if len(hit.SourceEventIDs) > 0 {
				canonical = hit.SourceEventIDs[0]
			}
			activityType := "process_knowledge"
			if hit.SourceScope == "platform_public" {
				activityType = "public_knowledge"
				canonical = ""
				hit.SourceEventIDs = nil
			}
			scope := hit.SourceScope
			if scope == "" {
				scope = "tenant_private"
			}
			citations = append(citations, Citation{Number: index + 1, DocumentID: hit.ChunkID, FactID: hit.KnowledgeID, CanonicalEventID: canonical, SourceEventIDs: hit.SourceEventIDs, ProjectID: hit.LogicalProjectID, ProjectName: hit.LogicalProjectID, ActivityType: activityType, Excerpt: hit.Content, Score: hit.Score, OccurredAt: hit.OccurredAt, KnowledgeID: hit.KnowledgeID, ChunkID: hit.ChunkID, SourceKind: scope, SourceScope: scope, PublicKnowledgeID: hit.PublicKnowledgeID, Revision: hit.Revision, AnonymousSourceTenantCount: hit.AnonymousSourceTenantCount, DecisionState: hit.DecisionState, ValidationState: hit.ValidationState, Applicability: hit.Applicability, Topic: hit.Topic, Domains: hit.Domains, Entities: hit.Entities})
		}
	} else {
		vectors, err := service.embedder.Embed(ctx, []string{job.Question})
		if err != nil || len(vectors) != 1 || len(vectors[0]) == 0 {
			if err == nil {
				err = fmt.Errorf("invalid query embedding")
			}
			return fail("query_embedding_failed", err)
		}
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
		for index, document := range documents {
			citations = append(citations, Citation{Number: index + 1, DocumentID: document.DocumentID, FactID: document.FactID, CanonicalEventID: document.CanonicalEventID, SourceEventIDs: document.SourceEventIDs, ProjectID: document.ProjectID, ProjectName: document.ProjectName, ActivityType: document.ActivityType, Excerpt: document.Excerpt, Score: document.Score, OccurredAt: document.OccurredAt, SourceKind: "activity_fact"})
		}
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

func (service *QueryService) queryRange(filters QueryFilters) (time.Time, time.Time) {
	if filters.From == "" && filters.To == "" {
		return time.Date(1970, 1, 1, 0, 0, 0, 0, service.location), time.Now().In(service.location)
	}
	from, _ := time.ParseInLocation("2006-01-02", filters.From, service.location)
	to, _ := time.ParseInLocation("2006-01-02", filters.To, service.location)
	return from, to
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
		validationErr := error(nil)
		if err == nil {
			validationErr = ValidateGeneratedAnswer(generated, citations)
			if validationErr == nil {
				validationErr = ValidateAnswerDomain(question, generated, selectCitations(generated.CitationNumbers, citations))
			}
		}
		if err == nil && validationErr == nil {
			selected := selectCitations(generated.CitationNumbers, citations)
			if len(selected) == 0 && len(citations) > 0 && generated.Mode != AnalysisMode {
				generated.CitationNumbers = citationNumbers(citations[:1])
			}
			return generated
		}
		if validationErr != nil {
			correction = validationErr.Error()
		} else {
			break
		}
	}
	verified := make([]Citation, 0, len(citations))
	for _, citation := range citations {
		if citation.KnowledgeID != "" && citationIsVerified(citation) {
			verified = append(verified, citation)
		}
	}
	if len(verified) > 0 {
		numbers := citationNumbers(verified)
		if mode == AnalysisMode {
			details := ""
			for index, citation := range verified {
				if index >= 3 {
					break
				}
				if details != "" {
					details += "\n"
				}
				details += citation.Excerpt
				if citation.Applicability != "" {
					details += "\n适用条件：" + citation.Applicability
				}
			}
			return GeneratedAnswer{Answer: "基于已验证过程知识，可以确认以下适用经验。", Mode: mode, Confidence: "medium", Details: truncate(details, 1800), CitationNumbers: numbers}
		}
		return GeneratedAnswer{Answer: truncate("基于已验证过程知识："+verified[0].Excerpt, 80), Mode: mode, Confidence: "medium", CitationNumbers: numbers}
	}
	knowledge := make([]Citation, 0, len(citations))
	for _, citation := range citations {
		if citation.KnowledgeID != "" && citation.ValidationState != "contradicted" {
			knowledge = append(knowledge, citation)
		}
	}
	if len(knowledge) > 0 {
		if len(knowledge) > 3 {
			knowledge = knowledge[:3]
		}
		numbers := citationNumbers(knowledge)
		if mode == AnalysisMode {
			parts := make([]string, 0, len(knowledge))
			for _, citation := range knowledge {
				summary := knowledgeConclusion(citation.Excerpt)
				if summary != "" {
					parts = append(parts, citation.Topic+"："+summary)
				}
			}
			return GeneratedAnswer{Answer: "基于现有过程知识，可以形成以下低置信度分析。", Mode: mode, Confidence: "low", Details: truncate(strings.Join(parts, "\n"), 1200), CitationNumbers: numbers}
		}
		return GeneratedAnswer{Answer: truncate("基于现有过程知识："+knowledgeConclusion(knowledge[0].Excerpt), 80), Mode: mode, Confidence: "low", CitationNumbers: numbers}
	}
	return GeneratedAnswer{Answer: "当前数据不足以确认。", Mode: mode, Confidence: "low", Details: "", CitationNumbers: []int{}}
}

func knowledgeConclusion(excerpt string) string {
	value := strings.TrimSpace(excerpt)
	if index := strings.Index(value, "结论："); index >= 0 {
		value = strings.TrimSpace(value[index+len("结论："):])
	}
	return truncate(value, 320)
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
