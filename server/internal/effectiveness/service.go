package effectiveness

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/aetheris-dev/aetheris/server/internal/auth"
	"github.com/aetheris-dev/aetheris/server/internal/authorization"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Totals struct {
	ActiveDays            int     `json:"active_days"`
	ActiveWindowMinutes   int     `json:"active_window_minutes"`
	SessionCount          int     `json:"session_count"`
	AverageSessionMinutes float64 `json:"average_session_minutes"`
	LongestSessionMinutes int     `json:"longest_session_minutes"`
	FocusBlockCount       int     `json:"focus_block_count"`
	FocusBlockMinutes     int     `json:"focus_block_minutes"`
	ContextSwitchCount    int     `json:"context_switch_count"`
	DeliveryEvents        int     `json:"delivery_events"`
	CodingEvents          int     `json:"coding_events"`
	TerminalEvents        int     `json:"terminal_events"`
	AICollaborationEvents int     `json:"ai_collaboration_events"`
	BrowserEvents         int     `json:"browser_events"`
	OtherEvents           int     `json:"other_events"`
	IDEFileOpenedEvents   int     `json:"ide_file_opened_events"`
	IDEEditSessions       int     `json:"ide_edit_sessions"`
	IDEFileSavedEvents    int     `json:"ide_file_saved_events"`
}

type TrendValue struct {
	Current       int      `json:"current"`
	Previous      int      `json:"previous"`
	Delta         int      `json:"delta"`
	PercentChange *float64 `json:"percent_change"`
}

type Coverage struct {
	CoveredDays   int        `json:"covered_days"`
	PeriodDays    int        `json:"period_days"`
	CoverageRatio float64    `json:"coverage_ratio"`
	SourceCount   int        `json:"source_count"`
	DeviceCount   int        `json:"device_count"`
	LastEventAt   *time.Time `json:"last_event_at,omitempty"`
	Insufficient  bool       `json:"insufficient"`
}

type Report struct {
	SubjectID               string                `json:"subject_id"`
	From                    string                `json:"from"`
	To                      string                `json:"to"`
	Timezone                string                `json:"timezone"`
	MetricDefinitionVersion int                   `json:"metric_definition_version"`
	Totals                  Totals                `json:"totals"`
	Daily                   []DailyMetrics        `json:"daily"`
	ProjectBreakdown        map[string]Breakdown  `json:"project_breakdown"`
	WorkRoleBreakdown       map[string]Breakdown  `json:"work_role_breakdown"`
	ActivityBreakdown       map[string]int        `json:"activity_breakdown"`
	Trends                  map[string]TrendValue `json:"trends"`
	Coverage                Coverage              `json:"coverage"`
	EvidenceEventIDs        []string              `json:"evidence_event_ids"`
	Definitions             map[string]string     `json:"definitions"`
}

type SubjectSummary struct {
	SubjectID           string  `json:"subject_id"`
	DisplayName         string  `json:"display_name"`
	ActiveDays          int     `json:"active_days"`
	ActiveWindowMinutes int     `json:"active_window_minutes"`
	EventCount          int     `json:"event_count"`
	CoverageRatio       float64 `json:"coverage_ratio"`
}

type Service struct {
	repository *Repository
	pool       *pgxpool.Pool
	location   *time.Location
}

func NewService(repository *Repository, pool *pgxpool.Pool, location *time.Location) *Service {
	return &Service{repository: repository, pool: pool, location: location}
}

func (service *Service) Location() *time.Location { return service.location }

func PercentChange(current, previous int) *float64 {
	if previous == 0 {
		return nil
	}
	value := math.Round((float64(current-previous)/float64(previous))*1000) / 10
	return &value
}

func ValidateQueryRange(from, to time.Time) error {
	if to.Before(from) {
		return fmt.Errorf("结束日期不能早于开始日期")
	}
	if int(to.Sub(from).Hours()/24)+1 > 90 {
		return fmt.Errorf("查询范围不能超过 90 天")
	}
	return nil
}

func ValidateRecomputeRange(from, to time.Time) error {
	if to.Before(from) || int(to.Sub(from).Hours()/24)+1 > 31 {
		return fmt.Errorf("重算范围必须为 1 到 31 天")
	}
	return nil
}

func CalculateCoverage(daily []DailyMetrics, periodDays int) Coverage {
	coverage := Coverage{PeriodDays: periodDays}
	sources := map[string]struct{}{}
	devices := map[string]struct{}{}
	for _, day := range daily {
		if day.ActiveWindowMinutes > 0 {
			coverage.CoveredDays++
		}
		for source := range day.SourceCounts {
			sources[source] = struct{}{}
		}
		for _, device := range day.DeviceIDs {
			devices[device] = struct{}{}
		}
		if day.LastEventAt != nil && (coverage.LastEventAt == nil || day.LastEventAt.After(*coverage.LastEventAt)) {
			value := *day.LastEventAt
			coverage.LastEventAt = &value
		}
	}
	if periodDays > 0 {
		coverage.CoverageRatio = math.Round((float64(coverage.CoveredDays)/float64(periodDays))*1000) / 1000
	}
	coverage.SourceCount = len(sources)
	coverage.DeviceCount = len(devices)
	coverage.Insufficient = coverage.CoverageRatio < 0.3
	return coverage
}

