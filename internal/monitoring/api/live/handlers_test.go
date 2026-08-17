package live

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	domainshared "github.com/kaecer68/atlas-go/internal/domain/shared"
	"github.com/kaecer68/atlas-go/internal/industry"
	"github.com/kaecer68/atlas-go/internal/monitoring/api/shared"
	"github.com/kaecer68/atlas-go/internal/monitoring/service"
)

func TestHandlePortfolioState_EquityCurveFields(t *testing.T) {
	tmpDir := t.TempDir()
	sessionsDir := filepath.Join(tmpDir, "sessions")

	session1 := domain.SessionSummary{
		SessionID:      "session-20260413-daily",
		PortfolioValue: 1000000.0,
		TotalTaxPaid:   5000.0,
		Regime:         domain.RegimeNeutral,
	}
	session2 := domain.SessionSummary{
		SessionID:      "session-20260414-daily",
		PortfolioValue: 1020000.0,
		TotalTaxPaid:   6000.0,
		Regime:         domain.RegimeNeutral,
	}

	summary1Path := filepath.Join(sessionsDir, "session-20260413-daily")
	if err := os.MkdirAll(summary1Path, 0o755); err != nil {
		t.Fatalf("os.MkdirAll: %v", err)
	}
	bytes1, _ := json.Marshal(session1)
	if err := os.WriteFile(filepath.Join(summary1Path, "summary.json"), bytes1, 0o644); err != nil {
		t.Fatalf("os.WriteFile: %v", err)
	}

	summary2Path := filepath.Join(sessionsDir, "session-20260414-daily")
	if err := os.MkdirAll(summary2Path, 0o755); err != nil {
		t.Fatalf("os.MkdirAll: %v", err)
	}
	bytes2, _ := json.Marshal(session2)
	if err := os.WriteFile(filepath.Join(summary2Path, "summary.json"), bytes2, 0o644); err != nil {
		t.Fatalf("os.WriteFile: %v", err)
	}

	liveStateDir := filepath.Join(tmpDir, "data", "state", "live", "state")
	if err := os.MkdirAll(liveStateDir, 0o755); err != nil {
		t.Fatalf("os.MkdirAll: %v", err)
	}
	portfolioState := map[string]any{
		"cash":           500000.0,
		"last_updated":   time.Now().Format(time.RFC3339),
		"realized_pnl":   0.0,
		"unrealized_pnl": 0.0,
	}
	psBytes, _ := json.Marshal(portfolioState)
	if err := os.WriteFile(filepath.Join(liveStateDir, "portfolio_state.json"), psBytes, 0o644); err != nil {
		t.Fatalf("os.WriteFile portfolio_state: %v", err)
	}

	svc := service.NewLiveService(tmpDir, tmpDir)

	h := &Handlers{
		LedgerDir: tmpDir,
		WorkDir:   tmpDir,
		Svc:       svc,
	}

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/portfolio-state", nil)

	adapted := shared.Get(func(r *http.Request) (int, any) {
		return h.HandlePortfolioState(r)
	})
	adapted.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var result map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("json.Unmarshal response: %v", err)
	}

	curve, ok := result["equity_curve"]
	if !ok {
		t.Fatal("equity_curve field not found in response")
	}

	if curve == nil {
		t.Fatal("equity_curve is nil")
	}

	curveSlice, ok := curve.([]any)
	if !ok {
		t.Fatalf("equity_curve is not an array, got %T", curve)
	}

	if len(curveSlice) < 2 {
		t.Fatalf("len(equity_curve) = %d, want at least 2", len(curveSlice))
	}

	point0, ok := curveSlice[0].(map[string]any)
	if !ok {
		t.Fatal("equity_curve[0] is not a map")
	}

	requiredKeys := []string{"label", "value", "currency", "after_tax_value", "tax_paid"}
	for _, key := range requiredKeys {
		if _, ok := point0[key]; !ok {
			t.Errorf("expected JSON key %q not found in equity_curve[0]", key)
		}
	}

	if point0["currency"] != "TWD" {
		t.Errorf("currency = %v, want TWD", point0["currency"])
	}
	if point0["after_tax_value"] != 995000.0 {
		t.Errorf("after_tax_value = %v, want 995000.0", point0["after_tax_value"])
	}
	if point0["tax_paid"] != 5000.0 {
		t.Errorf("tax_paid = %v, want 5000.0", point0["tax_paid"])
	}

	point1, ok := curveSlice[1].(map[string]any)
	if !ok {
		t.Fatal("equity_curve[1] is not a map")
	}
	if point1["after_tax_value"] != 1014000.0 {
		t.Errorf("point1 after_tax_value = %v, want 1014000.0", point1["after_tax_value"])
	}
	if point1["tax_paid"] != 6000.0 {
		t.Errorf("point1 tax_paid = %v, want 6000.0", point1["tax_paid"])
	}
}

