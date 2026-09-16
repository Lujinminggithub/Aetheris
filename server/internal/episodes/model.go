package episodes

import (
	"context"
	"time"
)

type Fact struct {
	EventID    string
	SubjectID  string
	DeviceID   string
	ProjectID  string
	SessionID  string
	EventType  string
	Role       string
	Content    string
	Summary    string
	OccurredAt time.Time
}

type Evidence struct {
	EventID string `json:"event_id"`
	Section string `json:"section"`
	Reason  string `json:"reason"`
}

type Action struct {
	EventID    string    `json:"event_id"`
	Actor      string    `json:"actor"`
	ActionType string    `json:"action_type"`
	Summary    string    `json:"summary"`
	OccurredAt time.Time `json:"occurred_at"`
}

type Validation struct {
	EventID        string    `json:"event_id"`
	ValidationType string    `json:"validation_type"`
	Result         string    `json:"result"`
	Summary        string    `json:"summary"`
	OccurredAt     time.Time `json:"occurred_at"`
}

type Episode struct {
	EpisodeID   string       `json:"episode_id"`
	SubjectID   string       `json:"subject_id"`
	DeviceID    string       `json:"device_id"`
	ProjectID   string       `json:"project_id"`
	ProjectName string       `json:"project_name,omitempty"`
	SessionID   string       `json:"session_id"`
	StartedAt   time.Time    `json:"started_at"`
	EndedAt     time.Time    `json:"ended_at"`
	Title       string       `json:"title"`
	Objective   string       `json:"objective"`
	Actions     []Action     `json:"actions"`
	Validations []Validation `json:"validations"`
	Outcome     string       `json:"outcome"`
	Confidence  string       `json:"confidence"`
	NeedsReview bool         `json:"needs_review"`
	Evidence    []Evidence   `json:"evidence"`
}

type Filter struct {
	ProjectID   string
	SubjectID   string
	DeviceID    string
	Status      string
	NeedsReview *bool
	Limit       int
	Offset      int
}

type Store interface {
	List(context.Context, string, Filter) ([]Episode, error)
	Get(context.Context, string, string) (Episode, error)
}
