package activities

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct{ pool *pgxpool.Pool }

func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

const activityWhere = `f.tenant_id=$1 AND f.rule_version=(SELECT MAX(rule_version) FROM clean_event_facts WHERE tenant_id=$1)
	 AND f.occurred_at >= $2 AND f.occurred_at < $3 AND f.quality_state IN ('accepted','merged') AND NOT f.excluded_from_effectiveness
	 AND f.fact_type <> 'command_fragment' AND ($4='' OR f.device_id=$4) AND ($5='' OR COALESCE(ep.logical_project_id,f.project_id)=$5)
	 AND ($6='' OR f.activity_type=$6) AND ($7='' OR f.message_role=$7)`

func (repository *Repository) Query(ctx context.Context, filter Filter) (Result, error) {
	result := Result{Devices: []Facet{}, Projects: []Facet{}, Activities: []Activity{}, ActivityCounts: map[string]int{"ai": 0, "terminal": 0, "ide": 0, "delivery": 0, "browser": 0, "version_control": 0, "other": 0}, Limit: filter.Limit, Offset: filter.Offset}
	args := []any{filter.TenantID, filter.From, filter.ToExclusive, filter.DeviceID, filter.ProjectID, filter.ActivityType, filter.MessageRole}
	if err := repository.pool.QueryRow(ctx, `SELECT COUNT(*) FROM clean_event_facts f LEFT JOIN current_event_projects ep ON ep.tenant_id=f.tenant_id AND ep.event_id=f.canonical_event_id WHERE `+activityWhere, args...).Scan(&result.Total); err != nil {
		return Result{}, err
	}

	deviceRows, err := repository.pool.Query(ctx, `SELECT f.device_id,d.hostname,COUNT(*),MAX(f.occurred_at) FROM clean_event_facts f JOIN devices d ON d.tenant_id=f.tenant_id AND d.id=f.device_id LEFT JOIN current_event_projects ep ON ep.tenant_id=f.tenant_id AND ep.event_id=f.canonical_event_id WHERE `+activityWhere+` GROUP BY f.device_id,d.hostname ORDER BY COUNT(*) DESC,d.hostname`, args...)
	if err != nil {
		return Result{}, err
	}
	for deviceRows.Next() {
		var item Facet
		if err := deviceRows.Scan(&item.ID, &item.Label, &item.Count, &item.LastActivityAt); err != nil {
			deviceRows.Close()
			return Result{}, err
		}
		result.Devices = append(result.Devices, item)
	}
	if err := deviceRows.Err(); err != nil {
		deviceRows.Close()
		return Result{}, err
	}
	deviceRows.Close()

	projectRows, err := repository.pool.Query(ctx, `SELECT COALESCE(ep.logical_project_id,f.project_id),'',MAX(COALESCE(ep.project_name,'未归属项目')),COUNT(*),MAX(f.occurred_at) FROM clean_event_facts f LEFT JOIN current_event_projects ep ON ep.tenant_id=f.tenant_id AND ep.event_id=f.canonical_event_id WHERE `+activityWhere+` GROUP BY COALESCE(ep.logical_project_id,f.project_id) ORDER BY COUNT(*) DESC,COALESCE(ep.logical_project_id,f.project_id)`, args...)
	if err != nil {
		return Result{}, err
	}
	for projectRows.Next() {
		var item Facet
		var hint, stored string
		if err := projectRows.Scan(&item.ID, &hint, &stored, &item.Count, &item.LastActivityAt); err != nil {
			projectRows.Close()
			return Result{}, err
		}
		item.Label = ProjectLabel("", hint, stored, item.ID)
		result.Projects = append(result.Projects, item)
	}
	if err := projectRows.Err(); err != nil {
		projectRows.Close()
		return Result{}, err
	}
	projectRows.Close()

	countRows, err := repository.pool.Query(ctx, `SELECT f.activity_type,COUNT(*) FROM clean_event_facts f LEFT JOIN current_event_projects ep ON ep.tenant_id=f.tenant_id AND ep.event_id=f.canonical_event_id WHERE `+activityWhere+` GROUP BY f.activity_type`, args...)
	if err != nil {
		return Result{}, err
	}
	for countRows.Next() {
		var key string
		var count int
		if err := countRows.Scan(&key, &count); err != nil {
			countRows.Close()
			return Result{}, err
		}
		result.ActivityCounts[key] = count
	}
	if err := countRows.Err(); err != nil {
		countRows.Close()
		return Result{}, err
	}
	countRows.Close()

	rows, err := repository.pool.Query(ctx, `SELECT f.fact_id,f.canonical_event_id,f.source_event_ids,f.device_id,d.hostname,COALESCE(ep.logical_project_id,f.project_id),COALESCE(ep.project_name,'未归属项目'),
	 COALESCE(e.payload->>'project_label',''),COALESCE(NULLIF(e.payload->>'project',''),NULLIF(e.payload->>'project_path',''),NULLIF(e.payload->>'cwd',''),NULLIF(e.provenance->>'project_root',''),''),
 f.activity_type,f.event_type,f.source,f.actor_origin,f.message_role,f.ai_tool,f.command_summary,f.command_text,f.occurred_at,COALESCE(e.payload,'{}'::jsonb)
	 FROM clean_event_facts f JOIN devices d ON d.tenant_id=f.tenant_id AND d.id=f.device_id
	 LEFT JOIN events e ON e.tenant_id=f.tenant_id AND e.event_id=f.canonical_event_id
	 LEFT JOIN current_event_projects ep ON ep.tenant_id=f.tenant_id AND ep.event_id=f.canonical_event_id WHERE `+activityWhere+`
 ORDER BY f.occurred_at DESC,f.fact_id DESC LIMIT $8 OFFSET $9`, append(args, filter.Limit, filter.Offset)...)
	if err != nil {
		return Result{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var item Activity
		var safeLabel, hint, storedName, commandSummary, commandText string
		var payloadRaw []byte
		if err := rows.Scan(&item.FactID, &item.CanonicalEventID, &item.SourceEventIDs, &item.DeviceID, &item.DeviceName, &item.ProjectID, &storedName, &safeLabel, &hint, &item.ActivityType, &item.EventType, &item.Source, &item.ActorOrigin, &item.MessageRole, &item.Tool, &commandSummary, &commandText, &item.OccurredAt, &payloadRaw); err != nil {
			return Result{}, err
		}
		payload := map[string]any{}
		_ = json.Unmarshal(payloadRaw, &payload)
		if strings.TrimSpace(storedName) != "" {
			item.ProjectName = storedName
		} else {
			item.ProjectName = ProjectLabel(safeLabel, hint, storedName, item.ProjectID)
		}
		item.Preview = Preview(item.EventType, item.ActorOrigin, commandSummary, commandText, payload)
		result.Activities = append(result.Activities, item)
	}
	return result, rows.Err()
}

func ValidateRange(from, to time.Time) bool {
	return !to.Before(from) && int(to.Sub(from).Hours()/24)+1 <= 90
}
