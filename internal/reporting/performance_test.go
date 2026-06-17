package reporting

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/domain/shared"
)

// costAdjustedOutcomes builds a synthetic outcome slice with mixed forward returns
// designed to verify the cost-adjusted win-rate threshold. Uses defaultWinRateThreshold
// (with small epsilons) so tests track the live config value automatically.
func costAdjustedOutcomes(agentID string) []domain.RecommendationOutcome {
	// Above threshold (long-tail distribution): 0.0025 down to threshold+0.0005/0.0001
	// using defaultWinRateThreshold-derived bounds to avoid hardcoded magic numbers.
	t := defaultWinRateThreshold
	positivesAbove := []float64{
		0.05, 0.045, 0.04, 0.035, 0.03, 0.025, 0.02, 0.015, 0.012, 0.01,
		0.009, 0.008, 0.007, 0.006, 0.005, 0.004, 0.003, 0.0025, t + 0.0005, t + 0.0001,
	}
	// Below threshold (small positive but not profitable after costs)
	positivesBelow := []float64{
		0.0001, 0.0002, 0.0003, 0.0004, 0.0005,
		0.0001, 0.0002, 0.0003, 0.0004, 0.0005,
		0.0001, 0.0002, 0.0003, 0.0004, 0.0005,
		0.0001, 0.0002, 0.0003,
	}
	negatives := []float64{
		-0.05, -0.045, -0.04, -0.035, -0.03, -0.025,
		-0.02, -0.015, -0.01, -0.005, -0.003, -0.001,
	}

	skill, layer := "", shared.AgentLayer("")
	if agentID != "" {
		skill = "tech"
		layer = shared.LayerSector
	}

	out := make([]domain.RecommendationOutcome, 0, len(positivesAbove)+len(positivesBelow)+len(negatives))
	appendRange := func(rs []float64) {
		for _, r := range rs {
			out = append(out, domain.RecommendationOutcome{
				AgentID: agentID, Skill: skill, Layer: layer,
				ForwardReturn: r, PassedGuards: true,
			})
		}
	}
	appendRange(positivesAbove)
	appendRange(positivesBelow)
	appendRange(negatives)
	return out
}

func TestGenerateReport_EmptyLedger(t *testing.T) {
	tmpDir := t.TempDir()
	report, err := GenerateReport(tmpDir, "all")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if report == nil {
		t.Fatal("expected non-nil report")
	}
	if report.Period != "all" {
		t.Errorf("expected period all, got %s", report.Period)
	}
	if len(report.TopAgents) != 0 {
		t.Errorf("expected no agents, got %d", len(report.TopAgents))
	}
}

func TestGenerateReport_SingleSession(t *testing.T) {
	tmpDir := setupTestLedger(t)
	report, err := GenerateReport(tmpDir, "all")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if report.StartingValue != 1_000_000 {
		t.Errorf("expected starting value 1000000, got %f", report.StartingValue)
	}
	if report.EndingValue != 1_000_000 {
		t.Errorf("expected ending value 1000000, got %f", report.EndingValue)
	}
	if report.TotalTaxPaid != 1000 {
		t.Errorf("expected tax paid 1000, got %f", report.TotalTaxPaid)
	}
	if report.AfterTaxValue != 999_000 {
		t.Errorf("expected after-tax value 999000, got %f", report.AfterTaxValue)
	}
	if report.TotalTrades != 2 {
		t.Errorf("expected 2 trades, got %d", report.TotalTrades)
	}
	if len(report.TopAgents) != 2 {
		t.Errorf("expected 2 agents, got %d", len(report.TopAgents))
	}
}

func TestGenerateReport_PeriodFiltering(t *testing.T) {
	tmpDir := setupTestLedgerWithOldAndNewSessions(t)

	reportAll, err := GenerateReport(tmpDir, "all")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reportAll.TotalTrades != 2 {
		t.Errorf("expected 2 trades for all, got %d", reportAll.TotalTrades)
	}

	report30d, err := GenerateReport(tmpDir, "30d")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if report30d.TotalTrades != 1 {
		t.Errorf("expected 1 trade for 30d, got %d", report30d.TotalTrades)
	}
}

