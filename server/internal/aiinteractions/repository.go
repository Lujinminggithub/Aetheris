package aiinteractions

import (
	"context"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Filter struct {
	TenantID    string
	DeviceID    string
	ProjectID   string
	MessageRole string
	From        time.Time
	ToExclusive time.Time
	Limit       int
	Offset      int
}

type DeviceFacet struct {
	DeviceID          string    `json:"device_id"`
	DeviceName        string    `json:"device_name"`
	EventCount        int       `json:"event_count"`
	LastInteractionAt time.Time `json:"last_interaction_at"`
}

type ProjectFacet struct {
	ProjectID         string    `json:"project_id"`
	ProjectName       string    `json:"project_name"`
	EventCount        int       `json:"event_count"`
	LastInteractionAt time.Time `json:"last_interaction_at"`
}

type Interaction struct {
	EventID         string    `json:"event_id"`
	DeviceID        string    `json:"device_id"`
	DeviceName      string    `json:"device_name"`
	ProjectID       string    `json:"project_id"`
	ProjectName     string    `json:"project_name"`
	MessageRole     string    `json:"message_role"`
	Tool            string    `json:"tool"`
	OccurredAt      time.Time `json:"occurred_at"`
	ContentPreview  string    `json:"content_preview"`
	InteractionKind string    `json:"interaction_kind"`
	ActorOrigin     string    `json:"actor_origin"`
	CommandType     string    `json:"command_type,omitempty"`
	QualityState    string    `json:"quality_state,omitempty"`
	SourceEventIDs  []string  `json:"source_event_ids,omitempty"`
}

type Result struct {
	Devices      []DeviceFacet  `json:"devices"`
	Projects     []ProjectFacet `json:"projects"`
	RoleCounts   map[string]int `json:"role_counts"`
	Interactions []Interaction  `json:"interactions"`
	Total        int            `json:"total"`
	Limit        int            `json:"limit"`
	Offset       int            `json:"offset"`
}

type Querier interface {
	Query(context.Context, Filter) (Result, error)
}

type Repository struct{ pool *pgxpool.Pool }

func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

func ProjectLabel(pathHint, projectID string) string {
	trimmed := strings.TrimRight(strings.TrimSpace(pathHint), `\/`)
	if trimmed == "" {
		return projectID
	}
	if index := strings.LastIndexAny(trimmed, `\/`); index >= 0 && index+1 < len(trimmed) {
		return trimmed[index+1:]
	}
	if len(trimmed) == 2 && trimmed[1] == ':' {
		return projectID
	}
	return trimmed
}

func NormalizeMessageRole(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "user":
		return "user"
	case "assistant":
		return "assistant"
	case "system":
		return "system"
	case "tool":
		return "tool"
	case "ai_tool":
		return "ai_tool"
	default:
		return "unknown"
	}
}

const roleExpression = `CASE WHEN e.event_type='ai.tool_call' THEN 'ai_tool' ELSE CASE LOWER(COALESCE(e.payload->>'role','')) WHEN 'user' THEN 'user' WHEN 'assistant' THEN 'assistant' WHEN 'system' THEN 'system' WHEN 'tool' THEN 'tool' ELSE 'unknown' END END`
const projectHintExpression = `COALESCE(NULLIF(e.payload->>'project',''),NULLIF(e.payload->>'project_path',''),NULLIF(e.payload->>'cwd',''),'')`

