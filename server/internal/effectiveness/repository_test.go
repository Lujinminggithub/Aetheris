package effectiveness

import (
	"reflect"
	"testing"
)

func TestEventPointProjectionMatchesScanOrder(t *testing.T) {
	want := []string{
		"event_id",
		"project_id",
		"project_name",
		"device_id",
		"event_type",
		"source",
		"work_role_code",
		"occurred_at",
	}
	if got := eventPointProjectionColumns(); !reflect.DeepEqual(got, want) {
		t.Fatalf("event point projection = %#v, want %#v", got, want)
	}
}
