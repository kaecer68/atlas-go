package monitoring

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/ledger"
)

// seedLedgerForSignals writes a JSONL ledger that trips the backtest
// CIRCUIT_BREAKER: 25 sessions of monotonically declining portfolio value
// (drawdown > 15%) plus losing outcomes from 20 agents.
func seedLedgerForSignals(t *testing.T, dir string) {
	t.Helper()
	store := ledger.NewStore(dir).(*ledger.Store)

	var outcomes []domain.RecommendationOutcome
	for i := range 25 {
		session := domain.ReplaySession{ID: fmt.Sprintf("session-seam-%02d", i)}
		summary := domain.SessionSummary{
			SessionID:      session.ID,
			Regime:         domain.RegimeRiskOff,
			PortfolioValue: 100000.0 - float64(i)*2500.0,
			EndingCash:     50000.0,
		}
		if err := store.RecordSessionSummary(session, summary); err != nil {
			t.Fatalf("RecordSessionSummary[%d]: %v", i, err)
		}
		var outs []domain.RecommendationOutcome
		for range 3 {
			out := domain.RecommendationOutcome{
				AgentID:       fmt.Sprintf("agent-seam-%02d", i),
				Symbol:        "2330",
				Side:          domain.SideBuy,
				Layer:         domain.LayerSector,
				Conviction:    1,
				Window:        "1d",
				ForwardReturn: -0.06,
				Hit:           false,
			}
			outs = append(outs, out)
			outcomes = append(outcomes, out)
		}
		if err := store.RecordSessionOutcomes(session, outs); err != nil {
			t.Fatalf("RecordSessionOutcomes[%d]: %v", i, err)
		}
	}
	if err := store.RecordOutcomes(outcomes); err != nil {
		t.Fatalf("RecordOutcomes: %v", err)
	}
}

// backtestSignalsBody serves GET /api/backtest/signals from the routes
// RegisterBacktestRoutes installs and returns the decoded body.
func backtestSignalsBody(t *testing.T, d *DashboardAPI) (int, map[string]any) {
	t.Helper()
	mux := http.NewServeMux()
	d.RegisterBacktestRoutes(mux)
	req := httptest.NewRequest(http.MethodGet, "/api/backtest/signals", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	var body map[string]any
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode signals response (status %d): %v", w.Code, err)
	}
	return w.Code, body
}

// TestRegisterBacktestRoutes_SignalEndpointReadsConfiguredBackend is the
// end-to-end half of the T1 proof: the route must read the store the
// configured backend resolves to, so real data reaches the response instead of
// the all-zero body production served from an unpopulated JSONL directory.
func TestRegisterBacktestRoutes_SignalEndpointReadsConfiguredBackend(t *testing.T) {
	dir := t.TempDir()
	seedLedgerForSignals(t, dir)
	t.Setenv("ATLAS_LEDGER_DIR", dir)
	t.Setenv("ATLAS_STORE_BACKEND", "jsonl")

	d := NewDashboardAPIWithGateway(".", dir, nil, NoopFetcher())
	status, body := backtestSignalsBody(t, d)

	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %v)", status, body)
	}
	raw, _ := body["active_signals"].([]any)
	signals := make([]string, 0, len(raw))
	for _, s := range raw {
		str, _ := s.(string)
		signals = append(signals, str)
	}
	if !slices.Contains(signals, "CIRCUIT_BREAKER") {
		t.Errorf("active_signals = %v, want CIRCUIT_BREAKER from the configured ledger", signals)
	}
	drawdown, _ := body["drawdown_pct"].(float64)
	if drawdown <= 0.15 {
		t.Errorf("drawdown_pct = %v, want > 0.15", body["drawdown_pct"])
	}
}

// TestRegisterBacktestRoutes_SignalEndpointFailsLoudWhenStoreUnbuildable pins
// the wiring failure path: when the backend-aware store cannot be built the
// endpoint answers 500 with an error, instead of silently reading JSONL and
// reporting a risk-free portfolio (the 2026-10 production defect).
func TestRegisterBacktestRoutes_SignalEndpointFailsLoudWhenStoreUnbuildable(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ATLAS_LEDGER_DIR", dir)
	t.Setenv("ATLAS_STORE_BACKEND", "mysql") // unknown backend ⇒ NewFullStore errors

	d := NewDashboardAPIWithGateway(".", dir, nil, NoopFetcher())
	status, body := backtestSignalsBody(t, d)

	if status != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500 when the store cannot be built (body %v)", status, body)
	}
	msg, _ := body["error"].(string)
	if !strings.Contains(msg, "store unavailable") {
		t.Errorf("error = %q, want it to name the store wiring failure", msg)
	}
}
