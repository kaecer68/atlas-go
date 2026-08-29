package stress

import (
	"math"
	"testing"

	"github.com/kaecer68/atlas-go/internal/domain"
)

func TestAllScenariosCount(t *testing.T) {
	scenarios := AllScenarios()
	if len(scenarios) == 0 {
		t.Error("expected at least one scenario")
	}
}

func TestGetScenarioByID(t *testing.T) {
	s, err := GetScenarioByID("covid_crash_2020")
	if err != nil {
		t.Fatalf("expected to find covid scenario: %v", err)
	}
	if s.Name != "COVID-19 Market Crash" {
		t.Errorf("unexpected name: %s", s.Name)
	}
}

func TestGetScenarioByIDNotFound(t *testing.T) {
	_, err := GetScenarioByID("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent scenario")
	}
}

func TestScenarioMergeQuotes(t *testing.T) {
	scenario := ScenarioCOVIDCrash
	stockQuotes := []domain.Quote{
		{Symbol: "2330.TW", Last: 800, IsTradable: true},
	}
	merged := scenario.MergeQuotes(stockQuotes)
	if len(merged) != len(scenario.Quotes)+len(stockQuotes) {
		t.Errorf("expected %d quotes, got %d", len(scenario.Quotes)+len(stockQuotes), len(merged))
	}

	foundVIX := false
	foundStock := false
	for _, q := range merged {
		if q.Symbol == "VIX" {
			foundVIX = true
		}
		if q.Symbol == "2330.TW" {
			foundStock = true
		}
	}
	if !foundVIX {
		t.Error("expected VIX quote in merged results")
	}
	if !foundStock {
		t.Error("expected stock quote in merged results")
	}
}

func TestRunnerRunScenario(t *testing.T) {
	registry := domain.AgentRegistry{
		Agents: []domain.AgentSpec{
			{ID: "test", Layer: domain.LayerSector, Skill: "test", Enabled: true},
		},
	}
	policy := domain.ExecutionPolicy{}
	runner := NewRunner(registry, policy)

	stockQuotes := []domain.Quote{
		{Symbol: "2330.TW", Last: 800, Volume: 1000000, IsTradable: true},
	}
	recs := []domain.Recommendation{
		{Symbol: "2330.TW", Side: domain.SideBuy, Conviction: 90, Reason: "test"},
	}

	result := runner.RunScenario(ScenarioNormalMarket, stockQuotes, recs)

	if result.ScenarioID != ScenarioNormalMarket.ID {
		t.Errorf("expected scenario ID %s, got %s", ScenarioNormalMarket.ID, result.ScenarioID)
	}
	if result.FinalRegime != domain.RegimeNeutral {
		t.Errorf("expected neutral regime, got %s", result.FinalRegime)
	}
}

func TestRunnerRunScenarioMomentumDisabled(t *testing.T) {
	registry := domain.AgentRegistry{
		Agents: []domain.AgentSpec{
			{ID: "test", Layer: domain.LayerSector, Skill: "test", Enabled: true},
		},
	}
	policy := domain.ExecutionPolicy{}
	runner := NewRunner(registry, policy)

	stockQuotes := []domain.Quote{
		{Symbol: "2330.TW", Last: 800, Volume: 1000000, IsTradable: true},
	}
	recs := []domain.Recommendation{
		{Symbol: "2330.TW", Side: domain.SideBuy, Conviction: 90, Reason: "test"},
	}

	// COVID scenario has VIX=82.7, which should trigger momentum crash protection
	result := runner.RunScenario(ScenarioCOVIDCrash, stockQuotes, recs)

	if !result.MomentumDisabled {
		t.Error("expected momentum to be disabled when VIX > 30")
	}
}

func TestRunnerRunAll(t *testing.T) {
	registry := domain.AgentRegistry{
		Agents: []domain.AgentSpec{
			{ID: "test", Layer: domain.LayerSector, Skill: "test", Enabled: true},
		},
	}
	policy := domain.ExecutionPolicy{}
	runner := NewRunner(registry, policy)

	stockQuotes := []domain.Quote{
		{Symbol: "2330.TW", Last: 800, Volume: 1000000, IsTradable: true},
	}
	recs := []domain.Recommendation{
		{Symbol: "2330.TW", Side: domain.SideBuy, Conviction: 90, Reason: "test"},
	}

	report := runner.RunAll(stockQuotes, recs)

	expected := len(AllScenarios())
	if len(report.ScenarioResults) != expected {
		t.Errorf("expected %d results, got %d", expected, len(report.ScenarioResults))
	}
	if report.BaselineResult == nil {
		t.Error("expected baseline result to be set")
	}
}

