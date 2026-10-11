package stockpicker

import (
	"math"
	"testing"
)

// TestWinRate_Basic 驗證樣本勝率的基本計算：0/10、5/10、10/10、0/0。
func TestWinRate_Basic(t *testing.T) {
	cases := []struct {
		name         string
		hits         int
		observations int
		want         float64
	}{
		{"zero hits", 0, 10, 0.0},
		{"half", 5, 10, 0.5},
		{"perfect", 10, 10, 1.0},
		{"no observations", 0, 0, 0.0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := WinRate(tc.hits, tc.observations)
			if got != tc.want {
				t.Fatalf("WinRate(%d, %d) = %v, want %v", tc.hits, tc.observations, got, tc.want)
			}
		})
	}
}

// TestWinRate_Boundary 驗證無效輸入（hits > observations）回傳 0 並文件化。
func TestWinRate_Boundary(t *testing.T) {
	if got := WinRate(11, 10); got != 0 {
		t.Fatalf("WinRate(11, 10) = %v, want 0 (invalid input documented as 0)", got)
	}
	if got := WinRate(-1, 10); got != 0 {
		t.Fatalf("WinRate(-1, 10) = %v, want 0 (invalid input documented as 0)", got)
	}
	if got := WinRate(5, -1); got != 0 {
		t.Fatalf("WinRate(5, -1) = %v, want 0 (invalid input documented as 0)", got)
	}
}

// TestWilsonScoreInterval_Basic 驗證 30 觀察 20 命中 95% CI 落在合理範圍（約 0.49–0.78）。
func TestWilsonScoreInterval_Basic(t *testing.T) {
	lower, upper := WilsonScoreInterval(20, 30, 0.95)
	if lower < 0.45 || lower > 0.55 {
		t.Fatalf("95%% CI lower = %v, want in [0.45, 0.55]", lower)
	}
	if upper < 0.75 || upper > 0.85 {
		t.Fatalf("95%% CI upper = %v, want in [0.75, 0.85]", upper)
	}
	if lower > upper {
		t.Fatalf("CI inverted: lower=%v > upper=%v", lower, upper)
	}

	// 99% CI 應比 95% CI 更寬且包含 95% CI。
	l99, u99 := WilsonScoreInterval(20, 30, 0.99)
	if l99 > lower || u99 < upper {
		t.Fatalf("99%% CI (%v, %v) should contain 95%% CI (%v, %v)", l99, u99, lower, upper)
	}
}

// TestWilsonScoreInterval_ZeroObservations 驗證零觀察回傳 (0, 0)。
func TestWilsonScoreInterval_ZeroObservations(t *testing.T) {
	lower, upper := WilsonScoreInterval(0, 0, 0.95)
	if lower != 0 || upper != 0 {
		t.Fatalf("WilsonScoreInterval(0, 0) = (%v, %v), want (0, 0)", lower, upper)
	}
}

// TestWilsonScoreInterval_InvalidInput 驗證 hits > observations 視為無效輸入回傳 (0, 0)。
func TestWilsonScoreInterval_InvalidInput(t *testing.T) {
	lower, upper := WilsonScoreInterval(11, 10, 0.95)
	if lower != 0 || upper != 0 {
		t.Fatalf("WilsonScoreInterval(11, 10) = (%v, %v), want (0, 0)", lower, upper)
	}
}

// TestCalibrationStatusFor 驗證 minSamples=30 的校準門檻：<30 calibrating，≥30 eligible。
func TestCalibrationStatusFor(t *testing.T) {
	if got := CalibrationStatusFor(0, 30); got != CalibrationCalibrating {
		t.Fatalf("CalibrationStatusFor(0, 30) = %q, want %q", got, CalibrationCalibrating)
	}
	if got := CalibrationStatusFor(29, 30); got != CalibrationCalibrating {
		t.Fatalf("CalibrationStatusFor(29, 30) = %q, want %q", got, CalibrationCalibrating)
	}
	if got := CalibrationStatusFor(30, 30); got != CalibrationEligible {
		t.Fatalf("CalibrationStatusFor(30, 30) = %q, want %q", got, CalibrationEligible)
	}
	if got := CalibrationStatusFor(100, 30); got != CalibrationEligible {
		t.Fatalf("CalibrationStatusFor(100, 30) = %q, want %q", got, CalibrationEligible)
	}
}