func TestHandlePortfolioState_EmptySessions(t *testing.T) {
	tmpDir := t.TempDir()

	svc := service.NewLiveService(tmpDir, tmpDir)

	h := &Handlers{
		LedgerDir: tmpDir,
		WorkDir:   tmpDir,
		Svc:       svc,
	}

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/portfolio-state", nil)

	adapted := shared.Get(func(r *http.Request) (int, any) {
		return h.HandlePortfolioState(r)
	})
	adapted.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var result map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("json.Unmarshal response: %v", err)
	}

	if _, ok := result["equity_curve"]; !ok {
		t.Fatalf("response missing equity_curve field")
	}
}

func TestHandlePortfolioState_JSONSerialization(t *testing.T) {
	resp := service.PortfolioStateResponse{
		SnapshotTime:   time.Now(),
		Cash:           500000.0,
		PortfolioValue: 1500000.0,
		CumulativePnL:  50000.0,
		PositionsCount: 5,
		EquityCurve: []service.EquityCurvePoint{
			{
				Label:         "session-20260413-daily",
				Value:         1000000.0,
				Currency:      "TWD",
				AfterTaxValue: 995000.0,
				TaxPaid:       5000.0,
			},
			{
				Label:         "session-20260414-daily",
				Value:         1020000.0,
				Currency:      "TWD",
				AfterTaxValue: 1014000.0,
				TaxPaid:       6000.0,
			},
		},
	}

	bytes, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("json.Marshal error: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(bytes, &decoded); err != nil {
		t.Fatalf("json.Unmarshal error: %v", err)
	}

	if decoded["cash"] != 500000.0 {
		t.Errorf("cash = %v, want 500000.0", decoded["cash"])
	}
	if decoded["portfolio_value"] != 1500000.0 {
		t.Errorf("portfolio_value = %v, want 1500000.0", decoded["portfolio_value"])
	}

	curve, ok := decoded["equity_curve"].([]any)
	if !ok {
		t.Fatal("equity_curve is not an array")
	}

	if len(curve) != 2 {
		t.Fatalf("len(equity_curve) = %d, want 2", len(curve))
	}

	point0 := curve[0].(map[string]any)
	for _, key := range []string{"label", "value", "currency", "after_tax_value", "tax_paid"} {
		if _, exists := point0[key]; !exists {
			t.Errorf("key %q missing from equity_curve[0]", key)
		}
	}
	if point0["currency"] != "TWD" {
		t.Errorf("currency = %v, want TWD", point0["currency"])
	}
}

// ---------------------------------------------------------------------------
// HandlePnLAttribution tests
// ---------------------------------------------------------------------------

func TestHandlePnLAttribution_WithSessionsAndOutcomes(t *testing.T) {
	tmpDir := t.TempDir()
	sessionsDir := filepath.Join(tmpDir, "sessions")

	// Create two sessions
	session1 := domain.SessionSummary{
		SessionID:      "session-20260413-daily",
		PortfolioValue: 1000000.0,
		TotalTaxPaid:   5000.0,
		Regime:         domain.RegimeNeutral,
		RecordedAt:     time.Date(2026, 4, 13, 12, 0, 0, 0, time.UTC),
	}
	session2 := domain.SessionSummary{
		SessionID:      "session-20260414-daily",
		PortfolioValue: 1020000.0,
		TotalTaxPaid:   6000.0,
		Regime:         domain.RegimeNeutral,
		RecordedAt:     time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC),
	}

	for i, s := range []domain.SessionSummary{session1, session2} {
		dir := filepath.Join(sessionsDir, s.SessionID)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("os.MkdirAll: %v", err)
		}
		b, _ := json.Marshal(s)
		if err := os.WriteFile(filepath.Join(dir, "summary.json"), b, 0o644); err != nil {
			t.Fatalf("os.WriteFile: %v", err)
		}
		_ = i
	}

	// Create recommendation outcomes for latest session
	outcomePath := filepath.Join(sessionsDir, "session-20260414-daily", "recommendation_outcomes.jsonl")
	outcomes := []domain.RecommendationOutcome{
		{
			AgentID:       "agent-tech-1",
			Symbol:        "2330",
			Side:          domain.SideBuy,
			ForwardReturn: 0.05,
			PassedGuards:  true,
			FactorScores: domainshared.FactorScores{
				Momentum: 0.6,
				Value:    0.4,
				Quality:  0.7,
				Agent:    0.8,
				Total:    0.6,
			},
		},
		{
			AgentID:       "agent-tech-1",
			Symbol:        "2311",
			Side:          domain.SideBuy,
			ForwardReturn: 0.03,
			PassedGuards:  true,
			FactorScores: domainshared.FactorScores{
				Momentum: 0.5,
				Value:    0.3,
				Quality:  0.6,
				Agent:    0.7,
				Total:    0.5,
			},
		},
		{
			AgentID:       "agent-finance-1",
			Symbol:        "2881",
			Side:          domain.SideBuy,
			ForwardReturn: 0.02,
			PassedGuards:  true,
			FactorScores: domainshared.FactorScores{
				Momentum: 0.4,
				Value:    0.5,
				Quality:  0.6,
				Agent:    0.5,
				Total:    0.5,
			},
		},
	}
	var jsonlLines []string
	for _, oc := range outcomes {
		b, _ := json.Marshal(oc)
		jsonlLines = append(jsonlLines, string(b))
	}
	if err := os.WriteFile(outcomePath, []byte(strings.Join(jsonlLines, "\n")), 0o644); err != nil {
		t.Fatalf("os.WriteFile outcomes: %v", err)
	}

	h := &Handlers{
		LedgerDir: tmpDir,
		WorkDir:   tmpDir,
	}

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/pnl-attribution", nil)

	adapted := shared.Get(func(r *http.Request) (int, any) {
		return h.HandlePnLAttribution(r)
	})
	adapted.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var result map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("json.Unmarshal response: %v", err)
	}

	if result["session_id"] != "session-20260414-daily" {
		t.Errorf("session_id = %v, want session-20260414-daily", result["session_id"])
	}

	if result["cumulative_pnl"] == nil {
		t.Error("cumulative_pnl should not be nil")
	}
}

