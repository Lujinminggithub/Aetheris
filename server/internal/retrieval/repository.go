package retrieval

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/aetheris-dev/aetheris/server/internal/activities"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct{ pool *pgxpool.Pool }

func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

func (repository *Repository) SyncDocuments(ctx context.Context, tenantID, model string, limit int) (int, error) {
	rows, err := repository.pool.Query(ctx, `SELECT f.fact_id,f.tenant_id,f.subject_id,f.device_id,f.project_id,f.rule_version,f.event_type,f.source,f.activity_type,f.actor_origin,f.message_role,f.ai_tool,f.command_type,f.command_summary,f.command_text,f.quality_state,f.excluded_from_effectiveness,f.occurred_at,COALESCE(e.payload,'{}'::jsonb)
        FROM clean_event_facts f LEFT JOIN events e ON e.tenant_id=f.tenant_id AND e.event_id=f.canonical_event_id
        LEFT JOIN retrieval_documents d ON d.tenant_id=f.tenant_id AND d.fact_id=f.fact_id AND d.rule_version=f.rule_version
		WHERE f.tenant_id=$1 AND f.rule_version=(SELECT MAX(rule_version) FROM clean_event_facts WHERE tenant_id=$1)
		  AND f.quality_state IN ('accepted','merged') AND NOT f.excluded_from_effectiveness AND f.fact_type<>'command_fragment'
		  AND (f.event_type<>'ai.message' OR f.message_role='user')
          AND (d.document_id IS NULL OR d.embedding_model<>$2) ORDER BY f.occurred_at LIMIT $3`, tenantID, model, limit)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	documents := []Document{}
	for rows.Next() {
		var fact SourceFact
		var payload []byte
		if err := rows.Scan(&fact.FactID, &fact.TenantID, &fact.SubjectID, &fact.DeviceID, &fact.ProjectID, &fact.RuleVersion, &fact.EventType, &fact.Source, &fact.ActivityType, &fact.ActorOrigin, &fact.MessageRole, &fact.AITool, &fact.CommandType, &fact.CommandSummary, &fact.CommandText, &fact.QualityState, &fact.Excluded, &fact.OccurredAt, &payload); err != nil {
			return 0, err
		}
		fact.Payload = map[string]any{}
		_ = json.Unmarshal(payload, &fact.Payload)
		if document, ok := BuildDocument(fact, model); ok {
			documents = append(documents, document)
		}
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	for _, document := range documents {
		_, err := repository.pool.Exec(ctx, `INSERT INTO retrieval_documents(tenant_id,document_id,fact_id,rule_version,subject_id,device_id,project_id,activity_type,occurred_at,content,content_hash,embedding_model,vector_key,index_status)
            VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,'pending')
            ON CONFLICT(tenant_id,fact_id,rule_version) DO UPDATE SET document_id=EXCLUDED.document_id,subject_id=EXCLUDED.subject_id,device_id=EXCLUDED.device_id,project_id=EXCLUDED.project_id,activity_type=EXCLUDED.activity_type,occurred_at=EXCLUDED.occurred_at,content=EXCLUDED.content,content_hash=EXCLUDED.content_hash,embedding_model=EXCLUDED.embedding_model,vector_key=EXCLUDED.vector_key,index_status=CASE WHEN retrieval_documents.content_hash=EXCLUDED.content_hash AND retrieval_documents.embedding_model=EXCLUDED.embedding_model THEN retrieval_documents.index_status ELSE 'pending' END,error_code=NULL,updated_at=NOW()`, document.TenantID, document.DocumentID, document.FactID, document.RuleVersion, document.SubjectID, document.DeviceID, document.ProjectID, document.ActivityType, document.OccurredAt, document.Content, document.ContentHash, document.EmbeddingModel, document.VectorKey)
		if err != nil {
			return 0, err
		}
	}
	return len(documents), nil
}

func (repository *Repository) Pending(ctx context.Context, tenantID, model string, limit int) ([]Document, error) {
	rows, err := repository.pool.Query(ctx, `SELECT document_id,tenant_id,fact_id,rule_version,subject_id,device_id,project_id,activity_type,occurred_at,content,content_hash,embedding_model,vector_key FROM retrieval_documents WHERE tenant_id=$1 AND embedding_model=$2 AND (index_status='pending' OR (index_status='failed' AND updated_at < NOW()-INTERVAL '60 seconds')) ORDER BY occurred_at LIMIT $3`, tenantID, model, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Document{}
	for rows.Next() {
		var item Document
		if err := rows.Scan(&item.DocumentID, &item.TenantID, &item.FactID, &item.RuleVersion, &item.SubjectID, &item.DeviceID, &item.ProjectID, &item.ActivityType, &item.OccurredAt, &item.Content, &item.ContentHash, &item.EmbeddingModel, &item.VectorKey); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (repository *Repository) MarkIndexed(ctx context.Context, tenantID string, ids []string) error {
	_, err := repository.pool.Exec(ctx, `UPDATE retrieval_documents SET index_status='indexed',error_code=NULL,indexed_at=NOW(),updated_at=NOW() WHERE tenant_id=$1 AND document_id=ANY($2)`, tenantID, ids)
	return err
}
func (repository *Repository) MarkFailed(ctx context.Context, tenantID string, ids []string, code string) error {
	_, err := repository.pool.Exec(ctx, `UPDATE retrieval_documents SET index_status='failed',error_code=$3,updated_at=NOW() WHERE tenant_id=$1 AND document_id=ANY($2)`, tenantID, ids, code)
	return err
}

type IndexStatus struct {
	Documents, Indexed, Pending, Failed int
	LastIndexedAt                       *time.Time
}

func (repository *Repository) Status(ctx context.Context, tenantID, model string) (IndexStatus, error) {
	var result IndexStatus
	err := repository.pool.QueryRow(ctx, `SELECT COUNT(*),COUNT(*) FILTER(WHERE index_status='indexed'),COUNT(*) FILTER(WHERE index_status='pending'),COUNT(*) FILTER(WHERE index_status='failed'),MAX(indexed_at) FROM retrieval_documents WHERE tenant_id=$1 AND embedding_model=$2`, tenantID, model).Scan(&result.Documents, &result.Indexed, &result.Pending, &result.Failed, &result.LastIndexedAt)
	return result, err
}

func (repository *Repository) CreateQuery(ctx context.Context, job QueryJob) error {
	filters, _ := json.Marshal(job.Filters)
	_, err := repository.pool.Exec(ctx, `INSERT INTO retrieval_query_jobs(id,tenant_id,actor_id,question,filters,status,progress) VALUES($1,$2,$3,$4,$5,'queued',0)`, job.ID, job.TenantID, job.ActorID, job.Question, filters)
	return err
}

func (repository *Repository) UpdateStage(ctx context.Context, id, status string, progress int, errorCode string) error {
	_, err := repository.pool.Exec(ctx, `UPDATE retrieval_query_jobs SET status=$2,progress=$3,error_code=NULLIF($4,''),started_at=COALESCE(started_at,NOW()),completed_at=CASE WHEN $2='failed' THEN NOW() ELSE completed_at END WHERE id=$1`, id, status, progress, errorCode)
	return err
}

func (repository *Repository) CompleteQuery(ctx context.Context, id, answer string, citations []Citation) error {
	raw, _ := json.Marshal(citations)
	_, err := repository.pool.Exec(ctx, `UPDATE retrieval_query_jobs SET status='completed',progress=100,answer=$2,citations=$3,error_code=NULL,completed_at=NOW() WHERE id=$1`, id, answer, raw)
	return err
}

func (repository *Repository) CompleteStructuredQuery(ctx context.Context, id string, answer GeneratedAnswer, citations []Citation) error {
	rawCitations, _ := json.Marshal(citations)
	rawNumbers, _ := json.Marshal(answer.CitationNumbers)
	_, err := repository.pool.Exec(ctx, `UPDATE retrieval_query_jobs SET status='completed',progress=100,answer=$2,answer_mode=$3,answer_confidence=$4,answer_details=$5,citation_numbers=$6,citations=$7,error_code=NULL,completed_at=NOW() WHERE id=$1`, id, answer.Answer, answer.Mode, answer.Confidence, answer.Details, rawNumbers, rawCitations)
	return err
}

func (repository *Repository) GetQuery(ctx context.Context, tenantID, actorID, id string) (QueryJob, error) {
	var job QueryJob
	var filters, citations []byte
	var answerMode, confidence, details string
	var citationNumbers []byte
	err := repository.pool.QueryRow(ctx, `SELECT id,tenant_id,actor_id,question,filters,status,progress,answer,answer_mode,answer_confidence,answer_details,citation_numbers,citations,COALESCE(error_code,''),created_at,completed_at FROM retrieval_query_jobs WHERE tenant_id=$1 AND actor_id=$2 AND id=$3`, tenantID, actorID, id).Scan(&job.ID, &job.TenantID, &job.ActorID, &job.Question, &filters, &job.Status, &job.Progress, &job.Answer, &answerMode, &confidence, &details, &citationNumbers, &citations, &job.ErrorCode, &job.CreatedAt, &job.CompletedAt)
	if err != nil {
		return QueryJob{}, err
	}
	_ = json.Unmarshal(filters, &job.Filters)
	job.Citations = []Citation{}
	_ = json.Unmarshal(citations, &job.Citations)
	job.AnswerMode = AnswerMode(answerMode)
	if !job.AnswerMode.Valid() {
		job.AnswerMode = DirectMode
	}
	job.Confidence = confidence
	if job.Confidence != "high" && job.Confidence != "medium" && job.Confidence != "low" {
		job.Confidence = "low"
	}
	job.Details = details
	_ = json.Unmarshal(citationNumbers, &job.CitationNumbers)
	return job, nil
}

func (repository *Repository) LoadDocuments(ctx context.Context, filter QueryFilter, hits []Hit) ([]RetrievedDocument, error) {
	ids := make([]string, 0, len(hits))
	scores := map[string]float64{}
	for _, hit := range hits {
		ids = append(ids, hit.DocumentID)
		scores[hit.DocumentID] = hit.Score
	}
	if len(ids) == 0 {
		return []RetrievedDocument{}, nil
	}
	rows, err := repository.pool.Query(ctx, `SELECT d.document_id,d.fact_id,f.canonical_event_id,f.source_event_ids,d.device_id,d.project_id,p.name,COALESCE(e.payload->>'project_label',''),COALESCE(NULLIF(e.payload->>'project',''),NULLIF(e.payload->>'project_path',''),NULLIF(e.payload->>'cwd',''),NULLIF(e.provenance->>'project_root',''),''),d.activity_type,d.content,d.occurred_at
        FROM retrieval_documents d JOIN clean_event_facts f ON f.tenant_id=d.tenant_id AND f.fact_id=d.fact_id AND f.rule_version=d.rule_version
        JOIN projects p ON p.tenant_id=d.tenant_id AND p.id=d.project_id LEFT JOIN events e ON e.tenant_id=f.tenant_id AND e.event_id=f.canonical_event_id
        WHERE d.tenant_id=$1 AND d.document_id=ANY($2) AND d.index_status='indexed' AND d.occurred_at >= $3 AND d.occurred_at < $4
          AND ($5='' OR d.device_id=$5) AND ($6='' OR d.project_id=$6) AND ($7='' OR d.activity_type=$7)`, filter.TenantID, ids, filter.From, filter.ToExclusive, filter.DeviceID, filter.ProjectID, filter.ActivityType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	byID := map[string]RetrievedDocument{}
	for rows.Next() {
		var item RetrievedDocument
		var stored, safe, hint string
		if err := rows.Scan(&item.DocumentID, &item.FactID, &item.CanonicalEventID, &item.SourceEventIDs, &item.DeviceID, &item.ProjectID, &stored, &safe, &hint, &item.ActivityType, &item.Excerpt, &item.OccurredAt); err != nil {
			return nil, err
		}
		item.ProjectName = activities.ProjectLabel(safe, hint, stored, item.ProjectID)
		item.Score = scores[item.DocumentID]
		item.Excerpt = truncate(item.Excerpt, 600)
		if item.ActivityType == "ai" {
			replies, eventIDs, err := repository.relatedAIContext(ctx, filter.TenantID, item.DeviceID, item.ProjectID, item.OccurredAt)
			if err != nil {
				return nil, err
			}
			item.Excerpt = CombineRelatedContext(item.Excerpt, replies)
			item.SourceEventIDs = append(item.SourceEventIDs, eventIDs...)
		}
		byID[item.DocumentID] = item
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	result := []RetrievedDocument{}
	for _, hit := range hits {
		if item, ok := byID[hit.DocumentID]; ok {
			result = append(result, item)
		}
	}
	return result, nil
}

func (repository *Repository) relatedAIContext(ctx context.Context, tenantID, deviceID, projectID string, occurredAt time.Time) ([]string, []string, error) {
	rows, err := repository.pool.Query(ctx, `SELECT f.canonical_event_id,COALESCE(e.payload->>'content','') FROM clean_event_facts f JOIN events e ON e.tenant_id=f.tenant_id AND e.event_id=f.canonical_event_id
        WHERE f.tenant_id=$1 AND f.rule_version=(SELECT MAX(rule_version) FROM clean_event_facts WHERE tenant_id=$1)
          AND f.device_id=$2 AND f.project_id=$3 AND f.event_type='ai.message' AND f.message_role='assistant'
          AND f.quality_state IN ('accepted','merged') AND NOT f.excluded_from_effectiveness
          AND f.occurred_at >= $4::timestamptz-INTERVAL '1 minute' AND f.occurred_at < $4::timestamptz+INTERVAL '15 minutes'
        ORDER BY f.occurred_at LIMIT 3`, tenantID, deviceID, projectID, occurredAt)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	replies := []string{}
	ids := []string{}
	for rows.Next() {
		var id, content string
		if err := rows.Scan(&id, &content); err != nil {
			return nil, nil, err
		}
		if strings.TrimSpace(content) != "" {
			ids = append(ids, id)
			replies = append(replies, truncate(content, 1200))
		}
	}
	return replies, ids, rows.Err()
}
