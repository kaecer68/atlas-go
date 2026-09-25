package industry

import (
	"math"
	"reflect"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
)

// Issue #1944 Batch 1: resolveCardConfig() used to return defaults
// unconditionally, so the calibration tracker injected via
// SetGlobalCycleCalibration (production: monitoring.IndustryService.
// SetCycleCalibration) never reached the cycle status card, while
// IndustryService documented the opposite. These tests pin the consumption
// contract: evidence-driven, and scale-preserving.

func withCalibration(t *testing.T, cal *CycleCalibration) {
	t.Helper()
	old := GetGlobalCycleCalibration()
	SetGlobalCycleCalibration(cal)
	t.Cleanup(func() { SetGlobalCycleCalibration(old) })
}

func TestResolveCardConfig_NoCalibrationUsesDefaults(t *testing.T) {
	withCalibration(t, nil)

	got := resolveCardConfig()
	if !reflect.DeepEqual(got, defaultCardConfig()) {
		t.Fatalf("nil calibration: got %+v, want defaults", got.LayerWeights)
	}
}

// TestResolveCardConfig_NoEvidenceIsIgnored guards rule 1 of the consumption
// contract: with zero recorded outcomes the injected tracker must not change
// the weights. CalibrateWeights() normalises to sum=1, so an unconditional
// call would rescale the composite coefficient by 1/0.85 while nothing was
// learned.
func TestResolveCardConfig_NoEvidenceIsIgnored(t *testing.T) {
	withCalibration(t, NewCycleCalibration(testCalibrationConfig()))

	got := resolveCardConfig()
	if !reflect.DeepEqual(got, defaultCardConfig()) {
		t.Fatalf("no evidence: got %+v, want defaults", got.LayerWeights)
	}
	if math.Abs(weightSum(got.LayerWeights)-0.85) > 1e-9 {
		t.Fatalf("funded weight sum = %v, want 0.85", weightSum(got.LayerWeights))
	}
}

// TestResolveCardConfig_BelowMinSamplesIsIgnored covers the tracker's own
// MinSamples gate: too few outcomes => no opinion => defaults.
func TestResolveCardConfig_BelowMinSamplesIsIgnored(t *testing.T) {
	cal := NewCycleCalibration(testCalibrationConfig()) // MinSamples = 10
	for i := range 3 {
		cal.RecordOutcome("s", time.Now(), map[string]float64{"silicon": 0.9}, -0.01-adjust(i))
	}
	withCalibration(t, cal)

	got := resolveCardConfig()
	assertWeightsWithin(t, defaultCardConfig().LayerWeights, got.LayerWeights, 1e-12)
}

// assertWeightsWithin compares two weight maps with an absolute tolerance
// (float arithmetic through CalibrateWeights is not bit-exact).
func assertWeightsWithin(t *testing.T, want, got map[string]float64, tol float64) {
	t.Helper()
	if len(want) != len(got) {
		t.Fatalf("weights = %+v, want %+v", got, want)
	}
	for layer, wantWeight := range want {
		gotWeight, ok := got[layer]
		if !ok {
			t.Fatalf("weights missing layer %q: %+v", layer, got)
		}
		if math.Abs(gotWeight-wantWeight) > tol {
			t.Fatalf("weight[%s] = %v, want %v (tol %v)", layer, gotWeight, wantWeight, tol)
		}
	}
}

func adjust(i int) float64 { return float64(i) * 1e-6 }

// TestResolveCardConfig_EvidenceRedistributesWeights pins the actual wiring:
// a layer whose signal matched the market direction (bullish signal on a
// down day => mismatch) is downweighted, a layer that matched is upweighted,
// layers with no metrics keep their default, and the funded sum is preserved.
func TestResolveCardConfig_EvidenceRedistributesWeights(t *testing.T) {
	cfg := testCalibrationConfig()
	cal := NewCycleCalibration(cfg)
	now := time.Now()
	for i := range cfg.MinSamples {
		// Down day: silicon is bullish (>0.5) => wrong; business_cycle is
		// bearish (<=0.5) => right.
		cal.RecordOutcome("sess", now.AddDate(0, 0, -i), map[string]float64{
			"silicon":        0.9,
			"business_cycle": 0.1,
		}, -0.01-float64(i)*1e-6)
	}
	withCalibration(t, cal)

	got := resolveCardConfig().LayerWeights
	if got["silicon"] >= 0.25 {
		t.Errorf("silicon (accuracy 0) should be downweighted below 0.25, got %v", got["silicon"])
	}
	if got["business_cycle"] <= 0.20 {
		t.Errorf("business_cycle (accuracy 1.0) should be upweighted above 0.20, got %v", got["business_cycle"])
	}
	// Layers without metrics keep their default SHARE. The absolute value can
	// drift by up to ~5e-5 because CalibrateWeights normalises through
	// normalizeWeights (4 dp rounding); the funded SUM is exact instead.
	if math.Abs(got["seasonal"]-0.15) > defaultWeightTolerance ||
		math.Abs(got["events"]-0.15) > defaultWeightTolerance ||
		math.Abs(got["supply_chain"]-0.10) > defaultWeightTolerance {
		t.Errorf("layers without metrics must keep their default share, got %+v", got)
	}
	if sum := weightSum(got); math.Abs(sum-0.85) > fundedSumTolerance {
		t.Errorf("funded weight sum = %v, want 0.85 (residual preserved)", sum)
	}
}