func TestGenerateReport_InvalidPeriod(t *testing.T) {
	tmpDir := setupTestLedger(t)
	report, err := GenerateReport(tmpDir, "invalid")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if report == nil {
		t.Fatal("expected non-nil report for invalid period")
	}
}

func TestGenerateMarkdownReport(t *testing.T) {
	report := &PerformanceReport{
		Period:           "all",
		StartDate:        time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		EndDate:          time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC),
		TotalReturn:      0.05,
		AnnualizedReturn: 0.80,
		SortinoRatio:     1.5,
		CalmarRatio:      26.67,
		MaxDrawdown:      0.03,
		StartingValue:    1_000_000,
		EndingValue:      1_050_000,
		AfterTaxValue:    1_040_000,
		TotalTaxPaid:     10_000,
		WinRate:          0.6,
		TotalTrades:      10,
		AvgWin:           0.02,
		AvgLoss:          -0.01,
		TopAgents: []AgentContribution{
			{AgentID: "agent-a", Skill: "tech", Layer: "sector", AggregateForwardReturn: 0.03, WinRate: 0.7, TradeCount: 5, AvgReturn: 0.006},
		},
		RegimeBreakdown: RegimeBreakdown{
			Regimes: map[string]RegimePerformance{
				"RISK_ON": {Regime: "RISK_ON", SessionCount: 5, AggregateForwardReturn: 0.05, WinRate: 0.6, AvgReturn: 0.01},
			},
		},
		MonthlyReturns: []MonthlyReturn{
			{Year: 2026, Month: 1, Return: 0.05, Label: "2026-01"},
		},
		GeneratedAt: time.Now(),
	}

	md := GenerateMarkdownReport(report)
	if !strings.Contains(md, "Performance Report") {
		t.Error("expected markdown to contain 'Performance Report'")
	}
	if !strings.Contains(md, "Key Metrics") {
		t.Error("expected markdown to contain 'Key Metrics'")
	}
	if !strings.Contains(md, "agent-a") {
		t.Error("expected markdown to contain agent-a")
	}
	if !strings.Contains(md, "RISK_ON") {
		t.Error("expected markdown to contain RISK_ON")
	}
	if !strings.Contains(md, "2026-01") {
		t.Error("expected markdown to contain 2026-01")
	}
	if !strings.Contains(md, "NT$") {
		t.Error("expected markdown to contain formatted NTD values")
	}
}

func TestGenerateMarkdownReport_Nil(t *testing.T) {
	md := GenerateMarkdownReport(nil)
	if !strings.Contains(md, "No report data available") {
		t.Error("expected nil report message")
	}
}

func float64Ptr(v float64) *float64 {
	return &v
}

func TestGenerateReport_SharpeNA(t *testing.T) {
	tmpDir := setupTestLedger(t)
	report, err := GenerateReport(tmpDir, "all")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if report.SharpeRatio != nil {
		t.Errorf("expected nil SharpeRatio for single-session report, got %v", *report.SharpeRatio)
	}
}

func TestGenerateMarkdownReport_SharpeNA(t *testing.T) {
	report := emptyReport("all")
	md := GenerateMarkdownReport(report)
	if !strings.Contains(md, "| Sharpe Ratio | N/A |") {
		t.Errorf("expected markdown to contain 'N/A' Sharpe, got:\n%s", md)
	}

	report2 := &PerformanceReport{
		Period:      "all",
		SharpeRatio: float64Ptr(1.234),
	}
	md2 := GenerateMarkdownReport(report2)
	if !strings.Contains(md2, "| Sharpe Ratio | 1.234 |") {
		t.Errorf("expected markdown to contain formatted Sharpe, got:\n%s", md2)
	}
}

