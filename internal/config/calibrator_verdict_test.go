package config

import (
	"strings"
	"testing"
)

// TestCalibratorVerdict_Table pins the report decision table of
// CalibrateParameters, including the arm that a mutation is most likely to
// delete: a run that wrote nothing because every write failed must not be
// reported as the benign "stable"/"unchanged" outcome (issue #1944 Batch 2,
// E-item; Batch A added the missing test).
//
// Why the table test calls the decision function instead of CalibrateParameters:
// the failed-write arm cannot be reached end to end today, and that is measured,
// not assumed.
//   - Every name the loop accepts is writable: GetParameter resolves either a
//     parameterTable entry or a map sub-key that already exists, and both are
//     settable. Enumerating all 245 names the shipped config resolves gave
//     245 writes accepted and 0 refused (probe, 2026-09-27).
//   - A name that is not resolvable never reaches the loop: the optimizer aborts
//     first with "calibrate: optimize: unknown parameter: <name>".
//
// The arm stays anyway: it is the guard against reporting failed writes as a
// healthy outcome, so it must keep a test even while it is defensive. If a
// future setter can fail (or the loop starts counting unresolvable names as
// failures), this table already describes the required behaviour.
func TestCalibratorVerdict_Table(t *testing.T) {
	tests := []struct {
		name          string
		appliedCount  int
		failedCount   int
		paramCount    int
		baseline      float64
		optScore      float64
		improvement   float64
		wantVerdict   string
		wantSummaryIn []string
		wantOtherThan []string
	}{
		{
			name:          "all writes failed is failed, not a benign verdict",
			appliedCount:  0,
			failedCount:   3,
			paramCount:    5,
			baseline:      0.10,
			optScore:      0.20,
			improvement:   100.0, // a real improvement exists, yet nothing was written
			wantVerdict:   "failed",
			wantSummaryIn: []string{"0/5 parameter changes applied", "3 set_parameter failures"},
			// Without the failed arm the run falls through to "stable" (improvement
			// > 0) or "unchanged", i.e. the operator reads a healthy outcome for a
			// run whose writes errored.
			wantOtherThan: []string{"calibrated", "stable", "unchanged"},
		},
		{
			name:          "partial write failure does not hide the applied changes",
			appliedCount:  2,
			failedCount:   1,
			paramCount:    5,
			baseline:      0.10,
			optScore:      0.20,
			improvement:   100.0,
			wantVerdict:   "calibrated",
			wantSummaryIn: []string{"applied 2/5 parameter changes", "1 set_parameter failures"},
			wantOtherThan: []string{"failed", "stable", "unchanged"},
		},
		{
			name:          "no changes needed and no failures is stable",
			appliedCount:  0,
			failedCount:   0,
			paramCount:    5,
			baseline:      0.10,
			optScore:      0.11,
			improvement:   10.0,
			wantVerdict:   "stable",
			wantSummaryIn: []string{"no significant changes", "+10.0%"},
			wantOtherThan: []string{"calibrated", "failed", "unchanged"},
		},
		{
			name:          "no improvement and no failures is unchanged",
			appliedCount:  0,
			failedCount:   0,
			paramCount:    5,
			baseline:      0.10,
			optScore:      0.10,
			improvement:   0.0,
			wantVerdict:   "unchanged",
			wantSummaryIn: []string{"current values optimal", "baseline=0.1000"},
			wantOtherThan: []string{"calibrated", "failed", "stable"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			verdict, summary := calibratorVerdict(tt.appliedCount, tt.failedCount, tt.paramCount,
				tt.baseline, tt.optScore, tt.improvement)
			if verdict != tt.wantVerdict {
				t.Errorf("verdict = %q, want %q (summary %q)", verdict, tt.wantVerdict, summary)
			}
			for _, want := range tt.wantSummaryIn {
				if !strings.Contains(summary, want) {
					t.Errorf("summary = %q, want it to contain %q", summary, want)
				}
			}
			for _, other := range tt.wantOtherThan {
				if verdict == other {
					t.Errorf("verdict = %q, must not be %q", verdict, other)
				}
			}
		})
	}
}