// TestCycleCalibration_ZeroWindowSizeKeepsOutcomes pins the fixed semantics of
// a zero window (issue #1944 Batch 2 item I3; Batch 1 pinned the bug):
// WindowSize <= 0 now means "keep every outcome" instead of
// outcomes[len-0:] == empty, which used to drop the sample the caller had just
// recorded and kept the layer metrics permanently empty.
//
// The all-zero config shipped in configs/parameters.json is ALSO merged up to
// the code defaults (MinSamples=10, WindowSize=30) since Batch 2, so the
// production tracker accumulates samples; see
// internal/config/parameters_merge.go and
// TestMergeIndustryDefaults_CycleCalibrationAllZero.
//
// The safety half of the old test is preserved below: even with samples
// present, an all-zero clamp/hit-rate config must not blank the card weights.
func TestCycleCalibration_ZeroWindowSizeKeepsOutcomes(t *testing.T) {
	cal := NewCycleCalibration(config.CycleCalibrationConfig{}) // all-zero block
	for i := range 20 {
		cal.RecordOutcome("sess", time.Now(), map[string]float64{"silicon": 0.9}, 0.01+float64(i)*1e-6)
	}
	if cal.GetOutcomeCount() != 20 {
		t.Fatalf("WindowSize=0 must not trim the window: %d outcomes retained, want 20", cal.GetOutcomeCount())
	}
	if len(cal.GetMetrics()) == 0 {
		t.Fatal("WindowSize=0 must not block metrics")
	}

	withCalibration(t, cal)
	assertWeightsWithin(t, defaultCardConfig().LayerWeights, resolveCardConfig().LayerWeights, 1e-12)
}

// Tolerances for the calibration consumption contract:
//   - fundedSumTolerance: the funded sum is preserved by construction (the
//     residue is absorbed into the largest layer), so this can be tight.
//   - defaultWeightTolerance: per-layer defaults can drift by one unit of
//     normalizeWeights' 4 dp rounding (~5e-5), so a tight per-layer bound would
//     be a false invariant (independent review G3 measured up to 2e-4 drift
//     with per-layer rounding; the sum is now exact instead).
const (
	fundedSumTolerance     = 1e-12
	defaultWeightTolerance = 5e-4
)

// TestApplyCycleCalibration_AllZeroWeightsFallsBackToDefaults documents the
// guard for the current production config (weight_clamp_min/max = 0, i.e. the
// clamp window collapses to zero): a tracker that clamps every layer to 0
// yields no usable weights, so the defaults are kept instead of blanking the
// card. Fixing the all-zero config itself is issue #1944 Batch 2 (I3).
func TestApplyCycleCalibration_AllZeroWeightsFallsBackToDefaults(t *testing.T) {
	cfg := testCalibrationConfig()
	cfg.WeightClampMin = 0
	cfg.WeightClampMax = 0
	cfg.MinSamples = 1
	cal := NewCycleCalibration(cfg)
	// Bullish signal per layer on an up day => every layer "correct" =>
	// upweighted => clamped to the [0,0] window => all weights 0.
	cal.RecordOutcome("s", time.Now(), map[string]float64{
		"silicon":        0.9,
		"business_cycle": 0.9,
		"seasonal":       1.2,
		"events":         1.2,
		"supply_chain":   0.5,
	}, 0.01)

	base := defaultCardConfig()
	// Batch 2: a degenerate clamp window is now a no-op inside CalibrateWeights
	// (WeightClampMax <= WeightClampMin), so the base weights survive instead of
	// being clamped to 0 for every layer that has metrics.
	if got := cal.CalibrateWeights(base.LayerWeights); !reflect.DeepEqual(got, base.LayerWeights) {
		t.Fatalf("degenerate clamp window must not alter weights, got %+v", got)
	}
	if got := applyCycleCalibration(base, cal); !reflect.DeepEqual(got, base) {
		t.Fatalf("all-zero calibration must fall back to defaults, got %+v", got.LayerWeights)
	}
}

// TestBuildCard_WithCalibrationStaysInClamp proves the wired path end-to-end:
// a card built with a calibration tracker present keeps the documented
// composite clamp (0.80-1.20).
func TestBuildCard_WithCalibrationStaysInClamp(t *testing.T) {
	cfg := testCalibrationConfig()
	cal := NewCycleCalibration(cfg)
	for i := range cfg.MinSamples {
		cal.RecordOutcome("sess", time.Now().AddDate(0, 0, -i), map[string]float64{
			"silicon":        0.9,
			"business_cycle": 0.1,
		}, -0.01-float64(i)*1e-6)
	}
	withCalibration(t, cal)

	builder := NewCycleStatusCardBuilder(NewSiliconCycleTracker(), NewCycleTracker(), NewSeasonalEngineFromConfig(nil), NewEventCalendar(), NewLinkageAnalyzer())
	card, err := builder.BuildCard(time.Now(), "semiconductor")
	if err != nil {
		t.Fatal(err)
	}
	if card.CompositeCoefficient < 0.80 || card.CompositeCoefficient > 1.20 {
		t.Fatalf("composite coefficient out of clamp range: %v", card.CompositeCoefficient)
	}
}
