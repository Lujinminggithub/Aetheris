package events

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type InsertStatus string

const (
	StatusAccepted   InsertStatus = "accepted"
	StatusDuplicate  InsertStatus = "duplicate"
	StatusTombstoned InsertStatus = "tombstoned"
)

type Scope struct {
	TenantID  string
	ProjectID string
}

type Filter struct {
	Limit        int
	ProjectID    string
	DeviceID     string
	SubjectID    string
	WorkRoleCode string
	EventType    string
}

type Repository struct{ pool *pgxpool.Pool }

type projectRegistration struct{ ID, Name, RootHint string }

func projectRecord(event Event) projectRegistration {
	name := event.ProjectID
	if label, ok := event.Payload["project_label"].(string); ok && strings.TrimSpace(label) != "" {
		name = strings.TrimSpace(label)
	}
	return projectRegistration{ID: event.ProjectID, Name: name, RootHint: ""}
}

func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

func (repository *Repository) Insert(ctx context.Context, tenantID string, event Event) (InsertStatus, error) {
	if tenantID == "" || tenantID != event.TenantID {
		return "", fmt.Errorf("event tenant does not match credential")
	}
	var tombstoned bool
	if err := repository.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM event_tombstones WHERE tenant_id=$1 AND event_id=$2)`, tenantID, event.EventID).Scan(&tombstoned); err != nil {
		return "", err
	}
	if tombstoned {
		return StatusTombstoned, nil
	}
	project := projectRecord(event)
	if _, err := repository.pool.Exec(ctx, `INSERT INTO projects(id,tenant_id,name,root_hint) VALUES($1,$2,$3,$4) ON CONFLICT(id) DO NOTHING`, project.ID, tenantID, project.Name, project.RootHint); err != nil {
		return "", err
	}
	payload, _ := json.Marshal(event.Payload)
	redaction, _ := json.Marshal(event.RedactionReport)
	provenance, _ := json.Marshal(event.Provenance)
	refs, _ := json.Marshal(event.ContentRefs)
	roleID, roleCode, roleVersion, roleSource := "", "", 0, ""
	if event.WorkRole != nil {
		roleID, roleCode, roleVersion, roleSource = event.WorkRole.RoleID, event.WorkRole.Code, event.WorkRole.Version, event.WorkRole.Source
	}
	var inserted bool
	err := repository.pool.QueryRow(ctx, `
        INSERT INTO events(event_id, tenant_id, subject_id, device_id, project_id, session_id, correlation_id,
            event_type, schema_version, source, source_version, occurred_at, ingested_at, payload, content_hash,
            content_refs, provenance, redaction_report, processing_grants, crypto_mode, key_version,
            work_role_id, work_role_code, work_role_version, role_source, supersedes_event_id)
        VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26)
        ON CONFLICT (event_id) DO NOTHING RETURNING TRUE`,
		event.EventID, tenantID, event.SubjectID, event.DeviceID, event.ProjectID, event.SessionID, event.CorrelationID,
		event.EventType, event.SchemaVersion, event.Source, event.SourceVersion, event.OccurredAt, event.IngestedAt,
		payload, event.ContentHash, refs, provenance, redaction, event.ProcessingGrants, event.CryptoMode, event.KeyVersion,
		roleID, roleCode, roleVersion, roleSource, event.SupersedesEventID).Scan(&inserted)
	if err == pgx.ErrNoRows {
		return StatusDuplicate, nil
	}
	if err != nil {
		return "", err
	}
	if inserted {
		return StatusAccepted, nil
	}
	return StatusDuplicate, nil
}

func (repository *Repository) List(ctx context.Context, scope Scope, filter Filter) ([]Event, error) {
	limit := filter.Limit
	if limit < 1 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}
	query := `SELECT event_json FROM (SELECT jsonb_build_object(
        'event_id', event_id, 'schema_version', schema_version, 'event_type', event_type,
        'tenant_id', tenant_id, 'subject_id', subject_id, 'device_id', device_id, 'project_id', project_id,
        'session_id', session_id, 'correlation_id', correlation_id, 'source', source, 'source_version', source_version,
        'occurred_at', occurred_at, 'ingested_at', ingested_at, 'payload', payload, 'content_hash', content_hash,
        'content_refs', content_refs, 'provenance', provenance, 'redaction_report', redaction_report,
		'processing_grants', processing_grants, 'crypto_mode', crypto_mode, 'key_version', key_version,
		'supersedes_event_id', supersedes_event_id, 'work_role', CASE WHEN work_role_id IS NULL THEN NULL ELSE jsonb_build_object('role_id', work_role_id, 'code', work_role_code, 'version', work_role_version, 'source', role_source) END) AS event_json, event_type, occurred_at, project_id, device_id, subject_id
        FROM events WHERE tenant_id=$1`
	args := []any{scope.TenantID}
	arg := 2
	for _, clause := range []struct{ value, sql string }{{filter.ProjectID, "project_id=$ARG"}, {filter.DeviceID, "device_id=$ARG"}, {filter.SubjectID, "subject_id=$ARG"}, {filter.WorkRoleCode, "work_role_code=$ARG"}, {filter.EventType, "event_type=$ARG"}} {
		if clause.value != "" {
			query += " AND " + replaceArg(clause.sql, arg)
			args = append(args, clause.value)
			arg++
		}
	}
	query += ` ORDER BY occurred_at DESC LIMIT $ARG) q`
	query = replaceArg(query, arg)
	args = append(args, limit)
	rows, err := repository.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Event{}
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var event Event
		if err := json.Unmarshal(raw, &event); err != nil {
			return nil, err
		}
		result = append(result, event)
	}
	return result, rows.Err()
}

func (repository *Repository) Get(ctx context.Context, scope Scope, eventID string) (Event, error) {
	row := repository.pool.QueryRow(ctx, `SELECT jsonb_build_object(
        'event_id', event_id, 'schema_version', schema_version, 'event_type', event_type,
        'tenant_id', tenant_id, 'subject_id', subject_id, 'device_id', device_id, 'project_id', project_id,
        'session_id', session_id, 'correlation_id', correlation_id, 'source', source, 'source_version', source_version,
        'occurred_at', occurred_at, 'ingested_at', ingested_at, 'payload', payload, 'content_hash', content_hash,
        'content_refs', content_refs, 'provenance', provenance, 'redaction_report', redaction_report,
        'processing_grants', processing_grants, 'crypto_mode', crypto_mode, 'key_version', key_version,
        'supersedes_event_id', supersedes_event_id, 'work_role', CASE WHEN work_role_id IS NULL THEN NULL ELSE jsonb_build_object('role_id', work_role_id, 'code', work_role_code, 'version', work_role_version, 'source', role_source) END)
        FROM events WHERE tenant_id=$1 AND event_id=$2`, scope.TenantID, eventID)
	var raw []byte
	if err := row.Scan(&raw); err != nil {
		return Event{}, err
	}
	var event Event
	if err := json.Unmarshal(raw, &event); err != nil {
		return Event{}, err
	}
	return event, nil
}

func (repository *Repository) Tombstone(ctx context.Context, scope Scope, eventID, reason, actorID string) error {
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM events WHERE tenant_id=$1 AND event_id=$2`, scope.TenantID, eventID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO event_tombstones(event_id, tenant_id, reason, deleted_at, deleted_by) VALUES($1,$2,$3,$4,$5) ON CONFLICT(event_id) DO UPDATE SET reason=EXCLUDED.reason, deleted_at=EXCLUDED.deleted_at, deleted_by=EXCLUDED.deleted_by`, eventID, scope.TenantID, reason[:min(256, len(reason))], time.Now().UTC(), actorID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO audit_logs(tenant_id, actor_type, actor_id, action, resource_type, resource_id, scope, outcome) VALUES($1,'user',$2,'event.tombstone','event',$3,$4,'success')`, scope.TenantID, actorID, eventID, map[string]string{"project_id": scope.ProjectID}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func replaceArg(input string, number int) string {
	return strings.ReplaceAll(input, "$ARG", fmt.Sprintf("$%d", number))
}
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
