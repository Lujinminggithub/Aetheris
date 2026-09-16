package effectiveness

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func eventPointProjectionColumns() []string {
	return []string{
		"event_id",
		"project_id",
		"project_name",
		"device_id",
		"event_type",
		"source",
		"work_role_code",
		"occurred_at",
	}
}

func (repository *Repository) RecomputeDay(ctx context.Context, tenantID, subjectID string, day time.Time, location *time.Location) error {
	localDay := day.In(location)
	start := time.Date(localDay.Year(), localDay.Month(), localDay.Day(), 0, 0, 0, 0, location)
	end := start.AddDate(0, 0, 1)
	projection := strings.Join(eventPointProjectionColumns(), ",")
	query := strings.ReplaceAll(`WITH coverage AS (
            SELECT MAX(rule_version) AS rule_version
            FROM clean_event_facts
            WHERE tenant_id=$1 AND subject_id=$2 AND occurred_at >= $3 AND occurred_at < $4
		), points(__EVENT_POINT_COLUMNS__) AS (
            SELECT e.event_id,COALESCE(ep.logical_project_id,e.project_id),COALESCE(ep.project_name,NULLIF(e.payload->>'project_label',''),NULLIF(e.payload->>'project',''),e.project_id),e.device_id,e.event_type,e.source,COALESCE(e.work_role_code,'') AS work_role_code,e.occurred_at
            FROM events e CROSS JOIN coverage c
            LEFT JOIN current_event_projects ep ON ep.tenant_id=e.tenant_id AND ep.event_id=e.event_id
            WHERE e.tenant_id=$1 AND e.subject_id=$2 AND e.occurred_at >= $3 AND e.occurred_at < $4
              AND (c.rule_version IS NULL
                OR (c.rule_version=1 AND e.event_type NOT IN ('terminal.command','ai.message','ai.tool_call'))
                OR (c.rule_version>=2 AND NOT (e.event_type IN ('terminal.command','ai.message','ai.tool_call','ide.activity','browser.page_view','git.commit','git.diff','svn.activity','process.observed') OR (c.rule_version>=3 AND e.event_type='application.activity') OR e.source LIKE 'core.visual_studio.%')))
            UNION ALL
            SELECT f.fact_id,COALESCE(ep.logical_project_id,f.project_id),COALESCE(ep.project_name,NULLIF(e.payload->>'project_label',''),NULLIF(e.payload->>'project',''),f.project_id),f.device_id,
              CASE WHEN f.fact_type='ai_interaction' THEN 'ai.message'
                   WHEN f.fact_type='activity' THEN f.event_type
                   WHEN f.actor_origin='ai' THEN 'ai.tool_call'
                   ELSE 'terminal.command' END,
              'clean.' || COALESCE(NULLIF(f.source,''),NULLIF(e.source,''),'fact'),COALESCE(e.work_role_code,''),f.occurred_at
            FROM clean_event_facts f CROSS JOIN coverage c
            LEFT JOIN events e ON e.tenant_id=f.tenant_id AND e.event_id=f.canonical_event_id
            LEFT JOIN current_event_projects ep ON ep.tenant_id=f.tenant_id AND ep.event_id=f.canonical_event_id
            WHERE c.rule_version IS NOT NULL AND f.tenant_id=$1 AND f.subject_id=$2
              AND f.rule_version=c.rule_version AND f.occurred_at >= $3 AND f.occurred_at < $4
              AND NOT f.excluded_from_effectiveness
        )
		SELECT __EVENT_POINT_COLUMNS__ FROM points ORDER BY occurred_at,event_id`, "__EVENT_POINT_COLUMNS__", projection)
	rows, err := repository.pool.Query(ctx, query, tenantID, subjectID, start.UTC(), end.UTC())
	if err != nil {
		return err
	}
	defer rows.Close()
	points := []EventPoint{}
	for rows.Next() {
		var point EventPoint
		if err := rows.Scan(&point.EventID, &point.ProjectID, &point.ProjectName, &point.DeviceID, &point.EventType, &point.Source, &point.WorkRoleCode, &point.OccurredAt); err != nil {
			return err
		}
		points = append(points, point)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	metrics := AggregateDay(points, start, location)
	projectJSON, _ := json.Marshal(metrics.ProjectBreakdown)
	roleJSON, _ := json.Marshal(metrics.WorkRoleBreakdown)
	sourceJSON, _ := json.Marshal(metrics.SourceCounts)
	deviceJSON, _ := json.Marshal(metrics.DeviceIDs)
	evidenceJSON, _ := json.Marshal(metrics.EvidenceEventIDs)
	_, err = repository.pool.Exec(ctx, `INSERT INTO subject_effectiveness_daily(
        tenant_id,subject_id,local_date,timezone,metric_definition_version,active_window_minutes,
        session_count,total_session_minutes,longest_session_minutes,focus_block_count,focus_block_minutes,
		context_switch_count,delivery_events,coding_events,terminal_events,ai_collaboration_events,browser_events,other_events,
		ide_file_opened_events,ide_edit_sessions,ide_file_saved_events,
        project_breakdown,work_role_breakdown,source_counts,device_ids,evidence_event_ids,first_event_at,last_event_at,computed_at)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26,$27,$28,NOW())
        ON CONFLICT(tenant_id,subject_id,local_date,timezone,metric_definition_version) DO UPDATE SET
        active_window_minutes=EXCLUDED.active_window_minutes,session_count=EXCLUDED.session_count,
        total_session_minutes=EXCLUDED.total_session_minutes,longest_session_minutes=EXCLUDED.longest_session_minutes,
        focus_block_count=EXCLUDED.focus_block_count,focus_block_minutes=EXCLUDED.focus_block_minutes,
        context_switch_count=EXCLUDED.context_switch_count,delivery_events=EXCLUDED.delivery_events,
        coding_events=EXCLUDED.coding_events,terminal_events=EXCLUDED.terminal_events,
		ai_collaboration_events=EXCLUDED.ai_collaboration_events,browser_events=EXCLUDED.browser_events,other_events=EXCLUDED.other_events,
		ide_file_opened_events=EXCLUDED.ide_file_opened_events,ide_edit_sessions=EXCLUDED.ide_edit_sessions,ide_file_saved_events=EXCLUDED.ide_file_saved_events,
        project_breakdown=EXCLUDED.project_breakdown,work_role_breakdown=EXCLUDED.work_role_breakdown,
        source_counts=EXCLUDED.source_counts,device_ids=EXCLUDED.device_ids,evidence_event_ids=EXCLUDED.evidence_event_ids,
        first_event_at=EXCLUDED.first_event_at,last_event_at=EXCLUDED.last_event_at,computed_at=NOW()`,
		tenantID, subjectID, metrics.LocalDate, metrics.Timezone, MetricDefinitionVersion, metrics.ActiveWindowMinutes,
		metrics.SessionCount, metrics.TotalSessionMinutes, metrics.LongestSessionMinutes, metrics.FocusBlockCount,
		metrics.FocusBlockMinutes, metrics.ContextSwitchCount, metrics.DeliveryEvents, metrics.CodingEvents,
		metrics.TerminalEvents, metrics.AICollaborationEvents, metrics.BrowserEvents, metrics.OtherEvents,
		metrics.IDEFileOpenedEvents, metrics.IDEEditSessions, metrics.IDEFileSavedEvents, projectJSON,
		roleJSON, sourceJSON, deviceJSON, evidenceJSON, metrics.FirstEventAt, metrics.LastEventAt)
	return err
}

func (repository *Repository) ListDaily(ctx context.Context, tenantID, subjectID string, from, to time.Time, timezone string) ([]DailyMetrics, error) {
	rows, err := repository.pool.Query(ctx, `SELECT local_date,timezone,metric_definition_version,active_window_minutes,
        session_count,total_session_minutes,longest_session_minutes,focus_block_count,focus_block_minutes,
		context_switch_count,delivery_events,coding_events,terminal_events,ai_collaboration_events,browser_events,other_events,
		ide_file_opened_events,ide_edit_sessions,ide_file_saved_events,
        project_breakdown,work_role_breakdown,source_counts,device_ids,evidence_event_ids,first_event_at,last_event_at
        FROM subject_effectiveness_daily WHERE tenant_id=$1 AND subject_id=$2 AND local_date BETWEEN $3 AND $4
        AND timezone=$5 AND metric_definition_version=$6 ORDER BY local_date`, tenantID, subjectID, from, to, timezone, MetricDefinitionVersion)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []DailyMetrics{}
	for rows.Next() {
		var item DailyMetrics
		var localDate time.Time
		var projectJSON, roleJSON, sourceJSON, deviceJSON, evidenceJSON []byte
		if err := rows.Scan(&localDate, &item.Timezone, &item.MetricDefinitionVersion, &item.ActiveWindowMinutes,
			&item.SessionCount, &item.TotalSessionMinutes, &item.LongestSessionMinutes, &item.FocusBlockCount,
			&item.FocusBlockMinutes, &item.ContextSwitchCount, &item.DeliveryEvents, &item.CodingEvents,
			&item.TerminalEvents, &item.AICollaborationEvents, &item.BrowserEvents, &item.OtherEvents,
			&item.IDEFileOpenedEvents, &item.IDEEditSessions, &item.IDEFileSavedEvents, &projectJSON, &roleJSON,
			&sourceJSON, &deviceJSON, &evidenceJSON, &item.FirstEventAt, &item.LastEventAt); err != nil {
			return nil, err
		}
		item.LocalDate = localDate.Format("2006-01-02")
		_ = json.Unmarshal(projectJSON, &item.ProjectBreakdown)
		_ = json.Unmarshal(roleJSON, &item.WorkRoleBreakdown)
		_ = json.Unmarshal(sourceJSON, &item.SourceCounts)
		_ = json.Unmarshal(deviceJSON, &item.DeviceIDs)
		_ = json.Unmarshal(evidenceJSON, &item.EvidenceEventIDs)
		result = append(result, item)
	}
	return result, rows.Err()
}

func (repository *Repository) ListSubjectIDs(ctx context.Context, tenantID string) ([]string, error) {
	rows, err := repository.pool.Query(ctx, `SELECT id FROM subjects WHERE tenant_id=$1 ORDER BY id`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		result = append(result, id)
	}
	return result, rows.Err()
}
