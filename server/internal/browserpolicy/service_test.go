package browserpolicy

import (
	"reflect"
	"testing"
)

func TestNormalizeDomainsCanonicalizesAndDeduplicates(t *testing.T) {
	got, err := NormalizeDomains([]string{
		" HTTPS://Docs.Example.com/path ",
		"docs.example.com.",
		"portal.example.com:8443",
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"docs.example.com", "portal.example.com"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("domains = %#v, want %#v", got, want)
	}
}

func TestNormalizeDomainsRejectsUnsafeValues(t *testing.T) {
	for _, value := range []string{"*.example.com", "user:pass@example.com", "bad_domain.example", "ftp://example.com", ""} {
		if _, err := NormalizeDomains([]string{value}); err == nil {
			t.Fatalf("expected %q to be rejected", value)
		}
	}
}

func TestValidatePolicyRequiresDomainWhenEnabled(t *testing.T) {
	if err := ValidatePolicy(true, nil); err == nil {
		t.Fatal("enabled policy without domains must fail")
	}
}
