package applicationpolicy

import "testing"

func TestDefaultPolicyIsEnabledAtRevisionZero(t *testing.T) {
	policy := DefaultPolicy()
	if !policy.Enabled || policy.Revision != 0 {
		t.Fatalf("policy=%#v", policy)
	}
}