func TestHandlePnLAttribution_EmptySessions(t *testing.T) {
	tmpDir := t.TempDir()
	sessionsDir := filepath.Join(tmpDir, "sessions")
	if err := os.MkdirAll(sessionsDir, 0o755); err != nil {
		t.Fatalf("os.MkdirAll: %v", err)
	}

	h := &Handlers{
		LedgerDir: tmpDir,
		WorkDir:   tmpDir,
	}

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/pnl-attribution", nil)

	adapted := shared.Get(func(r *http.Request) (int, any) {
		return h.HandlePnLAttribution(r)
	})
	adapted.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var result map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("json.Unmarshal response: %v", err)
	}

	// Empty response should have zero values
	if result["session_id"] != "" {
		t.Errorf("session_id = %v, want empty string", result["session_id"])
	}
}

func TestHandlePnLAttribution_MethodNotAllowed(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("ATLAS_API_KEY", "test-key")
	h := &Handlers{
		LedgerDir: tmpDir,
		WorkDir:   tmpDir,
	}

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/dashboard/pnl-attribution", nil)
	req.Header.Set("X-API-Key", "test-key")

	adapted := shared.Get(func(r *http.Request) (int, any) {
		return h.HandlePnLAttribution(r)
	})
	adapted.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

// ---------------------------------------------------------------------------
// HandleRiskExposure tests
// ---------------------------------------------------------------------------

func TestHandleRiskExposure_WithSessionsAndPositions(t *testing.T) {
	tmpDir := t.TempDir()
	sessionsDir := filepath.Join(tmpDir, "sessions")

	// Create sessions with portfolio values
	for i := 0; i < 35; i++ {
		sessionID := fmt.Sprintf("session-202604%02d-daily", i+1)
		dir := filepath.Join(sessionsDir, sessionID)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("os.MkdirAll: %v", err)
		}
		summary := domain.SessionSummary{
			SessionID:      sessionID,
			PortfolioValue: 1000000.0 + float64(i)*10000.0,
			Regime:         domain.RegimeNeutral,
			RecordedAt:     time.Date(2026, 4, i+1, 12, 0, 0, 0, time.UTC),
		}
		b, _ := json.Marshal(summary)
		if err := os.WriteFile(filepath.Join(dir, "summary.json"), b, 0o644); err != nil {
			t.Fatalf("os.WriteFile: %v", err)
		}
	}

	// Create live state directory
	liveBasePath := filepath.Join(tmpDir, "data", "state", "live", "state")
	if err := os.MkdirAll(liveBasePath, 0o755); err != nil {
		t.Fatalf("os.MkdirAll: %v", err)
	}

	// Create portfolio state
	portfolioState := map[string]any{
		"cash":           500000.0,
		"total_exposure": 500000.0,
		"available_cash": 400000.0,
		"day_pnl":        10000.0,
		"unrealized_pnl": 20000.0,
	}
	psBytes, _ := json.Marshal(portfolioState)
	if err := os.WriteFile(filepath.Join(liveBasePath, "portfolio_state.json"), psBytes, 0o644); err != nil {
		t.Fatalf("os.WriteFile portfolio_state: %v", err)
	}

	// Create positions
	positions := []map[string]any{
		{"symbol": "2330", "shares": 1000, "market_value": 900000.0, "avg_cost": 850000.0},
		{"symbol": "2311", "shares": 500, "market_value": 250000.0, "avg_cost": 240000.0},
	}
	posBytes, _ := json.Marshal(positions)
	if err := os.WriteFile(filepath.Join(liveBasePath, "positions.json"), posBytes, 0o644); err != nil {
		t.Fatalf("os.WriteFile positions: %v", err)
	}

	h := &Handlers{
		LedgerDir: tmpDir,
		WorkDir:   tmpDir,
	}

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/risk-exposure", nil)

	adapted := shared.Get(func(r *http.Request) (int, any) {
		return h.HandleRiskExposure(r)
	})
	adapted.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var result map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("json.Unmarshal response: %v", err)
	}

	if result["portfolio_value"] == nil {
		t.Error("portfolio_value should not be nil")
	}
	if result["position_count"] == nil {
		t.Error("position_count should not be nil")
	}
}