func TestCalculateSharpeRatio(t *testing.T) {
	tests := []struct {
		name    string
		returns []float64
		want    float64
	}{
		{"empty", []float64{}, 0},
		{"zero variance", []float64{0.01, 0.01, 0.01}, 0},
		{"normal", []float64{0.01, -0.005, 0.02, -0.01, 0.015}, 7.360},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CalculateSharpeRatio(tt.returns)
			if math.Abs(got-tt.want) > 0.001 {
				t.Errorf("CalculateSharpeRatio() = %f, want %f", got, tt.want)
			}
		})
	}
}

func TestCalculateCalmarRatio(t *testing.T) {
	tests := []struct {
		name             string
		annualizedReturn float64
		maxDrawdown      float64
		want             float64
	}{
		{"zero maxDD", 0.5, 0, 0},
		{"normal", 0.5, 0.2, 2.5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := calculateCalmarRatio(tt.annualizedReturn, tt.maxDrawdown)
			if math.Abs(got-tt.want) > 0.001 {
				t.Errorf("calculateCalmarRatio() = %f, want %f", got, tt.want)
			}
		})
	}
}

func TestCalculateTradeMetrics(t *testing.T) {
	outcomes := []domain.RecommendationOutcome{
		{ForwardReturn: 0.05, PassedGuards: true},
		{ForwardReturn: 0.03, PassedGuards: true},
		{ForwardReturn: -0.02, PassedGuards: true},
		{ForwardReturn: 0.01, PassedGuards: true},
		{ForwardReturn: -0.01, PassedGuards: true},
		{ForwardReturn: 0.02, PassedGuards: false},
	}

	winRate, totalTrades, realTrades, syntheticTrades, avgWin, avgLoss, profitFactor := calculateTradeMetrics(outcomes)

	if totalTrades != 5 {
		t.Errorf("expected 5 trades, got %d", totalTrades)
	}
	if realTrades != 5 {
		t.Errorf("expected 5 real trades, got %d", realTrades)
	}
	if syntheticTrades != 0 {
		t.Errorf("expected 0 synthetic trades, got %d", syntheticTrades)
	}
	if math.Abs(winRate-0.6) > 1e-9 {
		t.Errorf("expected win rate 0.6, got %f", winRate)
	}
	if math.Abs(avgWin-0.03) > 1e-9 {
		t.Errorf("expected avg win 0.03, got %f", avgWin)
	}
	if math.Abs(avgLoss+0.015) > 1e-9 {
		t.Errorf("expected avg loss -0.015, got %f", avgLoss)
	}
	if math.Abs(profitFactor-(0.09/0.03)) > 1e-9 {
		t.Errorf("expected profit factor %.4f, got %f", 0.09/0.03, profitFactor)
	}
}

func TestEmptyReport(t *testing.T) {
	r := emptyReport("30d")
	if r.Period != "30d" {
		t.Errorf("expected period 30d, got %s", r.Period)
	}
	if r.TopAgents == nil {
		t.Error("expected non-nil TopAgents")
	}
	if r.RegimeBreakdown.Regimes == nil {
		t.Error("expected non-nil Regimes")
	}
	if r.MonthlyReturns == nil {
		t.Error("expected non-nil MonthlyReturns")
	}
}

