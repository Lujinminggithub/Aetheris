package projectattribution

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct{ pool *pgxpool.Pool }

func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

func (repository *Repository) CreateJob(ctx context.Context, tenantID, actorID, mode string, ruleVersion int) (Job, error) {
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return Job{}, err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `INSERT INTO project_attribution_versions(tenant_id,rule_version,state,created_by)
		VALUES($1,$2,'calculating',$3)
		ON CONFLICT(tenant_id,rule_version) DO UPDATE SET state=CASE WHEN project_attribution_versions.state='active' THEN 'active' ELSE 'calculating' END,completed_at=NULL`, tenantID, ruleVersion, actorID); err != nil {
		return Job{}, err
	}
	id, err := newJobID()
	if err != nil {
		return Job{}, err
	}
	job := Job{ID: id, TenantID: tenantID, Mode: mode, RuleVersion: ruleVersion, State: "pending", CreatedBy: actorID}
	if err := tx.QueryRow(ctx, `INSERT INTO project_backfill_jobs(id,tenant_id,mode,rule_version,state,created_by)
		VALUES($1,$2,$3,$4,'pending',$5)
		RETURNING created_at,updated_at`, id, tenantID, mode, ruleVersion, actorID).Scan(&job.CreatedAt, &job.UpdatedAt); err != nil {
		return Job{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Job{}, err
	}
	return job, nil
}

func (repository *Repository) GetJob(ctx context.Context, tenantID, jobID string) (Job, error) {
	row := repository.pool.QueryRow(ctx, `SELECT id,tenant_id,mode,rule_version,state,scanned_count,assigned_count,unresolved_count,conflict_count,last_event_id,error_code,created_by,created_at,updated_at,completed_at
		FROM project_backfill_jobs WHERE tenant_id=$1 AND id=$2`, tenantID, jobID)
	return scanJob(row)
}

func (repository *Repository) Activate(ctx context.Context, tenantID string, ruleVersion int) error {
	return repository.switchVersion(ctx, tenantID, ruleVersion, false)
}

func (repository *Repository) Rollback(ctx context.Context, tenantID string, ruleVersion int) error {
	return repository.switchVersion(ctx, tenantID, ruleVersion, true)
}

