package cleaning

import (
	"testing"
	"time"
)

func evidence(id, eventType, command string, at time.Time) RawEvidence {
	return RawEvidence{EventID: id, TenantID: "tenant-1", SubjectID: "subject-1", DeviceID: "device-1", ProjectID: "project-1", EventType: eventType, OccurredAt: at, IngestedAt: at, Payload: map[string]any{"command": command}}
}

func TestNormalizeEventsJoinsBacktickContinuation(t *testing.T) {
	at := time.Date(2026, 9, 7, 10, 0, 0, 0, time.UTC)
	facts := NormalizeEvents([]RawEvidence{
		evidence("e1", "terminal.command", "Start-Process powershell.exe -ArgumentList `", at),
		evidence("e2", "terminal.command", `"-NoExit", "-Command", `+"`", at.Add(time.Millisecond)),
		evidence("e3", "terminal.command", `"Set-Location E:\code"`, at.Add(2*time.Millisecond)),
	}, 1)
	if len(facts) != 1 || facts[0].QualityState != "merged" || len(facts[0].SourceEventIDs) != 3 {
		t.Fatalf("facts = %#v", facts)
	}
	if facts[0].CommandText == "" || facts[0].ExcludedFromEffectiveness {
		t.Fatalf("merged fact = %#v", facts[0])
	}
}

func TestNormalizeEventsReordersOrphanParameter(t *testing.T) {
	at := time.Date(2026, 9, 7, 10, 0, 0, 0, time.UTC)
	facts := NormalizeEvents([]RawEvidence{
		evidence("event-5c4ee64b6c09bf92c94467ce3fa191eb", "terminal.command", "-Recurse -Filter Microsoft.DriverKit.Build.Tasks.17.0.dll", at),
		evidence("event-47023bfa1f504f4078647c8354685305", "terminal.command", `Get-ChildItem "D:\Program Files (x86)\Windows Kits\10\build"`, at.Add(time.Millisecond)),
	}, 1)
	if len(facts) != 1 || facts[0].CommandText != `Get-ChildItem "D:\Program Files (x86)\Windows Kits\10\build" -Recurse -Filter Microsoft.DriverKit.Build.Tasks.17.0.dll` {
		t.Fatalf("facts = %#v", facts)
	}
	if !contains(facts[0].ReasonCodes, "inferred_parameter_join") {
		t.Fatalf("reason codes = %#v", facts[0].ReasonCodes)
	}
}

func TestNormalizeEventsJoinsParameterAfterPrimaryCommand(t *testing.T) {
	at := time.Date(2026, 9, 7, 10, 0, 0, 0, time.UTC)
	facts := NormalizeEvents([]RawEvidence{
		evidence("event-47023bfa1f504f4078647c8354685305", "terminal.command", `Get-ChildItem "D:\Program Files (x86)\Windows Kits\10\build"`, at),
		evidence("event-5c4ee64b6c09bf92c94467ce3fa191eb", "terminal.command", "-Recurse -Filter Microsoft.DriverKit.Build.Tasks.17.0.dll", at.Add(time.Millisecond)),
	}, 1)
	if len(facts) != 1 || facts[0].CommandText != `Get-ChildItem "D:\Program Files (x86)\Windows Kits\10\build" -Recurse -Filter Microsoft.DriverKit.Build.Tasks.17.0.dll` {
		t.Fatalf("facts = %#v", facts)
	}
	if facts[0].MergeMethod != "inferred_parameter_join" || !contains(facts[0].ReasonCodes, "inferred_parameter_join") || len(facts[0].SourceEventIDs) != 2 {
		t.Fatalf("fact = %#v", facts[0])
	}
}

func TestNormalizeEventsUsesOddTrailingBacktickAsContinuation(t *testing.T) {
	at := time.Date(2026, 9, 7, 10, 0, 0, 0, time.UTC)
	facts := NormalizeEvents([]RawEvidence{
		evidence("event-c", "terminal.command", "Get-ChildItem C:\\\\Windows ``", at),
		evidence("event-5c4ee64b6c09bf92c94467ce3fa191eb", "terminal.command", "-Recurse -Filter Microsoft.DriverKit.Build.Tasks.17.0.dll", at.Add(time.Millisecond)),
		evidence("event-47023bfa1f504f4078647c8354685305", "terminal.command", `Get-ChildItem "D:\Program Files (x86)\Windows Kits\10\build"`, at.Add(2*time.Millisecond)),
	}, 1)
	if len(facts) != 2 {
		t.Fatalf("facts = %#v", facts)
	}
	if facts[1].MergeMethod != "inferred_parameter_join" || facts[1].CommandText != `Get-ChildItem "D:\Program Files (x86)\Windows Kits\10\build" -Recurse -Filter Microsoft.DriverKit.Build.Tasks.17.0.dll` {
		t.Fatalf("merged fact = %#v", facts[1])
	}
}

