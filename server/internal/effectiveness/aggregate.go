package effectiveness

import (
	"sort"
	"time"
)

type EventPoint struct {
	EventID, ProjectID, ProjectName, DeviceID, EventType, Source, WorkRoleCode string
	OccurredAt                                                                 time.Time
}

type Breakdown struct {
	EventCount          int `json:"event_count"`
	ActiveWindowMinutes int `json:"active_window_minutes"`
}

type DailyMetrics struct {
	LocalDate               string               `json:"local_date"`
	Timezone                string               `json:"timezone"`
	MetricDefinitionVersion int                  `json:"metric_definition_version"`
	ActiveWindowMinutes     int                  `json:"active_window_minutes"`
	SessionCount            int                  `json:"session_count"`
	TotalSessionMinutes     int                  `json:"total_session_minutes"`
	LongestSessionMinutes   int                  `json:"longest_session_minutes"`
	FocusBlockCount         int                  `json:"focus_block_count"`
	FocusBlockMinutes       int                  `json:"focus_block_minutes"`
	ContextSwitchCount      int                  `json:"context_switch_count"`
	DeliveryEvents          int                  `json:"delivery_events"`
	CodingEvents            int                  `json:"coding_events"`
	TerminalEvents          int                  `json:"terminal_events"`
	AICollaborationEvents   int                  `json:"ai_collaboration_events"`
	BrowserEvents           int                  `json:"browser_events"`
	OtherEvents             int                  `json:"other_events"`
	IDEFileOpenedEvents     int                  `json:"ide_file_opened_events"`
	IDEEditSessions         int                  `json:"ide_edit_sessions"`
	IDEFileSavedEvents      int                  `json:"ide_file_saved_events"`
	ProjectBreakdown        map[string]Breakdown `json:"project_breakdown"`
	WorkRoleBreakdown       map[string]Breakdown `json:"work_role_breakdown"`
	SourceCounts            map[string]int       `json:"source_counts"`
	DeviceIDs               []string             `json:"device_ids"`
	EvidenceEventIDs        []string             `json:"evidence_event_ids"`
	FirstEventAt            *time.Time           `json:"first_event_at,omitempty"`
	LastEventAt             *time.Time           `json:"last_event_at,omitempty"`
}

