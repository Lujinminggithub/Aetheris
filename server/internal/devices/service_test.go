package devices

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestCredentialLookupQualifiesJoinedColumns(t *testing.T) {
	sql := credentialLookupSQL()
	if !strings.Contains(sql, "SELECT c.tenant_id,c.device_id,d.subject_id") {
		t.Fatalf("credential query has ambiguous columns: %s", sql)
	}
}

func TestBootstrapRequestAcceptsSubjectName(t *testing.T) {
	var request BootstrapRequest
	if err := json.Unmarshal([]byte(`{"subject_name":"DOMAIN\\Alice"}`), &request); err != nil {
		t.Fatal(err)
	}
	if request.SubjectName != `DOMAIN\Alice` {
		t.Fatalf("subject name = %q", request.SubjectName)
	}
}

func TestHeartbeatStatusContainsCredentialIdentity(t *testing.T) {
	status := HeartbeatStatus{Status: "online", TenantID: "t1", SubjectID: "s1", DeviceID: "d1", ServerTime: time.Date(2026, 9, 6, 0, 0, 0, 0, time.UTC), ServerEventCount: 12, DataGeneration: "12:123"}
	raw, err := json.Marshal(status)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{`"tenant_id":"t1"`, `"subject_id":"s1"`, `"device_id":"d1"`, `"server_time":"2026-09-06T00:00:00Z"`, `"server_event_count":12`, `"data_generation":"12:123"`} {
		if !contains(string(raw), expected) {
			t.Fatalf("response %s missing %s", raw, expected)
		}
	}
}

func contains(value, expected string) bool {
	return len(value) >= len(expected) && json.Valid([]byte(value)) && stringIndex(value, expected) >= 0
}
func stringIndex(value, expected string) int {
	for index := 0; index+len(expected) <= len(value); index++ {
		if value[index:index+len(expected)] == expected {
			return index
		}
	}
	return -1
}
