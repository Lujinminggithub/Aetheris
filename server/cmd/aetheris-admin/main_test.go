package main

import (
	"reflect"
	"testing"
	"time"
)

func TestSplitEffectivenessRangesUsesInclusiveThirtyOneDayChunks(t *testing.T) {
	location := time.FixedZone("Asia/Shanghai", 8*60*60)
	from := time.Date(2026, 5, 8, 0, 0, 0, 0, location)
	to := time.Date(2026, 7, 16, 0, 0, 0, 0, location)

	got := splitEffectivenessRanges(from, to)
	want := [][2]string{
		{"2026-05-08", "2026-06-07"},
		{"2026-06-08", "2026-07-08"},
		{"2026-07-09", "2026-07-16"},
	}
	if len(got) != len(want) {
		t.Fatalf("range count = %d, want %d", len(got), len(want))
	}
	for index, item := range got {
		if item[0].Format("2006-01-02") != want[index][0] || item[1].Format("2006-01-02") != want[index][1] {
			t.Fatalf("range %d = %s..%s, want %s..%s", index, item[0].Format("2006-01-02"), item[1].Format("2006-01-02"), want[index][0], want[index][1])
		}
	}
}

func TestParseProcessKnowledgeBackfillArgs(t *testing.T) {
	got, err := parseProcessKnowledgeBackfillArgs([]string{"--mode", "apply", "--project", "logical-safe", "--version", "2"})
	if err != nil {
		t.Fatal(err)
	}
	want := processKnowledgeBackfillOptions{Mode: "apply", ProjectID: "logical-safe", Version: 2}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got=%+v want=%+v", got, want)
	}
}

func TestSplitCleaningRangesUsesDailyChunks(t *testing.T) {
	location := time.FixedZone("Asia/Shanghai", 8*60*60)
	from := time.Date(2026, 9, 3, 0, 0, 0, 0, location)
	to := time.Date(2026, 9, 5, 0, 0, 0, 0, location)

	got := splitCleaningRanges(from, to)
	want := [][2]string{
		{"2026-09-03", "2026-09-03"},
		{"2026-09-04", "2026-09-04"},
		{"2026-09-05", "2026-09-05"},
	}
	if len(got) != len(want) {
		t.Fatalf("range count = %d, want %d", len(got), len(want))
	}
	for index, item := range got {
		if item[0].Format("2006-01-02") != want[index][0] || item[1].Format("2006-01-02") != want[index][1] {
			t.Fatalf("range %d = %s..%s, want %s..%s", index, item[0].Format("2006-01-02"), item[1].Format("2006-01-02"), want[index][0], want[index][1])
		}
	}
}