// TestNetHit_CostDeducts 驗證淨報酬命中 = forwardReturn - costRate > 0。
func TestNetHit_CostDeducts(t *testing.T) {
	cases := []struct {
		name          string
		forwardReturn float64
		costRate      float64
		want          bool
	}{
		{"net positive", 0.01, 0.00585, true},
		{"net negative", 0.005, 0.00585, false},
		{"exactly breakeven is not a hit", 0.00585, 0.00585, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := NetHit(tc.forwardReturn, tc.costRate); got != tc.want {
				t.Fatalf("NetHit(%v, %v) = %v, want %v", tc.forwardReturn, tc.costRate, got, tc.want)
			}
		})
	}
}

// TestHit_NetOfCosts 驗證毛報酬 > 0 但淨報酬（扣 0.585%）< 0 時不得計 hit（k3 P1-5）。
func TestHit_NetOfCosts(t *testing.T) {
	const costRate = 0.00585
	// 毛報酬正、淨報酬負：不命中。
	if NetHit(0.004, costRate) {
		t.Fatalf("NetHit(0.004, %v) = true, want false (gross positive but net negative)", costRate)
	}
	// 毛報酬與淨報酬皆正：命中。
	if !NetHit(0.006, costRate) {
		t.Fatalf("NetHit(0.006, %v) = false, want true", costRate)
	}
}

// TestSignalWinRate_MixedOutcomes 驗證多筆 outcome 的勝率與 Wilson 區間。
func TestSignalWinRate_MixedOutcomes(t *testing.T) {
	const (
		costRate   = 0.00585
		minSamples = 30
		confidence = 0.95
	)
	outcomes := []SignalOutcome{
		{Symbol: "2330", TriggerDate: "2026-01-02", ForwardReturn: 0.02, Source: "stockpicker-momentum"},
		{Symbol: "2330", TriggerDate: "2026-01-09", ForwardReturn: 0.015, Source: "stockpicker-momentum"},
		{Symbol: "2330", TriggerDate: "2026-01-16", ForwardReturn: 0.01, Source: "stockpicker-momentum"},
		{Symbol: "2330", TriggerDate: "2026-01-23", ForwardReturn: 0.006, Source: "stockpicker-momentum"},
		{Symbol: "2330", TriggerDate: "2026-01-30", ForwardReturn: 0.004, Source: "stockpicker-momentum"},
		{Symbol: "2330", TriggerDate: "2026-02-06", ForwardReturn: 0.003, Source: "stockpicker-momentum"},
		{Symbol: "2330", TriggerDate: "2026-02-13", ForwardReturn: 0.0, Source: "stockpicker-momentum"},
		{Symbol: "2330", TriggerDate: "2026-02-20", ForwardReturn: -0.005, Source: "stockpicker-momentum"},
		{Symbol: "2330", TriggerDate: "2026-02-27", ForwardReturn: -0.01, Source: "stockpicker-momentum"},
		{Symbol: "2330", TriggerDate: "2026-03-06", ForwardReturn: -0.02, Source: "stockpicker-momentum"},
	}

	got, err := SignalWinRate(outcomes, costRate, minSamples, confidence)
	if err != nil {
		t.Fatalf("SignalWinRate returned error: %v", err)
	}
	if got.Symbol != "2330" {
		t.Fatalf("Symbol = %q, want 2330", got.Symbol)
	}
	if got.Source != "stockpicker-momentum" {
		t.Fatalf("Source = %q, want stockpicker-momentum", got.Source)
	}
	if got.Observations != 10 {
		t.Fatalf("Observations = %d, want 10", got.Observations)
	}
	// costRate=0.00585 下，ForwardReturn > 0.00585 者有 4 筆（0.02/0.015/0.01/0.006）。
	if got.Hits != 4 {
		t.Fatalf("Hits = %d, want 4", got.Hits)
	}
	if got.WinRate != 0.4 {
		t.Fatalf("WinRate = %v, want 0.4", got.WinRate)
	}
	if got.NetCostRate != costRate {
		t.Fatalf("NetCostRate = %v, want %v", got.NetCostRate, costRate)
	}
	if got.CalibrationStatus != CalibrationCalibrating {
		t.Fatalf("CalibrationStatus = %q, want %q", got.CalibrationStatus, CalibrationCalibrating)
	}

	wantAvg := (0.02 + 0.015 + 0.01 + 0.006 + 0.004 + 0.003 + 0.0 - 0.005 - 0.01 - 0.02) / 10
	if math.Abs(got.AvgForwardReturn-wantAvg) > 1e-12 {
		t.Fatalf("AvgForwardReturn = %v, want %v", got.AvgForwardReturn, wantAvg)
	}

	wantLower, wantUpper := WilsonScoreInterval(4, 10, confidence)
	if math.Abs(got.WilsonLower-wantLower) > 1e-12 || math.Abs(got.WilsonUpper-wantUpper) > 1e-12 {
		t.Fatalf("Wilson interval = (%v, %v), want (%v, %v)",
			got.WilsonLower, got.WilsonUpper, wantLower, wantUpper)
	}
}