func (repository *Repository) Query(ctx context.Context, filter Filter) (Result, error) {
	result := Result{
		Devices: []DeviceFacet{}, Projects: []ProjectFacet{}, Interactions: []Interaction{},
		RoleCounts: map[string]int{"user": 0, "assistant": 0, "ai_tool": 0, "system": 0, "tool": 0, "unknown": 0},
		Limit:      filter.Limit, Offset: filter.Offset,
	}
	deviceRows, err := repository.pool.Query(ctx, `SELECT e.device_id,d.hostname,COUNT(*),MAX(e.occurred_at)
        FROM events e JOIN devices d ON d.tenant_id=e.tenant_id AND d.id=e.device_id
        WHERE e.tenant_id=$1 AND e.event_type IN ('ai.message','ai.tool_call') AND e.occurred_at >= $2 AND e.occurred_at < $3
        GROUP BY e.device_id,d.hostname ORDER BY COUNT(*) DESC,d.hostname`, filter.TenantID, filter.From, filter.ToExclusive)
	if err != nil {
		return Result{}, err
	}
	for deviceRows.Next() {
		var item DeviceFacet
		if err := deviceRows.Scan(&item.DeviceID, &item.DeviceName, &item.EventCount, &item.LastInteractionAt); err != nil {
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

	projectRows, err := repository.pool.Query(ctx, `SELECT e.project_id,MAX(`+projectHintExpression+`),COUNT(*),MAX(e.occurred_at)
        FROM events e WHERE e.tenant_id=$1 AND e.event_type IN ('ai.message','ai.tool_call') AND e.occurred_at >= $2 AND e.occurred_at < $3
        AND ($4='' OR e.device_id=$4) GROUP BY e.project_id ORDER BY COUNT(*) DESC,e.project_id`,
		filter.TenantID, filter.From, filter.ToExclusive, filter.DeviceID)
	if err != nil {
		return Result{}, err
	}
	for projectRows.Next() {
		var item ProjectFacet
		var pathHint string
		if err := projectRows.Scan(&item.ProjectID, &pathHint, &item.EventCount, &item.LastInteractionAt); err != nil {
			projectRows.Close()
			return Result{}, err
		}
		item.ProjectName = ProjectLabel(pathHint, item.ProjectID)
		result.Projects = append(result.Projects, item)
	}
	if err := projectRows.Err(); err != nil {
		projectRows.Close()
		return Result{}, err
	}
	projectRows.Close()

	roleRows, err := repository.pool.Query(ctx, `SELECT `+roleExpression+`,COUNT(*) FROM events e
        WHERE e.tenant_id=$1 AND e.event_type IN ('ai.message','ai.tool_call') AND e.occurred_at >= $2 AND e.occurred_at < $3
        AND ($4='' OR e.device_id=$4) AND ($5='' OR e.project_id=$5) GROUP BY 1`,
		filter.TenantID, filter.From, filter.ToExclusive, filter.DeviceID, filter.ProjectID)
	if err != nil {
		return Result{}, err
	}
	for roleRows.Next() {
		var role string
		var count int
		if err := roleRows.Scan(&role, &count); err != nil {
			roleRows.Close()
			return Result{}, err
		}
		result.RoleCounts[NormalizeMessageRole(role)] = count
	}
	if err := roleRows.Err(); err != nil {
		roleRows.Close()
		return Result{}, err
	}
	roleRows.Close()

	baseArgs := []any{filter.TenantID, filter.From, filter.ToExclusive, filter.DeviceID, filter.ProjectID, filter.MessageRole}
	where := `e.tenant_id=$1 AND e.event_type IN ('ai.message','ai.tool_call') AND e.occurred_at >= $2 AND e.occurred_at < $3
        AND ($4='' OR e.device_id=$4) AND ($5='' OR e.project_id=$5) AND ($6='' OR ` + roleExpression + `=$6)`
	if err := repository.pool.QueryRow(ctx, `SELECT COUNT(*) FROM events e WHERE `+where, baseArgs...).Scan(&result.Total); err != nil {
		return Result{}, err
	}
	rows, err := repository.pool.Query(ctx, `SELECT e.event_id,e.device_id,d.hostname,e.project_id,`+projectHintExpression+`,`+roleExpression+`,
        COALESCE(NULLIF(e.payload->>'tool',''),REGEXP_REPLACE(e.source,'^core.ai.','')),e.occurred_at,
        LEFT(CASE WHEN e.event_type='ai.tool_call' THEN COALESCE(e.payload->>'command_summary','') ELSE COALESCE(e.payload->>'content','') END,240),
        CASE WHEN e.event_type='ai.tool_call' THEN 'tool_call' ELSE 'message' END,
        CASE WHEN e.event_type='ai.tool_call' THEN 'ai' WHEN LOWER(COALESCE(e.payload->>'role',''))='user' THEN 'human' ELSE 'ai' END,
        COALESCE(e.payload->>'command_type',''),COALESCE(f.quality_state,''),COALESCE(f.source_event_ids,ARRAY[e.event_id]::TEXT[])
        FROM events e JOIN devices d ON d.tenant_id=e.tenant_id AND d.id=e.device_id
        LEFT JOIN clean_event_facts f ON f.tenant_id=e.tenant_id AND f.canonical_event_id=e.event_id AND f.rule_version=1 WHERE `+where+`
        ORDER BY e.occurred_at DESC,e.event_id DESC LIMIT $7 OFFSET $8`, append(baseArgs, filter.Limit, filter.Offset)...)
	if err != nil {
		return Result{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var item Interaction
		var pathHint string
		if err := rows.Scan(&item.EventID, &item.DeviceID, &item.DeviceName, &item.ProjectID, &pathHint, &item.MessageRole, &item.Tool, &item.OccurredAt, &item.ContentPreview, &item.InteractionKind, &item.ActorOrigin, &item.CommandType, &item.QualityState, &item.SourceEventIDs); err != nil {
			return Result{}, err
		}
		item.ProjectName = ProjectLabel(pathHint, item.ProjectID)
		item.MessageRole = NormalizeMessageRole(item.MessageRole)
		result.Interactions = append(result.Interactions, item)
	}
	return result, rows.Err()
}
