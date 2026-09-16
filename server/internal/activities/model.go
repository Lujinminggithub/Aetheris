package activities

import (
	"context"
	"time"
)

type Filter struct {
	TenantID, DeviceID, ProjectID, ActivityType, MessageRole string
	From, ToExclusive                                        time.Time
	Limit, Offset                                            int
}

type Facet struct {
	ID             string    `json:"id"`
	Label          string    `json:"label"`
	Count          int       `json:"count"`
	LastActivityAt time.Time `json:"last_activity_at"`
}

type Activity struct {
	FactID           string    `json:"fact_id"`
	CanonicalEventID string    `json:"canonical_event_id"`
	DeviceID         string    `json:"device_id"`
	DeviceName       string    `json:"device_name"`
	ProjectID        string    `json:"project_id"`
	ProjectName      string    `json:"project_name"`
	ActivityType     string    `json:"activity_type"`
	EventType        string    `json:"event_type"`
	Source           string    `json:"source"`
	ActorOrigin      string    `json:"actor_origin"`
	MessageRole      string    `json:"message_role"`
	Tool             string    `json:"tool,omitempty"`
	Preview          string    `json:"preview"`
	OccurredAt       time.Time `json:"occurred_at"`
	SourceEventIDs   []string  `json:"source_event_ids"`
}

type Result struct {
	Devices        []Facet        `json:"devices"`
	Projects       []Facet        `json:"projects"`
	ActivityCounts map[string]int `json:"activity_counts"`
	Activities     []Activity     `json:"activities"`
	Total          int            `json:"total"`
	Limit          int            `json:"limit"`
	Offset         int            `json:"offset"`
}

type Querier interface {
	Query(context.Context, Filter) (Result, error)
}
