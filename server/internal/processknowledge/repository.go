package processknowledge

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/aetheris-dev/aetheris/server/internal/retrieval"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const sourceTurnsSQL = `SELECT f.tenant_id,f.subject_id,f.device_id,ep.logical_project_id,
    COALESCE(NULLIF(f.ai_tool,''),NULLIF(e.source,'')),
    COALESCE(NULLIF(e.session_id,''),NULLIF(e.payload->>'session_id','')),
    f.canonical_event_id,f.fact_id,f.message_role,f.event_type,COALESCE(e.payload->>'content',''),f.occurred_at
FROM clean_event_facts f
JOIN events e ON e.tenant_id=f.tenant_id AND e.event_id=f.canonical_event_id
JOIN current_event_projects ep ON ep.tenant_id=f.tenant_id AND ep.event_id=f.canonical_event_id
WHERE f.tenant_id=$1
  AND f.rule_version=(SELECT MAX(rule_version) FROM clean_event_facts WHERE tenant_id=$1)
  AND f.event_type IN ('ai.message','ai.tool_call')
  AND f.quality_state IN ('accepted','merged') AND NOT f.excluded_from_effectiveness
  AND ep.logical_project_id IS NOT NULL AND NOT ep.needs_review
  AND ($2='' OR ep.logical_project_id=$2)
  AND NOT EXISTS (SELECT 1 FROM process_turns t WHERE t.tenant_id=f.tenant_id AND f.canonical_event_id=ANY(t.source_event_ids) AND t.classification_version=$4)
