package httpapi

import (
	"github.com/aetheris-dev/aetheris/server/internal/activities"
	"github.com/aetheris-dev/aetheris/server/internal/events"
)

func projectEventForAdmin(event events.Event) events.Event {
	payload := make(map[string]any, len(event.Payload))
	for key, value := range event.Payload {
		payload[key] = value
	}
	hint := ""
	for _, key := range []string{"project_label", "project", "project_path", "cwd"} {
		if value, ok := payload[key].(string); ok && value != "" {
			hint = value
			break
		}
	}
	for _, key := range []string{"project", "project_path", "cwd"} {
		delete(payload, key)
	}
	if hint != "" {
		payload["project_label"] = activities.ProjectLabel(stringValue(payload["project_label"]), hint, "", event.ProjectID)
	}
	provenance := make(map[string]any, len(event.Provenance))
	for key, value := range event.Provenance {
		if key != "project_root" {
			provenance[key] = value
		}
	}
	event.Payload, event.Provenance = payload, provenance
	return event
}

func stringValue(value any) string { result, _ := value.(string); return result }
