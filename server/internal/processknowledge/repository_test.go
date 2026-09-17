package processknowledge

import (
	"strings"
	"testing"
)

func TestSourceTurnQueryUsesLogicalProjectAndStructuredSession(t *testing.T) {
	for _, fragment := range []string{
		"current_event_projects",
		"ep.logical_project_id IS NOT NULL",
		"COALESCE(NULLIF(e.session_id,''),NULLIF(e.payload->>'session_id',''))",
		"NOT EXISTS",
		"process_turns",
		"classification_version=$4",
		"f.command_summary",
	} {
		if !strings.Contains(sourceTurnsSQL, fragment) {
			t.Fatalf("source turn query missing %q", fragment)
		}
	}
	if strings.Contains(sourceTurnsSQL, "INTERVAL '15 minutes'") {
		t.Fatal("repository must not join structured replies by time window")
	}
}

func TestIncrementalTurnSequenceAppendsAfterExistingTurns(t *testing.T) {
	if got := nextTurnSequence(7, 0); got != 8 {
		t.Fatalf("sequence=%d", got)
	}
	if got := nextTurnSequence(-1, 3); got != 3 {
		t.Fatalf("initial sequence=%d", got)
	}
}

func TestKnowledgeExtractionWaitsForIdleSessionAndInitialVersionUsesShadow(t *testing.T) {
	if !strings.Contains(dirtySessionsSQL, "ended_at < NOW()-INTERVAL '5 minutes'") {
		t.Fatal("active sessions can be extracted before becoming idle")
	}
	if !strings.Contains(activateInitialVersionSQL, "'shadow'") || !strings.Contains(activateInitialVersionSQL, "ON CONFLICT") {
		t.Fatal("first completed version is not activated in shadow mode")
	}
	if !strings.Contains(dirtySessionsSQL, "logical_project_id=$3") {
		t.Fatal("backfill extraction is not scoped to its logical project")
	}
}

func TestDryRunCountIsNotLimitedToWorkerBatch(t *testing.T) {
	if strings.Contains(sourceTurnCountSQL, "LIMIT") || strings.Contains(sourceTurnCountSQL, "process_turns") {
		t.Fatalf("dry-run count must inspect all eligible source facts without materialization filters: %s", sourceTurnCountSQL)
	}
}

func TestJobClaimDoesNotReenterRunningJob(t *testing.T) {
	if strings.Contains(claimJobSQL, "state IN ('pending','running')") || !strings.Contains(claimJobSQL, "state='pending'") {
		t.Fatalf("unsafe job claim: %s", claimJobSQL)
	}
	if !strings.Contains(recoverStaleJobsSQL, "INTERVAL '10 minutes'") {
		t.Fatalf("stale job recovery is not bounded: %s", recoverStaleJobsSQL)
	}
	if !strings.Contains(activeJobsSQL, "state IN ('pending','running')") {
		t.Fatalf("incremental processing does not pause for backfills: %s", activeJobsSQL)
	}
}