func TestCalculateTradeMetrics_ExcludesSynthetic(t *testing.T) {
	outcomes := []domain.RecommendationOutcome{
		{ForwardReturn: 0.05, PassedGuards: true, IsSynthetic: true},
		{ForwardReturn: -0.10, PassedGuards: true, IsSynthetic: true},
		{ForwardReturn: 0.03, PassedGuards: true},
		{ForwardReturn: -0.01, PassedGuards: true},
		{ForwardReturn: 0.02, PassedGuards: true},
	}

	winRate, totalTrades, realTrades, syntheticTrades, avgWin, avgLoss, profitFactor := calculateTradeMetrics(outcomes)

	if totalTrades != 5 {
		t.Errorf("expected total trades 5, got %d", totalTrades)
	}
	if syntheticTrades != 2 {
		t.Errorf("expected synthetic trades 2, got %d", syntheticTrades)
	}
	if realTrades != 3 {
		t.Errorf("expected real trades 3, got %d", realTrades)
	}
	if math.Abs(winRate-2.0/3.0) > 1e-9 {
		t.Errorf("expected win rate %.4f, got %f", 2.0/3.0, winRate)
	}
	if math.Abs(avgWin-0.025) > 1e-9 {
		t.Errorf("expected avg win 0.025, got %f", avgWin)
	}
	if math.Abs(avgLoss+0.01) > 1e-9 {
		t.Errorf("expected avg loss -0.01, got %f", avgLoss)
	}
	if math.Abs(profitFactor-(0.05/0.01)) > 1e-9 {
		t.Errorf("expected profit factor %.4f, got %f", 0.05/0.01, profitFactor)
	}
}

func TestCalculateTradeMetrics_ProfitFactorZeroLosses(t *testing.T) {
	outcomes := []domain.RecommendationOutcome{
		{ForwardReturn: 0.05, PassedGuards: true},
		{ForwardReturn: 0.03, PassedGuards: true},
	}

	_, _, _, _, _, _, profitFactor := calculateTradeMetrics(outcomes)

	if profitFactor != 0 {
		t.Errorf("expected profit factor 0 when no losses, got %f", profitFactor)
	}
}

func TestCalculateTradeMetrics_CostAdjustedThreshold(t *testing.T) {
	outcomes := costAdjustedOutcomes("")

	winRate, totalTrades, realTrades, syntheticTrades, _, _, _ := calculateTradeMetrics(outcomes)

	if totalTrades != 50 {
		t.Errorf("expected 50 trades, got %d", totalTrades)
	}
	if realTrades != 50 {
		t.Errorf("expected 50 real trades, got %d", realTrades)
	}
	if syntheticTrades != 0 {
		t.Errorf("expected 0 synthetic trades, got %d", syntheticTrades)
	}
	expectedWinRate := 20.0 / 50.0 // len(positivesAbove)=20 of totalTrades=50
	if math.Abs(winRate-expectedWinRate) > 1e-9 {
		t.Errorf("expected win rate %.4f with cost-adjusted threshold, got %.4f", expectedWinRate, winRate)
	}
}

func TestCalculateTopAgents_CostAdjustedThreshold(t *testing.T) {
	outcomes := costAdjustedOutcomes("agent-a")

	agents := calculateTopAgents(outcomes, nil)
	if len(agents) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(agents))
	}
	expectedWinRate := 20.0 / 50.0 // len(positivesAbove)=20 of total=50
	if math.Abs(agents[0].WinRate-expectedWinRate) > 1e-9 {
		t.Errorf("expected agent win rate %.4f with cost-adjusted threshold, got %.4f", expectedWinRate, agents[0].WinRate)
	}
}

func TestCalculateTopAgents_DisplayNamePresent(t *testing.T) {
	outcomes := []domain.RecommendationOutcome{
		{AgentID: "agent-a", Skill: "tech", Layer: "sector", ForwardReturn: 0.05, PassedGuards: true},
	}
	names := map[string]string{"agent-a": "Alpha Agent"}

	agents := calculateTopAgents(outcomes, names)
	if len(agents) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(agents))
	}
	if agents[0].DisplayName != "Alpha Agent" {
		t.Errorf("expected display name Alpha Agent, got %s", agents[0].DisplayName)
	}
}

func TestCalculateTopAgents_DisplayNameFallback(t *testing.T) {
	outcomes := []domain.RecommendationOutcome{
		{AgentID: "agent-x", Skill: "macro", Layer: "context", ForwardReturn: 0.01, PassedGuards: true},
	}

	agents := calculateTopAgents(outcomes, nil)
	if len(agents) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(agents))
	}
	if agents[0].DisplayName != "agent-x" {
		t.Errorf("expected display name fallback agent-x, got %s", agents[0].DisplayName)
	}
}

