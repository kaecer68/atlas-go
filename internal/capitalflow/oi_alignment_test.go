package capitalflow

import (
	"errors"
	"testing"
)

func TestAssertOIDateAlignment_PassWhenIdentical(t *testing.T) {
	fm := map[string]float64{"2026-08-28": -82970, "2026-08-31": -83655}
	mac := map[string]float64{"2026-08-28": -82970, "2026-08-31": -83655}
	if err := AssertOIDateAlignment(fm, mac); err != nil {
		t.Fatalf("want aligned, got error: %v", err)
	}
}

func TestAssertOIDateAlignment_FailsOnSameDayDisagreement(t *testing.T) {
	// Same-day disagreement must fail even when the value equals the
	// PREVIOUS session (the observed carry-forward fingerprint).
	fm := map[string]float64{"2026-08-28": -82970, "2026-08-31": -83655}
	mac := map[string]float64{"2026-08-28": -82970, "2026-08-31": -82970} // 08-31 carried 08-28's value
	err := AssertOIDateAlignment(fm, mac)
	if err == nil {
		t.Fatal("want alignment failure for same-day disagreement")
	}
}

func TestAssertOIDateAlignment_NonOverlapDaysIgnored(t *testing.T) {
	fm := map[string]float64{"2021-06-01": -100}
	mac := map[string]float64{"2026-08-31": -83655}
	if err := AssertOIDateAlignment(fm, mac); err != nil {
		t.Fatalf("non-overlap days must not fail alignment, got: %v", err)
	}
}

func TestDetectMacroOILagPattern(t *testing.T) {
	fm := map[string]float64{"2026-08-28": -82970, "2026-08-31": -83655, "2026-09-01": -85000}
	mac := map[string]float64{
		"2026-08-28": -82970, // own session — not a lag
		"2026-08-31": -82970, // equals fm(prev trading day) — lag fingerprint
		"2026-09-01": -85000, // own session — not a lag
	}
	prev := map[string]string{"2026-08-31": "2026-08-28", "2026-09-01": "2026-08-31", "2026-08-28": "2026-08-27"}
	got := DetectMacroOILagPattern(fm, mac, func(d string) string { return prev[d] })
	if got != 1 {
		t.Fatalf("DetectMacroOILagPattern = %d, want 1", got)
	}
}

func TestMacroOISignalInputGuard_Blocked(t *testing.T) {
	err := MacroOISignalInputGuard()
	if !errors.Is(err, ErrMacroOISignalInputBlocked) {
		t.Fatalf("guard must return ErrMacroOISignalInputBlocked, got: %v", err)
	}
}
