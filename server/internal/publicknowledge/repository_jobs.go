package publicknowledge

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

func (repository *Repository) ProcessNext(ctx context.Context, limit int) (bool, error) {
	if limit < 1 {
		limit = 25
	}
	job, err := repository.claimJob(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if job.Mode == "reindex" {
		_, err = repository.pool.Exec(ctx, `UPDATE public_knowledge_chunks c SET index_status='pending',error_code='',updated_at=NOW()
            FROM public_knowledge_units u,public_knowledge_revisions r
            WHERE u.public_knowledge_id=c.public_knowledge_id AND u.current_revision=c.revision
              AND r.public_knowledge_id=c.public_knowledge_id AND r.revision=c.revision
              AND r.validation_state='platform_certified' AND u.publication_state IN ('published','pending_review')`)
		return true, repository.finishJob(ctx, job.ID, err)
	}
	if job.Mode != "build_candidates" && job.Mode != "rebuild" {
		err = fmt.Errorf("unsupported public knowledge job mode: %s", job.Mode)
		return true, repository.finishJob(ctx, job.ID, err)
	}
	privateItems, err := repository.eligiblePrivateKnowledge(ctx, job.LastTenantID, job.LastKnowledgeID, limit)
	if err != nil {
		return true, repository.finishJob(ctx, job.ID, err)
	}
	candidates, conflicts, failed := int64(0), int64(0), int64(0)
	for _, item := range privateItems {
		candidate, buildErr := BuildCandidate(item)
		if buildErr != nil {
			failed++
			continue
		}
		createdConflict, persistErr := repository.persistCandidate(ctx, candidate)
		if persistErr != nil {
			failed++
			continue
		}
		candidates++
		if createdConflict {
			conflicts++
		}
	}
	lastTenant, lastKnowledge := job.LastTenantID, job.LastKnowledgeID
	if len(privateItems) > 0 {
		last := privateItems[len(privateItems)-1]
		lastTenant, lastKnowledge = last.SourceTenantID, last.KnowledgeID
	}
	state := "pending"
	var completed any
	if len(privateItems) < limit {
		state, completed = "completed", "now"
	}
	_, err = repository.pool.Exec(ctx, `UPDATE public_knowledge_jobs SET state=$2,last_tenant_id=$3,last_knowledge_id=$4,scanned_count=scanned_count+$5,candidate_count=candidate_count+$6,conflict_count=conflict_count+$7,failed_count=failed_count+$8,error_code=CASE WHEN $8>0 THEN 'candidate_build_partial_failure' ELSE '' END,updated_at=NOW(),completed_at=CASE WHEN $9::text='now' THEN NOW() ELSE NULL END WHERE id=$1`, job.ID, state, lastTenant, lastKnowledge, len(privateItems), candidates, conflicts, failed, completed)
	return true, err
}

func (repository *Repository) claimJob(ctx context.Context) (Job, error) {
	var job Job
	err := repository.pool.QueryRow(ctx, `WITH candidate AS (
        SELECT id FROM public_knowledge_jobs WHERE state='pending' ORDER BY created_at,id FOR UPDATE SKIP LOCKED LIMIT 1
    ) UPDATE public_knowledge_jobs j SET state='running',updated_at=NOW() FROM candidate c WHERE j.id=c.id
      RETURNING j.id,j.mode,j.state,j.last_tenant_id,j.last_knowledge_id,j.scanned_count,j.candidate_count,j.conflict_count,j.failed_count,j.error_code,j.created_by,j.created_at,j.updated_at,j.completed_at`).Scan(
		&job.ID, &job.Mode, &job.State, &job.LastTenantID, &job.LastKnowledgeID, &job.ScannedCount, &job.CandidateCount, &job.ConflictCount, &job.FailedCount, &job.ErrorCode, &job.CreatedBy, &job.CreatedAt, &job.UpdatedAt, &job.CompletedAt,
	)
	return job, err
}

func (repository *Repository) finishJob(ctx context.Context, id string, jobErr error) error {
	if jobErr != nil {
		_, _ = repository.pool.Exec(ctx, `UPDATE public_knowledge_jobs SET state='failed',failed_count=failed_count+1,error_code='public_knowledge_job_failed',updated_at=NOW(),completed_at=NOW() WHERE id=$1`, id)
		return jobErr
	}
	_, err := repository.pool.Exec(ctx, `UPDATE public_knowledge_jobs SET state='completed',error_code='',updated_at=NOW(),completed_at=NOW() WHERE id=$1`, id)
	return err
}

func (repository *Repository) eligiblePrivateKnowledge(ctx context.Context, tenantCursor, knowledgeCursor string, limit int) ([]PrivateKnowledge, error) {
	rows, err := repository.pool.Query(ctx, `SELECT u.tenant_id,u.knowledge_id,u.revision,u.session_id,u.topic,u.knowledge_type,u.problem,u.conclusion,u.rationale,u.applicability,u.caveats,u.alternatives,u.decision_state,u.validation_state,COALESCE(lp.display_name,'')
        FROM process_knowledge_units u
        JOIN process_knowledge_state state ON state.tenant_id=u.tenant_id AND state.active_version=u.version
        LEFT JOIN logical_projects lp ON lp.tenant_id=u.tenant_id AND lp.id=u.logical_project_id
        WHERE u.lifecycle_state='active' AND (u.validation_state='verified' OR u.decision_state='accepted')
          AND (u.tenant_id>$1 OR (u.tenant_id=$1 AND u.knowledge_id>$2))
        ORDER BY u.tenant_id,u.knowledge_id LIMIT $3`, tenantCursor, knowledgeCursor, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]PrivateKnowledge, 0)
	for rows.Next() {
		var item PrivateKnowledge
		var projectName string
		if err = rows.Scan(&item.SourceTenantID, &item.KnowledgeID, &item.Revision, &item.SessionID, &item.Topic, &item.KnowledgeType, &item.Problem, &item.Conclusion, &item.Rationale, &item.Applicability, &item.Caveats, &item.Alternatives, &item.DecisionState, &item.ValidationState, &projectName); err != nil {
			return nil, err
		}
		item.SourceContentHash = hashText(item.Problem + "\x00" + item.Conclusion + "\x00" + item.Rationale)
		if projectName != "" {
			item.SensitiveTerms = []string{projectName}
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (repository *Repository) persistCandidate(ctx context.Context, candidate Candidate) (bool, error) {
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	unit, revision, source := candidate.Unit, candidate.Revision, candidate.Source
	_, err = tx.Exec(ctx, `INSERT INTO public_knowledge_units(public_knowledge_id,canonical_topic,knowledge_type,publication_state,current_revision)
        VALUES($1,$2,$3,$4,$5) ON CONFLICT(public_knowledge_id) DO UPDATE SET updated_at=NOW()`, unit.ID, unit.CanonicalTopic, unit.KnowledgeType, unit.PublicationState, unit.CurrentRevision)
	if err != nil {
		return false, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO public_knowledge_revisions(public_knowledge_id,revision,problem_pattern,conclusion,rationale,applicability,caveats,alternatives,validation_state,anonymous_source_tenant_count,independent_session_count,canonical_hash,extractor_version,redaction_version,review_policy_version)
        VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,1,1,$10,$11,$12,$13) ON CONFLICT(public_knowledge_id,revision) DO NOTHING`, unit.ID, revision.Revision, revision.ProblemPattern, revision.Conclusion, revision.Rationale, revision.Applicability, revision.Caveats, revision.Alternatives, revision.ValidationState, revision.CanonicalHash, revision.ExtractorVersion, revision.RedactionVersion, revision.ReviewPolicyVersion)
	if err != nil {
		return false, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO public_knowledge_sources(source_id,public_knowledge_id,public_revision,source_tenant_id,source_knowledge_id,source_revision,relation,independence_group,source_content_hash,external_source_hash,import_batch_id,redaction_state)
        VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12) ON CONFLICT DO NOTHING`, source.ID, source.PublicKnowledgeID, source.PublicRevision, source.SourceTenantID, source.SourceKnowledgeID, source.SourceRevision, source.Relation, source.IndependenceGroup, source.SourceContentHash, source.ExternalSourceHash, source.ImportBatchID, source.RedactionState)
	if err != nil {
		return false, err
	}
	_, err = tx.Exec(ctx, `UPDATE public_knowledge_revisions r SET
        anonymous_source_tenant_count=counts.tenant_count,
        independent_session_count=counts.independent_count,
        validation_state=CASE WHEN r.validation_state='platform_certified' THEN r.validation_state WHEN counts.tenant_count>=2 AND counts.independent_count>=2 THEN 'cross_tenant_corroborated' ELSE r.validation_state END
        FROM (SELECT COUNT(DISTINCT source_tenant_id) tenant_count,COUNT(DISTINCT independence_group) independent_count FROM public_knowledge_sources WHERE public_knowledge_id=$1 AND public_revision=$2 AND relation='supports' AND redaction_state='passed') counts
        WHERE r.public_knowledge_id=$1 AND r.revision=$2`, unit.ID, revision.Revision)
	if err != nil {
		return false, err
	}
	var otherID string
	err = tx.QueryRow(ctx, `SELECT public_knowledge_id FROM public_knowledge_units WHERE canonical_topic=$1 AND public_knowledge_id<>$2 AND publication_state<>'withdrawn' ORDER BY updated_at DESC LIMIT 1`, unit.CanonicalTopic, unit.ID).Scan(&otherID)
	createdConflict := false
	if err == nil {
		left, right := unit.ID, otherID
		if left > right {
			left, right = right, left
		}
		conflictID := "public-conflict-" + hashText(left + "\x00" + right)[:32]
		command, commandErr := tx.Exec(ctx, `INSERT INTO public_knowledge_conflicts(conflict_id,left_public_knowledge_id,left_revision,right_public_knowledge_id,right_revision,conflict_kind,safe_summary)
			SELECT $1,left_unit.public_knowledge_id,left_unit.current_revision,right_unit.public_knowledge_id,right_unit.current_revision,'conclusion','同一主题存在不同规范结论，请核对适用条件。'
			FROM public_knowledge_units left_unit JOIN public_knowledge_units right_unit ON right_unit.public_knowledge_id=$3 WHERE left_unit.public_knowledge_id=$2
			ON CONFLICT(conflict_id) DO NOTHING`, conflictID, left, right)
		if commandErr != nil {
			return false, commandErr
		}
		createdConflict = command.RowsAffected() > 0
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return false, err
	}
	return createdConflict, tx.Commit(ctx)
}

var _ Processor = (*Repository)(nil)
var _ IndexRepository = (*Repository)(nil)
var _ SearchRepository = (*Repository)(nil)
