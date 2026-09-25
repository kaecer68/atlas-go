package industry

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestSummarizeCalibrationHealth_MissingSection asserts the false-healthy
// trap is closed: when parameters.json has no industry.seasonal_patterns
// section, the summary must report critical / no_calibration_data rather
// than collapsing to healthy via PatternCount=0.
func TestSummarizeCalibrationHealth_MissingSection(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "parameters.json")
	if err := os.WriteFile(path, []byte(`{"industry": {}}`), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	summary, err := SummarizeCalibrationHealth(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if summary.Health != HealthCritical {
		t.Errorf("Health = %q, want %q", summary.Health, HealthCritical)
	}
	if summary.Reason != ReasonNoCalibrationData {
		t.Errorf("Reason = %q, want %q", summary.Reason, ReasonNoCalibrationData)
	}
}

// TestSummarizeCalibrationHealth_MalformedValue asserts that when
// seasonal_patterns.value exists but is not a JSON array (e.g., an
// object or a scalar), the summary reports critical / malformed_calibration_data.
func TestSummarizeCalibrationHealth_MalformedValue(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "parameters.json")
	if err := os.WriteFile(path, []byte(`{"industry":{"seasonal_patterns":{"value":"oops"}}}`), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	summary, err := SummarizeCalibrationHealth(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if summary.Health != HealthCritical {
		t.Errorf("Health = %q, want %q", summary.Health, HealthCritical)
	}
	if summary.Reason != ReasonMalformedData {
		t.Errorf("Reason = %q, want %q", summary.Reason, ReasonMalformedData)
	}
}

// TestSummarizeCalibrationHealth_EmptyArray asserts that an empty pattern
// array reports degraded / no_calibrated_patterns, not healthy.
func TestSummarizeCalibrationHealth_EmptyArray(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "parameters.json")
	if err := os.WriteFile(path, []byte(`{"industry":{"seasonal_patterns":{"value":[]}}}`), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	summary, err := SummarizeCalibrationHealth(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if summary.Health != HealthDegraded {
		t.Errorf("Health = %q, want %q", summary.Health, HealthDegraded)
	}
	if summary.Reason != ReasonNoCalibratedPatterns {
		t.Errorf("Reason = %q, want %q", summary.Reason, ReasonNoCalibratedPatterns)
	}
}

// TestSummarizeCalibrationHealth_AllHealthy covers the happy path: 3
// patterns, all Darwinian-compliant, sufficient observations.
func TestSummarizeCalibrationHealth_AllHealthy(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "parameters.json")
	body := `{
		"industry":{
			"seasonal_patterns":{
				"value":[
					{
						"adjustment_factor":1.0,
						"historical_accuracy":0.65,
						"avg_market_return":0.04,
						"calibration_observations":5,
						"calibration_verdict":"validated",
						"calibration_timestamp":"2026-06-01T00:00:00Z"
					},
					{
						"adjustment_factor":0.8,
						"historical_accuracy":0.7,
						"avg_market_return":-0.02,
						"calibration_observations":6,
						"calibration_verdict":"validated",
						"calibration_timestamp":"2026-06-02T00:00:00Z"
					},
					{
						"adjustment_factor":1.5,
						"historical_accuracy":0.55,
						"avg_market_return":0.10,
						"calibration_observations":4,
						"calibration_verdict":"overstated",
						"calibration_timestamp":"2026-05-15T00:00:00Z"
					}
				]
			}
		}
	}`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	summary, err := SummarizeCalibrationHealth(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if summary.Health != HealthHealthy {
		t.Errorf("Health = %q, want %q (Reason=%q)", summary.Health, HealthHealthy, summary.Reason)
	}
	if summary.PatternCount != 3 {
		t.Errorf("PatternCount = %d, want 3", summary.PatternCount)
	}
	if summary.TotalObservations != 15 {
		t.Errorf("TotalObservations = %d, want 15", summary.TotalObservations)
	}
	if summary.DarwinianViolations != 0 {
		t.Errorf("DarwinianViolations = %d, want 0", summary.DarwinianViolations)
	}
	if summary.OutOfRangeCount != 0 {
		t.Errorf("OutOfRangeCount = %d, want 0", summary.OutOfRangeCount)
	}
	if summary.LastCalibratedAt == nil {
		t.Fatalf("LastCalibratedAt should be set to 2026-06-02")
	}
	if !summary.LastCalibratedAt.Equal(time.Date(2026, 6, 2, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("LastCalibratedAt = %v, want 2026-06-02 UTC", summary.LastCalibratedAt)
	}
	if summary.VerdictCounts["validated"] != 2 {
		t.Errorf("VerdictCounts[validated] = %d, want 2", summary.VerdictCounts["validated"])
	}
	if summary.VerdictCounts["overstated"] != 1 {
		t.Errorf("VerdictCounts[overstated] = %d, want 1", summary.VerdictCounts["overstated"])
	}
}

// TestSummarizeCalibrationHealth_DarwinianViolation asserts a single
// out-of-Darwinian pattern is sufficient to mark health=critical.
func TestSummarizeCalibrationHealth_DarwinianViolation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "parameters.json")
	body := `{"industry":{"seasonal_patterns":{"value":[
		{"adjustment_factor":3.0,"historical_accuracy":0.5,"avg_market_return":0.05,"calibration_observations":4}
	]}}}`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	summary, err := SummarizeCalibrationHealth(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if summary.Health != HealthCritical {
		t.Errorf("Health = %q, want %q", summary.Health, HealthCritical)
	}
	if summary.Reason != ReasonDarwinianViolations {
		t.Errorf("Reason = %q, want %q", summary.Reason, ReasonDarwinianViolations)
	}
	if summary.DarwinianViolations != 1 {
		t.Errorf("DarwinianViolations = %d, want 1", summary.DarwinianViolations)
	}
}

// TestSummarizeCalibrationHealth_InsufficientSamples asserts the
// degraded-but-not-critical path: patterns are valid but under-observed.
func TestSummarizeCalibrationHealth_InsufficientSamples(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "parameters.json")
	body := `{"industry":{"seasonal_patterns":{"value":[
		{"adjustment_factor":1.0,"historical_accuracy":0.6,"avg_market_return":0.02,"calibration_observations":1},
		{"adjustment_factor":1.2,"historical_accuracy":0.6,"avg_market_return":0.02,"calibration_observations":2}
	]}}}`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	summary, err := SummarizeCalibrationHealth(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if summary.Health != HealthDegraded {
		t.Errorf("Health = %q, want %q", summary.Health, HealthDegraded)
	}
	if summary.Reason != ReasonInsufficientSamples {
		t.Errorf("Reason = %q, want %q", summary.Reason, ReasonInsufficientSamples)
	}
}

// TestPeriodReturn_RejectsNaN guards the new non-finite input contract.
func TestPeriodReturn_RejectsNaN(t *testing.T) {
	if got := periodReturn([]float64{0.1, math.NaN(), 0.2}); got != 0 {
		t.Errorf("periodReturn with NaN = %v, want 0", got)
	}
}

// TestPeriodReturn_RejectsInf guards the new non-finite input contract.
func TestPeriodReturn_RejectsInf(t *testing.T) {
	if got := periodReturn([]float64{0.1, math.Inf(1), 0.2}); got != 0 {
		t.Errorf("periodReturn with +Inf = %v, want 0", got)
	}
	if got := periodReturn([]float64{0.1, math.Inf(-1), 0.2}); got != 0 {
		t.Errorf("periodReturn with -Inf = %v, want 0", got)
	}
}

// TestPeriodReturn_FiniteInputsUnchanged ensures the guard does not
// affect legitimate compound return computation.
func TestPeriodReturn_FiniteInputsUnchanged(t *testing.T) {
	got := periodReturn([]float64{0.01, 0.01, 0.01})
	want := math.Pow(1.01, 3) - 1
	if math.Abs(got-want) > 1e-12 {
		t.Errorf("periodReturn = %v, want %v", got, want)
	}
}

// TestSummarizeCalibrationHealth_NoObservationsIsUnknownNotCritical pins
// I17(a) (issue #1944): when the per-pattern calibration fields have no
// producer, TotalObservations stays 0 and the calibration quality is simply
// unknown. Reporting critical there is a false signal — the reader cannot tell
// "never calibrated" from "calibrated values are broken".
//
// The value-domain fact must NOT disappear: it is reported on its own
// machine-readable axis (AdjustmentFactorStatus + OutOfRangePatterns).
func TestSummarizeCalibrationHealth_NoObservationsIsUnknownNotCritical(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "parameters.json")
	// Mirrors the production configs/parameters.json shape: 13 patterns, zero
	// calibration_observations, and out-of-range adjustment_factor values.
	body := `{"industry":{"seasonal_patterns":{"value":[
		{"id":"dividend_season","adjustment_factor":-0.2634376289519962,"historical_accuracy":0.4,"avg_market_return":-0.0003826577806334154},
		{"id":"year_end_positioning","adjustment_factor":3.44143401207912,"historical_accuracy":1,"avg_market_return":0.0022324430826447224},
		{"id":"election_local","adjustment_factor":1.1,"historical_accuracy":0.6,"avg_market_return":0.02}
	]}}}`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	summary, err := SummarizeCalibrationHealth(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if summary.Health == HealthCritical {
		t.Errorf("Health = %q, want a non-critical status when there are zero observations", summary.Health)
	}
	if summary.Health != HealthUnknown {
		t.Errorf("Health = %q, want %q", summary.Health, HealthUnknown)
	}
	if summary.Reason != ReasonNoObservations {
		t.Errorf("Reason = %q, want %q", summary.Reason, ReasonNoObservations)
	}
	if summary.ObservationStatus != ObservationNoObservations {
		t.Errorf("ObservationStatus = %q, want %q", summary.ObservationStatus, ObservationNoObservations)
	}
	if summary.CalibrationEvidence != CalibrationEvidenceNone {
		t.Errorf("CalibrationEvidence = %q, want %q", summary.CalibrationEvidence, CalibrationEvidenceNone)
	}
	// The out-of-range fact must still be visible on its own axis.
	if summary.AdjustmentFactorStatus != AdjustmentFactorOutOfRange {
		t.Errorf("AdjustmentFactorStatus = %q, want %q", summary.AdjustmentFactorStatus, AdjustmentFactorOutOfRange)
	}
	if summary.DarwinianViolations != 2 {
		t.Errorf("DarwinianViolations = %d, want 2", summary.DarwinianViolations)
	}
	if len(summary.OutOfRangePatterns) != 2 {
		t.Fatalf("OutOfRangePatterns = %v, want 2 entries", summary.OutOfRangePatterns)
	}
	got := strings.Join(summary.OutOfRangePatterns, ",")
	if got != "dividend_season,year_end_positioning" {
		t.Errorf("OutOfRangePatterns = %q, want dividend_season,year_end_positioning", got)
	}
}

// TestSummarizeCalibrationHealth_NoObservationsInRangeValues covers the clean
// case: zero observations but all declared values legal. Health must still be
// unknown (not healthy, not critical) and the value axis must read in_range.
func TestSummarizeCalibrationHealth_NoObservationsInRangeValues(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "parameters.json")
	body := `{"industry":{"seasonal_patterns":{"value":[
		{"id":"a","adjustment_factor":1.15,"historical_accuracy":0.7,"avg_market_return":0.032}
	]}}}`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	summary, err := SummarizeCalibrationHealth(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if summary.Health != HealthUnknown || summary.Reason != ReasonNoObservations {
		t.Errorf("got %q/%q, want %q/%q", summary.Health, summary.Reason, HealthUnknown, ReasonNoObservations)
	}
	if summary.AdjustmentFactorStatus != AdjustmentFactorInRange {
		t.Errorf("AdjustmentFactorStatus = %q, want %q", summary.AdjustmentFactorStatus, AdjustmentFactorInRange)
	}
	if len(summary.OutOfRangePatterns) != 0 {
		t.Errorf("OutOfRangePatterns = %v, want empty", summary.OutOfRangePatterns)
	}
}

// TestSummarizeCalibrationHealth_MissingAdjustmentFactorIsViolation verifies a
// missing adjustment_factor is counted and named (it would otherwise read as 0
// and silently erase the pattern's adjustment).
func TestSummarizeCalibrationHealth_MissingAdjustmentFactorIsViolation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "parameters.json")
	body := `{"industry":{"seasonal_patterns":{"value":[
		{"id":"no_factor","historical_accuracy":0.6,"avg_market_return":0.02,"calibration_observations":5}
	]}}}`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	summary, err := SummarizeCalibrationHealth(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if summary.DarwinianViolations != 1 {
		t.Errorf("DarwinianViolations = %d, want 1", summary.DarwinianViolations)
	}
	if summary.AdjustmentFactorStatus != AdjustmentFactorOutOfRange {
		t.Errorf("AdjustmentFactorStatus = %q, want %q", summary.AdjustmentFactorStatus, AdjustmentFactorOutOfRange)
	}
	if len(summary.OutOfRangePatterns) != 1 || summary.OutOfRangePatterns[0] != "no_factor" {
		t.Errorf("OutOfRangePatterns = %v, want [no_factor]", summary.OutOfRangePatterns)
	}
}
