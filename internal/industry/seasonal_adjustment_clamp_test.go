package industry

import (
	"math"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
)

// outOfRangeClampEngine builds an engine whose active patterns deliberately carry
// out-of-range adjustment_factor values taken from the production
// configs/parameters.json (issue #1944 / I17 real evidence):
//   - 3.4414 (year_end_positioning, above the Darwinian max 2.5)
//   - -0.2634 (dividend_season, negative)
func outOfRangeClampEngine() *SeasonalEngine {
	cfg := &config.ParametersConfig{}
	cfg.Industry.SeasonalPatterns.Value = []config.SeasonalPatternConfig{
		{
			ID: "hot_factor", StartMonth: 1, StartDay: 1, EndMonth: 12, EndDay: 31,
			FavoredIndustries: []string{"semis"}, AdjustmentFactor: 3.4414,
		},
		{
			ID: "negative_factor", StartMonth: 1, StartDay: 1, EndMonth: 12, EndDay: 31,
			FavoredIndustries: []string{"banks"}, AvoidedIndustries: []string{"shipping"},
			AdjustmentFactor: -0.2634,
		},
	}
	return NewSeasonalEngineFromConfig(cfg)
}

// TestClampAdjustmentFactor pins the single authoritative clamp contract.
func TestClampAdjustmentFactor(t *testing.T) {
	tests := []struct {
		name string
		in   float64
		want float64
	}{
		{"negative_is_neutral", -0.2634, 1.0},
		{"zero_is_neutral", 0, 1.0},
		{"nan_is_neutral", math.NaN(), 1.0},
		{"negative_inf_is_neutral", math.Inf(-1), 1.0},
		{"below_floor_clamps_up", 0.1, DarwinianMinAdjustment},
		{"at_floor_unchanged", DarwinianMinAdjustment, DarwinianMinAdjustment},
		{"in_range_unchanged", 1.2, 1.2},
		{"at_ceiling_unchanged", DarwinianMaxAdjustment, DarwinianMaxAdjustment},
		{"above_ceiling_clamps_down", 3.4414, DarwinianMaxAdjustment},
		{"positive_inf_clamps_down", math.Inf(1), DarwinianMaxAdjustment},
	}
	for _, tc := range tests {
		if got := ClampAdjustmentFactor(tc.in); got != tc.want {
			t.Errorf("%s: ClampAdjustmentFactor(%v) = %v, want %v", tc.name, tc.in, got, tc.want)
		}
	}
}

// TestIsAdjustmentFactorInRange pins the predicate that the health summary and
// the engine warning share.
func TestIsAdjustmentFactorInRange(t *testing.T) {
	tests := []struct {
		in   float64
		want bool
	}{
		{-0.2634, false},
		{0, false},
		{math.NaN(), false},
		{DarwinianMinAdjustment, true},
		{1.0, true},
		{DarwinianMaxAdjustment, true},
		{3.4414, false},
	}
	for _, tc := range tests {
		if got := IsAdjustmentFactorInRange(tc.in); got != tc.want {
			t.Errorf("IsAdjustmentFactorInRange(%v) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

// TestGetPatternAdjustment_ClampsOutOfRangeFactors verifies I17(b): an
// out-of-range adjustment_factor is never multiplied raw. Before the fix a
// negative factor produced a negative (or floor-pinned 0.01) product, and 3.44
// multiplied unbounded.
func TestGetPatternAdjustment_ClampsOutOfRangeFactors(t *testing.T) {
	engine := outOfRangeClampEngine()
	now := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)

	if got := engine.GetPatternAdjustment("semis", now); math.Abs(got-DarwinianMaxAdjustment) > 1e-9 {
		t.Errorf("favored with factor 3.4414 = %v, want clamped %v", got, DarwinianMaxAdjustment)
	}
	// Negative factor: direction contradicted, so the factor is treated as
	// neutral. The important invariant is that no negative multiplier (and no
	// 1/-0.26 = -3.80 boost for avoided industries) reaches the product.
	if got := engine.GetPatternAdjustment("banks", now); got != 1.0 {
		t.Errorf("favored with negative factor = %v, want neutral 1.0", got)
	}
	if got := engine.GetPatternAdjustment("shipping", now); got != 1.0 {
		t.Errorf("avoided with negative factor = %v, want neutral 1.0", got)
	}
}

// TestGetIndustryImpact_MatchesClampedMultiplier verifies the reported impact
// factor equals what GetPatternAdjustment actually multiplies.
func TestGetIndustryImpact_MatchesClampedMultiplier(t *testing.T) {
	engine := outOfRangeClampEngine()

	impact, adj := engine.GetIndustryImpact("hot_factor", "semis")
	if impact != "favored" || math.Abs(adj-DarwinianMaxAdjustment) > 1e-9 {
		t.Errorf("got %s/%v, want favored/%v", impact, adj, DarwinianMaxAdjustment)
	}

	impact, adj = engine.GetIndustryImpact("negative_factor", "shipping")
	if impact != "avoided" || adj != 1.0 {
		t.Errorf("got %s/%v, want avoided/1 (negative factor is neutral)", impact, adj)
	}
}

// TestGetAdjustmentBreakdown_ClampsOutOfRangeFactors verifies the per-layer
// breakdown uses the same clamped values as the composite.
func TestGetAdjustmentBreakdown_ClampsOutOfRangeFactors(t *testing.T) {
	engine := outOfRangeClampEngine()
	now := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)

	ab := engine.GetAdjustmentBreakdown("semis", now)
	if math.Abs(ab.DirectMatch-DarwinianMaxAdjustment) > 1e-9 {
		t.Errorf("DirectMatch = %v, want %v", ab.DirectMatch, DarwinianMaxAdjustment)
	}
	if math.Abs(ab.Composite-DarwinianMaxAdjustment) > 1e-9 {
		t.Errorf("Composite = %v, want %v", ab.Composite, DarwinianMaxAdjustment)
	}
	if ab.Composite <= 0 {
		t.Errorf("Composite must stay positive, got %v", ab.Composite)
	}
}

// TestEngineWithoutOutOfRangeFactorsStaysUnchanged guards against the clamp
// silently changing legitimate patterns.
func TestEngineWithoutOutOfRangeFactorsStaysUnchanged(t *testing.T) {
	cfg := &config.ParametersConfig{}
	cfg.Industry.SeasonalPatterns.Value = []config.SeasonalPatternConfig{
		{
			ID: "normal", StartMonth: 1, StartDay: 1, EndMonth: 12, EndDay: 31,
			FavoredIndustries: []string{"semis"}, AvoidedIndustries: []string{"shipping"},
			AdjustmentFactor: 1.25,
		},
	}
	engine := NewSeasonalEngineFromConfig(cfg)
	now := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)

	if got := engine.GetPatternAdjustment("semis", now); math.Abs(got-1.25) > 1e-9 {
		t.Errorf("favored = %v, want 1.25 (unclamped)", got)
	}
	if got := engine.GetPatternAdjustment("shipping", now); math.Abs(got-0.8) > 1e-9 {
		t.Errorf("avoided = %v, want 0.8 (unclamped)", got)
	}
}
