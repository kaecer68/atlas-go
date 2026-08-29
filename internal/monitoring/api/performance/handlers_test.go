package performance

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/monitoring/service"
)

func TestHandleReport_NoData(t *testing.T) {
	tmpDir := t.TempDir()
	svc := service.NewPerformanceService(ledger.NewStore(tmpDir), tmpDir)
	h := NewHandlers(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/performance-report?period=all", nil)

	status, data := h.HandleReport(req)
	if status != http.StatusOK {
		t.Fatalf("expected status 200, got %d", status)
	}

	report, ok := data.(map[string]any)
	if ok {
		if report["period"] != "all" {
			t.Errorf("expected period all, got %v", report["period"])
		}
	}
}

func TestHandleReport_SingleSession(t *testing.T) {
	tmpDir := setupTestLedger(t)
	svc := service.NewPerformanceService(ledger.NewStore(tmpDir), tmpDir)
	h := NewHandlers(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/performance-report?period=all", nil)

	status, data := h.HandleReport(req)
	if status != http.StatusOK {
		t.Fatalf("expected status 200, got %d", status)
	}

	body, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	if result["period"] != "all" {
		t.Errorf("expected period all, got %v", result["period"])
	}
	if result["total_trades"] == nil {
		t.Error("expected total_trades field")
	}
}

func TestHandleReport_PeriodFiltering(t *testing.T) {
	tmpDir := setupTestLedgerWithMultipleSessions(t)
	svc := service.NewPerformanceService(ledger.NewStore(tmpDir), tmpDir)
	h := NewHandlers(svc)

	periods := []string{"30d", "90d", "1y", "all"}
	for _, period := range periods {
		req := httptest.NewRequest(http.MethodGet, "/api/dashboard/performance-report?period="+period, nil)
		status, data := h.HandleReport(req)
		if status != http.StatusOK {
			t.Errorf("period %s: expected status 200, got %d", period, status)
		}
		if data == nil {
			t.Errorf("period %s: expected non-nil data", period)
		}
	}
}

func TestHandleExport(t *testing.T) {
	tmpDir := setupTestLedger(t)
	svc := service.NewPerformanceService(ledger.NewStore(tmpDir), tmpDir)
	h := NewHandlers(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/performance-report/export?period=all", nil)
	w := httptest.NewRecorder()

	status, _ := h.HandleExport(w, req)
	if status != 0 {
		t.Fatalf("expected status 0 (already written), got %d", status)
	}

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	contentType := resp.Header.Get("Content-Type")
	if !strings.Contains(contentType, "text/markdown") {
		t.Errorf("expected text/markdown content type, got %s", contentType)
	}

	body := w.Body.String()
	if !strings.Contains(body, "Performance Report") {
		t.Error("expected markdown to contain 'Performance Report'")
	}
	if !strings.Contains(body, "Key Metrics") {
		t.Error("expected markdown to contain 'Key Metrics'")
	}
}

func TestHandleReport_DefaultPeriod(t *testing.T) {
	tmpDir := setupTestLedger(t)
	svc := service.NewPerformanceService(ledger.NewStore(tmpDir), tmpDir)
	h := NewHandlers(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/performance-report", nil)

	status, data := h.HandleReport(req)
	if status != http.StatusOK {
		t.Fatalf("expected status 200, got %d", status)
	}
	if data == nil {
		t.Fatal("expected non-nil data")
	}
}

func TestHandleReport_SnakeCaseJSON(t *testing.T) {
	tmpDir := setupTestLedger(t)
	svc := service.NewPerformanceService(ledger.NewStore(tmpDir), tmpDir)
	h := NewHandlers(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/performance-report?period=all", nil)

	_, data := h.HandleReport(req)
	body, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	snakeCaseFields := []string{
		"total_return", "annualized_return", "sharpe_ratio", "max_drawdown",
		"starting_value", "ending_value", "after_tax_value", "total_tax_paid",
		"win_rate", "total_trades", "avg_win", "avg_loss",
		"top_agents", "regime_breakdown", "monthly_returns",
	}

	for _, field := range snakeCaseFields {
		if _, ok := result[field]; !ok {
			t.Errorf("expected snake_case field %q in JSON response", field)
		}
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
		OutcomeCount:   5,
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

func setupTestLedgerWithMultipleSessions(t *testing.T) string {
	t.Helper()
	tmpDir := t.TempDir()

	sessions := []struct {
		id        string
		value     float64
		fwdReturn float64
	}{
		{"session-20260101-daily", 1_000_000, 0.05},
		{"session-20260102-daily", 1_050_000, 0.03},
		{"session-20260103-daily", 1_080_000, -0.01},
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
			ForwardReturn: sess.fwdReturn,
			Hit:           sess.fwdReturn > 0,
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
