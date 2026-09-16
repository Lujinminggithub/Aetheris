package adapterhealth

import (
	"context"
	"time"
)

type Snapshot struct {
	TenantID                 string     `json:"tenant_id,omitempty"`
	DeviceID                 string     `json:"device_id"`
	AdapterID                string     `json:"adapter_id"`
	State                    string     `json:"state"`
	CapabilityVersion        string     `json:"capability_version"`
	DetectedFormat           string     `json:"detected_format,omitempty"`
	LastScanAt               *time.Time `json:"last_scan_at,omitempty"`
	LastSuccessAt            *time.Time `json:"last_success_at,omitempty"`
	LastEventAt              *time.Time `json:"last_event_at,omitempty"`
	Discovered               int        `json:"discovered"`
	Parsed                   int        `json:"parsed"`
	Skipped                  int        `json:"skipped"`
	Failed                   int        `json:"failed"`
	LagSeconds               int64      `json:"lag_seconds"`
	ErrorCode                string     `json:"error_code,omitempty"`
	ErrorStage               string     `json:"error_stage,omitempty"`
	ComponentState           string     `json:"component_state,omitempty"`
	ComponentVersion         string     `json:"component_version,omitempty"`
	ProtocolVersion          int        `json:"protocol_version,omitempty"`
	VSCodeVersion            string     `json:"vscode_version,omitempty"`
	LastComponentHeartbeatAt *time.Time `json:"last_component_heartbeat_at,omitempty"`
	PendingEvents            int        `json:"pending_events"`
	SentEvents               int64      `json:"sent_events"`
	DroppedEvents            int64      `json:"dropped_events"`
}

type Store interface {
	Upsert(context.Context, Snapshot) error
	List(context.Context, string, string) ([]Snapshot, error)
}

func ValidComponentState(value string) bool {
	switch value {
	case "", "active", "install_declined", "paused_by_user", "not_installed", "pending_install", "awaiting_activation", "inactive_in_vscode", "bridge_offline", "unsupported_remote_host", "incompatible", "error":
		return true
	default:
		return false
	}
}
