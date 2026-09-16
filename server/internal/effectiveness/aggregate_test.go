package effectiveness

import (
	"fmt"
	"testing"
	"time"
)

func at(value string) time.Time {
	result, err := time.Parse(time.RFC3339, value)
	if err != nil {
		panic(err)
	}
	return result
}

func TestAggregateDayDeduplicatesDevicesInSameBucket(t *testing.T) {
	events := []EventPoint{
		{EventID: "e1", DeviceID: "d1", ProjectID: "p1", EventType: "ide.activity", Source: "core.vscode", OccurredAt: at("2026-09-06T01:01:00Z")},
		{EventID: "e2", DeviceID: "d2", ProjectID: "p1", EventType: "ide.activity", Source: "core.vscode", OccurredAt: at("2026-09-06T01:04:00Z")},
	}
	got := AggregateDay(events, at("2026-09-06T00:00:00Z"), time.UTC)
	if got.ActiveWindowMinutes != 5 {
		t.Fatalf("active minutes = %d", got.ActiveWindowMinutes)
	}
	if len(got.DeviceIDs) != 2 {
		t.Fatalf("device ids = %#v", got.DeviceIDs)
	}
}

func TestAggregateDayUsesProjectNameForBreakdown(t *testing.T) {
	events := []EventPoint{{EventID: "e1", DeviceID: "d1", ProjectID: "project-abc", ProjectName: "jtagent", EventType: "ide.activity", Source: "core.vscode", OccurredAt: at("2026-09-06T01:01:00Z")}}
	got := AggregateDay(events, at("2026-09-06T00:00:00Z"), time.UTC)
	if _, ok := got.ProjectBreakdown["jtagent"]; !ok {
		t.Fatalf("project breakdown = %#v", got.ProjectBreakdown)
	}
	if _, ok := got.ProjectBreakdown["project-abc"]; ok {
		t.Fatalf("raw project id leaked into breakdown: %#v", got.ProjectBreakdown)
	}
}

func TestAggregateDayUsesStrictThirtyMinuteSessionBoundary(t *testing.T) {
	events := []EventPoint{
		{EventID: "e1", ProjectID: "p1", OccurredAt: at("2026-09-06T00:00:00Z")},
		{EventID: "e2", ProjectID: "p1", OccurredAt: at("2026-09-06T00:30:00Z")},
		{EventID: "e3", ProjectID: "p1", OccurredAt: at("2026-09-06T01:01:00Z")},
	}
	got := AggregateDay(events, at("2026-09-06T00:00:00Z"), time.UTC)
	if got.SessionCount != 2 {
		t.Fatalf("session count = %d", got.SessionCount)
	}
}

func TestAggregateDayFindsTwentyFiveMinuteFocusBlock(t *testing.T) {
	events := make([]EventPoint, 0, 5)
	for index := 0; index < 5; index++ {
		events = append(events, EventPoint{
			EventID: fmt.Sprintf("e%d", index), ProjectID: "p1", EventType: "ide.activity",
			OccurredAt: at("2026-09-06T00:00:00Z").Add(time.Duration(index*5) * time.Minute),
		})
	}
	got := AggregateDay(events, at("2026-09-06T00:00:00Z"), time.UTC)
	if got.FocusBlockCount != 1 || got.FocusBlockMinutes != 25 {
		t.Fatalf("focus = %d blocks, %d minutes", got.FocusBlockCount, got.FocusBlockMinutes)
	}
}

func TestAggregateDayCountsProjectSwitchOnlyInsideSession(t *testing.T) {
	events := []EventPoint{
		{EventID: "e1", ProjectID: "p1", OccurredAt: at("2026-09-06T00:00:00Z")},
		{EventID: "e2", ProjectID: "p2", OccurredAt: at("2026-09-06T00:05:00Z")},
		{EventID: "e3", ProjectID: "p1", OccurredAt: at("2026-09-06T00:40:01Z")},
	}
	got := AggregateDay(events, at("2026-09-06T00:00:00Z"), time.UTC)
	if got.ContextSwitchCount != 1 {
		t.Fatalf("context switches = %d", got.ContextSwitchCount)
	}
}

func TestClassifyUsesDeliveryBeforeAISource(t *testing.T) {
	if got := Classify("git.commit", "core.ai.codex"); got != ActivityDelivery {
		t.Fatalf("class = %s", got)
	}
}

func TestMetricDefinitionVersionIncludesUnifiedFacts(t *testing.T) {
	if MetricDefinitionVersion != 6 {
		t.Fatalf("metric definition version = %d", MetricDefinitionVersion)
	}
}