func TestNormalizeEventsQuarantinesUnmatchedFragment(t *testing.T) {
	facts := NormalizeEvents([]RawEvidence{evidence("e1", "terminal.command", "-Filter *.dll", time.Now())}, 1)
	if len(facts) != 1 || facts[0].FactType != "command_fragment" || !facts[0].ExcludedFromEffectiveness || facts[0].QualityState != "quarantined" {
		t.Fatalf("facts = %#v", facts)
	}
}

func TestNormalizeEventsMergesAIToolAndTerminalEvidenceByHash(t *testing.T) {
	at := time.Date(2026, 9, 7, 10, 0, 0, 0, time.UTC)
	terminal := evidence("terminal-1", "terminal.command", "redacted command", at)
	terminal.Payload["command_hash"] = "hmac-sha256:same"
	ai := RawEvidence{EventID: "ai-1", TenantID: "tenant-1", SubjectID: "subject-1", DeviceID: "device-1", ProjectID: "project-1", EventType: "ai.tool_call", OccurredAt: at.Add(time.Second), IngestedAt: at.Add(time.Second), Payload: map[string]any{"actor_origin": "ai", "tool": "codex", "command_type": "vcs", "command_summary": "执行版本控制操作", "command_hash": "hmac-sha256:same"}}

	facts := NormalizeEvents([]RawEvidence{terminal, ai}, 1)

	if len(facts) != 1 || facts[0].ActorOrigin != "ai" || facts[0].CommandText != "" || len(facts[0].SourceEventIDs) != 2 || facts[0].CanonicalEventID != "ai-1" {
		t.Fatalf("facts = %#v", facts)
	}
}

func TestNormalizeEventsUsesIngestionWindowWhenTerminalHistoryHasNoTimestampOrProject(t *testing.T) {
	aiTime := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	terminalTime := aiTime.AddDate(0, 0, 6)
	terminal := evidence("terminal-history", "terminal.command", "redacted command", terminalTime)
	terminal.ProjectID = "fallback-project"
	terminal.IngestedAt = terminalTime
	terminal.Payload["command_hash"] = "hmac-sha256:historical"
	ai := RawEvidence{EventID: "ai-authoritative", TenantID: "tenant-1", SubjectID: "subject-1", DeviceID: "device-1", ProjectID: "real-project", EventType: "ai.tool_call", OccurredAt: aiTime, IngestedAt: terminalTime.Add(20 * time.Minute), Payload: map[string]any{"actor_origin": "ai", "tool": "codex", "tool_call_type": "shell", "command_type": "vcs", "command_summary": "执行版本控制操作", "command_hash": "hmac-sha256:historical"}}

	facts := NormalizeEvents([]RawEvidence{terminal, ai}, 1)

	if len(facts) != 1 || facts[0].CanonicalEventID != "ai-authoritative" || facts[0].ProjectID != "real-project" || !contains(facts[0].ReasonCodes, "project_inferred_from_ai") {
		t.Fatalf("facts = %#v", facts)
	}
}

func TestNormalizeEventsConsumesTerminalEvidenceOnlyOnce(t *testing.T) {
	at := time.Date(2026, 9, 7, 10, 0, 0, 0, time.UTC)
	terminal := evidence("terminal-once", "terminal.command", "redacted", at)
	terminal.Payload["command_hash"] = "hmac-sha256:once"
	aiOne := RawEvidence{EventID: "ai-one", TenantID: "tenant-1", SubjectID: "subject-1", DeviceID: "device-1", ProjectID: "project-1", EventType: "ai.tool_call", OccurredAt: at, IngestedAt: at, Payload: map[string]any{"tool": "codex", "tool_call_type": "shell", "command_hash": "hmac-sha256:once"}}
	aiTwo := aiOne
	aiTwo.EventID = "ai-two"
	aiTwo.OccurredAt = at.Add(time.Second)
	aiTwo.IngestedAt = at.Add(time.Second)

	facts := NormalizeEvents([]RawEvidence{terminal, aiOne, aiTwo}, 1)

	merged := 0
	for _, fact := range facts {
		if fact.MergeMethod == "ai_terminal_hash_match" {
			merged++
		}
	}
	if len(facts) != 2 || merged != 1 {
		t.Fatalf("facts = %#v", facts)
	}
}

func TestNormalizeEventsQuarantinesUnresolvedAutomationToolCall(t *testing.T) {
	at := time.Date(2026, 9, 7, 10, 0, 0, 0, time.UTC)
	automation := RawEvidence{EventID: "ai-dynamic", TenantID: "tenant-1", SubjectID: "subject-1", DeviceID: "device-1", ProjectID: "project-1", EventType: "ai.tool_call", OccurredAt: at, IngestedAt: at, Payload: map[string]any{"actor_origin": "ai", "tool": "codex", "tool_call_type": "automation", "command_type": "other", "command_summary": "执行终端操作", "command_hash": "hmac-sha256:wrapper"}}

	facts := NormalizeEvents([]RawEvidence{automation}, 1)

	if len(facts) != 1 || facts[0].QualityState != "quarantined" || !facts[0].ExcludedFromEffectiveness || !contains(facts[0].ReasonCodes, "unresolved_automation_tool_call") {
		t.Fatalf("facts = %#v", facts)
	}
}