func TestCalculateTopAgents_SyntheticSeparation(t *testing.T) {
	outcomes := []domain.RecommendationOutcome{
		{AgentID: "agent-a", Skill: "tech", Layer: "sector", ForwardReturn: 0.05, PassedGuards: true},
		{AgentID: "agent-a", Skill: "tech", Layer: "sector", ForwardReturn: -0.02, PassedGuards: true, IsSynthetic: true},
		{AgentID: "agent-a", Skill: "tech", Layer: "sector", ForwardReturn: -0.01, PassedGuards: true},
	}

	agents := calculateTopAgents(outcomes, nil)
	if len(agents) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(agents))
	}
	a := agents[0]
	if a.TradeCount != 3 {
		t.Errorf("expected trade count 3, got %d", a.TradeCount)
	}
	if a.RealTradeCount != 2 {
		t.Errorf("expected real trade count 2, got %d", a.RealTradeCount)
	}
	if a.SyntheticTradeCount != 1 {
		t.Errorf("expected synthetic trade count 1, got %d", a.SyntheticTradeCount)
	}
	if a.WinRate != 0.5 {
		t.Errorf("expected win rate 0.5 (real only), got %f", a.WinRate)
	}
}

func TestCalculateTopAgents_SharpeMinSamples5(t *testing.T) {
	outcomes := []domain.RecommendationOutcome{
		{AgentID: "agent-a", Skill: "tech", Layer: "sector", ForwardReturn: 0.01, PassedGuards: true},
		{AgentID: "agent-a", Skill: "tech", Layer: "sector", ForwardReturn: -0.005, PassedGuards: true},
		{AgentID: "agent-a", Skill: "tech", Layer: "sector", ForwardReturn: 0.02, PassedGuards: true},
		{AgentID: "agent-a", Skill: "tech", Layer: "sector", ForwardReturn: -0.01, PassedGuards: true},
		{AgentID: "agent-a", Skill: "tech", Layer: "sector", ForwardReturn: 0.015, PassedGuards: true},
	}

	agents := calculateTopAgents(outcomes, nil)
	if len(agents) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(agents))
	}
	if agents[0].SharpeLike == nil {
		t.Fatal("expected non-nil SharpeLike with 5 real samples")
	}
	if *agents[0].SharpeLike == 0 {
		t.Errorf("expected non-zero Sharpe with 5 real samples, got %v", *agents[0].SharpeLike)
	}
}

func TestCalculateTopAgents_SharpeInsufficientSamples(t *testing.T) {
	outcomes := []domain.RecommendationOutcome{
		{AgentID: "agent-a", Skill: "tech", Layer: "sector", ForwardReturn: 0.01, PassedGuards: true},
		{AgentID: "agent-a", Skill: "tech", Layer: "sector", ForwardReturn: -0.005, PassedGuards: true},
		{AgentID: "agent-a", Skill: "tech", Layer: "sector", ForwardReturn: 0.02, PassedGuards: true},
	}

	agents := calculateTopAgents(outcomes, nil)
	if len(agents) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(agents))
	}
	if agents[0].SharpeLike != nil {
		t.Errorf("expected nil SharpeLike for < 5 samples, got %v", *agents[0].SharpeLike)
	}
}