func (repository *Repository) switchVersion(ctx context.Context, tenantID string, ruleVersion int, rollback bool) error {
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var state string
	if err := tx.QueryRow(ctx, `SELECT state FROM project_attribution_versions WHERE tenant_id=$1 AND rule_version=$2 FOR UPDATE`, tenantID, ruleVersion).Scan(&state); err != nil {
		return err
	}
	if state != "completed" && state != "active" && state != "rolled_back" {
		return fmt.Errorf("归属版本尚未完成")
	}
	var current *int
	_ = tx.QueryRow(ctx, `SELECT active_rule_version FROM project_attribution_state WHERE tenant_id=$1 FOR UPDATE`, tenantID).Scan(&current)
	if current != nil && *current != ruleVersion {
		previousState := "completed"
		if rollback {
			previousState = "rolled_back"
		}
		if _, err := tx.Exec(ctx, `UPDATE project_attribution_versions SET state=$3 WHERE tenant_id=$1 AND rule_version=$2`, tenantID, *current, previousState); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE project_attribution_versions SET state='active',activated_at=NOW() WHERE tenant_id=$1 AND rule_version=$2`, tenantID, ruleVersion); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO project_attribution_state(tenant_id,active_rule_version,previous_rule_version,updated_at)
		VALUES($1,$2,$3,NOW()) ON CONFLICT(tenant_id) DO UPDATE SET previous_rule_version=project_attribution_state.active_rule_version,active_rule_version=EXCLUDED.active_rule_version,updated_at=NOW()`, tenantID, ruleVersion, current); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (repository *Repository) ProcessNext(ctx context.Context, batchSize int) (bool, error) {
	if batchSize < 1 {
		batchSize = 10000
	}
	row := repository.pool.QueryRow(ctx, `WITH candidate AS (
		SELECT id FROM project_backfill_jobs WHERE state='pending' ORDER BY created_at,id FOR UPDATE SKIP LOCKED LIMIT 1
	) UPDATE project_backfill_jobs j SET state='running',updated_at=NOW() FROM candidate c WHERE j.id=c.id
	RETURNING j.id,j.tenant_id,j.mode,j.rule_version,j.state,j.scanned_count,j.assigned_count,j.unresolved_count,j.conflict_count,j.last_event_id,j.error_code,j.created_by,j.created_at,j.updated_at,j.completed_at`)
	job, err := scanJob(row)
	if err == pgx.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if err := repository.processJob(ctx, job, batchSize); err != nil {
		_, _ = repository.pool.Exec(ctx, `UPDATE project_backfill_jobs SET state='failed',error_code=$2,updated_at=NOW(),completed_at=NOW() WHERE id=$1`, job.ID, "backfill_processing_failed")
		_, _ = repository.pool.Exec(ctx, `UPDATE project_attribution_versions SET state='failed',completed_at=NOW() WHERE tenant_id=$1 AND rule_version=$2 AND state<>'active'`, job.TenantID, job.RuleVersion)
		return true, err
	}
	return true, nil
}

func (repository *Repository) processJob(ctx context.Context, job Job, batchSize int) error {
	for {
		rows, err := repository.loadBatch(ctx, job.TenantID, job.LastEventID, batchSize)
		if err != nil {
			return err
		}
		if len(rows) == 0 {
			_, err := repository.pool.Exec(ctx, `UPDATE project_backfill_jobs SET state='completed',updated_at=NOW(),completed_at=NOW() WHERE id=$1`, job.ID)
			if err == nil {
				_, err = repository.pool.Exec(ctx, `UPDATE project_attribution_versions SET state='completed',completed_at=NOW() WHERE tenant_id=$1 AND rule_version=$2 AND state<>'active'`, job.TenantID, job.RuleVersion)
			}
			return err
		}
		attributions := ResolveBatch(rows)
		if err := repository.saveBatch(ctx, job, rows, attributions); err != nil {
			return err
		}
		job.LastEventID = rows[len(rows)-1].Evidence.EventID
		for _, attribution := range attributions {
			job.ScannedCount++
			if attribution.LogicalProjectID != "" {
				job.AssignedCount++
			} else if len(attribution.Method) >= 9 && attribution.Method[:9] == "ambiguous" {
				job.ConflictCount++
			} else {
				job.UnresolvedCount++
			}
		}
	}
}

func (repository *Repository) loadBatch(ctx context.Context, tenantID, afterEventID string, batchSize int) ([]BatchEvidence, error) {
	rows, err := repository.pool.Query(ctx, `SELECT e.event_id,e.device_id,e.project_id,e.session_id,e.source,
		COALESCE(NULLIF(e.payload->>'project_label',''),NULLIF(e.payload->>'project',''),''),
		l.logical_project_id,l.id,
		COALESCE(labels.ids,ARRAY[]::TEXT[]),
		inherited.logical_project_id,inherited.project_location_id
		FROM events e
		LEFT JOIN project_locations l ON l.tenant_id=e.tenant_id AND l.device_id=e.device_id AND l.local_project_id=e.project_id
		LEFT JOIN project_attribution_state attribution_state ON attribution_state.tenant_id=e.tenant_id
		LEFT JOIN project_attributions inherited ON inherited.tenant_id=e.tenant_id
			AND inherited.event_id=e.supersedes_event_id
			AND inherited.rule_version=attribution_state.active_rule_version
		LEFT JOIN LATERAL (
			SELECT ARRAY_AGG(p.id ORDER BY p.id) ids FROM logical_projects p
			WHERE p.tenant_id=e.tenant_id
			AND LOWER(p.display_name)=LOWER(COALESCE(NULLIF(e.payload->>'project_label',''),NULLIF(e.payload->>'project',''),''))
			AND COALESCE(NULLIF(e.payload->>'project_label',''),NULLIF(e.payload->>'project',''),'') NOT IN ('Codex','Claude Code','Cursor','GitHub Copilot')
		) labels ON TRUE
		WHERE e.tenant_id=$1 AND e.event_id>$2 ORDER BY e.event_id LIMIT $3`, tenantID, afterEventID, batchSize)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []BatchEvidence{}
	for rows.Next() {
		var item BatchEvidence
		var exactProject, exactLocation, inheritedProject, inheritedLocation *string
		var labels []string
		if err := rows.Scan(&item.Evidence.EventID, &item.Evidence.DeviceID, &item.Evidence.LegacyProjectID, &item.Evidence.SessionID, &item.Evidence.Source, &item.Evidence.ProjectLabel, &exactProject, &exactLocation, &labels, &inheritedProject, &inheritedLocation); err != nil {
			return nil, err
		}
		if exactProject != nil {
			item.Candidates.Exact = []Candidate{{LogicalProjectID: *exactProject, LocationID: valueOrEmpty(exactLocation)}}
		}
		for _, id := range labels {
			item.Candidates.Label = append(item.Candidates.Label, Candidate{LogicalProjectID: id})
		}
		if inheritedProject != nil {
			item.Candidates.Inherited = []Candidate{{LogicalProjectID: *inheritedProject, LocationID: valueOrEmpty(inheritedLocation)}}
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (repository *Repository) saveBatch(ctx context.Context, job Job, rows []BatchEvidence, attributions []Attribution) error {
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if job.Mode == "apply" {
		batch := &pgx.Batch{}
		for index, attribution := range attributions {
			evidence, _ := json.Marshal(attribution.Evidence)
			batch.Queue(`INSERT INTO project_attributions(tenant_id,event_id,rule_version,logical_project_id,project_location_id,method,confidence,evidence,needs_review)
				VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)
				ON CONFLICT(tenant_id,event_id,rule_version) DO UPDATE SET logical_project_id=EXCLUDED.logical_project_id,project_location_id=EXCLUDED.project_location_id,method=EXCLUDED.method,confidence=EXCLUDED.confidence,evidence=EXCLUDED.evidence,needs_review=EXCLUDED.needs_review`,
				job.TenantID, rows[index].Evidence.EventID, job.RuleVersion, nullableString(attribution.LogicalProjectID), nullableString(attribution.LocationID), attribution.Method, attribution.Confidence, evidence, attribution.NeedsReview)
		}
		results := tx.SendBatch(ctx, batch)
		for range attributions {
			if _, err := results.Exec(); err != nil {
				_ = results.Close()
				return err
			}
		}
		if err := results.Close(); err != nil {
			return err
		}
	}
	var assigned, unresolved, conflicts int64
	for _, attribution := range attributions {
		if attribution.LogicalProjectID != "" {
			assigned++
		} else if len(attribution.Method) >= 9 && attribution.Method[:9] == "ambiguous" {
			conflicts++
		} else {
			unresolved++
		}
	}
	_, err = tx.Exec(ctx, `UPDATE project_backfill_jobs SET scanned_count=scanned_count+$2,assigned_count=assigned_count+$3,
		unresolved_count=unresolved_count+$4,conflict_count=conflict_count+$5,last_event_id=$6,updated_at=NOW() WHERE id=$1`,
		job.ID, len(rows), assigned, unresolved, conflicts, rows[len(rows)-1].Evidence.EventID)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

type rowScanner interface{ Scan(...any) error }

func scanJob(row rowScanner) (Job, error) {
	var job Job
	var completed *time.Time
	err := row.Scan(&job.ID, &job.TenantID, &job.Mode, &job.RuleVersion, &job.State, &job.ScannedCount, &job.AssignedCount,
		&job.UnresolvedCount, &job.ConflictCount, &job.LastEventID, &job.ErrorCode, &job.CreatedBy, &job.CreatedAt, &job.UpdatedAt, &completed)
	if completed != nil {
		job.CompletedAt = *completed
	}
	return job, err
}

func newJobID() (string, error) {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return fmt.Sprintf("project-backfill-%x", value), nil
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func valueOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
