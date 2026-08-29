package charter

import (
	"math"
	"testing"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// Reference values cross-checked with scipy 1.17.1:
//
//	baseline = [0.01, -0.005, 0.02, 0, -0.01, 0.015, 0.005, -0.008, 0.012, 0.003]
//	feature  = [0.012, -0.004, 0.022, 0.002, -0.009, 0.017, 0.007, -0.006, 0.014, 0.005]
//	scipy.stats.ttest_rel → t=13.5, p=2.806720467051984e-07, mean diff=0.0018

func testReturns() ([]float64, []float64) {
	baseline := []float64{0.01, -0.005, 0.02, 0.0, -0.01, 0.015, 0.005, -0.008, 0.012, 0.003}
	feature := []float64{0.012, -0.004, 0.022, 0.002, -0.009, 0.017, 0.007, -0.006, 0.014, 0.005}
	return baseline, feature
}

func TestPairedTTestKnownInput(t *testing.T) {
	baseline, feature := testReturns()
	res := PairedTTest(baseline, feature)
	if math.Abs(res.T-13.5) > 1e-9 {
		t.Errorf("t = %.10f, want 13.5", res.T)
	}
	if math.Abs(res.P-2.806720467051984e-07) > 1e-10 {
		t.Errorf("p = %.12g, want 2.806720467051984e-07", res.P)
	}
	if res.DF != 9 {
		t.Errorf("df = %d, want 9", res.DF)
	}
	if math.Abs(res.MeanDiff-0.0018) > 1e-12 {
		t.Errorf("mean diff = %.10f, want 0.0018", res.MeanDiff)
	}
	if !res.Significant {
		t.Error("p < 0.05 → significant should be true")
	}
}

func TestPairedTTestIdenticalSeries(t *testing.T) {
	x := []float64{0.01, -0.005, 0.02, 0.0}
	res := PairedTTest(x, x)
	if res.T != 0 || res.P != 1 {
		t.Errorf("identical series: t=%v p=%v, want t=0 p=1", res.T, res.P)
	}
	if res.Significant {
		t.Error("identical series must not be significant")
	}
}

func TestPairedTTestTooShort(t *testing.T) {
	res := PairedTTest([]float64{0.01}, []float64{0.012})
	if res.DF != 0 || res.Significant {
		t.Errorf("n<2: df=%d significant=%v, want df=0 significant=false", res.DF, res.Significant)
	}
}

func TestPairedTTestLengthMismatchTruncates(t *testing.T) {
	baseline, feature := testReturns()
	res := PairedTTest(baseline[:5], feature) // truncates to 5
	if res.DF != 4 {
		t.Errorf("df = %d, want 4 (truncated to 5 pairs)", res.DF)
	}
}

// ─── BCa bootstrap ────────────────────────────────────────────────────────

func TestBCaBootstrapSignificantPositiveShift(t *testing.T) {
	// 200 days: feature returns are baseline + 0.001 every day → every paired
	// difference is positive → the mean-difference statistic is positive for
	// every resample → CI must exclude 0.
	n := 200
	baseline := make([]float64, n)
	feature := make([]float64, n)
	for i := range n {
		baseline[i] = 0.001 * float64(i%10)
		feature[i] = baseline[i] + 0.001
	}
	meanDiff := func(b, f []float64) float64 {
		var sum float64
		m := len(b)
		if len(f) < m {
			m = len(f)
		}
		for i := 0; i < m; i++ {
			sum += f[i] - b[i]
		}
		return sum / float64(m)
	}
	res := BCaBootstrap(baseline, feature, meanDiff, 1000, 0.05)
	if math.Abs(res.Observed-0.001) > 1e-12 {
		t.Errorf("observed = %.10f, want 0.001", res.Observed)
	}
	if res.CI95Low <= 0 || res.CI95High <= 0 {
		t.Errorf("CI = [%.6f, %.6f], want both endpoints > 0", res.CI95Low, res.CI95High)
	}
	if !res.Significant {
		t.Error("positive-shift sample must be significant (CI excludes 0)")
	}
}

func TestBCaBootstrapIdenticalSamples(t *testing.T) {
	x := []float64{0.01, -0.005, 0.02, 0.0, -0.01, 0.015, 0.005, -0.008, 0.012, 0.003}
	meanDiff := func(b, f []float64) float64 {
		var sum float64
		for i := range b {
			sum += f[i] - b[i]
		}
		return sum / float64(len(b))
	}
	res := BCaBootstrap(x, x, meanDiff, 1000, 0.05)
	if res.Observed != 0 {
		t.Errorf("observed = %v, want 0", res.Observed)
	}
	if res.Significant {
		t.Error("identical samples must not be significant")
	}
}

func TestBCaBootstrapSharpeDiff(t *testing.T) {
	baseline, feature := testReturns()
	res := BCaBootstrap(baseline, feature, SharpeDiff, 2000, 0.05)
	// Observed Sharpe diff is deterministic (annualized √252).
	if math.Abs(res.Observed-2.592851901090862) > 1e-6 {
		t.Errorf("observed sharpe diff = %.9f, want 2.592851901", res.Observed)
	}
	if res.CI95High <= res.CI95Low {
		t.Errorf("CI degenerate: [%f, %f]", res.CI95Low, res.CI95High)
	}
	if !res.Significant {
		t.Errorf("feature has clearly higher Sharpe → CI [%f, %f] should exclude 0", res.CI95Low, res.CI95High)
	}
}

// TestBCaBootstrapDegenerateMaxDrawdown verifies the degenerate guard: for a
// path-dependent statistic (MaxDrawdown) whose resampled distribution cannot
// reproduce the observed value, the CI must not claim significance.
func TestBCaBootstrapDegenerateMaxDrawdown(t *testing.T) {
	// Feature arm has one deep drawdown late in the window; resampling the
	// daily returns destroys the path, so bootstrap drawdowns are ~0 while the
	// observed diff is large — the textbook degenerate case.
	baseline := make([]float64, 200)
	feature := make([]float64, 200)
	for i := range 200 {
		baseline[i] = 100 + float64(i) // monotone up, no drawdown
		feature[i] = 100 + float64(i)
	}
	feature[150] = 50 // one deep drawdown in the feature arm
	// NB: BCaBootstrap takes return-series for SharpeDiff; here we pass equity
	// curves and the MaxDrawdownDiff statistic.
	res := BCaBootstrap(baseline, feature, MaxDrawdownDiff, 2000, 0.05)
	if !res.Degenerate {
		t.Errorf("expected degenerate flag for path-dependent MaxDrawdown, got %+v", res)
	}
	if res.Significant {
		t.Error("degenerate bootstrap must not claim significance")
	}
}

// ─── nonlinear metrics ────────────────────────────────────────────────────

func TestMaxDrawdownKnownInput(t *testing.T) {
	equity := []float64{100, 102, 101, 105, 103, 99, 104, 108}
	// peak 105 → trough 99: (105-99)/105 = 0.057142857...
	got := MaxDrawdown(equity)
	if math.Abs(got-0.05714285714285714) > 1e-12 {
		t.Errorf("MaxDrawdown = %.10f, want 0.0571428571", got)
	}
}

func TestMaxDrawdownFlatAndEmpty(t *testing.T) {
	if got := MaxDrawdown(nil); got != 0 {
		t.Errorf("empty: got %v, want 0", got)
	}
	if got := MaxDrawdown([]float64{100, 100, 100}); got != 0 {
		t.Errorf("flat: got %v, want 0", got)
	}
}

func TestMaxDrawdownDiffSignConvention(t *testing.T) {
	shallow := []float64{100, 101, 102, 101, 103} // 0.98% drawdown
	deep := []float64{100, 105, 95, 100}          // 9.5% drawdown
	// baseline deep, feature shallow → feature better → positive diff.
	diff := MaxDrawdownDiff(deep, shallow)
	if diff <= 0 {
		t.Errorf("MaxDrawdownDiff(deep, shallow) = %f, want > 0 (feature shallower)", diff)
	}
}

// ─── options ──────────────────────────────────────────────────────────────

func TestStepwiseArms(t *testing.T) {
	arms := StepwiseArms()
	if len(arms) != 5 {
		t.Fatalf("len(StepwiseArms()) = %d, want 5", len(arms))
	}
	names := ArmNames()
	expected := []string{"PeriodOnly", "+StrategyFilter", "+MacroFlow", "+CashReserve", "+ConvictionFloor"}
	for i := range expected {
		if names[i] != expected[i] {
			t.Errorf("arm %d name = %q, want %q", i, names[i], expected[i])
		}
	}
	// Cumulative: each arm enables all previous switches.
	prev := 0
	for i, arm := range arms {
		cnt := 0
		for _, on := range []bool{arm.PeriodOnly, arm.StrategyFilter, arm.MacroFlow, arm.CashReserve, arm.ConvictionFloor} {
			if on {
				cnt++
			}
		}
		if cnt != i+1 {
			t.Errorf("arm %d enables %d switches, want %d (cumulative)", i, cnt, i+1)
		}
		if !arm.Enabled() {
			t.Errorf("arm %d must be enabled", i)
		}
		_ = prev
	}
	if !AllOn().Enabled() {
		t.Error("AllOn() must be enabled")
	}
	if len(AllOn().Names()) != 5 {
		t.Errorf("AllOn().Names() = %v, want 5 names", AllOn().Names())
	}
}

func TestConvictionFloorDelta(t *testing.T) {
	cases := []struct {
		period domain.MarketPeriod
		want   int
	}{
		{domain.PeriodBlackSwan, 20},
		{domain.PeriodDownturn, 10},
		{domain.PeriodTurnaroundDown, 10},
		{domain.PeriodBull, 0},
		{domain.PeriodPlateau, 0},
		{domain.PeriodConsolidation, 0},
		{domain.PeriodTurnaroundUp, 0},
		{"unknown", 0},
	}
	for _, c := range cases {
		if got := ConvictionFloorDelta(c.period); got != c.want {
			t.Errorf("ConvictionFloorDelta(%q) = %d, want %d", c.period, got, c.want)
		}
	}
}

// ─── helpers ──────────────────────────────────────────────────────────────

func TestEmpiricalQuantile(t *testing.T) {
	sorted := []float64{1, 2, 3, 4}
	if got := empiricalQuantile(sorted, 0); got != 1 {
		t.Errorf("p=0 → %v, want 1", got)
	}
	if got := empiricalQuantile(sorted, 1); got != 4 {
		t.Errorf("p=1 → %v, want 4", got)
	}
	if got := empiricalQuantile(sorted, 0.5); got != 2.5 {
		t.Errorf("p=0.5 → %v, want 2.5", got)
	}
	if got := empiricalQuantile(sorted, 1.0/3.0); math.Abs(got-2.0) > 1e-12 {
		t.Errorf("p=1/3 → %v, want 2", got)
	}
}
