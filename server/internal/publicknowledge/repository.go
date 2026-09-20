package publicknowledge

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const listUnitsSQL = `SELECT u.public_knowledge_id,u.canonical_topic,u.knowledge_type,u.publication_state,u.current_revision,
    r.problem_pattern,r.conclusion,r.rationale,r.applicability,r.caveats,r.alternatives,r.validation_state,
    r.anonymous_source_tenant_count,r.independent_session_count,r.canonical_hash,r.extractor_version,r.redaction_version,r.review_policy_version,
    r.created_at,u.created_at,u.updated_at
FROM public_knowledge_units u
JOIN public_knowledge_revisions r ON r.public_knowledge_id=u.public_knowledge_id AND r.revision=u.current_revision
WHERE ($1='' OR u.publication_state=$1) AND ($2='' OR r.validation_state=$2) AND ($3='' OR u.canonical_topic ILIKE $3)
ORDER BY u.updated_at DESC,u.public_knowledge_id LIMIT $4 OFFSET $5`

const countUnitsSQL = `SELECT COUNT(*) FROM public_knowledge_units u
JOIN public_knowledge_revisions r ON r.public_knowledge_id=u.public_knowledge_id AND r.revision=u.current_revision
WHERE ($1='' OR u.publication_state=$1) AND ($2='' OR r.validation_state=$2) AND ($3='' OR u.canonical_topic ILIKE $3)`

const getUnitSQL = `SELECT u.public_knowledge_id,u.canonical_topic,u.knowledge_type,u.publication_state,u.current_revision,
    r.problem_pattern,r.conclusion,r.rationale,r.applicability,r.caveats,r.alternatives,r.validation_state,
    r.anonymous_source_tenant_count,r.independent_session_count,r.canonical_hash,r.extractor_version,r.redaction_version,r.review_policy_version,
    r.created_at,u.created_at,u.updated_at
FROM public_knowledge_units u
JOIN public_knowledge_revisions r ON r.public_knowledge_id=u.public_knowledge_id AND r.revision=u.current_revision
WHERE u.public_knowledge_id=$1`

type Repository struct{ pool *pgxpool.Pool }

func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