func TestHandleRiskExposure_InsufficientData(t *testing.T) {
	tmpDir := t.TempDir()
	sessionsDir := filepath.Join(tmpDir, "sessions")

	// Create only a few sessions (less than 30 needed for VaR)
	for i := 0; i < 5; i++ {
		sessionID := fmt.Sprintf("session-202604%02d-daily", i+1)
		dir := filepath.Join(sessionsDir, sessionID)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("os.MkdirAll: %v", err)
		}
		summary := domain.SessionSummary{
			SessionID:      sessionID,
			PortfolioValue: 1000000.0 + float64(i)*10000.0,
			Regime:         domain.RegimeNeutral,
		}
		b, _ := json.Marshal(summary)
		if err := os.WriteFile(filepath.Join(dir, "summary.json"), b, 0o644); err != nil {
			t.Fatalf("os.WriteFile: %v", err)
		}
	}

	h := &Handlers{
		LedgerDir: tmpDir,
		WorkDir:   tmpDir,
	}

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/risk-exposure", nil)

	adapted := shared.Get(func(r *http.Request) (int, any) {
		return h.HandleRiskExposure(r)
	})
	adapted.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var result map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("json.Unmarshal response: %v", err)
	}

	// Should flag insufficient data
	if result["insufficient_data"] != true {
		t.Errorf("insufficient_data = %v, want true", result["insufficient_data"])
	}
}

func TestHandleRiskExposure_MethodNotAllowed(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("ATLAS_API_KEY", "test-key")
	h := &Handlers{
		LedgerDir: tmpDir,
		WorkDir:   tmpDir,
	}

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/dashboard/risk-exposure", nil)
	req.Header.Set("X-API-Key", "test-key")

	adapted := shared.Get(func(r *http.Request) (int, any) {
		return h.HandleRiskExposure(r)
	})
	adapted.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

// ---------------------------------------------------------------------------
// HandleLiveStatus tests
// ---------------------------------------------------------------------------

func TestHandleLiveStatus_WithCircuitBreaker(t *testing.T) {
	tmpDir := t.TempDir()
	liveBasePath := filepath.Join(tmpDir, "data", "state", "live", "state")
	if err := os.MkdirAll(liveBasePath, 0o755); err != nil {
		t.Fatalf("os.MkdirAll: %v", err)
	}

	cbDir := filepath.Join(tmpDir, "data", "state")
	if err := os.MkdirAll(cbDir, 0o755); err != nil {
		t.Fatalf("os.MkdirAll: %v", err)
	}
	cbState := map[string]any{
		"state":            "normal",
		"state_changed_at": time.Now().Format(time.RFC3339),
		"consecutive_sl":   0,
		"cooldown_until":   time.Now().Add(10 * time.Minute).Format(time.RFC3339),
		"intraday_peak":    1050000.0,
		"day_start_value":  1000000.0,
	}
	cbBytes, _ := json.Marshal(cbState)
	if err := os.WriteFile(filepath.Join(cbDir, "circuit_breaker_state.json"), cbBytes, 0o644); err != nil {
		t.Fatalf("os.WriteFile circuit_breaker: %v", err)
	}

	// Create portfolio state
	portfolioState := map[string]any{
		"cash":           500000.0,
		"total_exposure": 500000.0,
		"available_cash": 400000.0,
		"day_pnl":        10000.0,
		"unrealized_pnl": 20000.0,
	}
	psBytes, _ := json.Marshal(portfolioState)
	if err := os.WriteFile(filepath.Join(liveBasePath, "portfolio_state.json"), psBytes, 0o644); err != nil {
		t.Fatalf("os.WriteFile portfolio_state: %v", err)
	}

	svc := service.NewLiveService(tmpDir, tmpDir)
	h := &Handlers{
		LedgerDir: tmpDir,
		WorkDir:   tmpDir,
		Svc:       svc,
	}

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/live-status", nil)

	adapted := shared.Get(func(r *http.Request) (int, any) {
		return h.HandleLiveStatus(r)
	})
	adapted.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var result map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("json.Unmarshal response: %v", err)
	}

	cb, ok := result["circuit_breaker"].(map[string]any)
	if !ok {
		t.Fatal("circuit_breaker should be a map")
	}
	if cb["state"] != "normal" {
		t.Errorf("circuit_breaker.state = %v, want normal", cb["state"])
	}
}

