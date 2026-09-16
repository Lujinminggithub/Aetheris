package events

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"
)

func TestParseAndValidateAcceptsCanonicalEvent(t *testing.T) {
	raw := map[string]any{
		"event_id": "event-1", "schema_version": 1, "event_type": "process.observed",
		"tenant_id": "tenant-1", "subject_id": "subject-1", "device_id": "device-1", "project_id": "project-1",
		"session_id": "session-1", "correlation_id": "correlation-1", "source": "core.process", "source_version": "1.0.0",
		"occurred_at": "2026-09-06T00:00:00Z", "ingested_at": "2026-09-06T00:00:00Z",
		"payload": map[string]any{"name": "研发"}, "redaction_report": map[string]any{}, "processing_grants": []any{"server_ingest"},
	}
	hashInput := map[string]any{}
	for key, value := range raw {
		hashInput[key] = value
	}
	canonical, _ := canonicalASCII(hashInput)
	digest := sha256.Sum256(canonical)
	raw["content_hash"] = hex.EncodeToString(digest[:])
	payload, _ := json.Marshal(raw)
	event, err := ParseAndValidate(payload)
	if err != nil {
		t.Fatal(err)
	}
	if event.EventID != "event-1" {
		t.Fatalf("event id = %s", event.EventID)
	}
}

func TestParseAndValidateRejectsHashMismatch(t *testing.T) {
	payload := []byte(`{"event_id":"event-1","schema_version":1,"event_type":"process.observed","tenant_id":"tenant-1","subject_id":"subject-1","device_id":"device-1","project_id":"project-1","session_id":"session-1","correlation_id":"correlation-1","source":"core.process","source_version":"1.0.0","occurred_at":"2026-09-06T00:00:00Z","ingested_at":"2026-09-06T00:00:00Z","payload":{},"content_hash":"0000000000000000000000000000000000000000000000000000000000000000","redaction_report":{},"processing_grants":[]}`)
	if _, err := ParseAndValidate(payload); err == nil {
		t.Fatal("expected hash mismatch")
	}
}

func TestParseAndValidateAcceptsPythonCanonicalUnicode(t *testing.T) {
	payload := []byte(`{"consent_policy_version":"1","content_hash":"849cee27cb37bf915d8ee84cd28a0254a0a5b69da6c9648590ce57a8894ef22a","content_refs":[],"correlation_id":"event-unicode","crypto_mode":"device-redacted","device_id":"device-1","event_id":"event-unicode","event_type":"ai.message","ingested_at":"2026-09-07T06:00:01Z","key_version":null,"occurred_at":"2026-09-07T06:00:00Z","payload":{"content":"\u4fee\u590d\u95ee\u9898","role":"user"},"processing_grants":["server_ingest"],"project_id":"project-1","provenance":{},"redaction_report":{"replacement_count":0,"rules":[]},"schema_version":1,"session_id":"session-1","source":"core.ai.codex","source_version":"0.4.3","subject_id":"subject-1","supersedes_event_id":null,"tenant_id":"tenant-1"}`)

	event, err := ParseAndValidate(payload)
	if err != nil {
		t.Fatal(err)
	}
	if event.Payload["content"] != "修复问题" {
		t.Fatalf("content = %#v", event.Payload["content"])
	}
}

func TestProjectRecordDoesNotPersistClientPath(t *testing.T) {
	record := projectRecord(Event{ProjectID: "project-123", Provenance: map[string]any{"project_root": `C:\secret\repo`}})
	if record.ID != "project-123" || record.Name != "project-123" || record.RootHint != "" {
		t.Fatalf("project record = %#v", record)
	}
}
