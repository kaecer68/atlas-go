package portfolio

import (
	"math"
	"reflect"
	"testing"
)

func TestWeightedScore_EmptyWeights(t *testing.T) {
	if got := weightedScore(map[FactorType]float64{FactorMomentum: 1.0}, map[FactorType]float64{}); got != 0 {
		t.Errorf("expected 0 for empty weights, got %v", got)
	}
}

func TestWeightedScore_EmptyScores(t *testing.T) {
	if got := weightedScore(map[FactorType]float64{}, map[FactorType]float64{FactorMomentum: 0.5}); got != 0 {
		t.Errorf("expected 0 for empty scores, got %v", got)
	}
}

func TestWeightedScore_SingleFactor(t *testing.T) {
	// 0.7 score × 0.5 weight = 0.35
	got := weightedScore(
		map[FactorType]float64{FactorMomentum: 0.7},
		map[FactorType]float64{FactorMomentum: 0.5},
	)
	if math.Abs(got-0.35) > 1e-9 {
		t.Errorf("expected 0.35, got %v", got)
	}
}

func TestWeightedScore_MultiFactor(t *testing.T) {
	// (0.8 × 0.4) + (0.6 × 0.3) + (0.2 × 0.3) = 0.32 + 0.18 + 0.06 = 0.56
	scores := map[FactorType]float64{
		FactorMomentum: 0.8, FactorValue: 0.6, FactorQuality: 0.2,
	}
	weights := map[FactorType]float64{
		FactorMomentum: 0.4, FactorValue: 0.3, FactorQuality: 0.3,
	}
	got := weightedScore(scores, weights)
	if math.Abs(got-0.56) > 1e-9 {
		t.Errorf("expected 0.56, got %v", got)
	}
}

func TestWeightedScore_IgnoresMissingScore(t *testing.T) {
	// Only FactorMomentum has a score; FactorValue is missing.
	// 0.5 × 0.4 + 0 × 0.6 = 0.2
	scores := map[FactorType]float64{FactorMomentum: 0.5}
	weights := map[FactorType]float64{
		FactorMomentum: 0.4, FactorValue: 0.6,
	}
	got := weightedScore(scores, weights)
	if math.Abs(got-0.2) > 1e-9 {
		t.Errorf("expected 0.2, got %v", got)
	}
}

func TestSortedNames_Alphabetical(t *testing.T) {
	w := map[FactorType]float64{
		FactorQuality:  0.2,
		FactorMomentum: 0.25,
		FactorValue:    0.2,
		FactorInstSent: 0.1,
	}
	got := sortedNames(w)
	want := []string{"institutional_sentiment", "momentum", "quality", "value"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("expected %v, got %v", want, got)
	}
}

func TestSortedNames_Empty(t *testing.T) {
	got := sortedNames(map[FactorType]float64{})
	if len(got) != 0 {
		t.Errorf("expected empty slice, got %v", got)
	}
}

func TestVecToWeights_NormalizesToSumOne(t *testing.T) {
	// x sums to 2.0 → after normalization each becomes x/2.0
	x := []float64{0.4, 0.6, 0.4, 0.6}
	names := []string{"momentum", "value", "quality", "agent"}
	fallback := map[FactorType]float64{}
	got := vecToWeights(x, names, fallback)
	for _, ft := range []FactorType{FactorMomentum, FactorValue, FactorQuality, FactorAgent} {
		if math.Abs(got[ft]-0.2) > 1e-9 && math.Abs(got[ft]-0.3) > 1e-9 {
			t.Errorf("factor %s: expected 0.2 or 0.3, got %v", ft, got[ft])
		}
	}
	var total float64
	for _, v := range got {
		total += v
	}
	if math.Abs(total-1.0) > 1e-9 {
		t.Errorf("normalized sum expected 1.0, got %v", total)
	}
}

func TestVecToWeights_FillsMissingFromFallback(t *testing.T) {
	// x=[0.5] single element → raw={momentum:0.5}, total=0.5, normalize: 0.5/0.5=1.0
	// Then fallback fills value=0.2 and quality=0.5 (raw has only momentum key).
	x := []float64{0.5}
	names := []string{"momentum"}
	fallback := map[FactorType]float64{
		FactorMomentum: 0.3,
		FactorValue:    0.2,
		FactorQuality:  0.5,
	}
	got := vecToWeights(x, names, fallback)
	if math.Abs(got[FactorMomentum]-1.0) > 1e-9 {
		t.Errorf("expected momentum=1.0 (normalized from 0.5/0.5), got %v", got[FactorMomentum])
	}
	if got[FactorValue] != 0.2 {
		t.Errorf("expected value=0.2 from fallback, got %v", got[FactorValue])
	}
	if got[FactorQuality] != 0.5 {
		t.Errorf("expected quality=0.5 from fallback, got %v", got[FactorQuality])
	}
}

func TestVecToWeights_AllZeroNoNormalize(t *testing.T) {
	// When x is all zeros: raw has the keys with value 0, total=0, no
	// normalization, AND fallback only fills keys NOT in raw — so fallback
	// is ignored. Both keys remain 0.0.
	x := []float64{0.0, 0.0}
	names := []string{"momentum", "value"}
	fallback := map[FactorType]float64{FactorMomentum: 0.6, FactorValue: 0.4}
	got := vecToWeights(x, names, fallback)
	if got[FactorMomentum] != 0.0 {
		t.Errorf("expected momentum=0.0 (raw already has key), got %v", got[FactorMomentum])
	}
	if got[FactorValue] != 0.0 {
		t.Errorf("expected value=0.0 (raw already has key), got %v", got[FactorValue])
	}
}