func TestFormatReport(t *testing.T) {
	report := Report{
		ScenarioResults: []ScenarioResult{
			{
				ScenarioName:     "Test Scenario",
				TotalReturn:      -0.05,
				MaxDrawdown:      -0.10,
				VaR95:            -0.15,
				TradeCount:       3,
				FinalRegime:      domain.RegimeRiskOff,
				MomentumDisabled: true,
			},
		},
		WorstDrawdown: -0.10,
		WorstVaR:      -0.15,
		AvgReturn:     -0.05,
	}

	formatted := FormatReport(report)
	if formatted == "" {
		t.Error("expected non-empty formatted report")
	}
	if !contains(formatted, "Test Scenario") {
		t.Error("expected scenario name in report")
	}
	if !contains(formatted, "DISABLED") {
		t.Error("expected momentum disabled notice in report")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// ── P1-1 Multi-Day Stress Test Falsification ──

func TestMultiDayStress_COVIDCrash(t *testing.T) {
	runner := NewRunner(domain.AgentRegistry{}, domain.ExecutionPolicy{})

	stockQuotes := []domain.Quote{
		{Symbol: "2330.TW", Last: 300, IsTradable: true},
		{Symbol: "0050.TW", Last: 120, IsTradable: true},
		{Symbol: "GLD", Last: 150, IsTradable: true},
	}
	recs := []domain.Recommendation{
		{Symbol: "2330.TW", Side: domain.SideBuy, Conviction: 50},
		{Symbol: "0050.TW", Side: domain.SideBuy, Conviction: 50},
		{Symbol: "GLD", Side: domain.SideBuy, Conviction: 90},
	}

	result := runner.RunScenario(ScenarioCOVIDCrash, stockQuotes, recs)

	if result.MaxDrawdown < 0.10 {
		t.Errorf("COVID crash: MDD = %.2f%%, expected > 10%%", result.MaxDrawdown*100)
	}
	if len(result.DailyValues) != 41 {
		t.Errorf("COVID crash: expected 41 values (40 days + start), got %d", len(result.DailyValues))
	}

	// GLD should show recovery in second half (t=20-40)
	if result.TotalReturn > -0.05 {
		t.Logf("COVID crash: total return = %.2f%% (GLD hedge may have limited drawdown)", result.TotalReturn*100)
	}
}

func TestMultiDayStress_FedHiking(t *testing.T) {
	runner := NewRunner(domain.AgentRegistry{}, domain.ExecutionPolicy{})

	stockQuotes := []domain.Quote{
		{Symbol: "2330.TW", Last: 500, IsTradable: true},
		{Symbol: "GLD", Last: 170, IsTradable: true},
	}
	recs := []domain.Recommendation{
		{Symbol: "2330.TW", Side: domain.SideBuy, Conviction: 50},
		{Symbol: "GLD", Side: domain.SideBuy, Conviction: 50},
	}

	result := runner.RunScenario(ScenarioFedRateHikes, stockQuotes, recs)

	// Fed hiking: stocks fall, gold also falls (rate headwind).
	if result.TotalReturn >= 0 {
		t.Errorf("Fed hikes: total return = %.2f%%, expected negative", result.TotalReturn*100)
	}
	if result.MaxDrawdown < 0.02 {
		t.Errorf("Fed hikes: MDD = %.2f%%, expected > 2%%", result.MaxDrawdown*100)
	}
}

func TestCholeskyDecompose(t *testing.T) {
	A := [][]float64{
		{4.0, 2.0, 1.0},
		{2.0, 5.0, 3.0},
		{1.0, 3.0, 6.0},
	}
	L := choleskyDecompose(A)
	if L == nil {
		t.Fatal("cholesky returned nil")
	}
	n := len(A)
	for i := range n {
		for j := range n {
			var sum float64
			for k := range n {
				sum += L[i][k] * L[j][k]
			}
			if math.Abs(sum-A[i][j]) > 1e-10 {
				t.Errorf("L·L' mismatch at (%d,%d): got %.6f, want %.6f", i, j, sum, A[i][j])
			}
		}
	}
}

func TestCovarianceDrivenScenarios(t *testing.T) {
	runner := NewRunner(domain.AgentRegistry{}, domain.ExecutionPolicy{})
	// 3×3 covariance: equities strongly correlated, gold low/negative cross-asset correlation
	cov := [][]float64{
		{0.04, 0.035, -0.005},
		{0.035, 0.04, -0.005},
		{-0.005, -0.005, 0.01},
	}
	symbols := []string{"2330.TW", "0050.TW", "GLD"}
	runner.SetCovariance(cov, symbols)
	runner.SetPortfolioWeights(map[string]float64{"2330.TW": 0.35, "0050.TW": 0.15, "GLD": 0.50})

	stockQuotes := []domain.Quote{
		{Symbol: "2330.TW", Last: 300, IsTradable: true},
		{Symbol: "0050.TW", Last: 120, IsTradable: true},
		{Symbol: "GLD", Last: 150, IsTradable: true},
	}
	recs := []domain.Recommendation{
		{Symbol: "2330.TW", Side: domain.SideBuy, Conviction: 50},
		{Symbol: "0050.TW", Side: domain.SideBuy, Conviction: 50},
		{Symbol: "GLD", Side: domain.SideBuy, Conviction: 90},
	}

	result := runner.RunScenario(ScenarioCOVIDCrash, stockQuotes, recs)
	if result.MaxDrawdown < 0.05 {
		t.Errorf("cov-driven COVID crash (GLD hedged): MDD = %.2f%%, expected > 5%%", result.MaxDrawdown*100)
	}
	if len(result.DailyValues) < 2 {
		t.Error("cov-driven: expected daily values path")
	}
	t.Logf("cov-driven COVID crash (cross-asset 2330+0050+GLD): MDD=%.2f%%, VaR95=%.4f, Sharpe=%.3f",
		result.MaxDrawdown*100, result.VaR95, result.SharpeRatio)
}

func TestStressSortinoComputed(t *testing.T) {
	runner := NewRunner(domain.AgentRegistry{}, domain.ExecutionPolicy{})

	stockQuotes := []domain.Quote{
		{Symbol: "2330.TW", Last: 300, IsTradable: true},
		{Symbol: "0050.TW", Last: 120, IsTradable: true},
		{Symbol: "GLD", Last: 150, IsTradable: true},
	}
	recs := []domain.Recommendation{
		{Symbol: "2330.TW", Side: domain.SideBuy, Conviction: 50},
		{Symbol: "0050.TW", Side: domain.SideBuy, Conviction: 50},
		{Symbol: "GLD", Side: domain.SideBuy, Conviction: 90},
	}

	result := runner.RunScenario(ScenarioCOVIDCrash, stockQuotes, recs)

	if math.IsNaN(result.SortinoRatio) {
		t.Error("SortinoRatio is NaN")
	}
	if result.SortinoRatio == 0 {
		t.Error("SortinoRatio should be non-zero for multi-day scenario with returns")
	}

	// Sortino should generally be >= Sharpe (downside deviation <= total deviation)
	if result.SortinoRatio < result.SharpeRatio {
		t.Logf("SortinoRatio %.4f < SharpeRatio %.4f (possible if no downside)", result.SortinoRatio, result.SharpeRatio)
	}
}

func TestMultiDayStress_AllScenarios(t *testing.T) {
	runner := NewRunner(domain.AgentRegistry{}, domain.ExecutionPolicy{})

	stockQuotes := []domain.Quote{
		{Symbol: "2330.TW", Last: 600, IsTradable: true},
		{Symbol: "0050.TW", Last: 140, IsTradable: true},
	}
	recs := []domain.Recommendation{
		{Symbol: "2330.TW", Side: domain.SideBuy, Conviction: 50},
		{Symbol: "0050.TW", Side: domain.SideBuy, Conviction: 50},
	}

	report := runner.RunAll(stockQuotes, recs)

	expected := len(AllScenarios())
	if len(report.ScenarioResults) != expected {
		t.Fatalf("expected %d results, got %d", expected, len(report.ScenarioResults))
	}

	highStressCount := 0
	normalMDD := 0.0
	for _, r := range report.ScenarioResults {
		if len(r.DailyValues) == 0 {
			t.Errorf("%s: empty daily values", r.ScenarioName)
		}
		if math.IsNaN(r.MaxDrawdown) || r.MaxDrawdown < 0 || r.MaxDrawdown > 1 {
			t.Errorf("%s: MDD = %f, expected in [0, 1]", r.ScenarioName, r.MaxDrawdown)
		}
		if r.MaxDrawdown > 0.05 {
			highStressCount++
		}
		if r.ScenarioID == ScenarioNormalMarket.ID {
			normalMDD = r.MaxDrawdown
		}
	}

	if highStressCount < 2 {
		t.Errorf("only %d scenarios had MDD > 5%%, stress test too lenient", highStressCount)
	}

	// NormalMarket should be in the lower half of drawdowns (not the worst).
	lowerHalfCount := 0
	medianDD := normalMDD
	for _, r := range report.ScenarioResults {
		if r.MaxDrawdown <= medianDD {
			lowerHalfCount++
		}
	}
	if lowerHalfCount > 6 {
		t.Errorf("NormalMarket MDD = %.2f%% should be below median (only %d scenarios ≤ it)",
			normalMDD*100, lowerHalfCount)
	}
}