ORDER BY f.occurred_at,f.fact_id LIMIT $3`

const dirtySessionsSQL = `SELECT session_id,subject_id,device_id,logical_project_id,ai_tool,source_session_id,association_method,association_confidence,started_at,ended_at FROM process_sessions WHERE tenant_id=$1 AND dirty=TRUE AND ended_at < NOW()-INTERVAL '5 minutes' ORDER BY updated_at,session_id LIMIT $2`
const activateInitialVersionSQL = `INSERT INTO process_knowledge_state(tenant_id,mode,active_version,canary_percent,updated_at) VALUES($1,'shadow',$2,0,NOW()) ON CONFLICT(tenant_id) DO NOTHING`
const sourceTurnCountSQL = `SELECT COUNT(*) FROM clean_event_facts f JOIN current_event_projects ep ON ep.tenant_id=f.tenant_id AND ep.event_id=f.canonical_event_id WHERE f.tenant_id=$1 AND f.rule_version=(SELECT MAX(rule_version) FROM clean_event_facts WHERE tenant_id=$1) AND f.event_type IN ('ai.message','ai.tool_call') AND f.quality_state IN ('accepted','merged') AND NOT f.excluded_from_effectiveness AND ep.logical_project_id IS NOT NULL AND NOT ep.needs_review AND ($2='' OR ep.logical_project_id=$2)`
const recoverStaleJobsSQL = `UPDATE process_knowledge_jobs SET state='pending',error_code='stale_job_recovered',updated_at=NOW() WHERE state='running' AND updated_at < NOW()-INTERVAL '10 minutes'`
const claimJobSQL = `WITH candidate AS (SELECT id FROM process_knowledge_jobs WHERE state='pending' ORDER BY created_at,id FOR UPDATE SKIP LOCKED LIMIT 1) UPDATE process_knowledge_jobs j SET state='running',updated_at=NOW() FROM candidate c WHERE j.id=c.id RETURNING j.id,j.tenant_id,j.mode,COALESCE(j.logical_project_id,''),j.version,j.state,j.last_session_id,j.scanned_count,j.candidate_count,j.verified_count,j.conflict_count,j.failed_count,j.error_code,j.created_by,j.created_at,j.updated_at,j.completed_at`

type Repository struct {
	pool           *pgxpool.Pool
	embeddingModel string
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool, embeddingModel: "embeddinggemma"}
}
func (repository *Repository) WithEmbeddingModel(model string) *Repository {
	if model != "" {
		repository.embeddingModel = model
	}
	return repository
}

func (repository *Repository) CreateJob(ctx context.Context, tenantID, actorID, mode, projectID string, version int) (Job, error) {
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return Job{}, err
	}
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, `INSERT INTO process_knowledge_versions(tenant_id,version,state,extractor_version,chunk_version,cleaning_rule_version,attribution_rule_version,created_by)
        VALUES($1,$2,'building','deterministic-v1','semantic-v1',(SELECT COALESCE(MAX(rule_version),1) FROM clean_event_facts WHERE tenant_id=$1),(SELECT active_rule_version FROM project_attribution_state WHERE tenant_id=$1),$3)
        ON CONFLICT(tenant_id,version) DO UPDATE SET state=CASE WHEN process_knowledge_versions.state='active' THEN 'active' ELSE 'building' END`, tenantID, version, actorID)
	if err != nil {
		return Job{}, err
	}
	id, err := randomID("knowledge-job")
	if err != nil {
		return Job{}, err
	}
	job := Job{ID: id, TenantID: tenantID, Mode: mode, LogicalProjectID: projectID, Version: version, State: "pending", CreatedBy: actorID}
	err = tx.QueryRow(ctx, `INSERT INTO process_knowledge_jobs(id,tenant_id,mode,version,logical_project_id,state,created_by) VALUES($1,$2,$3,$4,NULLIF($5,''),'pending',$6) RETURNING created_at,updated_at`, id, tenantID, mode, version, projectID, actorID).Scan(&job.CreatedAt, &job.UpdatedAt)
	if err != nil {
		return Job{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Job{}, err
	}
	return job, nil
}

func (repository *Repository) GetJob(ctx context.Context, tenantID, id string) (Job, error) {
	return scanKnowledgeJob(repository.pool.QueryRow(ctx, `SELECT id,tenant_id,mode,COALESCE(logical_project_id,''),version,state,last_session_id,scanned_count,candidate_count,verified_count,conflict_count,failed_count,error_code,created_by,created_at,updated_at,completed_at FROM process_knowledge_jobs WHERE tenant_id=$1 AND id=$2`, tenantID, id))
}

func (repository *Repository) ProcessNext(ctx context.Context, batchSize int) (bool, error) {
	if batchSize < 1 {
		batchSize = 100
	}
	if _, err := repository.pool.Exec(ctx, recoverStaleJobsSQL); err != nil {
		return false, err
	}
	job, err := scanKnowledgeJob(repository.pool.QueryRow(ctx, claimJobSQL))
	if err == pgx.ErrNoRows {
		return repository.processIncremental(ctx, batchSize)
	}
	if err != nil {
		return false, err
	}
	if job.Mode == "dry_run" {
		var count int64
		err = repository.pool.QueryRow(ctx, sourceTurnCountSQL, job.TenantID, job.LogicalProjectID).Scan(&count)
		if err == nil {
			_, err = repository.pool.Exec(ctx, `UPDATE process_knowledge_jobs SET state='completed',scanned_count=$2,completed_at=NOW(),updated_at=NOW() WHERE id=$1`, job.ID, count)
		}
		return true, err
	}
	synced, err := repository.syncSessions(ctx, job.TenantID, job.LogicalProjectID, batchSize, fmt.Sprintf("knowledge-v%d", job.Version))
	if err != nil {
		return true, repository.failJob(ctx, job, err)
	}
	extracted, verified, conflicts := 0, 0, 0
	if synced < batchSize {
		extracted, verified, conflicts, err = repository.extractDirtySessions(ctx, job.TenantID, job.Version, batchSize)
	}
	if err != nil {
		return true, repository.failJob(ctx, job, err)
	}
	state := "pending"
	var completed any = nil
	if synced < batchSize && extracted < batchSize {
		state = "completed"
		completed = time.Now().UTC()
	}
	_, err = repository.pool.Exec(ctx, `UPDATE process_knowledge_jobs SET state=$2,scanned_count=scanned_count+$3,candidate_count=candidate_count+$4,verified_count=verified_count+$5,conflict_count=conflict_count+$6,updated_at=NOW(),completed_at=$7 WHERE id=$1`, job.ID, state, synced, extracted, verified, conflicts, completed)
	if err == nil && state == "completed" {
		_, err = repository.pool.Exec(ctx, `UPDATE process_knowledge_versions SET state='completed',completed_at=NOW() WHERE tenant_id=$1 AND version=$2 AND state<>'active'`, job.TenantID, job.Version)
		if err == nil {
			_, err = repository.pool.Exec(ctx, activateInitialVersionSQL, job.TenantID, job.Version)
		}
	}
	return true, err
}

func (repository *Repository) processIncremental(ctx context.Context, batchSize int) (bool, error) {
	var tenantID string
	var version int
	err := repository.pool.QueryRow(ctx, `SELECT tenant_id,active_version FROM process_knowledge_state WHERE active_version IS NOT NULL ORDER BY updated_at LIMIT 1`).Scan(&tenantID, &version)
	if err == pgx.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	synced, err := repository.syncSessions(ctx, tenantID, "", batchSize, fmt.Sprintf("knowledge-v%d", version))
	if err != nil {
		return false, err
	}
	extracted, _, _, err := repository.extractDirtySessions(ctx, tenantID, version, batchSize)
	return synced > 0 || extracted > 0, err
}

func (repository *Repository) syncSessions(ctx context.Context, tenantID, projectID string, limit int, classificationVersion string) (int, error) {
	rows, err := repository.pool.Query(ctx, sourceTurnsSQL, tenantID, projectID, limit, classificationVersion)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	source := []SourceTurn{}
	for rows.Next() {
		var item SourceTurn
		if err := rows.Scan(&item.TenantID, &item.SubjectID, &item.DeviceID, &item.LogicalProjectID, &item.AITool, &item.SessionID, &item.EventID, &item.FactID, &item.Role, &item.EventType, &item.Content, &item.OccurredAt); err != nil {
			return 0, err
		}
		source = append(source, item)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	if len(source) == 0 {
		return 0, nil
	}
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	for _, session := range AssembleSessions(source) {
		eventIDs := make([]string, 0, len(session.Turns))
		for _, turn := range session.Turns {
			eventIDs = append(eventIDs, turn.Source.EventID)
		}
		_, err = tx.Exec(ctx, `INSERT INTO process_sessions(tenant_id,session_id,subject_id,device_id,logical_project_id,ai_tool,source_session_id,association_method,association_confidence,state,started_at,ended_at,source_event_ids,cleaning_rule_version,attribution_rule_version,dirty)
            VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,'idle',$10,$11,$12,(SELECT MAX(rule_version) FROM clean_event_facts WHERE tenant_id=$1),(SELECT active_rule_version FROM project_attribution_state WHERE tenant_id=$1),TRUE)
            ON CONFLICT(tenant_id,session_id) DO UPDATE SET started_at=LEAST(process_sessions.started_at,EXCLUDED.started_at),ended_at=GREATEST(process_sessions.ended_at,EXCLUDED.ended_at),source_event_ids=process_sessions.source_event_ids||EXCLUDED.source_event_ids,dirty=TRUE,updated_at=NOW()`, session.TenantID, session.ID, session.SubjectID, session.DeviceID, session.LogicalProjectID, session.AITool, session.SourceSessionID, session.AssociationMethod, session.AssociationConfidence, session.StartedAt, session.EndedAt, eventIDs)
		if err != nil {
			return 0, err
		}
		var existingMax int
		if err := tx.QueryRow(ctx, `SELECT COALESCE(MAX(sequence),-1) FROM process_turns WHERE tenant_id=$1 AND session_id=$2`, session.TenantID, session.ID).Scan(&existingMax); err != nil {
			return 0, err
		}
		for _, turn := range session.Turns {
			hash := sha256.Sum256([]byte(turn.Source.Content))
			if _, err = tx.Exec(ctx, `UPDATE process_turns SET statement_kind='system_context',classification_version=$3 WHERE tenant_id=$1 AND $2=ANY(source_event_ids) AND classification_version<>$3`, session.TenantID, turn.Source.EventID, classificationVersion); err != nil {
				return 0, err
			}
			_, err = tx.Exec(ctx, `INSERT INTO process_turns(tenant_id,turn_id,session_id,sequence,message_role,statement_kind,safe_content,content_hash,source_event_ids,classification_method,classification_version,occurred_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,'deterministic',$10,$11) ON CONFLICT(tenant_id,turn_id) DO UPDATE SET message_role=EXCLUDED.message_role,statement_kind=EXCLUDED.statement_kind,safe_content=EXCLUDED.safe_content,content_hash=EXCLUDED.content_hash,classification_method=EXCLUDED.classification_method,classification_version=EXCLUDED.classification_version,occurred_at=EXCLUDED.occurred_at`, session.TenantID, turn.ID, session.ID, nextTurnSequence(existingMax, turn.Sequence), normalizeRole(turn.Source.Role), turn.Kind, turn.Source.Content, hex.EncodeToString(hash[:]), []string{turn.Source.EventID}, classificationVersion, turn.Source.OccurredAt)
			if err != nil {
				return 0, err
			}
		}
	}
	return len(source), tx.Commit(ctx)
}

func (repository *Repository) extractDirtySessions(ctx context.Context, tenantID string, version, limit int) (int, int, int, error) {
	rows, err := repository.pool.Query(ctx, dirtySessionsSQL, tenantID, limit)
	if err != nil {
		return 0, 0, 0, err
	}
	defer rows.Close()
	type sessionRow struct{ draft SessionDraft }
	sessions := []sessionRow{}
	for rows.Next() {
		var row sessionRow
		row.draft.TenantID = tenantID
		if err := rows.Scan(&row.draft.ID, &row.draft.SubjectID, &row.draft.DeviceID, &row.draft.LogicalProjectID, &row.draft.AITool, &row.draft.SourceSessionID, &row.draft.AssociationMethod, &row.draft.AssociationConfidence, &row.draft.StartedAt, &row.draft.EndedAt); err != nil {
			return 0, 0, 0, err
		}
		sessions = append(sessions, row)
	}
	if err := rows.Err(); err != nil {
		return 0, 0, 0, err
	}
	verified, conflicts := 0, 0
	for _, row := range sessions {
		turnRows, err := repository.pool.Query(ctx, `SELECT turn_id,sequence,message_role,statement_kind,safe_content,occurred_at,COALESCE(source_event_ids[1],''),'' FROM process_turns WHERE tenant_id=$1 AND session_id=$2 ORDER BY sequence`, tenantID, row.draft.ID)
		if err != nil {
			return 0, verified, conflicts, err
		}
		for turnRows.Next() {
			var turn TurnDraft
			var role string
			if err := turnRows.Scan(&turn.ID, &turn.Sequence, &role, &turn.Kind, &turn.Source.Content, &turn.Source.OccurredAt, &turn.Source.EventID, &turn.Source.FactID); err != nil {
				turnRows.Close()
				return 0, verified, conflicts, err
			}
			turn.Source.Role = role
			row.draft.Turns = append(row.draft.Turns, turn)
		}
		turnRows.Close()
		units := ExtractKnowledge(row.draft)
		tx, err := repository.pool.Begin(ctx)
		if err != nil {
			return 0, verified, conflicts, err
		}
		for _, unit := range units {
			if unit.ValidationState == "verified" {
				verified++
			}
			if unit.ValidationState == "contradicted" {
				conflicts++
			}
			_, err = tx.Exec(ctx, `INSERT INTO process_knowledge_units(tenant_id,knowledge_id,revision,version,logical_project_id,session_id,topic,knowledge_type,problem,intent,constraints_text,conclusion,rationale,alternatives,applicability,caveats,decision_state,validation_state,lifecycle_state,extractor,extractor_version) VALUES($1,$2,1,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,'deterministic','v1') ON CONFLICT(tenant_id,knowledge_id,revision) DO NOTHING`, tenantID, unit.ID, version, row.draft.LogicalProjectID, row.draft.ID, unit.Topic, unit.KnowledgeType, unit.Problem, unit.Intent, unit.Constraints, unit.Conclusion, unit.Rationale, unit.Alternatives, unit.Applicability, unit.Caveats, unit.DecisionState, unit.ValidationState, unit.LifecycleState)
			if err != nil {
				tx.Rollback(ctx)
				return 0, verified, conflicts, err
			}
			for _, evidence := range unit.Evidence {
				_, err = tx.Exec(ctx, `INSERT INTO process_knowledge_evidence(tenant_id,knowledge_id,revision,evidence_id,evidence_kind,event_id,fact_id,turn_id,supports_section,relation,reason_code) VALUES($1,$2,1,$3,$4,NULLIF($5,''),NULLIF($6,''),NULLIF($7,''),$8,$9,$10) ON CONFLICT DO NOTHING`, tenantID, unit.ID, evidence.ID, evidence.Kind, evidence.EventID, evidence.FactID, evidence.TurnID, evidence.Section, evidence.Relation, evidence.ReasonCode)
				if err != nil {
					tx.Rollback(ctx)
					return 0, verified, conflicts, err
				}
			}
			for _, chunk := range ChunkKnowledge(unit, 600, 1000) {
				vectorKey := knowledgeVectorKey(tenantID, chunk.ID)
				_, err = tx.Exec(ctx, `INSERT INTO process_knowledge_chunks(tenant_id,chunk_id,knowledge_id,revision,version,logical_project_id,session_id,chunk_index,topic,knowledge_type,decision_state,validation_state,content,search_text,content_hash,embedding_model,vector_key,index_status,occurred_at) VALUES($1,$2,$3,1,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,'pending',$17) ON CONFLICT(tenant_id,chunk_id) DO NOTHING`, tenantID, chunk.ID, unit.ID, version, row.draft.LogicalProjectID, row.draft.ID, chunk.Index, unit.Topic, unit.KnowledgeType, unit.DecisionState, unit.ValidationState, chunk.Content, chunk.SearchText, chunk.ContentHash, repository.embeddingModel, vectorKey, unit.OccurredAt)
				if err != nil {
					tx.Rollback(ctx)
					return 0, verified, conflicts, err
				}
			}
		}
		_, err = tx.Exec(ctx, `UPDATE process_sessions SET dirty=FALSE,updated_at=NOW() WHERE tenant_id=$1 AND session_id=$2`, tenantID, row.draft.ID)
		if err != nil {
			tx.Rollback(ctx)
			return 0, verified, conflicts, err
		}
		if err = tx.Commit(ctx); err != nil {
			return 0, verified, conflicts, err
		}
	}
	return len(sessions), verified, conflicts, nil
}

func (repository *Repository) Activate(ctx context.Context, tenantID string, version int, mode string, canary int) error {
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var state string
	if err = tx.QueryRow(ctx, `SELECT state FROM process_knowledge_versions WHERE tenant_id=$1 AND version=$2 FOR UPDATE`, tenantID, version).Scan(&state); err != nil {
		return err
	}
	if state != "completed" && state != "active" {
		return fmt.Errorf("知识版本尚未完成")
	}
	var previous *int
	_ = tx.QueryRow(ctx, `SELECT active_version FROM process_knowledge_state WHERE tenant_id=$1 FOR UPDATE`, tenantID).Scan(&previous)
	_, err = tx.Exec(ctx, `UPDATE process_knowledge_versions SET state='active',activated_at=NOW() WHERE tenant_id=$1 AND version=$2`, tenantID, version)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO process_knowledge_state(tenant_id,mode,active_version,previous_version,canary_percent) VALUES($1,$2,$3,$4,$5) ON CONFLICT(tenant_id) DO UPDATE SET mode=EXCLUDED.mode,previous_version=process_knowledge_state.active_version,active_version=EXCLUDED.active_version,canary_percent=EXCLUDED.canary_percent,updated_at=NOW()`, tenantID, mode, version, previous, canary)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func (repository *Repository) Rollback(ctx context.Context, tenantID string, version int) error {
	return repository.Activate(ctx, tenantID, version, "active", 0)
}