func TestHandleLiveStatus_MethodNotAllowed(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("ATLAS_API_KEY", "test-key")
	svc := service.NewLiveService(tmpDir, tmpDir)
	h := &Handlers{
		LedgerDir: tmpDir,
		WorkDir:   tmpDir,
		Svc:       svc,
	}

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/dashboard/live-status", nil)
	req.Header.Set("X-API-Key", "test-key")

	adapted := shared.Get(func(r *http.Request) (int, any) {
		return h.HandleLiveStatus(r)
	})
	adapted.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

// ---------------------------------------------------------------------------
// HandleTradeHistory tests
// ---------------------------------------------------------------------------

func TestHandleTradeHistory_WithTrades(t *testing.T) {
	tmpDir := t.TempDir()
	sessionsDir := filepath.Join(tmpDir, "sessions")
	sessionDir := filepath.Join(sessionsDir, "session-20260414-daily")
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatalf("os.MkdirAll: %v", err)
	}

	// Create trade history as JSONL (one JSON object per line)
	trades := []domain.TradeRecord{
		{Symbol: "2330", Side: domain.SideBuy, Quantity: 1000, Price: 900.0, Amount: 900000.0, Timestamp: time.Now()},
		{Symbol: "2311", Side: domain.SideBuy, Quantity: 500, Price: 500.0, Amount: 250000.0, Timestamp: time.Now()},
	}
	var lines []string
	for _, tr := range trades {
		b, _ := json.Marshal(tr)
		lines = append(lines, string(b))
	}
	if err := os.WriteFile(filepath.Join(sessionDir, "trades.jsonl"), []byte(strings.Join(lines, "\n")), 0o644); err != nil {
		t.Fatalf("os.WriteFile trades.jsonl: %v", err)
	}

	svc := service.NewLiveService(tmpDir, tmpDir)
	h := &Handlers{
		LedgerDir: tmpDir,
		WorkDir:   tmpDir,
		Svc:       svc,
	}

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/trade-history", nil)

	adapted := shared.Get(func(r *http.Request) (int, any) {
		return h.HandleTradeHistory(r)
	})
	adapted.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var result []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("json.Unmarshal response: %v", err)
	}

	if len(result) != 2 {
		t.Errorf("len(trades) = %d, want 2", len(result))
	}
}

func TestHandleTradeHistory_EmptyTrades(t *testing.T) {
	tmpDir := t.TempDir()
	svc := service.NewLiveService(tmpDir, tmpDir)
	h := &Handlers{
		LedgerDir: tmpDir,
		WorkDir:   tmpDir,
		Svc:       svc,
	}

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/trade-history", nil)

	adapted := shared.Get(func(r *http.Request) (int, any) {
		return h.HandleTradeHistory(r)
	})
	adapted.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var result []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("json.Unmarshal response: %v", err)
	}

	if len(result) != 0 {
		t.Errorf("len(trades) = %d, want 0", len(result))
	}
}