func TestEvalWeights_NoOrdersReturnsNaN(t *testing.T) {
	// evalWeights(nil, w) divides by len(orders) = 0 → NaN. The function
	// does NOT have a guard for empty orders; document the actual behavior.
	got := evalWeights(nil, map[FactorType]float64{FactorMomentum: 0.5})
	if !math.IsNaN(got) {
		t.Errorf("expected NaN for empty orders (0/0 division), got %v", got)
	}
}

func TestEvalWeights_LowCoverageNegative(t *testing.T) {
	// 1 order with positive score → coverage=1.0 > 0.95 → -1.0
	orders := []CalibratedOrder{{ForwardReturn: 0.1, FactorScores: map[FactorType]float64{FactorMomentum: 1.0}}}
	got := evalWeights(orders, map[FactorType]float64{FactorMomentum: 1.0})
	if got != -1.0 {
		t.Errorf("expected -1.0 for coverage > 0.95, got %v", got)
	}
}

func TestEvalWeights_MidCoverage(t *testing.T) {
	// 10 orders, 5 with positive score → coverage=0.5 (in [0.05, 0.95])
	// pnl = sum of positive forward_returns
	// score = pnl/sqrt(10) + 0.5*0.5
	orders := make([]CalibratedOrder, 10)
	for i := range orders {
		orders[i] = CalibratedOrder{
			ForwardReturn: 0.01,
			FactorScores:  map[FactorType]float64{FactorMomentum: float64(i%2)*2.0 - 1.0}, // -1, 1, -1, 1, ...
		}
	}
	got := evalWeights(orders, map[FactorType]float64{FactorMomentum: 1.0})
	// 5 buys × 0.01 = 0.05, sqrt(10) ≈ 3.162
	// expected ≈ 0.05/3.162 + 0.25 ≈ 0.2658
	if math.Abs(got-0.2658) > 0.01 {
		t.Errorf("expected ~0.2658, got %v", got)
	}
}

func TestBuildWeightChanges_SkipsSmallDeltas(t *testing.T) {
	// delta < 1% threshold → skipped.
	// 0.251 vs 0.25 = (0.001/0.25)*100 = 0.4% → skipped
	before := map[FactorType]float64{FactorMomentum: 0.25}
	after := map[FactorType]float64{FactorMomentum: 0.251}
	cs := buildWeightChanges(before, after, 25)
	if len(cs) != 0 {
		t.Errorf("expected 0 changes for delta<1%%, got %d", len(cs))
	}
}

func TestBuildWeightChanges_HighConfidence(t *testing.T) {
	// n=20 and delta >5% → high confidence
	before := map[FactorType]float64{FactorMomentum: 0.20}
	after := map[FactorType]float64{FactorMomentum: 0.30} // +50%
	cs := buildWeightChanges(before, after, 20)
	if len(cs) != 1 {
		t.Fatalf("expected 1 change, got %d", len(cs))
	}
	if cs[0].Confidence != "high" {
		t.Errorf("expected high confidence, got %q", cs[0].Confidence)
	}
	if math.Abs(cs[0].DeltaPct-50.0) > 0.01 {
		t.Errorf("expected delta=50%%, got %v", cs[0].DeltaPct)
	}
}

func TestBuildWeightChanges_MediumConfidence(t *testing.T) {
	// n=10, delta >3% → medium
	before := map[FactorType]float64{FactorMomentum: 0.20}
	after := map[FactorType]float64{FactorMomentum: 0.25} // +25%
	cs := buildWeightChanges(before, after, 10)
	if len(cs) != 1 {
		t.Fatalf("expected 1 change, got %d", len(cs))
	}
	if cs[0].Confidence != "medium" {
		t.Errorf("expected medium confidence, got %q", cs[0].Confidence)
	}
}

func TestBuildWeightChanges_LowConfidence(t *testing.T) {
	// n=5, delta >5% → low (n<10)
	before := map[FactorType]float64{FactorMomentum: 0.10}
	after := map[FactorType]float64{FactorMomentum: 0.20} // +100%
	cs := buildWeightChanges(before, after, 5)
	if len(cs) != 1 {
		t.Fatalf("expected 1 change, got %d", len(cs))
	}
	if cs[0].Confidence != "low" {
		t.Errorf("expected low confidence for n<10, got %q", cs[0].Confidence)
	}
}

func TestSplitLines_BasicNewline(t *testing.T) {
	got := splitLines("a\nb\nc")
	want := []string{"a", "b", "c"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("expected %v, got %v", want, got)
	}
}

func TestSplitLines_TrailingNewline(t *testing.T) {
	// Trailing \n is consumed by the for-loop; the `st < len(s)` final
	// branch is `6 < 6` = false, so no empty trailing element is added.
	got := splitLines("a\nb\nc\n")
	want := []string{"a", "b", "c"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("expected %v, got %v", want, got)
	}
}

func TestSplitLines_Empty(t *testing.T) {
	if got := splitLines(""); len(got) != 0 {
		t.Errorf("expected 0 entries for empty string, got %d (%v)", len(got), got)
	}
}

func TestSplitLines_NoNewline(t *testing.T) {
	got := splitLines("hello")
	want := []string{"hello"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("expected %v, got %v", want, got)
	}
}