func TestClassifyAIToolCallAsAICollaboration(t *testing.T) {
	if got := Classify("ai.tool_call", "clean.core.ai.codex"); got != ActivityAI {
		t.Fatalf("class = %s", got)
	}
}

func TestAggregateDayCountsBrowserAsIndependentActivity(t *testing.T) {
	events := []EventPoint{{
		EventID: "browser-1", ProjectID: "p1", EventType: "browser.page_view", Source: "core.browser",
		OccurredAt: at("2026-09-06T01:01:00Z"),
	}}
	got := AggregateDay(events, at("2026-09-06T00:00:00Z"), time.UTC)
	if got.BrowserEvents != 1 || got.OtherEvents != 0 {
		t.Fatalf("browser=%d other=%d", got.BrowserEvents, got.OtherEvents)
	}
}

func TestAggregateDayCountsExplainableVSCodeBehavior(t *testing.T) {
	events := []EventPoint{
		{EventID: "open-1", ProjectID: "p1", EventType: "ide.file_opened", Source: "clean.core.vscode.extension", OccurredAt: at("2026-09-06T01:01:00Z")},
		{EventID: "edit-1", ProjectID: "p1", EventType: "ide.file_edited", Source: "clean.core.vscode.extension", OccurredAt: at("2026-09-06T01:02:00Z")},
		{EventID: "save-1", ProjectID: "p1", EventType: "ide.file_saved", Source: "clean.core.vscode.extension", OccurredAt: at("2026-09-06T01:03:00Z")},
	}
	got := AggregateDay(events, at("2026-09-06T00:00:00Z"), time.UTC)
	if got.IDEFileOpenedEvents != 1 || got.IDEEditSessions != 1 || got.IDEFileSavedEvents != 1 {
		t.Fatalf("VS Code metrics = %#v", got)
	}
	if got.CodingEvents != 2 || got.DeliveryEvents != 1 {
		t.Fatalf("classification coding=%d delivery=%d", got.CodingEvents, got.DeliveryEvents)
	}
}

func TestApplicationOCRDoesNotDuplicateActiveWindowWithNativeFact(t *testing.T) {
	events := []EventPoint{
		{EventID: "native-1", DeviceID: "d1", ProjectID: "p1", EventType: "ide.activity", Source: "clean.core.vscode", OccurredAt: at("2026-09-15T01:01:00Z")},
		{EventID: "ocr-1", DeviceID: "d1", ProjectID: "p1", EventType: "application.activity", Source: "clean.core.application.ocr", OccurredAt: at("2026-09-15T01:02:00Z")},
	}
	got := AggregateDay(events, at("2026-09-15T00:00:00Z"), time.UTC)
	if got.ActiveWindowMinutes != 5 || Classify("application.activity", "core.application.ocr") != ActivityCoding {
		t.Fatalf("metrics = %#v", got)
	}
}

func TestUnifiedFactRuleDoesNotReadCoveredRawEvents(t *testing.T) {
	for _, event := range []EventPoint{
		{EventType: "ai.message", Source: "core.ai.codex"},
		{EventType: "terminal.command", Source: "core.terminal"},
		{EventType: "git.commit", Source: "core.git"},
		{EventType: "ide.activity", Source: "core.vscode"},
		{EventType: "browser.page_view", Source: "core.browser"},
	} {
		if ShouldReadRawEvent(2, event.EventType, event.Source) {
			t.Fatalf("rule 2 read covered raw event: %#v", event)
		}
	}
	if !ShouldReadRawEvent(1, "git.commit", "core.git") {
		t.Fatal("rule 1 must retain raw Git compatibility")
	}
	if !ShouldReadRawEvent(2, "custom.audit", "core.custom") {
		t.Fatal("unknown raw activity was lost")
	}
}

func TestAggregateDayLimitsEvidenceToFiftyEvents(t *testing.T) {
	events := make([]EventPoint, 0, 60)
	for index := 0; index < 60; index++ {
		events = append(events, EventPoint{EventID: fmt.Sprintf("e-%02d", index), ProjectID: "p1", OccurredAt: at("2026-09-06T00:00:00Z").Add(time.Duration(index) * time.Minute)})
	}
	got := AggregateDay(events, at("2026-09-06T00:00:00Z"), time.UTC)
	if len(got.EvidenceEventIDs) != 50 {
		t.Fatalf("evidence count = %d", len(got.EvidenceEventIDs))
	}
}