func TestHandleTradeHistory_MethodNotAllowed(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("ATLAS_API_KEY", "test-key")
	t.Setenv("ATLAS_API_KEY", "test-key")
	svc := service.NewLiveService(tmpDir, tmpDir)
	h := &Handlers{
		LedgerDir: tmpDir,
		WorkDir:   tmpDir,
		Svc:       svc,
	}

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/dashboard/trade-history", nil)
	req.Header.Set("X-API-Key", "test-key")

	adapted := shared.Get(func(r *http.Request) (int, any) {
		return h.HandleTradeHistory(r)
	})
	adapted.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

// ---------------------------------------------------------------------------
// Helper function tests
// ---------------------------------------------------------------------------

func TestSectorLabel_WithClassifier(t *testing.T) {
	tree := industry.NewClassificationTree()
	seg := &industry.IndustrySegment{ID: "tech", Name: "科技", RepresentativeStocks: []string{"2330", "2311"}}
	tree.AddSegment(seg)

	h := &Handlers{
		Classifier: tree,
	}

	if got := h.sectorLabel("tech"); got != "科技" {
		t.Errorf("sectorLabel(tech) = %q, want %q", got, "科技")
	}
	if got := h.sectorLabel("unknown"); got != "unknown" {
		t.Errorf("sectorLabel(unknown) = %q, want %q", got, "unknown")
	}
}

func TestSectorLabel_WithoutClassifier(t *testing.T) {
	h := &Handlers{}

	if got := h.sectorLabel("other"); got != "其他" {
		t.Errorf("sectorLabel(other) = %q, want %q", got, "其他")
	}
	if got := h.sectorLabel("unknown"); got != "unknown" {
		t.Errorf("sectorLabel(unknown) = %q, want %q", got, "unknown")
	}
}

func TestAgentLayer_WithMap(t *testing.T) {
	h := &Handlers{
		AgentLayerMap: map[string]string{
			"agent-tech-1": "sector",
			"agent-bench":  "baseline",
		},
	}

	if got := h.agentLayer("agent-tech-1"); got != "sector" {
		t.Errorf("agentLayer(agent-tech-1) = %q, want %q", got, "sector")
	}
	if got := h.agentLayer("unknown"); got != "" {
		t.Errorf("agentLayer(unknown) = %q, want empty", got)
	}
}

func TestAgentLayer_WithoutMap(t *testing.T) {
	h := &Handlers{}

	if got := h.agentLayer("any-agent"); got != "" {
		t.Errorf("agentLayer(any-agent) = %q, want empty", got)
	}
}

func TestBuildAgentLayerMap_ValidConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "configs")
	if err := os.MkdirAll(configPath, 0o755); err != nil {
		t.Fatalf("os.MkdirAll: %v", err)
	}

	config := map[string]any{
		"agents": []map[string]any{
			{"id": "agent-tech-1", "layer": "sector", "name": "Tech Agent 1"},
			{"id": "agent-finance-1", "layer": "sector", "name": "Finance Agent 1"},
			{"id": "agent-bench", "layer": "baseline", "name": "Baseline"},
		},
	}
	configBytes, _ := json.Marshal(config)
	if err := os.WriteFile(filepath.Join(configPath, "agents.json"), configBytes, 0o644); err != nil {
		t.Fatalf("os.WriteFile agents.json: %v", err)
	}

	m := BuildAgentLayerMap(tmpDir)

	if got := m["agent-tech-1"]; got != "sector" {
		t.Errorf("agent-tech-1 layer = %q, want %q", got, "sector")
	}
	if got := m["agent-finance-1"]; got != "sector" {
		t.Errorf("agent-finance-1 layer = %q, want %q", got, "sector")
	}
	if got := m["agent-bench"]; got != "baseline" {
		t.Errorf("agent-bench layer = %q, want %q", got, "baseline")
	}
}

func TestBuildAgentLayerMap_NoConfig(t *testing.T) {
	tmpDir := t.TempDir()
	m := BuildAgentLayerMap(tmpDir)

	if len(m) != 0 {
		t.Errorf("len(m) = %d, want 0", len(m))
	}
}

func TestGetSymbolSector(t *testing.T) {
	symMap := map[string]string{
		"2330": "tech",
		"2311": "tech",
		"2881": "finance",
	}

	if got := getSymbolSector("2330", symMap); got != "tech" {
		t.Errorf("getSymbolSector(2330) = %q, want %q", got, "tech")
	}
	if got := getSymbolSector("unknown", symMap); got != "other" {
		t.Errorf("getSymbolSector(unknown) = %q, want %q", got, "other")
	}
}

func TestBuildSymbolSectorMap_WithClassifier(t *testing.T) {
	tree := industry.NewClassificationTree()
	seg := &industry.IndustrySegment{
		ID:                   "tech",
		Name:                 "科技",
		RepresentativeStocks: []string{"2330", "2311", "2454"},
	}
	tree.AddSegment(seg)

	m := buildSymbolSectorMap(tree)

	if got := m["2330"]; got != "tech" {
		t.Errorf("m[2330] = %q, want %q", got, "tech")
	}
	if got := m["2454"]; got != "tech" {
		t.Errorf("m[2454] = %q, want %q", got, "tech")
	}
	if got := m["unknown"]; got != "" {
		t.Errorf("m[unknown] = %q, want empty", got)
	}
}

func TestBuildSymbolSectorMap_WithoutClassifier(t *testing.T) {
	m := buildSymbolSectorMap(nil)

	if len(m) != 0 {
		t.Errorf("len(m) = %d, want 0", len(m))
	}
}