// TestCalculateTopAgents_SharpeStdDevZero verifies that when an agent's
// returns have zero standard deviation (all identical values), SharpeLike
// stays nil so the frontend renders "N/A" instead of the misleading "0.00".
// This locks in Issue 3 of PR #562 code review.
func TestCalculateTopAgents_SharpeStdDevZero(t *testing.T) {
	outcomes := []domain.RecommendationOutcome{
		{AgentID: "agent-a", Skill: "tech", Layer: "sector", ForwardReturn: 0.02, PassedGuards: true},
		{AgentID: "agent-a", Skill: "tech", Layer: "sector", ForwardReturn: 0.02, PassedGuards: true},
		{AgentID: "agent-a", Skill: "tech", Layer: "sector", ForwardReturn: 0.02, PassedGuards: true},
		{AgentID: "agent-a", Skill: "tech", Layer: "sector", ForwardReturn: 0.02, PassedGuards: true},
		{AgentID: "agent-a", Skill: "tech", Layer: "sector", ForwardReturn: 0.02, PassedGuards: true},
	}

	agents := calculateTopAgents(outcomes, nil)
	if len(agents) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(agents))
	}
	if agents[0].SharpeLike != nil {
		t.Errorf("expected nil SharpeLike when std-dev=0 (renders N/A), got %v", *agents[0].SharpeLike)
	}
}

// TestCalculateRegimeBreakdown_CostAdjustedThreshold verifies that regime
// win-rate uses winRateThreshold() instead of `r > 0`, matching the
// behavior of calculateTradeMetrics and calculateTopAgents. This locks in
// Issue 1 of PR #562 code review.
func TestCalculateRegimeBreakdown_CostAdjustedThreshold(t *testing.T) {
	summaries := []domain.SessionSummary{
		{SessionID: "session-20260101-daily", Regime: domain.RegimeRiskOn},
	}

	// 5 returns for RISK_ON: 2 above threshold (wins), 2 between 0 and
	// threshold (cost-failed positives), 1 negative.
	// Old `r > 0` logic: 3 wins / 5 = 0.6 (FALSE positive — 0.0005 isn't a real win).
	// New threshold logic: 2 wins / 5 = 0.4.
	outcomes := []domain.RecommendationOutcome{
		{Window: "session-20260101-daily", ForwardReturn: 0.05, PassedGuards: true},
		{Window: "session-20260101-daily", ForwardReturn: 0.03, PassedGuards: true},
		{Window: "session-20260101-daily", ForwardReturn: 0.0005, PassedGuards: true},
		{Window: "session-20260101-daily", ForwardReturn: 0.0001, PassedGuards: true},
		{Window: "session-20260101-daily", ForwardReturn: -0.01, PassedGuards: true},
	}

	breakdown := calculateRegimeBreakdown(summaries, outcomes)
	riskOn, ok := breakdown.Regimes["RISK_ON"]
	if !ok {
		t.Fatalf("expected RISK_ON regime, got %v", breakdown.Regimes)
	}
	expectedWinRate := 2.0 / 5.0
	if math.Abs(riskOn.WinRate-expectedWinRate) > 1e-9 {
		t.Errorf("expected regime win rate %.4f with cost-adjusted threshold, got %.4f", expectedWinRate, riskOn.WinRate)
	}
}

func TestFindRegimeForWindow_DateMatch(t *testing.T) {
	summaries := []domain.SessionSummary{
		{SessionID: "session-20260101-daily", Regime: domain.RegimeRiskOn},
		{SessionID: "session-20260102-daily", Regime: domain.RegimeRiskOff},
	}

	if got := findRegimeForWindow(summaries, "session-20260101-daily"); got != "RISK_ON" {
		t.Errorf("expected exact match RISK_ON, got %s", got)
	}
	if got := findRegimeForWindow(summaries, "session-20260102-nightly"); got != "RISK_OFF" {
		t.Errorf("expected date-match fallback RISK_OFF, got %s", got)
	}
	if got := findRegimeForWindow(summaries, "session-20261231-daily"); got != "" {
		t.Errorf("expected empty for unmatched window, got %s", got)
	}
}

