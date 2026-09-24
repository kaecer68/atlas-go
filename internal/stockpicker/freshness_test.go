// Tests for the per-symbol flow-file freshness contract (issue #1945).
package stockpicker

import (
	"testing"
	"time"
)

func mustFlowDate(t *testing.T, s string) time.Time {
	t.Helper()
	d, ok := FlowDate(s)
	if !ok {
		t.Fatalf("FlowDate(%q) = not ok", s)
	}
	return d
}

func TestFlowDate(t *testing.T) {
	if d := mustFlowDate(t, "2026-08-27"); d.Format("2006-01-02") != "2026-08-27" || d.Location() != time.UTC {
		t.Fatalf("FlowDate = %v (loc %v), want 2026-08-27 UTC", d, d.Location())
	}
	for _, bad := range []string{"", "20260827", "2026-13-01", "not-a-date"} {
		if _, ok := FlowDate(bad); ok {
			t.Errorf("FlowDate(%q) = ok, want not ok", bad)
		}
	}
}

// TestFlowStale pins the predicate both consumers share: a flow point is
// stale when the evaluation date is further ahead than maxAgeDays.
func TestFlowStale(t *testing.T) {
	tests := []struct {
		name   string
		date   string
		asOf   string
		maxAge int
		want   bool
	}{
		{"same day", "2026-09-23", "2026-09-23", 7, false},
		{"inside limit", "2026-09-17", "2026-09-23", 7, false},
		{"exactly at limit", "2026-09-16", "2026-09-23", 7, false},
		{"one day past limit", "2026-09-15", "2026-09-23", 7, true},
		// Production case: the file froze at 2026-08-27 and the gate ran on
		// 2026-09-23 — 27 days of silence, silently accepted before the fix.
		{"production freeze", "2026-08-27", "2026-09-23", 7, true},
		{"explicit limit zero falls back to default", "2026-09-15", "2026-09-23", 0, true},
		{"negative limit falls back to default", "2026-09-17", "2026-09-23", -1, false},
		{"tightened limit", "2026-09-20", "2026-09-23", 1, true},
		{"empty date is stale", "", "2026-09-23", 7, true},
		{"malformed date is stale", "2026-8-7", "2026-09-23", 7, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := FlowStale(tc.date, mustFlowDate(t, tc.asOf), tc.maxAge)
			if got != tc.want {
				t.Fatalf("FlowStale(%q, %s, %d) = %v, want %v", tc.date, tc.asOf, tc.maxAge, got, tc.want)
			}
		})
	}
}

// TestFlowStaleIgnoresTimeOfDay: only calendar dates matter, so a decision at
// 23:59 local does not count an extra day of staleness.
func TestFlowStaleIgnoresTimeOfDay(t *testing.T) {
	date := "2026-09-16"
	asOf := time.Date(2026, 9, 23, 23, 59, 59, 0, time.FixedZone("CST", 8*3600))
	if FlowStale(date, asOf, 7) {
		t.Fatal("FlowStale = true at 23:59 on the 7th day, want false")
	}
}