func TestComputeSectorFactorExposure(t *testing.T) {
	h := &Handlers{}

	outcomes := []domain.RecommendationOutcome{
		{
			AgentID:       "agent-tech-1",
			Symbol:        "2330",
			Side:          domain.SideBuy,
			ForwardReturn: 0.05,
			PassedGuards:  true,
			FactorScores: domainshared.FactorScores{
				Momentum: 0.6,
				Value:    0.4,
				Quality:  0.7,
				Agent:    0.8,
				Total:    0.6,
			},
		},
		{
			AgentID:       "agent-tech-1",
			Symbol:        "2311",
			Side:          domain.SideBuy,
			ForwardReturn: 0.03,
			PassedGuards:  true,
			FactorScores: domainshared.FactorScores{
				Momentum: 0.5,
				Value:    0.3,
				Quality:  0.6,
				Agent:    0.7,
				Total:    0.5,
			},
		},
	}

	symSectorMap := map[string]string{
		"2330": "tech",
		"2311": "tech",
	}

	sectorExp, factorExp := h.computeSectorFactorExposure(outcomes, 1000000.0, symSectorMap)

	if len(sectorExp) != 1 {
		t.Errorf("len(sectorExp) = %d, want 1", len(sectorExp))
	}
	if sectorExp[0].Sector != "tech" {
		t.Errorf("sectorExp[0].Sector = %q, want %q", sectorExp[0].Sector, "tech")
	}
	if factorExp.Momentum == 0 {
		t.Error("factorExp.Momentum should not be 0")
	}
}

func TestComputeSectorFactorExposure_EmptyOutcomes(t *testing.T) {
	h := &Handlers{}

	sectorExp, factorExp := h.computeSectorFactorExposure(nil, 1000000.0, nil)

	if len(sectorExp) != 0 {
		t.Errorf("len(sectorExp) = %d, want 0", len(sectorExp))
	}
	if factorExp.Momentum != 0 {
		t.Error("factorExp.Momentum should be 0 for empty outcomes")
	}
}

func TestLoadRecommendationOutcomes_WithSession(t *testing.T) {
	tmpDir := t.TempDir()
	sessionsDir := filepath.Join(tmpDir, "sessions")
	sessionDir := filepath.Join(sessionsDir, "session-20260414-daily")
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatalf("os.MkdirAll: %v", err)
	}

	outcomes := []domain.RecommendationOutcome{
		{
			AgentID:       "agent-tech-1",
			Symbol:        "2330",
			ForwardReturn: 0.05,
			PassedGuards:  true,
			FactorScores: domainshared.FactorScores{
				Momentum: 0.6,
				Value:    0.4,
				Quality:  0.7,
				Agent:    0.8,
				Total:    0.6,
			},
		},
	}

	var lines []string
	for _, oc := range outcomes {
		b, _ := json.Marshal(oc)
		lines = append(lines, string(b))
	}
	if err := os.WriteFile(filepath.Join(sessionDir, "recommendation_outcomes.jsonl"), []byte(strings.Join(lines, "\n")), 0o644); err != nil {
		t.Fatalf("os.WriteFile: %v", err)
	}

	got, err := loadRecommendationOutcomes(tmpDir, "session-20260414-daily")
	if err != nil {
		t.Fatalf("loadRecommendationOutcomes: %v", err)
	}

	if len(got) != 1 {
		t.Errorf("len(got) = %d, want 1", len(got))
	}
	if got[0].Symbol != "2330" {
		t.Errorf("got[0].Symbol = %q, want %q", got[0].Symbol, "2330")
	}
}

func TestLoadRecommendationOutcomes_LatestSession(t *testing.T) {
	tmpDir := t.TempDir()
	sessionsDir := filepath.Join(tmpDir, "sessions")

	// Create two sessions
	for i, sessionID := range []string{"session-20260413-daily", "session-20260414-daily"} {
		dir := filepath.Join(sessionsDir, sessionID)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("os.MkdirAll: %v", err)
		}
		summary := domain.SessionSummary{SessionID: sessionID, PortfolioValue: 1000000.0 + float64(i)*10000}
		b, _ := json.Marshal(summary)
		if err := os.WriteFile(filepath.Join(dir, "summary.json"), b, 0o644); err != nil {
			t.Fatalf("os.WriteFile: %v", err)
		}

		if sessionID == "session-20260414-daily" {
			outcome := domain.RecommendationOutcome{
				AgentID:       "agent-tech-1",
				Symbol:        "2330",
				ForwardReturn: 0.05,
				PassedGuards:  true,
			}
			b, _ := json.Marshal(outcome)
			if err := os.WriteFile(filepath.Join(dir, "recommendation_outcomes.jsonl"), b, 0o644); err != nil {
				t.Fatalf("os.WriteFile: %v", err)
			}
		}
	}

	got, err := loadRecommendationOutcomes(tmpDir, "")
	if err != nil {
		t.Fatalf("loadRecommendationOutcomes: %v", err)
	}

	// Should load from latest session (session-20260414-daily)
	if len(got) != 1 {
		t.Errorf("len(got) = %d, want 1", len(got))
	}
}