func (service *Service) Report(ctx context.Context, principal authorization.Principal, subjectID string, from, to time.Time) (Report, error) {
	if err := authorization.Require(principal, "effectiveness:read", authorization.Scope{TenantID: principal.TenantID}); err != nil {
		return Report{}, err
	}
	if principal.AccessRole == "member" {
		var linked bool
		if err := service.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM user_subject_links WHERE tenant_id=$1 AND user_id=$2 AND subject_id=$3)`, principal.TenantID, principal.ID, subjectID).Scan(&linked); err != nil || !linked {
			return Report{}, fmt.Errorf("只能读取本人效能数据")
		}
	}
	if err := ValidateQueryRange(from, to); err != nil {
		return Report{}, err
	}
	daily, err := service.repository.ListDaily(ctx, principal.TenantID, subjectID, from, to, service.location.String())
	if err != nil {
		return Report{}, err
	}
	periodDays := int(to.Sub(from).Hours()/24) + 1
	previousTo := from.AddDate(0, 0, -1)
	previousFrom := previousTo.AddDate(0, 0, -(periodDays - 1))
	previous, err := service.repository.ListDaily(ctx, principal.TenantID, subjectID, previousFrom, previousTo, service.location.String())
	if err != nil {
		return Report{}, err
	}
	return BuildReport(subjectID, from, to, service.location, daily, previous), nil
}

func BuildReport(subjectID string, from, to time.Time, location *time.Location, daily, previous []DailyMetrics) Report {
	totals, projects, roles, evidence := combineDaily(daily)
	previousTotals, _, _, _ := combineDaily(previous)
	return Report{
		SubjectID: subjectID, From: from.Format("2006-01-02"), To: to.Format("2006-01-02"), Timezone: location.String(),
		MetricDefinitionVersion: MetricDefinitionVersion, Totals: totals, Daily: daily, ProjectBreakdown: projects,
		WorkRoleBreakdown: roles, ActivityBreakdown: map[string]int{
			"delivery": totals.DeliveryEvents, "coding": totals.CodingEvents, "terminal": totals.TerminalEvents,
			"ai_collaboration": totals.AICollaborationEvents, "other": totals.OtherEvents,
			"browser": totals.BrowserEvents,
		},
		Trends: map[string]TrendValue{
			"active_window_minutes": trend(totals.ActiveWindowMinutes, previousTotals.ActiveWindowMinutes),
			"session_count":         trend(totals.SessionCount, previousTotals.SessionCount),
			"focus_block_minutes":   trend(totals.FocusBlockMinutes, previousTotals.FocusBlockMinutes),
		},
		Coverage: CalculateCoverage(daily, int(to.Sub(from).Hours()/24)+1), EvidenceEventIDs: evidence,
		Definitions: map[string]string{
			"active_window_minutes":  "有事件的不重复 5 分钟窗口，不等同于工时",
			"session_count":          "相邻事件间隔超过 30 分钟时开始新会话",
			"focus_block_minutes":    "同一项目连续信号达到 25 分钟的时段",
			"context_switch_count":   "同一会话内相邻事件所属项目发生变化的次数",
			"ide_file_opened_events": "VS Code 扩展上报的代码文件打开事件数",
			"ide_edit_sessions":      "VS Code 扩展按保存、切换、关闭或空闲聚合的编辑会话数",
			"ide_file_saved_events":  "VS Code 扩展上报的代码文件保存事件数",
		},
	}
}

func combineDaily(daily []DailyMetrics) (Totals, map[string]Breakdown, map[string]Breakdown, []string) {
	totals := Totals{}
	projects, roles := map[string]Breakdown{}, map[string]Breakdown{}
	evidence, seen := []string{}, map[string]struct{}{}
	for _, day := range daily {
		if day.ActiveWindowMinutes > 0 {
			totals.ActiveDays++
		}
		totals.ActiveWindowMinutes += day.ActiveWindowMinutes
		totals.SessionCount += day.SessionCount
		totals.AverageSessionMinutes += float64(day.TotalSessionMinutes)
		if day.LongestSessionMinutes > totals.LongestSessionMinutes {
			totals.LongestSessionMinutes = day.LongestSessionMinutes
		}
		totals.FocusBlockCount += day.FocusBlockCount
		totals.FocusBlockMinutes += day.FocusBlockMinutes
		totals.ContextSwitchCount += day.ContextSwitchCount
		totals.DeliveryEvents += day.DeliveryEvents
		totals.CodingEvents += day.CodingEvents
		totals.TerminalEvents += day.TerminalEvents
		totals.AICollaborationEvents += day.AICollaborationEvents
		totals.BrowserEvents += day.BrowserEvents
		totals.OtherEvents += day.OtherEvents
		totals.IDEFileOpenedEvents += day.IDEFileOpenedEvents
		totals.IDEEditSessions += day.IDEEditSessions
		totals.IDEFileSavedEvents += day.IDEFileSavedEvents
		mergeBreakdown(projects, day.ProjectBreakdown)
		mergeBreakdown(roles, day.WorkRoleBreakdown)
		for _, id := range day.EvidenceEventIDs {
			if len(evidence) >= 200 {
				break
			}
			if _, ok := seen[id]; !ok {
				seen[id] = struct{}{}
				evidence = append(evidence, id)
			}
		}
	}
	if totals.SessionCount > 0 {
		totals.AverageSessionMinutes = math.Round((totals.AverageSessionMinutes/float64(totals.SessionCount))*10) / 10
	} else {
		totals.AverageSessionMinutes = 0
	}
	return totals, projects, roles, evidence
}

func mergeBreakdown(target, source map[string]Breakdown) {
	for key, value := range source {
		current := target[key]
		current.EventCount += value.EventCount
		current.ActiveWindowMinutes += value.ActiveWindowMinutes
		target[key] = current
	}
}

func trend(current, previous int) TrendValue {
	return TrendValue{Current: current, Previous: previous, Delta: current - previous, PercentChange: PercentChange(current, previous)}
}

func (service *Service) ListSubjects(ctx context.Context, principal authorization.Principal, from, to time.Time) ([]SubjectSummary, error) {
	if err := authorization.Require(principal, "effectiveness:read", authorization.Scope{TenantID: principal.TenantID}); err != nil {
		return nil, err
	}
	if err := ValidateQueryRange(from, to); err != nil {
		return nil, err
	}
	periodDays := int(to.Sub(from).Hours()/24) + 1
	query := `SELECT s.id,s.display_name,COUNT(d.local_date) FILTER(WHERE d.active_window_minutes>0),COALESCE(SUM(d.active_window_minutes),0),
		COALESCE(SUM(d.delivery_events+d.coding_events+d.terminal_events+d.ai_collaboration_events+d.browser_events+d.other_events),0)
        FROM subjects s LEFT JOIN subject_effectiveness_daily d ON d.tenant_id=s.tenant_id AND d.subject_id=s.id
        AND d.local_date BETWEEN $2 AND $3 AND d.timezone=$4 AND d.metric_definition_version=$5
        WHERE s.tenant_id=$1`
	args := []any{principal.TenantID, from, to, service.location.String(), MetricDefinitionVersion}
	if principal.AccessRole == "member" {
		query += ` AND EXISTS(SELECT 1 FROM user_subject_links l WHERE l.tenant_id=s.tenant_id AND l.user_id=$6 AND l.subject_id=s.id)`
		args = append(args, principal.ID)
	}
	query += ` GROUP BY s.id,s.display_name ORDER BY s.display_name`
	rows, err := service.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []SubjectSummary{}
	for rows.Next() {
		var item SubjectSummary
		if err := rows.Scan(&item.SubjectID, &item.DisplayName, &item.ActiveDays, &item.ActiveWindowMinutes, &item.EventCount); err != nil {
			return nil, err
		}
		item.CoverageRatio = math.Round((float64(item.ActiveDays)/float64(periodDays))*1000) / 1000
		result = append(result, item)
	}
	return result, rows.Err()
}

func (service *Service) RecomputeRange(ctx context.Context, tenantID string, from, to time.Time) (string, error) {
	if err := ValidateRecomputeRange(from, to); err != nil {
		return "", err
	}
	token, err := auth.RandomToken(12)
	if err != nil {
		return "", err
	}
	jobID := "effectiveness-" + token
	_, err = service.pool.Exec(ctx, `INSERT INTO effectiveness_recompute_jobs(id,tenant_id,date_from,date_to,timezone,metric_definition_version,status) VALUES($1,$2,$3,$4,$5,$6,'queued')`, jobID, tenantID, from, to, service.location.String(), MetricDefinitionVersion)
	if err != nil {
		return "", err
	}
	go service.runJob(context.Background(), jobID, tenantID, from, to)
	return jobID, nil
}

func (service *Service) runJob(ctx context.Context, jobID, tenantID string, from, to time.Time) {
	_, _ = service.pool.Exec(ctx, `UPDATE effectiveness_recompute_jobs SET status='running',started_at=NOW() WHERE id=$1`, jobID)
	subjects, err := service.repository.ListSubjectIDs(ctx, tenantID)
	for day := from; err == nil && !day.After(to); day = day.AddDate(0, 0, 1) {
		for _, subjectID := range subjects {
			if err = service.repository.RecomputeDay(ctx, tenantID, subjectID, day, service.location); err != nil {
				break
			}
		}
	}
	if err != nil {
		_, _ = service.pool.Exec(ctx, `UPDATE effectiveness_recompute_jobs SET status='failed',error_code='recompute_failed',completed_at=NOW() WHERE id=$1`, jobID)
		return
	}
	_, _ = service.pool.Exec(ctx, `UPDATE effectiveness_recompute_jobs SET status='completed',completed_at=NOW() WHERE id=$1`, jobID)
}
