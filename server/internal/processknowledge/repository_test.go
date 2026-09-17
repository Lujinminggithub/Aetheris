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
	if !strings.Contains(sourceTurnsSQL, "newer.rule_version>f.rule_version") || strings.Contains(sourceTurnsSQL, "DISTINCT ON") || strings.Contains(sourceTurnsSQL, "f.rule_version=(SELECT MAX(rule_version)") {
		t.Fatal("source turns must use the latest version of each fact instead of the latest partial backfill version")
	}
	if !strings.Contains(sourceTurnsSQL, "source_event_ids @> ARRAY[f.canonical_event_id]") {
		t.Fatal("source turn anti-join must use the GIN-indexable array containment operator")
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
	if !strings.Contains(sourceTurnCountSQL, "newer.rule_version>f.rule_version") || strings.Contains(sourceTurnCountSQL, "DISTINCT ON") || strings.Contains(sourceTurnCountSQL, "f.rule_version=(SELECT MAX(rule_version)") {
		t.Fatal("dry-run count must include historical facts whose date range has not been recomputed with the newest rule")
	}
}

func TestApplyBackfillMarksAllScopedSessionsDirtyForCompleteVersion(t *testing.T) {
	if !strings.Contains(markBackfillSessionsDirtySQL, "dirty=TRUE") || !strings.Contains(markBackfillSessionsDirtySQL, "logical_project_id=$2") {
		t.Fatalf("backfill does not rebuild a complete scoped snapshot: %s", markBackfillSessionsDirtySQL)
	}
}

func TestKeywordQueryRanksExactTermFrequencyBeforeTrigramSimilarity(t *testing.T) {
	for _, fragment := range []string{"unnest($4::text[])", "replace(lower(c.search_text)", "exact_hits DESC"} {
		if !strings.Contains(keywordCandidatesSQL, fragment) {
			t.Fatalf("keyword query missing %q", fragment)
		}
	}
}

func TestPendingKnowledgeChunksPrioritizeActiveVersionAndNewestCreatedBatch(t *testing.T) {
	if !strings.Contains(pendingChunksSQL, "active_version") || !strings.Contains(pendingChunksSQL, "created_at DESC,chunk_id") {
		t.Fatalf("unsafe knowledge index order: %s", pendingChunksSQL)
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
