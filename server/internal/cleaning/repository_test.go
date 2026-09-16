package cleaning

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

type recordingRangeTx struct{ queries []string }

func (tx *recordingRangeTx) Exec(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
	tx.queries = append(tx.queries, strings.TrimSpace(sql))
	return pgconn.NewCommandTag("OK"), nil
}

func TestWriteReplacementUpsertsBeforeDeletingObsoleteFacts(t *testing.T) {
	tx := &recordingRangeTx{}
	facts := []Fact{{FactID: "fact-1", TenantID: "tenant-1", SubjectID: "subject-1", DeviceID: "device-1", ProjectID: "project-1", RuleVersion: 2, OccurredAt: time.Now(), FactType: "activity", EventType: "git.commit", Source: "core.git", ActivityType: "version_control", ActorOrigin: "unknown", MessageRole: "unknown", QualityState: "accepted", Confidence: "high", MergeMethod: "none", SourceEventIDs: []string{"event-1"}, CanonicalEventID: "event-1"}}

	if err := writeReplacement(context.Background(), tx, "tenant-1", 2, time.Now().Add(-time.Hour), time.Now().Add(time.Hour), facts); err != nil {
		t.Fatal(err)
	}
	if len(tx.queries) != 2 || !strings.HasPrefix(tx.queries[0], "INSERT INTO clean_event_facts") || !strings.HasPrefix(tx.queries[1], "DELETE FROM clean_event_facts") {
		t.Fatalf("queries = %#v", tx.queries)
	}
}
