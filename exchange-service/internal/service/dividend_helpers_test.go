package service_test

import (
	"testing"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/service"
)

func TestQuarterPeriod(t *testing.T) {
	cases := map[string]string{
		"2026-01-15": "2026-Q1",
		"2026-03-31": "2026-Q1",
		"2026-04-01": "2026-Q2",
		"2026-06-30": "2026-Q2",
		"2026-09-30": "2026-Q3",
		"2026-12-31": "2026-Q4",
	}
	for in, want := range cases {
		ts, _ := time.Parse("2006-01-02", in)
		if got := service.QuarterPeriod(ts); got != want {
			t.Errorf("QuarterPeriod(%s)=%s, want %s", in, got, want)
		}
	}
}

func TestIsLastWorkingDayOfQuarter(t *testing.T) {
	// 2026-06-30 is a Tuesday — the last working day of Q2.
	if d, _ := time.Parse("2006-01-02", "2026-06-30"); !service.IsLastWorkingDayOfQuarter(d) {
		t.Error("2026-06-30 (Tue) should be the last working day of Q2")
	}
	// 2026-03-31 is a Tuesday — last working day of Q1.
	if d, _ := time.Parse("2006-01-02", "2026-03-31"); !service.IsLastWorkingDayOfQuarter(d) {
		t.Error("2026-03-31 (Tue) should be the last working day of Q1")
	}
	// Not a quarter-end month.
	if d, _ := time.Parse("2006-01-02", "2026-04-30"); service.IsLastWorkingDayOfQuarter(d) {
		t.Error("2026-04-30 is not in a quarter-closing month")
	}
	// A quarter-end month but not its last working day.
	if d, _ := time.Parse("2006-01-02", "2026-06-29"); service.IsLastWorkingDayOfQuarter(d) {
		t.Error("2026-06-29 is not the last working day of June")
	}
	// 2026-05-31 is a Sunday; May isn't a quarter close anyway.
	if d, _ := time.Parse("2006-01-02", "2026-05-31"); service.IsLastWorkingDayOfQuarter(d) {
		t.Error("May is not a quarter-closing month")
	}
}

// TestLastWorkingDayWeekendBackoff confirms a weekend month-end rolls back to
// Friday: 2024-03-31 was a Sunday, so the last working day is 2024-03-29 (Fri).
func TestLastWorkingDayWeekendBackoff(t *testing.T) {
	if d, _ := time.Parse("2006-01-02", "2024-03-29"); !service.IsLastWorkingDayOfQuarter(d) {
		t.Error("2024-03-29 (Fri) should be the last working day of Q1 2024")
	}
	if d, _ := time.Parse("2006-01-02", "2024-03-31"); service.IsLastWorkingDayOfQuarter(d) {
		t.Error("2024-03-31 (Sun) is a weekend, not the last working day")
	}
}