// TestSignalWinRate_MismatchedSymbol 驗證不同 symbol 回傳錯誤。
func TestSignalWinRate_MismatchedSymbol(t *testing.T) {
	outcomes := []SignalOutcome{
		{Symbol: "2330", Source: "stockpicker-momentum"},
		{Symbol: "2317", Source: "stockpicker-momentum"},
	}
	if _, err := SignalWinRate(outcomes, 0.00585, 30, 0.95); err == nil {
		t.Fatal("SignalWinRate with mismatched symbols: want error, got nil")
	}
}

// TestSignalWinRate_MismatchedSource 驗證不同 source 回傳錯誤。
func TestSignalWinRate_MismatchedSource(t *testing.T) {
	outcomes := []SignalOutcome{
		{Symbol: "2330", Source: "stockpicker-momentum"},
		{Symbol: "2330", Source: "research-agent-1"},
	}
	if _, err := SignalWinRate(outcomes, 0.00585, 30, 0.95); err == nil {
		t.Fatal("SignalWinRate with mismatched sources: want error, got nil")
	}
}

// TestSignalWinRate_Empty 驗證空切片：0 observations、calibrating、無 error。
func TestSignalWinRate_Empty(t *testing.T) {
	got, err := SignalWinRate(nil, 0.00585, 30, 0.95)
	if err != nil {
		t.Fatalf("SignalWinRate(nil) returned error: %v", err)
	}
	if got.Observations != 0 {
		t.Fatalf("Observations = %d, want 0", got.Observations)
	}
	if got.Hits != 0 {
		t.Fatalf("Hits = %d, want 0", got.Hits)
	}
	if got.WinRate != 0 {
		t.Fatalf("WinRate = %v, want 0", got.WinRate)
	}
	if got.WilsonLower != 0 || got.WilsonUpper != 0 {
		t.Fatalf("Wilson interval = (%v, %v), want (0, 0)", got.WilsonLower, got.WilsonUpper)
	}
	if got.CalibrationStatus != CalibrationCalibrating {
		t.Fatalf("CalibrationStatus = %q, want %q", got.CalibrationStatus, CalibrationCalibrating)
	}
	if got.AvgForwardReturn != 0 {
		t.Fatalf("AvgForwardReturn = %v, want 0", got.AvgForwardReturn)
	}
}

// --- condition-level (cross-symbol) aggregation (issue #1865) ---

func TestConditionWinRate_CrossSymbol(t *testing.T) {
	outcomes := []SignalOutcome{
		{Symbol: "2330", TriggerDate: "2026-08-01", ForwardReturn: 0.02, CostRate: 0.00585, Source: "stockpicker-momentum-20d-positive"},
		{Symbol: "2454", TriggerDate: "2026-08-03", ForwardReturn: -0.01, CostRate: 0.00585, Source: "stockpicker-momentum-20d-positive"},
		{Symbol: "2317", TriggerDate: "2026-08-05", ForwardReturn: 0.03, CostRate: 0.00585, Source: "stockpicker-momentum-20d-positive"},
		// A different source must be skipped (defensive; the store query
		// already filters by source).
		{Symbol: "2330", TriggerDate: "2026-08-05", ForwardReturn: 0.99, CostRate: 0.00585, Source: "stockpicker-foreign-3d-net-buy"},
	}
	s := ConditionWinRate("stockpicker-momentum-20d-positive", outcomes, 0.00585, 30, 0.95)
	if s.Observations != 3 || s.Symbols != 3 {
		t.Errorf("observations=%d symbols=%d, want 3/3", s.Observations, s.Symbols)
	}
	// Hits: 0.02 > 0.00585 ✓, -0.01 ✗, 0.03 ✓ → 2/3
	if s.Hits != 2 {
		t.Errorf("hits=%d, want 2", s.Hits)
	}
	if s.Direction != "buy" {
		t.Errorf("direction=%q, want buy", s.Direction)
	}
	if s.ConditionID != "momentum-20d-positive" {
		t.Errorf("condition_id=%q", s.ConditionID)
	}
	if s.DataStart != "2026-08-01" || s.DataEnd != "2026-08-05" {
		t.Errorf("date range %s~%s", s.DataStart, s.DataEnd)
	}
	if s.CalibrationStatus != string(CalibrationCalibrating) {
		t.Errorf("3 obs < 30 min samples must be calibrating, got %q", s.CalibrationStatus)
	}
}

