package backtest

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/ledger"
)

// fakeSignalStore is an in-memory ledger.FullStore for the signals endpoint.
// It embeds a real JSONL *ledger.Store (built on an empty directory, so any
// read that bypasses the injected rows returns nothing) and overrides the two
// reads SignalEngine performs.
type fakeSignalStore struct {
	*ledger.Store
	outcomes  []domain.RecommendationOutcome
	summaries []domain.SessionSummary
}

func newFakeSignalStore(t *testing.T) *fakeSignalStore {
	t.Helper()
	return &fakeSignalStore{Store: ledger.NewStore(t.TempDir()).(*ledger.Store)}
}

func (f *fakeSignalStore) LoadOutcomes() ([]domain.RecommendationOutcome, error) {
	return f.outcomes, nil
}

func (f *fakeSignalStore) LoadOutcomesFromSessions() ([]domain.RecommendationOutcome, error) {
	return f.outcomes, nil
}

func (f *fakeSignalStore) LoadSessionSummaries() ([]domain.SessionSummary, error) {
	return f.summaries, nil
}

var _ ledger.FullStore = (*fakeSignalStore)(nil)

func (f *fakeSignalStore) seedBreakerData() {
	for i := range 25 {
		f.summaries = append(f.summaries, domain.SessionSummary{
			SessionID:      "sess-signal-" + string(rune('a'+i)),
			Regime:         domain.RegimeRiskOff,
			PortfolioValue: 100000.0 - float64(i)*2000.0,
			EndingCash:     50000.0,
		})
	}
	for i := range 20 {
		for j := range 3 {
			f.outcomes = append(f.outcomes, domain.RecommendationOutcome{
				AgentID:       "agent-signal-" + string(rune('a'+i)),
				Symbol:        "2330",
				Side:          domain.SideBuy,
				Layer:         domain.LayerSector,
				Conviction:    1,
				Window:        "1d",
				ForwardReturn: -0.06 + 0.001*float64(j),
				Hit:           false,
			})
		}
	}
}

// TestHandleBacktestSignals_InjectedStoreReturnsRealSignals is the endpoint
// half of the T1 proof: when the wiring injects the backend-aware store, the
// response carries the real metrics and the CIRCUIT_BREAKER signal instead of
// the all-zero body production served.
func TestHandleBacktestSignals_InjectedStoreReturnsRealSignals(t *testing.T) {
	store := newFakeSignalStore(t)
	store.seedBreakerData()

	h := newTestHandlers(t)
	h.WithStore(store)

	status, body := h.HandleBacktestSignals(getRequest(t, "/api/backtest/signals"))
	assertStatus(t, status, http.StatusOK)
	m := assertJSONKey(t, body, "active_signals")

	signals, _ := m["active_signals"].([]any)
	found := false
	for _, s := range signals {
		if s == "CIRCUIT_BREAKER" {
			found = true
		}
	}
	if !found {
		t.Errorf("active_signals = %v, want CIRCUIT_BREAKER from the injected store", signals)
	}
	drawdown, _ := m["drawdown_pct"].(float64)
	if drawdown <= 0.15 {
		t.Errorf("drawdown_pct = %v, want > 0.15", m["drawdown_pct"])
	}
}

// TestHandleBacktestSignals_LegacyEmptyDirIsAllZeroNoError pins the exact
// production symptom this PR removes: without an injected store the handler
// reads the JSONL directory, and an empty directory yields 200 + all-zero
// metrics with no error — indistinguishable from a healthy portfolio.
func TestHandleBacktestSignals_LegacyEmptyDirIsAllZeroNoError(t *testing.T) {
	h := newTestHandlers(t)

	status, body := h.HandleBacktestSignals(getRequest(t, "/api/backtest/signals"))
	assertStatus(t, status, http.StatusOK)
	m := assertJSONKey(t, body, "active_signals")

	if signals, _ := m["active_signals"].([]any); len(signals) != 0 {
		t.Errorf("active_signals = %v, want empty for a missing JSONL ledger", signals)
	}
	for _, key := range []string{"var_95", "var_99", "sharpe_short", "sharpe_long", "drawdown_pct"} {
		if got, _ := m[key].(float64); got != 0 {
			t.Errorf("%s = %v, want 0 (the defect this PR fixes)", key, m[key])
		}
	}
}

// TestHandleBacktestSignals_StoreUnavailableFailsLoud verifies the wiring
// failure path: an unbuildable backend-aware store must surface as 500, never
// as a silent JSONL fallback serving zeros.
func TestHandleBacktestSignals_StoreUnavailableFailsLoud(t *testing.T) {
	h := newTestHandlers(t)
	h.WithStoreError(errors.New("fullstore: postgres backend requires SetPostgresPool before NewFullStore"))

	status, body := h.HandleBacktestSignals(getRequest(t, "/api/backtest/signals"))
	assertStatus(t, status, http.StatusInternalServerError)

	m := assertJSONKey(t, body, "error")
	msg, _ := m["error"].(string)
	if !strings.Contains(msg, "store unavailable") {
		t.Errorf("error = %q, want it to name the store wiring failure", msg)
	}
}