func setupTestLedger(t *testing.T) string {
	t.Helper()
	tmpDir := t.TempDir()
	sessionsDir := filepath.Join(tmpDir, "sessions", "session-20260101-daily")
	if err := os.MkdirAll(sessionsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	summary := domain.SessionSummary{
		SessionID:      "session-20260101-daily",
		Regime:         domain.RegimeRiskOn,
		PortfolioValue: 1_000_000,
		EndingCash:     100_000,
		OutcomeCount:   2,
		TotalTaxPaid:   1000,
		RecordedAt:     time.Now(),
	}

	summaryData, err := json.Marshal(summary)
	if err != nil {
		t.Fatalf("marshal summary: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sessionsDir, "summary.json"), summaryData, 0o644); err != nil {
		t.Fatalf("write summary: %v", err)
	}

	outcomes := []domain.RecommendationOutcome{
		{
			AgentID:       "agent-a",
			Skill:         "tech",
			Layer:         "sector",
			Symbol:        "2330",
			Side:          "buy",
			Window:        "session-20260101-daily",
			ForwardReturn: 0.05,
			Hit:           true,
			PassedGuards:  true,
		},
		{
			AgentID:       "agent-b",
			Skill:         "value",
			Layer:         "style",
			Symbol:        "2881",
			Side:          "buy",
			Window:        "session-20260101-daily",
			ForwardReturn: -0.02,
			Hit:           false,
			PassedGuards:  true,
		},
	}

	outcomeFile, err := os.Create(filepath.Join(sessionsDir, "recommendation_outcomes.jsonl"))
	if err != nil {
		t.Fatalf("create outcomes file: %v", err)
	}
	defer outcomeFile.Close()

	enc := json.NewEncoder(outcomeFile)
	for _, oc := range outcomes {
		if err := enc.Encode(oc); err != nil {
			t.Fatalf("encode outcome: %v", err)
		}
	}

	return tmpDir
}

func setupTestLedgerWithOldAndNewSessions(t *testing.T) string {
	t.Helper()
	tmpDir := t.TempDir()

	now := time.Now()
	oldDate := now.AddDate(0, 0, -60)
	newDate := now.AddDate(0, 0, -1)

	sessions := []struct {
		id       string
		date     time.Time
		value    float64
		isRecent bool
	}{
		{
			id:       "session-" + oldDate.Format("20060102") + "-daily",
			date:     oldDate,
			value:    900_000,
			isRecent: false,
		},
		{
			id:       "session-" + newDate.Format("20060102") + "-daily",
			date:     newDate,
			value:    1_000_000,
			isRecent: true,
		},
	}

	for _, sess := range sessions {
		sessionsDir := filepath.Join(tmpDir, "sessions", sess.id)
		if err := os.MkdirAll(sessionsDir, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}

		summary := domain.SessionSummary{
			SessionID:      sess.id,
			Regime:         domain.RegimeRiskOn,
			PortfolioValue: sess.value,
			EndingCash:     100_000,
			OutcomeCount:   2,
			TotalTaxPaid:   500,
			RecordedAt:     time.Now(),
		}

		summaryData, err := json.Marshal(summary)
		if err != nil {
			t.Fatalf("marshal summary: %v", err)
		}
		if err := os.WriteFile(filepath.Join(sessionsDir, "summary.json"), summaryData, 0o644); err != nil {
			t.Fatalf("write summary: %v", err)
		}

		outcome := domain.RecommendationOutcome{
			AgentID:       "agent-a",
			Skill:         "tech",
			Layer:         "sector",
			Symbol:        "2330",
			Side:          "buy",
			Window:        sess.id,
			ForwardReturn: 0.05,
			Hit:           true,
			PassedGuards:  true,
		}

		outcomeFile, err := os.Create(filepath.Join(sessionsDir, "recommendation_outcomes.jsonl"))
		if err != nil {
			t.Fatalf("create outcomes file: %v", err)
		}
		if err := json.NewEncoder(outcomeFile).Encode(outcome); err != nil {
			outcomeFile.Close()
			t.Fatalf("encode outcome: %v", err)
		}
		outcomeFile.Close()
	}

	return tmpDir
}
