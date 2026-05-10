package live

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
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
	if err := os.MkdirAll(summary1Path, 0755); err != nil {
		t.Fatalf("os.MkdirAll: %v", err)
	}
	bytes1, _ := json.Marshal(session1)
	if err := os.WriteFile(filepath.Join(summary1Path, "summary.json"), bytes1, 0644); err != nil {
		t.Fatalf("os.WriteFile: %v", err)
	}

	summary2Path := filepath.Join(sessionsDir, "session-20260414-daily")
	if err := os.MkdirAll(summary2Path, 0755); err != nil {
		t.Fatalf("os.MkdirAll: %v", err)
	}
	bytes2, _ := json.Marshal(session2)
	if err := os.WriteFile(filepath.Join(summary2Path, "summary.json"), bytes2, 0644); err != nil {
		t.Fatalf("os.WriteFile: %v", err)
	}

	liveStateDir := filepath.Join(tmpDir, "data", "state", "live", "state")
	if err := os.MkdirAll(liveStateDir, 0755); err != nil {
		t.Fatalf("os.MkdirAll: %v", err)
	}
	portfolioState := map[string]any{
		"cash":           500000.0,
		"last_updated":   time.Now().Format(time.RFC3339),
		"realized_pnl":   0.0,
		"unrealized_pnl": 0.0,
	}
	psBytes, _ := json.Marshal(portfolioState)
	if err := os.WriteFile(filepath.Join(liveStateDir, "portfolio_state.json"), psBytes, 0644); err != nil {
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