func (repository *Repository) List(ctx context.Context, filter ListFilter) ([]Unit, int, error) {
	topic := ""
	if filter.Topic != "" {
		topic = "%" + filter.Topic + "%"
	}
	var total int
	if err := repository.pool.QueryRow(ctx, countUnitsSQL, filter.PublicationState, filter.ValidationState, topic).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := repository.pool.Query(ctx, listUnitsSQL, filter.PublicationState, filter.ValidationState, topic, filter.Limit, filter.Offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := make([]Unit, 0)
	for rows.Next() {
		item, scanErr := scanUnit(rows)
		if scanErr != nil {
			return nil, 0, scanErr
		}
		items = append(items, item)
	}
	return items, total, rows.Err()
}

func (repository *Repository) Get(ctx context.Context, id, viewerTenantID string) (Unit, error) {
	item, err := scanUnit(repository.pool.QueryRow(ctx, getUnitSQL, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Unit{}, ErrNotFound
	}
	if err != nil {
		return Unit{}, err
	}
	reviewRows, err := repository.pool.Query(ctx, `SELECT review_id,revision,action,actor_id,reason,created_at FROM public_knowledge_reviews WHERE public_knowledge_id=$1 ORDER BY created_at,review_id`, id)
	if err != nil {
		return Unit{}, err
	}
	for reviewRows.Next() {
		var review Review
		if err = reviewRows.Scan(&review.ID, &review.Revision, &review.Action, &review.ActorID, &review.Reason, &review.CreatedAt); err != nil {
			reviewRows.Close()
			return Unit{}, err
		}
		item.Reviews = append(item.Reviews, review)
	}
	err = reviewRows.Err()
	reviewRows.Close()
	if err != nil || viewerTenantID == "" {
		return item, err
	}
	evidenceRows, err := repository.pool.Query(ctx, `SELECT source_knowledge_id,source_revision,relation FROM public_knowledge_sources WHERE public_knowledge_id=$1 AND public_revision=$2 AND source_tenant_id=$3 ORDER BY source_knowledge_id,source_revision`, id, item.CurrentRevision, viewerTenantID)
	if err != nil {
		return Unit{}, err
	}
	defer evidenceRows.Close()
	for evidenceRows.Next() {
		var evidence PrivateEvidenceRef
		if err = evidenceRows.Scan(&evidence.KnowledgeID, &evidence.Revision, &evidence.Relation); err != nil {
			return Unit{}, err
		}
		item.PrivateEvidence = append(item.PrivateEvidence, evidence)
	}
	return item, evidenceRows.Err()
}

func (repository *Repository) ApplyReview(ctx context.Context, command ReviewCommand, transition Transition) (Unit, error) {
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return Unit{}, err
	}
	defer tx.Rollback(ctx)
	var currentRevision int
	var currentPublication string
	var currentValidation string
	err = tx.QueryRow(ctx, `SELECT u.current_revision,u.publication_state,r.validation_state FROM public_knowledge_units u JOIN public_knowledge_revisions r ON r.public_knowledge_id=u.public_knowledge_id AND r.revision=u.current_revision WHERE u.public_knowledge_id=$1 FOR UPDATE OF u,r`, command.KnowledgeID).Scan(&currentRevision, &currentPublication, &currentValidation)
	if errors.Is(err, pgx.ErrNoRows) {
		return Unit{}, ErrNotFound
	}
	if err != nil {
		return Unit{}, err
	}
	if currentRevision != command.ExpectedRevision {
		return Unit{}, ErrRevisionConflict
	}
	if _, err = tx.Exec(ctx, `UPDATE public_knowledge_revisions SET validation_state=$3 WHERE public_knowledge_id=$1 AND revision=$2`, command.KnowledgeID, currentRevision, transition.ValidationState); err != nil {
		return Unit{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE public_knowledge_units SET publication_state=$2,updated_at=NOW() WHERE public_knowledge_id=$1`, command.KnowledgeID, transition.PublicationState); err != nil {
		return Unit{}, err
	}
	reviewID, err := randomID("public-review")
	if err != nil {
		return Unit{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO public_knowledge_reviews(review_id,public_knowledge_id,revision,action,actor_id,actor_tenant_id,reason,validation_snapshot,publication_snapshot) VALUES($1,$2,$3,$4,$5,NULLIF($6,''),$7,$8,$9)`, reviewID, command.KnowledgeID, currentRevision, command.Action, command.ActorID, command.ActorTenantID, command.Reason, transition.ValidationState, transition.PublicationState); err != nil {
		return Unit{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Unit{}, err
	}
	return repository.Get(ctx, command.KnowledgeID, command.ActorTenantID)
}

func (repository *Repository) CreateJob(ctx context.Context, actorID, mode string) (Job, error) {
	id, err := randomID("public-knowledge-job")
	if err != nil {
		return Job{}, err
	}
	job := Job{ID: id, Mode: mode, State: "pending", CreatedBy: actorID}
	err = repository.pool.QueryRow(ctx, `INSERT INTO public_knowledge_jobs(id,mode,state,created_by) VALUES($1,$2,'pending',$3) RETURNING created_at,updated_at`, id, mode, actorID).Scan(&job.CreatedAt, &job.UpdatedAt)
	return job, err
}

func (repository *Repository) GetJob(ctx context.Context, id string) (Job, error) {
	var job Job
	err := repository.pool.QueryRow(ctx, `SELECT id,mode,state,last_tenant_id,last_knowledge_id,scanned_count,candidate_count,conflict_count,failed_count,error_code,created_by,created_at,updated_at,completed_at FROM public_knowledge_jobs WHERE id=$1`, id).Scan(
		&job.ID, &job.Mode, &job.State, &job.LastTenantID, &job.LastKnowledgeID, &job.ScannedCount, &job.CandidateCount, &job.ConflictCount, &job.FailedCount, &job.ErrorCode, &job.CreatedBy, &job.CreatedAt, &job.UpdatedAt, &job.CompletedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Job{}, ErrNotFound
	}
	return job, err
}

type rowScanner interface{ Scan(...any) error }

func scanUnit(row rowScanner) (Unit, error) {
	var item Unit
	err := row.Scan(
		&item.ID, &item.CanonicalTopic, &item.KnowledgeType, &item.PublicationState, &item.CurrentRevision,
		&item.Revision.ProblemPattern, &item.Revision.Conclusion, &item.Revision.Rationale, &item.Revision.Applicability, &item.Revision.Caveats, &item.Revision.Alternatives, &item.Revision.ValidationState,
		&item.Revision.AnonymousSourceTenantCount, &item.Revision.IndependentSessionCount, &item.Revision.CanonicalHash, &item.Revision.ExtractorVersion, &item.Revision.RedactionVersion, &item.Revision.ReviewPolicyVersion,
		&item.Revision.CreatedAt, &item.CreatedAt, &item.UpdatedAt,
	)
	item.Revision.Revision = item.CurrentRevision
	return item, err
}

func randomID(prefix string) (string, error) {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return "", fmt.Errorf("generate %s id: %w", prefix, err)
	}
	return prefix + "-" + hex.EncodeToString(value), nil
}
