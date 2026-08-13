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
	"github.com/kaecer68/atlas-go/internal/ledger"
)

func TestGenerateReport_EmptyLedger(t *testing.T) {
	tmpDir := t.TempDir()
	report, err := GenerateReport(ledger.NewStore(tmpDir), tmpDir, "all")
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
	report, err := GenerateReport(ledger.NewStore(tmpDir), tmpDir, "all")
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

	reportAll, err := GenerateReport(ledger.NewStore(tmpDir), tmpDir, "all")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reportAll.TotalTrades != 2 {
		t.Errorf("expected 2 trades for all, got %d", reportAll.TotalTrades)
	}

	report30d, err := GenerateReport(ledger.NewStore(tmpDir), tmpDir, "30d")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if report30d.TotalTrades != 1 {
		t.Errorf("expected 1 trade for 30d, got %d", report30d.TotalTrades)
	}
}

func TestGenerateReport_InvalidPeriod(t *testing.T) {
	tmpDir := setupTestLedger(t)
	report, err := GenerateReport(ledger.NewStore(tmpDir), tmpDir, "invalid")
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
	report, err := GenerateReport(ledger.NewStore(tmpDir), tmpDir, "all")
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
		want    *float64
	}{
		{"empty", []float64{}, nil},
		{"single", []float64{0.01}, nil},
		{"zero variance", []float64{0.01, 0.01, 0.01}, float64Ptr(0)},
		{"normal", []float64{0.01, -0.005, 0.02, -0.01, 0.015}, float64Ptr(7.360)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CalculateSharpeRatio(tt.returns)
			if tt.want == nil {
				if got != nil {
					t.Errorf("CalculateSharpeRatio() = %v, want nil", *got)
				}
				return
			}
			if got == nil {
				t.Errorf("CalculateSharpeRatio() = nil, want %f", *tt.want)
				return
			}
			if math.Abs(*got-*tt.want) > 0.001 {
				t.Errorf("CalculateSharpeRatio() = %f, want %f", *got, *tt.want)
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
		t.Errorf("expected non-nil SharpeLike with 5 real samples")
	} else if *agents[0].SharpeLike == 0 {
		t.Errorf("expected non-zero Sharpe with 5 real samples, got %f", *agents[0].SharpeLike)
	}
}

// TestFilterSummariesByDate_AllPeriodDropsUnparseable verifies the
// defensive guard in filterSummariesByDate that drops summaries with an
// unparseable SessionID (e.g. a corrupted summary.json that decodes to
// SessionID=""). Without it, period=all would sort the empty SessionID
// first and surface start_date=0001-01-01 / starting_value=0.
//
// The caller (GenerateReport) runs slices.SortFunc on SessionID before
// invoking the filter, so the input here is pre-sorted to mirror that
// call-site contract. The function itself is not responsible for sorting.
func TestFilterSummariesByDate_AllPeriodDropsUnparseable(t *testing.T) {
	now := time.Now()

	summaries := []domain.SessionSummary{
		// First in the slice (would be the corrupted entry in real life,
		// because an empty SessionID sorts to the very front of the slice
		// and then survives into the period=all branch). The guard must
		// drop it regardless of its position.
		{SessionID: "", PortfolioValue: 0, RecordedAt: time.Time{}},
		// Healthy entries, pre-sorted by SessionID (the real call site
		// guarantees this via slices.SortFunc before invoking the filter).
		{SessionID: "session-20260101-daily", PortfolioValue: 1_000_000, RecordedAt: now},
		{SessionID: "session-20260601-daily", PortfolioValue: 1_100_000, RecordedAt: now},
	}

	got := filterSummariesByDate(summaries, time.Time{})
	if len(got) != 2 {
		t.Fatalf("expected 2 summaries (corrupted dropped), got %d: %+v", len(got), got)
	}
	// Even though the corrupted entry was first in the input slice, the
	// guard must remove it before anything else touches it.
	if got[0].SessionID != "session-20260101-daily" {
		t.Errorf("expected first survivor session-20260101-daily, got %q", got[0].SessionID)
	}
	if got[1].SessionID != "session-20260601-daily" {
		t.Errorf("expected second survivor session-20260601-daily, got %q", got[1].SessionID)
	}
	for _, s := range got {
		if s.SessionID == "" {
			t.Errorf("unparseable summary slipped through: %+v", s)
		}
	}
}

// TestFilterSummariesByDate_CutoffDropsCorrupted verifies that for
// time-bounded periods (30d/90d/1y) the date filter is still applied and
// unparseable SessionIDs are also dropped.
func TestFilterSummariesByDate_CutoffDropsCorrupted(t *testing.T) {
	now := time.Now()
	oldDate := now.AddDate(0, 0, -60)
	recentDate := now.AddDate(0, 0, -5)

	summaries := []domain.SessionSummary{
		// Corrupted: empty SessionID — must be dropped regardless of cutoff.
		{SessionID: ""},
		// Old but parseable — must be dropped by 30d cutoff.
		{SessionID: "session-" + oldDate.Format("20060102") + "-daily"},
		// Recent and parseable — must be kept.
		{SessionID: "session-" + recentDate.Format("20060102") + "-daily"},
	}

	got := filterSummariesByDate(summaries, now.AddDate(0, 0, -30))
	if len(got) != 1 {
		t.Fatalf("expected 1 survivor, got %d: %+v", len(got), got)
	}
	if got[0].SessionID != "session-"+recentDate.Format("20060102")+"-daily" {
		t.Errorf("expected recent session, got %q", got[0].SessionID)
	}
}

// TestGenerateReport_CorruptedSummaryInAllPeriod is the end-to-end
// regression test for the bug discovered on 2026-07-30:
// session-20260326-daily/summary.json was written with PascalCase keys,
// leaving every domain.SessionSummary field at its zero value. With the
// defensive guards in Store.LoadSessionSummaries and
// filterSummariesByDate, the corrupted entry is dropped and the report
// falls back to the next parseable session instead of surfacing
// start_date=0001-01-01 and starting_value=0.
func TestGenerateReport_CorruptedSummaryInAllPeriod(t *testing.T) {
	tmpDir := t.TempDir()

	// 1) Corrupted PascalCase summary (mimics the production bug).
	corruptDir := filepath.Join(tmpDir, "sessions", "session-20260326-daily")
	if err := os.MkdirAll(corruptDir, 0o755); err != nil {
		t.Fatalf("mkdir corrupt: %v", err)
	}
	pascalJSON := []byte(`{
  "SessionID": "session-20260326-daily",
  "Regime": "NEUTRAL",
  "PortfolioValue": 0,
  "EndingCash": 0,
  "OutcomeCount": 0,
  "RecordedAt": "2026-06-27T22:25:54Z"
}`)
	if err := os.WriteFile(filepath.Join(corruptDir, "summary.json"), pascalJSON, 0o644); err != nil {
		t.Fatalf("write corrupt: %v", err)
	}

	// 2) Two healthy sessions so the report has real start/end anchors.
	writeSession(t, tmpDir, "session-20260101-daily", 1_000_000, 100_000, 0, 0)
	writeSession(t, tmpDir, "session-20260102-daily", 1_050_000, 100_000, 1, 100)

	report, err := GenerateReport(ledger.NewStore(tmpDir), tmpDir, "all")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if report == nil {
		t.Fatal("expected non-nil report")
	}
	if report.StartDate.IsZero() {
		t.Errorf("expected non-zero start_date, got zero (corrupted summary was used as filtered[0])")
	}
	expectedStart := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if !report.StartDate.Equal(expectedStart) {
		t.Errorf("expected start_date 2026-01-01, got %s", report.StartDate.Format("2006-01-02"))
	}
	if report.StartingValue != 1_000_000 {
		t.Errorf("expected starting_value 1,000,000, got %f", report.StartingValue)
	}
	if report.EndingValue != 1_050_000 {
		t.Errorf("expected ending_value 1,050,000, got %f", report.EndingValue)
	}
}

// TestGenerateReport_ZeroValueSessionExcluded verifies that a session with a
// parseable SessionID but zero PortfolioValue/EndingCash (e.g. legacy SQLite
// rows written before the summary_json backfill, perf-report-zero BL-01
// follow-up) is excluded from the equity curve instead of becoming a 0-valued
// starting point that collapses total_return to 0 and max_drawdown to 100%.
func TestGenerateReport_ZeroValueSessionExcluded(t *testing.T) {
	tmpDir := t.TempDir()

	// Healthy sessions anchor the window.
	writeSession(t, tmpDir, "session-20260710-daily", 2_980_000, 2_950_000, 0, 0)
	writeSession(t, tmpDir, "session-20260714-daily", 3_050_000, 2_900_000, 1, 100)

	// Zero-value session with a PARSEABLE SessionID — must not become
	// filtered[0] with pv=0.
	zeroDir := filepath.Join(tmpDir, "sessions", "session-20260716-daily")
	if err := os.MkdirAll(zeroDir, 0o755); err != nil {
		t.Fatalf("mkdir zero-value session: %v", err)
	}
	if err := os.WriteFile(filepath.Join(zeroDir, "summary.json"), []byte(`{
  "session_id": "session-20260716-daily",
  "regime": "RISK_ON",
  "portfolio_value": 0,
  "ending_cash": 0,
  "outcome_count": 0,
  "recorded_at": "2026-07-16T13:30:00Z"
}`), 0o644); err != nil {
		t.Fatalf("write zero-value summary: %v", err)
	}

	report, err := GenerateReport(ledger.NewStore(tmpDir), tmpDir, "all")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if report == nil {
		t.Fatal("expected non-nil report")
	}
	if report.StartingValue != 2_980_000 {
		t.Errorf("expected starting_value 2,980,000 (zero-value session excluded), got %f", report.StartingValue)
	}
	if report.EndingValue != 3_050_000 {
		t.Errorf("expected ending_value 3,050,000, got %f", report.EndingValue)
	}
	if report.TotalReturn <= 0 {
		t.Errorf("expected positive total_return from healthy sessions, got %f", report.TotalReturn)
	}
	if report.MaxDrawdown >= 0.9 {
		t.Errorf("expected max_drawdown well below 90%% (zero-value session polluted curve), got %f", report.MaxDrawdown)
	}
}

// writeSession is a small helper that creates a session-* directory with a
// snake_case summary.json and a recommendation_outcomes.jsonl containing a
// single outcome. Used by TestGenerateReport_CorruptedSummaryInAllPeriod.
func writeSession(t *testing.T, baseDir, sessionID string, portfolioValue, endingCash float64, outcomeCount int, totalTaxPaid float64) {
	t.Helper()
	sessDir := filepath.Join(baseDir, "sessions", sessionID)
	if err := os.MkdirAll(sessDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", sessionID, err)
	}
	summary := domain.SessionSummary{
		SessionID:      sessionID,
		Regime:         domain.RegimeRiskOn,
		PortfolioValue: portfolioValue,
		EndingCash:     endingCash,
		OutcomeCount:   outcomeCount,
		TotalTaxPaid:   totalTaxPaid,
		RecordedAt:     time.Now(),
	}
	summaryData, err := json.Marshal(summary)
	if err != nil {
		t.Fatalf("marshal %s: %v", sessionID, err)
	}
	if err := os.WriteFile(filepath.Join(sessDir, "summary.json"), summaryData, 0o644); err != nil {
		t.Fatalf("write summary %s: %v", sessionID, err)
	}
	if outcomeCount > 0 {
		outcome := domain.RecommendationOutcome{
			AgentID:       "agent-a",
			Skill:         "tech",
			Layer:         domain.AgentLayer("sector"),
			Symbol:        "2330",
			Side:          "buy",
			Window:        sessionID,
			ForwardReturn: 0.05,
			Hit:           true,
			PassedGuards:  true,
		}
		f, err := os.Create(filepath.Join(sessDir, "recommendation_outcomes.jsonl"))
		if err != nil {
			t.Fatalf("create outcomes %s: %v", sessionID, err)
		}
		if err := json.NewEncoder(f).Encode(outcome); err != nil {
			_ = f.Close()
			t.Fatalf("encode outcome %s: %v", sessionID, err)
		}
		_ = f.Close()
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
	// ISO window format (2006-01-02) — RecommendationOutcome.Window is stored as
	// "2026-01-01" (no session- prefix), so the regime lookup must also match it.
	if got := findRegimeForWindow(summaries, "2026-01-01"); got != "RISK_ON" {
		t.Errorf("expected ISO-format window match RISK_ON, got %s", got)
	}
	if got := findRegimeForWindow(summaries, "2026-01-02"); got != "RISK_OFF" {
		t.Errorf("expected ISO-format window match RISK_OFF, got %s", got)
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

// TestCalculateTradeMetrics_CostAdjustedThreshold verifies that the win-rate
// classification uses the cost-adjusted threshold (default 0.002) instead of
// raw ForwardReturn > 0. ForwardReturn values in (-0.002, +0.002) should NOT
// count as wins because they don't cover transaction cost + slippage.
func TestCalculateTradeMetrics_CostAdjustedThreshold(t *testing.T) {
	outcomes := []domain.RecommendationOutcome{
		{PassedGuards: true, ForwardReturn: 0.005},
		{PassedGuards: true, ForwardReturn: 0.001},
		{PassedGuards: true, ForwardReturn: -0.001},
		{PassedGuards: true, ForwardReturn: -0.01},
		{PassedGuards: true, ForwardReturn: 0.02},
		{PassedGuards: true, ForwardReturn: 0.0005, IsSynthetic: true},
	}
	winRate, totalTrades, realTrades, _, _, _, _ := calculateTradeMetrics(outcomes)
	if totalTrades != 6 {
		t.Errorf("expected 6 total trades (incl. synthetic), got %d", totalTrades)
	}
	if realTrades != 5 {
		t.Errorf("expected 5 real trades (synthetic excluded), got %d", realTrades)
	}
	expected := 0.4
	if math.Abs(winRate-expected) > 0.001 {
		t.Errorf("expected win rate %.3f, got %.3f", expected, winRate)
	}
}

// TestCalculateTopAgents_SharpeInsufficientSamples verifies that agents with
// fewer than 5 real samples produce a nil SharpeLike (not 0.00).
func TestCalculateTopAgents_SharpeInsufficientSamples(t *testing.T) {
	outcomes := []domain.RecommendationOutcome{
		{PassedGuards: true, AgentID: "agent-x", Skill: "tech", Layer: shared.AgentLayer("sector"), ForwardReturn: 0.01},
		{PassedGuards: true, AgentID: "agent-x", Skill: "tech", Layer: shared.AgentLayer("sector"), ForwardReturn: 0.02},
		{PassedGuards: true, AgentID: "agent-x", Skill: "tech", Layer: shared.AgentLayer("sector"), ForwardReturn: -0.005},
	}
	agents := calculateTopAgents(outcomes, nil)
	if len(agents) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(agents))
	}
	if agents[0].SharpeLike != nil {
		t.Errorf("expected nil SharpeLike with <5 samples, got %v", *agents[0].SharpeLike)
	}
}

// TestCalculateRegimeBreakdown_CostAdjustedThreshold verifies that regime
// breakdown win-rate classification also uses the cost-adjusted threshold.
func TestCalculateRegimeBreakdown_CostAdjustedThreshold(t *testing.T) {
	summaries := []domain.SessionSummary{
		{SessionID: "2026-06-15-tw", Regime: domain.RegimeRiskOn},
	}
	outcomes := []domain.RecommendationOutcome{
		{PassedGuards: true, Regime: string(domain.RegimeRiskOn), Window: "2026-06-15-tw", ForwardReturn: 0.005},
		{PassedGuards: true, Regime: string(domain.RegimeRiskOn), Window: "2026-06-15-tw", ForwardReturn: 0.001},
		{PassedGuards: true, Regime: string(domain.RegimeRiskOn), Window: "2026-06-15-tw", ForwardReturn: -0.001},
		{PassedGuards: true, Regime: string(domain.RegimeRiskOn), Window: "2026-06-15-tw", ForwardReturn: 0.02},
	}
	breakdown := calculateRegimeBreakdown(summaries, outcomes)
	regime, ok := breakdown.Regimes["RISK_ON"]
	if !ok {
		t.Fatalf("expected RISK_ON regime, got %v", breakdown.Regimes)
	}
	if regime.SessionCount != 1 {
		t.Errorf("expected 1 session for RISK_ON, got %d", regime.SessionCount)
	}
	if math.Abs(regime.WinRate-0.5) > 0.001 {
		t.Errorf("expected win rate 0.5, got %.3f", regime.WinRate)
	}
	expected := 0.005 + 0.001 - 0.001 + 0.02
	if math.Abs(regime.AggregateForwardReturn-expected) > 0.001 {
		t.Errorf("expected aggregate %.3f, got %.3f", expected, regime.AggregateForwardReturn)
	}
}

// TestCalculateTopAgents_ExcludesAllZeroForward verifies that an agent whose
// real outcomes all carry a zero forward return (SQLite fallback signature)
// is excluded from the contribution table — showing it as 0.00% would mislead.
func TestCalculateTopAgents_ExcludesAllZeroForward(t *testing.T) {
	outcomes := []domain.RecommendationOutcome{
		// agent-a: genuine nonzero forward returns → kept
		{PassedGuards: true, AgentID: "agent-a", Skill: "tech", ForwardReturn: 0.01},
		{PassedGuards: true, AgentID: "agent-a", Skill: "tech", ForwardReturn: 0.02},
		// agent-b: all-zero forward returns (SQLite fallback signature) → dropped
		{PassedGuards: true, AgentID: "agent-b", Skill: "sector", ForwardReturn: 0},
		{PassedGuards: true, AgentID: "agent-b", Skill: "sector", ForwardReturn: 0},
		{PassedGuards: true, AgentID: "agent-b", Skill: "sector", ForwardReturn: 0},
	}
	agents := calculateTopAgents(outcomes, nil)
	if len(agents) != 1 {
		t.Fatalf("expected 1 agent (all-zero excluded), got %d: %+v", len(agents), agents)
	}
	if agents[0].AgentID != "agent-a" {
		t.Errorf("expected agent-a to be kept, got %s", agents[0].AgentID)
	}
}

// TestCalculateRegimeBreakdown_UsesOutcomeRegime verifies that the regime
// breakdown attributes each outcome to the regime recorded on the outcome
// itself (oc.Regime) rather than reverse-looking-up by oc.Window. This is the
// fix for the "market regime performance" gap: outcome.Window is the
// evaluation window (e.g. 2026-07-14), not the session date, so the old
// findRegimeForWindow lookup mis-attributed RISK_OFF sessions to "unknown".
func TestCalculateRegimeBreakdown_UsesOutcomeRegime(t *testing.T) {
	summaries := []domain.SessionSummary{
		{SessionID: "session-20260718-daily", Regime: domain.RegimeRiskOff},
	}
	// Outcome carries its own regime; Window is an unrelated evaluation date
	// (no matching session). The old code would fall to "unknown".
	outcomes := []domain.RecommendationOutcome{
		{PassedGuards: true, Regime: string(domain.RegimeRiskOff), Window: "2026-07-14", ForwardReturn: 0.01497555},
		{PassedGuards: true, Regime: string(domain.RegimeRiskOff), Window: "2026-07-14", ForwardReturn: 0.00263158},
	}
	breakdown := calculateRegimeBreakdown(summaries, outcomes)

	if _, ok := breakdown.Regimes["unknown"]; ok {
		t.Errorf("expected no 'unknown' regime bucket, got %v", breakdown.Regimes)
	}
	regime, ok := breakdown.Regimes["RISK_OFF"]
	if !ok {
		t.Fatalf("expected RISK_OFF regime, got %v", breakdown.Regimes)
	}
	expected := 0.01497555 + 0.00263158
	if math.Abs(regime.AggregateForwardReturn-expected) > 0.0001 {
		t.Errorf("expected aggregate %.5f, got %.5f", expected, regime.AggregateForwardReturn)
	}
	if regime.SessionCount != 1 {
		t.Errorf("expected 1 session for RISK_OFF, got %d", regime.SessionCount)
	}
}

// TestFormatSharpeLike verifies the markdown renderer handles nil values.
func TestFormatSharpeLike(t *testing.T) {
	if got := formatSharpeLike(nil); got != "N/A" {
		t.Errorf("expected N/A for nil, got %q", got)
	}
	v := 1.5
	if got := formatSharpeLike(&v); got != "1.50" {
		t.Errorf("expected 1.50, got %q", got)
	}
}

// TestWinRateThreshold_Default verifies the helper returns the configured
// threshold (0.002 default) when config is loaded.
func TestWinRateThreshold_Default(t *testing.T) {
	got := winRateThreshold()
	if got != 0.002 {
		t.Errorf("expected default winRateThreshold 0.002, got %f", got)
	}
}