func TestLoadRecommendationOutcomes_InvalidSessionID(t *testing.T) {
	tmpDir := t.TempDir()

	_, err := loadRecommendationOutcomes(tmpDir, "session/invalid")
	if err == nil {
		t.Error("expected error for invalid session ID")
	}
}

func TestPnLAttributionResponse_JSONSerialization(t *testing.T) {
	resp := PnLAttributionResponse{
		SnapshotTime:     time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC),
		SessionID:        "session-20260414-daily",
		StartingValue:    1000000.0,
		CurrentValue:     1020000.0,
		CumulativePnL:    20000.0,
		CumulativeRetPct: 0.02,
		AgentAttribution: []AgentAttribution{
			{AgentID: "agent-tech-1", AgentName: "Tech Agent 1", Layer: "sector", TotalReturn: 0.05, Count: 2, AvgReturn: 0.025},
		},
		SectorAttribution: []SectorAttribution{
			{Sector: "tech", SectorLabel: "科技", TotalReturn: 0.05, Count: 2, AvgReturn: 0.025},
		},
		FactorAttribution: FactorAttribution{
			Momentum: FactorDetail{AvgScore: 0.55, AvgReturn: 0.02, Contribution: 0.011},
			Value:    FactorDetail{AvgScore: 0.35, AvgReturn: 0.02, Contribution: 0.007},
			Quality:  FactorDetail{AvgScore: 0.65, AvgReturn: 0.02, Contribution: 0.013},
			Agent:    FactorDetail{AvgScore: 0.75, AvgReturn: 0.02, Contribution: 0.015},
			Total:    FactorDetail{AvgScore: 0.575, AvgReturn: 0.02, Contribution: 0.0115},
		},
		SymbolAttribution: []SymbolAttribution{
			{Symbol: "2330", TotalReturn: 0.05, Count: 1, AvgReturn: 0.05, Side: "long"},
		},
	}

	bytes, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("json.Marshal error: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(bytes, &decoded); err != nil {
		t.Fatalf("json.Unmarshal error: %v", err)
	}

	if decoded["session_id"] != "session-20260414-daily" {
		t.Errorf("session_id = %v, want session-20260414-daily", decoded["session_id"])
	}
	if decoded["cumulative_pnl"] != 20000.0 {
		t.Errorf("cumulative_pnl = %v, want 20000.0", decoded["cumulative_pnl"])
	}

	agentAttr, ok := decoded["agent_attribution"].([]any)
	if !ok {
		t.Fatal("agent_attribution should be an array")
	}
	if len(agentAttr) != 1 {
		t.Errorf("len(agent_attribution) = %d, want 1", len(agentAttr))
	}
}

func TestRiskExposureResponse_JSONSerialization(t *testing.T) {
	resp := RiskExposureResponse{
		SnapshotTime:   time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC),
		VaR95:          0.02,
		VaR99:          0.03,
		CVaR95:         0.025,
		MaxDrawdownPct: 0.05,
		PortfolioValue: 1000000.0,
		CashRatio:      0.4,
		PositionCount:  5,
		SectorExposure: []SectorExposure{
			{Sector: "tech", SectorLabel: "科技", Weight: 0.6, EstValue: 600000.0},
		},
		FactorExposure: FactorExposureInline{
			Momentum: 0.55,
			Value:    0.35,
			Quality:  0.65,
			Agent:    0.75,
			Total:    0.575,
		},
		Concentration: []PositionConcentration{
			{Symbol: "2330", MarketValue: 500000.0, Weight: 0.5},
		},
		DataPoints:       35,
		InsufficientData: false,
	}

	bytes, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("json.Marshal error: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(bytes, &decoded); err != nil {
		t.Fatalf("json.Unmarshal error: %v", err)
	}

	if decoded["var_95"] != 0.02 {
		t.Errorf("var_95 = %v, want 0.02", decoded["var_95"])
	}
	if decoded["insufficient_data"] != false {
		t.Errorf("insufficient_data = %v, want false", decoded["insufficient_data"])
	}

	sectorExp, ok := decoded["sector_exposure"].([]any)
	if !ok {
		t.Fatal("sector_exposure should be an array")
	}
	if len(sectorExp) != 1 {
		t.Errorf("len(sector_exposure) = %d, want 1", len(sectorExp))
	}
}