func TestNormalizeEventsCreatesFactsForUnifiedActivities(t *testing.T) {
	at := time.Date(2026, 9, 7, 10, 0, 0, 0, time.UTC)
	events := []RawEvidence{
		{EventID: "git-1", TenantID: "tenant-1", SubjectID: "subject-1", DeviceID: "device-1", ProjectID: "project-1", EventType: "git.commit", Source: "core.git", OccurredAt: at, IngestedAt: at, Payload: map[string]any{"subject": "修复接口"}},
		{EventID: "ide-1", TenantID: "tenant-1", SubjectID: "subject-1", DeviceID: "device-1", ProjectID: "project-1", EventType: "ide.activity", Source: "core.vscode", OccurredAt: at.Add(time.Second), IngestedAt: at.Add(time.Second), Payload: map[string]any{"active_file": "main.go"}},
		{EventID: "browser-1", TenantID: "tenant-1", SubjectID: "subject-1", DeviceID: "device-1", ProjectID: "project-1", EventType: "browser.page_view", Source: "core.browser", OccurredAt: at.Add(2 * time.Second), IngestedAt: at.Add(2 * time.Second), Payload: map[string]any{"domain": "docs.example.com"}},
	}

	facts := NormalizeEvents(events, 2)

	if len(facts) != 3 {
		t.Fatalf("facts = %#v", facts)
	}
	want := []string{"version_control", "ide", "browser"}
	for index, fact := range facts {
		if fact.FactType != "activity" || fact.ActivityType != want[index] || fact.EventType != events[index].EventType || fact.ExcludedFromEffectiveness {
			t.Fatalf("fact[%d] = %#v", index, fact)
		}
	}
}

func TestSafeCommandDisplayNeverReturnsAICommandText(t *testing.T) {
	ai := Fact{ActorOrigin: "ai", CommandText: "secret raw command", CommandSummary: "执行测试"}
	human := Fact{ActorOrigin: "human", CommandText: "git status", CommandSummary: "版本控制"}

	if got := SafeCommandDisplay(ai); got != "执行测试" {
		t.Fatalf("AI display = %q", got)
	}
	if got := SafeCommandDisplay(human); got != "git status" {
		t.Fatalf("human display = %q", got)
	}
}

func TestVSCodeEventsCreateHumanAndSystemFacts(t *testing.T) {
	at := time.Date(2026, 9, 14, 8, 0, 0, 0, time.UTC)
	events := []RawEvidence{
		{EventID: "open-1", TenantID: "tenant-1", SubjectID: "subject-1", DeviceID: "device-1", ProjectID: "project-1", EventType: "ide.file_opened", Source: "core.vscode.extension", OccurredAt: at, IngestedAt: at, Payload: map[string]any{"relative_path": "src/app.ts", "language_id": "typescript"}},
		{EventID: "save-1", TenantID: "tenant-1", SubjectID: "subject-1", DeviceID: "device-1", ProjectID: "project-1", EventType: "ide.file_saved", Source: "core.vscode.extension", OccurredAt: at, IngestedAt: at, Payload: map[string]any{"relative_path": "src/app.ts"}},
		{EventID: "extension-1", TenantID: "tenant-1", SubjectID: "subject-1", DeviceID: "device-1", ProjectID: "project-1", EventType: "ide.extension_changed", Source: "core.vscode.extension", OccurredAt: at, IngestedAt: at, Payload: map[string]any{"extension_id": "vendor.private"}},
	}

	facts := NormalizeEvents(events, 2)

	if len(facts) != 3 || facts[0].ActorOrigin != "human" || facts[0].ActivityType != "ide" {
		t.Fatalf("open fact = %#v", facts)
	}
	if facts[1].ActorOrigin != "human" || facts[1].ActivityType != "delivery" {
		t.Fatalf("save fact = %#v", facts[1])
	}
	if facts[2].ActorOrigin != "system" || facts[2].ActivityType != "ide" {
		t.Fatalf("extension fact = %#v", facts[2])
	}
}

func TestOCRFallbackFactIsLowConfidenceHumanApplicationActivity(t *testing.T) {
	at := time.Date(2026, 9, 15, 8, 0, 0, 0, time.UTC)
	evidence := RawEvidence{EventID: "app-1", TenantID: "tenant-1", SubjectID: "subject-1", DeviceID: "device-1", ProjectID: "project-1", EventType: "application.activity", Source: "core.application.ocr", OccurredAt: at, IngestedAt: at, Payload: map[string]any{"application_name": "designer.exe", "window_context": "查看配置页面", "visible_text": "不应进入摘要", "capture_method": "ocr_fallback"}}

	facts := NormalizeEvents([]RawEvidence{evidence}, 3)

	if len(facts) != 1 || facts[0].ActivityType != "application" || facts[0].ActorOrigin != "human" || facts[0].Confidence != "low" || !contains(facts[0].ReasonCodes, "ocr_fallback") {
		t.Fatalf("fact = %#v", facts)
	}
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
