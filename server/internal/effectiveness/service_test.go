package effectiveness

import (
	"testing"
	"time"
)

func TestPercentChangeIsNilWhenPreviousIsZero(t *testing.T) {
	if got := PercentChange(10, 0); got != nil {
		t.Fatalf("percent change = %v", *got)
	}
}

func TestPercentChangeUsesPreviousAsDenominator(t *testing.T) {
	got := PercentChange(15, 10)
	if got == nil || *got != 50 {
		t.Fatalf("percent change = %v", got)
	}
}

func TestValidateQueryRangeLimitsNinetyDays(t *testing.T) {
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := ValidateQueryRange(from, from.AddDate(0, 0, 89)); err != nil {
		t.Fatal(err)
	}
	if err := ValidateQueryRange(from, from.AddDate(0, 0, 90)); err == nil {
		t.Fatal("expected 91 calendar days to be rejected")
	}
}

func TestCoverageUsesCalendarDays(t *testing.T) {
	daily := []DailyMetrics{{LocalDate: "2026-09-01", ActiveWindowMinutes: 5}, {LocalDate: "2026-09-03", ActiveWindowMinutes: 5}}
	coverage := CalculateCoverage(daily, 4)
	if coverage.CoveredDays != 2 || coverage.PeriodDays != 4 || coverage.CoverageRatio != 0.5 {
		t.Fatalf("coverage = %#v", coverage)
	}
}