func AggregateDay(events []EventPoint, day time.Time, location *time.Location) DailyMetrics {
	if location == nil {
		location = time.UTC
	}
	localDay := day.In(location)
	start := time.Date(localDay.Year(), localDay.Month(), localDay.Day(), 0, 0, 0, 0, location)
	end := start.AddDate(0, 0, 1)
	filtered := make([]EventPoint, 0, len(events))
	for _, event := range events {
		local := event.OccurredAt.In(location)
		if !local.Before(start) && local.Before(end) {
			filtered = append(filtered, event)
		}
	}
	sort.SliceStable(filtered, func(i, j int) bool { return filtered[i].OccurredAt.Before(filtered[j].OccurredAt) })

	metrics := DailyMetrics{
		LocalDate:               start.Format("2006-01-02"),
		Timezone:                location.String(),
		MetricDefinitionVersion: MetricDefinitionVersion,
		ProjectBreakdown:        map[string]Breakdown{},
		WorkRoleBreakdown:       map[string]Breakdown{},
		SourceCounts:            map[string]int{},
	}
	if len(filtered) == 0 {
		return metrics
	}
	first, last := filtered[0].OccurredAt, filtered[len(filtered)-1].OccurredAt
	metrics.FirstEventAt, metrics.LastEventAt = &first, &last

	allBuckets := map[int64]struct{}{}
	deviceSet := map[string]struct{}{}
	projectBuckets := map[string]map[int64]struct{}{}
	roleBuckets := map[string]map[int64]struct{}{}
	projectFocusBuckets := map[string]map[int64]time.Time{}

	for index, event := range filtered {
		bucket := fiveMinuteBucket(event.OccurredAt, location)
		bucketKey := bucket.Unix()
		allBuckets[bucketKey] = struct{}{}
		if event.DeviceID != "" {
			deviceSet[event.DeviceID] = struct{}{}
		}
		project := event.ProjectName
		if project == "" {
			project = event.ProjectID
		}
		if project == "" {
			project = "未分配项目"
		}
		role := event.WorkRoleCode
		if role == "" {
			role = "未分配角色"
		}
		incrementBreakdown(metrics.ProjectBreakdown, project)
		incrementBreakdown(metrics.WorkRoleBreakdown, role)
		addBucket(projectBuckets, project, bucketKey)
		addBucket(roleBuckets, role, bucketKey)
		if projectFocusBuckets[project] == nil {
			projectFocusBuckets[project] = map[int64]time.Time{}
		}
		projectFocusBuckets[project][bucketKey] = bucket
		metrics.SourceCounts[event.Source]++
		switch event.EventType {
		case "ide.file_opened":
			metrics.IDEFileOpenedEvents++
		case "ide.file_edited":
			metrics.IDEEditSessions++
		case "ide.file_saved":
			metrics.IDEFileSavedEvents++
		}
		switch Classify(event.EventType, event.Source) {
		case ActivityDelivery:
			metrics.DeliveryEvents++
		case ActivityCoding:
			metrics.CodingEvents++
		case ActivityTerminal:
			metrics.TerminalEvents++
		case ActivityAI:
			metrics.AICollaborationEvents++
		case ActivityBrowser:
			metrics.BrowserEvents++
		default:
			metrics.OtherEvents++
		}
		if index < 50 {
			metrics.EvidenceEventIDs = append(metrics.EvidenceEventIDs, event.EventID)
		}
		if index > 0 {
			previous := filtered[index-1]
			if event.OccurredAt.Sub(previous.OccurredAt) <= 30*time.Minute && previous.ProjectID != "" && event.ProjectID != "" && previous.ProjectID != event.ProjectID {
				metrics.ContextSwitchCount++
			}
		}
	}

	metrics.ActiveWindowMinutes = len(allBuckets) * 5
	for key, value := range metrics.ProjectBreakdown {
		value.ActiveWindowMinutes = len(projectBuckets[key]) * 5
		metrics.ProjectBreakdown[key] = value
	}
	for key, value := range metrics.WorkRoleBreakdown {
		value.ActiveWindowMinutes = len(roleBuckets[key]) * 5
		metrics.WorkRoleBreakdown[key] = value
	}
	for deviceID := range deviceSet {
		metrics.DeviceIDs = append(metrics.DeviceIDs, deviceID)
	}
	sort.Strings(metrics.DeviceIDs)
	computeSessions(filtered, location, &metrics)
	computeFocusBlocks(projectFocusBuckets, &metrics)
	return metrics
}

func fiveMinuteBucket(value time.Time, location *time.Location) time.Time {
	local := value.In(location)
	return time.Date(local.Year(), local.Month(), local.Day(), local.Hour(), local.Minute()/5*5, 0, 0, location)
}

func incrementBreakdown(values map[string]Breakdown, key string) {
	value := values[key]
	value.EventCount++
	values[key] = value
}

func addBucket(values map[string]map[int64]struct{}, key string, bucket int64) {
	if values[key] == nil {
		values[key] = map[int64]struct{}{}
	}
	values[key][bucket] = struct{}{}
}

func computeSessions(events []EventPoint, location *time.Location, metrics *DailyMetrics) {
	currentBuckets := map[int64]struct{}{}
	finish := func() {
		if len(currentBuckets) == 0 {
			return
		}
		minutes := len(currentBuckets) * 5
		metrics.SessionCount++
		metrics.TotalSessionMinutes += minutes
		if minutes > metrics.LongestSessionMinutes {
			metrics.LongestSessionMinutes = minutes
		}
	}
	for index, event := range events {
		if index > 0 && event.OccurredAt.Sub(events[index-1].OccurredAt) > 30*time.Minute {
			finish()
			currentBuckets = map[int64]struct{}{}
		}
		currentBuckets[fiveMinuteBucket(event.OccurredAt, location).Unix()] = struct{}{}
	}
	finish()
}

func computeFocusBlocks(projectBuckets map[string]map[int64]time.Time, metrics *DailyMetrics) {
	for _, bucketMap := range projectBuckets {
		buckets := make([]time.Time, 0, len(bucketMap))
		for _, bucket := range bucketMap {
			buckets = append(buckets, bucket)
		}
		sort.Slice(buckets, func(i, j int) bool { return buckets[i].Before(buckets[j]) })
		start := 0
		for index := 1; index <= len(buckets); index++ {
			if index < len(buckets) && buckets[index].Sub(buckets[index-1]) <= 10*time.Minute {
				continue
			}
			minutes := (index - start) * 5
			if minutes >= 25 {
				metrics.FocusBlockCount++
				metrics.FocusBlockMinutes += minutes
			}
			start = index
		}
	}
}