func TestConditionWinRate_AvoidDirection(t *testing.T) {
	outcomes := []SignalOutcome{
		{Symbol: "2330", TriggerDate: "2026-08-01", ForwardReturn: -0.05, CostRate: 0.00585, Source: "stockpicker-price-volume-top-divergence"},
	}
	s := ConditionWinRate("stockpicker-price-volume-top-divergence", outcomes, 0.00585, 30, 0.95)
	if s.Direction != "avoid" {
		t.Errorf("top divergence must carry direction=avoid, got %q", s.Direction)
	}
	if s.Hits != 0 || s.WinRate != 0 {
		t.Errorf("net-negative outcome must not hit: %+v", s)
	}
}

func TestConditionWinRate_Empty(t *testing.T) {
	s := ConditionWinRate("stockpicker-momentum-20d-positive", nil, 0.00585, 30, 0.95)
	if s.Observations != 0 || s.CalibrationStatus != string(CalibrationCalibrating) {
		t.Errorf("empty input must be zero calibrating summary, got %+v", s)
	}
}

// TestShrunkRate_ShrinksSmallSamples 小樣本高勝率向母體先驗收縮，大樣本貼近原始勝率。
func TestShrunkRate_ShrinksSmallSamples(t *testing.T) {
	const priorHits, priorMisses = 50.0, 50.0 // 母體先驗勝率 0.5

	small := ShrunkRate(4, 5, priorHits, priorMisses) // 原始 0.8
	if want := 54.0 / 105.0; math.Abs(small-want) > 1e-9 {
		t.Fatalf("ShrunkRate(4, 5, 50, 50) = %v, want %v", small, want)
	}
	if small >= 0.8 {
		t.Fatalf("small-sample rate must shrink toward prior, got %v (raw 0.8)", small)
	}

	large := ShrunkRate(600, 1000, priorHits, priorMisses) // 原始 0.6
	if want := 650.0 / 1100.0; math.Abs(large-want) > 1e-9 {
		t.Fatalf("ShrunkRate(600, 1000, 50, 50) = %v, want %v", large, want)
	}

	// 排序意義：10 戰 9 勝的收縮值應低於 1000 戰 550 勝（小樣本證據不足）。
	tiny := ShrunkRate(9, 10, priorHits, priorMisses)
	big := ShrunkRate(550, 1000, priorHits, priorMisses)
	if tiny >= big {
		t.Fatalf("shrunk tiny-sample 0.9 (%v) must rank below large-sample 0.55 (%v)", tiny, big)
	}
}

// TestShrunkRate_Boundary 零先驗退化為 WinRate；無效輸入回傳 0。
func TestShrunkRate_Boundary(t *testing.T) {
	if got := ShrunkRate(7, 10, 0, 0); got != WinRate(7, 10) {
		t.Fatalf("zero prior must degenerate to WinRate: got %v, want %v", got, WinRate(7, 10))
	}
	for _, tc := range [][2]int{{-1, 10}, {11, 10}, {5, 0}, {5, -1}} {
		if got := ShrunkRate(tc[0], tc[1], 50, 50); got != 0 {
			t.Fatalf("ShrunkRate(%d, %d, 50, 50) = %v, want 0 (invalid input)", tc[0], tc[1], got)
		}
	}
	if got := ShrunkRate(5, 10, -1, -1); got != WinRate(5, 10) {
		t.Fatalf("negative prior must be treated as zero: got %v, want %v", got, WinRate(5, 10))
	}
}

