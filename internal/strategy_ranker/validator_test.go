package strategy_ranker

import (
	"math"
	"testing"
)

func TestTotalReturnPct(t *testing.T) {
	tests := []struct {
		name    string
		returns []float64
		want    float64
	}{
		{"empty", []float64{}, 0},
		{"singleZero", []float64{0}, 0},
		{"positive5pct", []float64{0.05}, 5.0},
		{"negative3pct", []float64{-0.03}, -3.0},
		{"twoDays", []float64{0.01, 0.02}, 3.02},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := totalReturnPct(tt.returns)
			if math.Abs(got-tt.want) > 1e-6 {
				t.Errorf("totalReturnPct() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMaxDrawdownPct(t *testing.T) {
	tests := []struct {
		name    string
		returns []float64
		want    float64
	}{
		{"empty", []float64{}, 0},
		{"noDrawdown", []float64{0.01, 0.01, 0.01}, 0},
		{"simpleDD", []float64{0.1, -0.05}, 5.0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := maxDrawdownPct(tt.returns)
			if math.Abs(got-tt.want) > 1e-4 {
				t.Errorf("maxDrawdownPct() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestWinRate(t *testing.T) {
	tests := []struct {
		name    string
		returns []float64
		want    float64
	}{
		{"empty", []float64{}, 0},
		{"allWin", []float64{0.01, 0.02, 0.005}, 1.0},
		{"allLoss", []float64{-0.01, -0.02}, 0},
		{"mixed", []float64{0.01, -0.01, 0.02, 0}, 0.5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := winRate(tt.returns)
			if math.Abs(got-tt.want) > 1e-9 {
				t.Errorf("winRate() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPearsonCorrelation(t *testing.T) {
	// 完美正相關
	x := []float64{1, 2, 3, 4, 5}
	y := []float64{2, 4, 6, 8, 10}
	r := pearsonCorrelation(x, y)
	if math.Abs(r-1.0) > 1e-9 {
		t.Errorf("pearsonCorrelation(perfect positive) = %v, want 1.0", r)
	}

	// 完美負相關
	yNeg := []float64{10, 8, 6, 4, 2}
	r = pearsonCorrelation(x, yNeg)
	if math.Abs(r+1.0) > 1e-9 {
		t.Errorf("pearsonCorrelation(perfect negative) = %v, want -1.0", r)
	}

	// 無相關（零變異）
	z := []float64{5, 5, 5, 5, 5}
	r = pearsonCorrelation(x, z)
	if r != 0 {
		t.Errorf("pearsonCorrelation(zero variance) = %v, want 0", r)
	}
}

func TestValidateBasic(t *testing.T) {
	v := NewValidator()

	dailyReturns := []float64{
		0.01, -0.005, 0.02, 0.015, -0.01,
		0.01, -0.005, 0.02, 0.015, -0.01,
		0.01, -0.005, 0.02, 0.015, -0.01,
		0.01, -0.005, 0.02, 0.015, -0.01,
		0.01, -0.005, 0.02, 0.015, -0.01,
	}
	taiexReturns := []float64{
		0.005, -0.003, 0.01, 0.01, -0.005,
		0.005, -0.003, 0.01, 0.01, -0.005,
		0.005, -0.003, 0.01, 0.01, -0.005,
		0.005, -0.003, 0.01, 0.01, -0.005,
		0.005, -0.003, 0.01, 0.01, -0.005,
	}

	report := v.Validate("test_strategy", "測試策略", dailyReturns, taiexReturns)
	if report == nil {
		t.Fatal("Validate() returned nil")
	}

	if report.StrategyID != "test_strategy" {
		t.Errorf("StrategyID = %q, want %q", report.StrategyID, "test_strategy")
	}
	if report.SampleDays != 25 {
		t.Errorf("SampleDays = %d, want 25", report.SampleDays)
	}
	if report.WinRate < 0 || report.WinRate > 1 {
		t.Errorf("WinRate = %v, expected 0~1", report.WinRate)
	}
	if report.TaiexCorrelation != nil && (*report.TaiexCorrelation < -1 || *report.TaiexCorrelation > 1) {
		t.Errorf("TaiexCorrelation = %v, expected -1~1", *report.TaiexCorrelation)
	}
	if report.AnnualizedReturn == nil || report.TotalReturn == nil || report.MaxDrawdown == nil || report.AlphaScore == nil {
		t.Error("Validate() should populate all non-Sharpe metrics for sufficient samples")
	}
}

func TestValidateMismatchedLengths(t *testing.T) {
	v := NewValidator()
	report := v.Validate("x", "y", []float64{0.01}, []float64{0.01, 0.02})
	if report != nil {
		t.Error("Validate() should return nil for mismatched lengths")
	}
}

func TestRank(t *testing.T) {
	reports := []*StrategyReport{
		{StrategyID: "momentum", StrategyName: "純動能", SharpeRatio: new(0.8), WinRate: 0.55, AlphaScore: new(5.0), MaxDrawdown: new(25.0)},
		{StrategyID: "defensive", StrategyName: "防禦型", SharpeRatio: new(1.2), WinRate: 0.65, AlphaScore: new(-2.0), MaxDrawdown: new(10.0)},
		{StrategyID: "growth", StrategyName: "成長動能", SharpeRatio: new(0.9), WinRate: 0.60, AlphaScore: new(8.0), MaxDrawdown: new(30.0)},
		{StrategyID: "value", StrategyName: "價值投資", SharpeRatio: new(1.0), WinRate: 0.58, AlphaScore: new(3.0), MaxDrawdown: new(18.0)},
		{StrategyID: "all_weather", StrategyName: "全天候", SharpeRatio: new(0.7), WinRate: 0.52, AlphaScore: new(0.0), MaxDrawdown: new(15.0)},
	}

	ranked := Rank(reports)
	if len(ranked) != 5 {
		t.Fatalf("Rank() returned %d reports, want 5", len(ranked))
	}

	// 排名遞增
	for i := range ranked {
		if ranked[i].Rank != i+1 {
			t.Errorf("ranked[%d].Rank = %d, want %d", i, ranked[i].Rank, i+1)
		}
	}

	// 分數遞減
	for i := 0; i < len(ranked)-1; i++ {
		if ranked[i].Score < ranked[i+1].Score {
			t.Errorf("ranked[%d].Score = %v < ranked[%d].Score = %v (should be descending)",
				i, ranked[i].Score, i+1, ranked[i+1].Score)
		}
	}

	AssignTiers(ranked)
	if ranked[0].Tier != "premium" || ranked[1].Tier != "premium" {
		t.Errorf("Top 2 should be premium, got %q, %q", ranked[0].Tier, ranked[1].Tier)
	}
	if ranked[2].Tier != "registered" || ranked[3].Tier != "registered" {
		t.Errorf("Rank 3-4 should be registered, got %q, %q", ranked[2].Tier, ranked[3].Tier)
	}
	if ranked[4].Tier != "free" {
		t.Errorf("Rank 5 should be free, got %q", ranked[4].Tier)
	}
}