func (repository *Repository) Summary(ctx context.Context, tenantID string) (Summary, error) {
	var s Summary
	err := repository.pool.QueryRow(ctx, `SELECT (SELECT COUNT(*) FROM process_sessions WHERE tenant_id=$1),(SELECT COUNT(*) FROM process_knowledge_units WHERE tenant_id=$1 AND lifecycle_state='active'),(SELECT COUNT(*) FROM process_knowledge_units WHERE tenant_id=$1 AND validation_state='verified' AND lifecycle_state='active'),(SELECT COUNT(*) FROM process_knowledge_units WHERE tenant_id=$1 AND validation_state='contradicted' AND lifecycle_state='active'),(SELECT COUNT(*) FROM process_sessions WHERE tenant_id=$1 AND association_confidence='low'),(SELECT COUNT(*) FROM process_knowledge_chunks WHERE tenant_id=$1),COALESCE((SELECT active_version FROM process_knowledge_state WHERE tenant_id=$1),0),COALESCE((SELECT mode FROM process_knowledge_state WHERE tenant_id=$1),'shadow')`, tenantID).Scan(&s.Sessions, &s.Units, &s.Verified, &s.Conflicts, &s.Unattributed, &s.Chunks, &s.ActiveVersion, &s.Mode)
	return s, err
}

