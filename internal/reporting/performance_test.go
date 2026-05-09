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
)

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
		SharpeRatio:      1.2,
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
			{AgentID: "agent-a", Skill: "tech", Layer: "sector", TotalReturn: 0.03, WinRate: 0.7, TradeCount: 5, AvgReturn: 0.006},
		},
		RegimeBreakdown: RegimeBreakdown{
			Regimes: map[string]RegimePerformance{
				"RISK_ON": {Regime: "RISK_ON", SessionCount: 5, TotalReturn: 0.05, WinRate: 0.6, AvgReturn: 0.01},
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

func TestCalculateSharpeRatio(t *testing.T) {
	tests := []struct {
		name    string
		returns []float64
		want    float64
	}{
		{"empty", []float64{}, 0},
		{"zero variance", []float64{0.01, 0.01, 0.01}, 0},
		{"normal", []float64{0.01, -0.005, 0.02, -0.01, 0.015}, 8.228},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := calculateSharpeRatio(tt.returns)
			if math.Abs(got-tt.want) > 0.001 {
				t.Errorf("calculateSharpeRatio() = %f, want %f", got, tt.want)
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

	winRate, totalTrades, avgWin, avgLoss := calculateTradeMetrics(outcomes)

	if totalTrades != 5 {
		t.Errorf("expected 5 trades, got %d", totalTrades)
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