// TestBreakEvenCostRate_Median 損益兩平成本 = forward return 中位數。
func TestBreakEvenCostRate_Median(t *testing.T) {
	if got := BreakEvenCostRate([]float64{0.01, 0.03, 0.02}); got != 0.02 {
		t.Fatalf("BreakEvenCostRate(odd) = %v, want 0.02 (median)", got)
	}
	if got := BreakEvenCostRate([]float64{0.04, 0.01, 0.03, 0.02}); got != 0.025 {
		t.Fatalf("BreakEvenCostRate(even) = %v, want 0.025 (mean of middles)", got)
	}
	if got := BreakEvenCostRate(nil); got != 0 {
		t.Fatalf("BreakEvenCostRate(nil) = %v, want 0", got)
	}
}

// TestBreakEvenCostRate_Semantics 高於中位數的成本使勝率跌破 5 成；不改動輸入。
func TestBreakEvenCostRate_Semantics(t *testing.T) {
	returns := []float64{0.03, -0.02, 0.01}
	be := BreakEvenCostRate(returns)
	if be != 0.01 {
		t.Fatalf("BreakEvenCostRate = %v, want 0.01", be)
	}
	if returns[0] != 0.03 || returns[1] != -0.02 || returns[2] != 0.01 {
		t.Fatalf("BreakEvenCostRate must not mutate input: %v", returns)
	}
	hitAbove := 0
	for _, r := range returns {
		if NetHit(r, be+1e-9) {
			hitAbove++
		}
	}
	if float64(hitAbove)/float64(len(returns)) >= 0.5 {
		t.Fatalf("cost above break-even must push win rate below 0.5, got %d/3 hits", hitAbove)
	}
}

// TestRegimeFilteredWinRate_Stratum 同一切片按 regime 過濾後口徑與 SignalWinRate 一致。
func TestRegimeFilteredWinRate_Stratum(t *testing.T) {
	outcomes := []SignalOutcome{
		{Symbol: "2330", TriggerDate: "2026-08-01", ForwardReturn: 0.03, Source: "stockpicker-x", Regime: "RISK_ON"},
		{Symbol: "2330", TriggerDate: "2026-08-02", ForwardReturn: 0.02, Source: "stockpicker-x", Regime: "RISK_ON"},
		{Symbol: "2330", TriggerDate: "2026-08-03", ForwardReturn: -0.01, Source: "stockpicker-x", Regime: "RISK_ON"},
		{Symbol: "2330", TriggerDate: "2026-08-04", ForwardReturn: 0.05, Source: "stockpicker-x", Regime: "RISK_OFF"},
		{Symbol: "2330", TriggerDate: "2026-08-05", ForwardReturn: 0.04, Source: "stockpicker-x"},
	}
	on, err := RegimeFilteredWinRate(outcomes, "RISK_ON", 0.00585, 30, 0.95)
	if err != nil {
		t.Fatalf("RegimeFilteredWinRate RISK_ON err: %v", err)
	}
	if on.Observations != 3 || on.Hits != 2 {
		t.Fatalf("RISK_ON stratum = %+v, want obs=3 hits=2", on)
	}
	off, err := RegimeFilteredWinRate(outcomes, "RISK_OFF", 0.00585, 30, 0.95)
	if err != nil {
		t.Fatalf("RegimeFilteredWinRate RISK_OFF err: %v", err)
	}
	if off.Observations != 1 || off.Hits != 1 {
		t.Fatalf("RISK_OFF stratum = %+v, want obs=1 hits=1", off)
	}
	// 空 regime 參數對應未標記列（UnknownRegime，永不回填）。
	unknown, err := RegimeFilteredWinRate(outcomes, "", 0.00585, 30, 0.95)
	if err != nil {
		t.Fatalf("RegimeFilteredWinRate unknown err: %v", err)
	}
	if unknown.Observations != 1 || unknown.CalibrationStatus != CalibrationCalibrating {
		t.Fatalf("unknown stratum = %+v, want obs=1 calibrating", unknown)
	}
}

// TestRegimeFilteredWinRate_Empty 空輸入回傳 0 observations 且無 error。
func TestRegimeFilteredWinRate_Empty(t *testing.T) {
	s, err := RegimeFilteredWinRate(nil, "RISK_ON", 0.00585, 30, 0.95)
	if err != nil {
		t.Fatalf("empty input must not error, got %v", err)
	}
	if s.Observations != 0 || s.CalibrationStatus != CalibrationCalibrating {
		t.Fatalf("empty input = %+v, want 0 obs calibrating", s)
	}
}