func (repository *Repository) ListUnits(ctx context.Context, filter ListFilter) ([]KnowledgeUnit, int, error) {
	args := []any{filter.TenantID, filter.LogicalProjectID, filter.Topic, filter.ValidationState, filter.DecisionState, filter.Limit, filter.Offset}
	where := `tenant_id=$1 AND ($2='' OR logical_project_id=$2) AND ($3='' OR topic=$3) AND ($4='' OR validation_state=$4) AND ($5='' OR decision_state=$5)`
	var total int
	if err := repository.pool.QueryRow(ctx, `SELECT COUNT(*) FROM process_knowledge_units WHERE `+where, args[:5]...).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := repository.pool.Query(ctx, `SELECT knowledge_id,revision,version,logical_project_id,session_id,topic,knowledge_type,problem,intent,constraints_text,conclusion,rationale,alternatives,applicability,caveats,decision_state,validation_state,lifecycle_state,created_at FROM process_knowledge_units WHERE `+where+` ORDER BY updated_at DESC LIMIT $6 OFFSET $7`, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := []KnowledgeUnit{}
	for rows.Next() {
		var item KnowledgeUnit
		if err := rows.Scan(&item.ID, &item.Revision, &item.Version, &item.LogicalProjectID, &item.SessionID, &item.Topic, &item.KnowledgeType, &item.Problem, &item.Intent, &item.Constraints, &item.Conclusion, &item.Rationale, &item.Alternatives, &item.Applicability, &item.Caveats, &item.DecisionState, &item.ValidationState, &item.LifecycleState, &item.OccurredAt); err != nil {
			return nil, 0, err
		}
		items = append(items, item)
	}
	return items, total, rows.Err()
}

func (repository *Repository) GetUnit(ctx context.Context, tenantID, id string) (KnowledgeUnit, error) {
	var item KnowledgeUnit
	err := repository.pool.QueryRow(ctx, `SELECT knowledge_id,revision,version,logical_project_id,session_id,topic,knowledge_type,problem,intent,constraints_text,conclusion,rationale,alternatives,applicability,caveats,decision_state,validation_state,lifecycle_state,created_at FROM process_knowledge_units WHERE tenant_id=$1 AND knowledge_id=$2 ORDER BY revision DESC LIMIT 1`, tenantID, id).Scan(&item.ID, &item.Revision, &item.Version, &item.LogicalProjectID, &item.SessionID, &item.Topic, &item.KnowledgeType, &item.Problem, &item.Intent, &item.Constraints, &item.Conclusion, &item.Rationale, &item.Alternatives, &item.Applicability, &item.Caveats, &item.DecisionState, &item.ValidationState, &item.LifecycleState, &item.OccurredAt)
	if err != nil {
		return KnowledgeUnit{}, err
	}
	rows, err := repository.pool.Query(ctx, `SELECT evidence_id,evidence_kind,COALESCE(event_id,''),COALESCE(fact_id,''),COALESCE(turn_id,''),supports_section,relation,reason_code FROM process_knowledge_evidence WHERE tenant_id=$1 AND knowledge_id=$2 AND revision=$3 ORDER BY created_at,evidence_id`, tenantID, id, item.Revision)
	if err != nil {
		return KnowledgeUnit{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var evidence EvidenceDraft
		if err := rows.Scan(&evidence.ID, &evidence.Kind, &evidence.EventID, &evidence.FactID, &evidence.TurnID, &evidence.Section, &evidence.Relation, &evidence.ReasonCode); err != nil {
			return KnowledgeUnit{}, err
		}
		item.Evidence = append(item.Evidence, evidence)
	}
	return item, rows.Err()
}

func (repository *Repository) failJob(ctx context.Context, job Job, cause error) error {
	_, _ = repository.pool.Exec(ctx, `UPDATE process_knowledge_jobs SET state='failed',failed_count=failed_count+1,error_code='process_knowledge_failed',completed_at=NOW(),updated_at=NOW() WHERE id=$1`, job.ID)
	return cause
}

type rowScanner interface{ Scan(...any) error }

func scanKnowledgeJob(row rowScanner) (Job, error) {
	var job Job
	err := row.Scan(&job.ID, &job.TenantID, &job.Mode, &job.LogicalProjectID, &job.Version, &job.State, &job.LastSessionID, &job.ScannedCount, &job.CandidateCount, &job.VerifiedCount, &job.ConflictCount, &job.FailedCount, &job.ErrorCode, &job.CreatedBy, &job.CreatedAt, &job.UpdatedAt, &job.CompletedAt)
	return job, err
}
func randomID(prefix string) (string, error) {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return fmt.Sprintf("%s-%x", prefix, value), nil
}
func normalizeRole(value string) string {
	switch strings.ToLower(value) {
	case "user", "assistant", "system", "tool":
		return strings.ToLower(value)
	default:
		return "unknown"
	}
}

func nextTurnSequence(existingMax, relative int) int { return existingMax + 1 + relative }

func (repository *Repository) PendingChunks(ctx context.Context, tenantID, model string, limit int) ([]ChunkRecord, error) {
	rows, err := repository.pool.Query(ctx, `SELECT chunk_id,tenant_id,logical_project_id,session_id,topic,search_text,vector_key,occurred_at FROM process_knowledge_chunks WHERE tenant_id=$1 AND embedding_model=$2 AND (index_status='pending' OR (index_status='failed' AND updated_at<NOW()-INTERVAL '60 seconds')) ORDER BY occurred_at,chunk_id LIMIT $3`, tenantID, model, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []ChunkRecord{}
	for rows.Next() {
		var item ChunkRecord
		if err := rows.Scan(&item.ChunkID, &item.TenantID, &item.LogicalProjectID, &item.SessionID, &item.Topic, &item.SearchText, &item.VectorKey, &item.OccurredAt); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (repository *Repository) MarkChunksIndexed(ctx context.Context, tenantID string, ids []string) error {
	_, err := repository.pool.Exec(ctx, `UPDATE process_knowledge_chunks SET index_status='indexed',error_code=NULL,indexed_at=NOW(),updated_at=NOW() WHERE tenant_id=$1 AND chunk_id=ANY($2)`, tenantID, ids)
	return err
}

func (repository *Repository) MarkChunksFailed(ctx context.Context, tenantID string, ids []string, code string) error {
	_, err := repository.pool.Exec(ctx, `UPDATE process_knowledge_chunks SET index_status='failed',error_code=$3,updated_at=NOW() WHERE tenant_id=$1 AND chunk_id=ANY($2)`, tenantID, ids, code)
	return err
}

func (repository *Repository) LoadVectorCandidates(ctx context.Context, query retrieval.KnowledgeQuery, hits []retrieval.Hit) ([]SearchCandidate, error) {
	ids := make([]string, 0, len(hits))
	ranks := map[string]int{}
	for index, hit := range hits {
		ids = append(ids, hit.DocumentID)
		ranks[hit.DocumentID] = index + 1
	}
	if len(ids) == 0 {
		return []SearchCandidate{}, nil
	}
	rows, err := repository.pool.Query(ctx, `SELECT c.chunk_id,c.knowledge_id,c.session_id,c.logical_project_id,c.topic,c.knowledge_type,c.decision_state,c.validation_state,c.content,u.applicability,c.occurred_at,ARRAY(SELECT DISTINCT evidence.event_id FROM process_knowledge_evidence evidence WHERE evidence.tenant_id=c.tenant_id AND evidence.knowledge_id=c.knowledge_id AND evidence.revision=c.revision AND evidence.event_id IS NOT NULL) FROM process_knowledge_chunks c JOIN process_knowledge_units u ON u.tenant_id=c.tenant_id AND u.knowledge_id=c.knowledge_id AND u.revision=c.revision JOIN process_knowledge_state state ON state.tenant_id=c.tenant_id AND state.active_version=c.version WHERE c.tenant_id=$1 AND c.chunk_id=ANY($2) AND c.index_status='indexed' AND ($3='' OR c.logical_project_id=$3) AND c.occurred_at >= $4 AND c.occurred_at < $5`, query.TenantID, ids, query.LogicalProjectID, query.From, query.ToExclusive)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	byID := map[string]SearchCandidate{}
	for rows.Next() {
		var item SearchCandidate
		if err := rows.Scan(&item.ChunkID, &item.KnowledgeID, &item.SessionID, &item.LogicalProjectID, &item.Topic, &item.KnowledgeType, &item.DecisionState, &item.ValidationState, &item.Content, &item.Applicability, &item.OccurredAt, &item.SourceEventIDs); err != nil {
			return nil, err
		}
		item.Rank = ranks[item.ChunkID]
		byID[item.ChunkID] = item
	}
	result := []SearchCandidate{}
	for _, id := range ids {
		if item, ok := byID[id]; ok {
			result = append(result, item)
		}
	}
	return result, rows.Err()
}

func (repository *Repository) KeywordCandidates(ctx context.Context, query retrieval.KnowledgeQuery, terms []string, limit int) ([]SearchCandidate, error) {
	search := strings.Join(terms, " ")
	if search == "" {
		search = query.Question
	}
	rows, err := repository.pool.Query(ctx, `SELECT c.chunk_id,c.knowledge_id,c.session_id,c.logical_project_id,c.topic,c.knowledge_type,c.decision_state,c.validation_state,c.content,u.applicability,c.occurred_at,ARRAY(SELECT DISTINCT evidence.event_id FROM process_knowledge_evidence evidence WHERE evidence.tenant_id=c.tenant_id AND evidence.knowledge_id=c.knowledge_id AND evidence.revision=c.revision AND evidence.event_id IS NOT NULL),similarity(c.search_text,$4) score FROM process_knowledge_chunks c JOIN process_knowledge_units u ON u.tenant_id=c.tenant_id AND u.knowledge_id=c.knowledge_id AND u.revision=c.revision JOIN process_knowledge_state state ON state.tenant_id=c.tenant_id AND state.active_version=c.version WHERE c.tenant_id=$1 AND ($2='' OR c.logical_project_id=$2) AND c.occurred_at >= $5 AND c.occurred_at < $6 AND (c.search_text % $4 OR c.search_text ILIKE '%'||$4||'%') ORDER BY score DESC,c.occurred_at DESC LIMIT $3`, query.TenantID, query.LogicalProjectID, limit, search, query.From, query.ToExclusive)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []SearchCandidate{}
	rank := 0
	for rows.Next() {
		rank++
		var item SearchCandidate
		var rawScore float64
		if err := rows.Scan(&item.ChunkID, &item.KnowledgeID, &item.SessionID, &item.LogicalProjectID, &item.Topic, &item.KnowledgeType, &item.DecisionState, &item.ValidationState, &item.Content, &item.Applicability, &item.OccurredAt, &item.SourceEventIDs, &rawScore); err != nil {
			return nil, err
		}
		item.Rank, item.Score = rank, rawScore
		result = append(result, item)
	}
	return result, rows.Err()
}
