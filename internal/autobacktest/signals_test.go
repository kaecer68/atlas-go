package autobacktest

import (
	"slices"
	"testing"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/ledger"
)

func TestSignalString(t *testing.T) {
	tests := []struct {
		signal Signal
		want   string
	}{
		{SignalNone, "NONE"},
		{SignalVaRWarning, "VaR_WARNING"},
		{SignalSharpeDegradation, "SHARPE_DEGRADATION"},
		{SignalCircuitBreaker, "CIRCUIT_BREAKER"},
	}

	for _, tt := range tests {
		if got := tt.signal.String(); got != tt.want {
			t.Errorf("Signal(%v).String() = %q, want %q", tt.signal, got, tt.want)
		}
	}
}

func TestSignalEngineEvaluateNoData(t *testing.T) {
	dir := t.TempDir()
	eng, err := NewSignalEngine(dir)
	if err != nil {
		t.Fatalf("NewSignalEngine: %v", err)
	}

	sigs, err := eng.Evaluate()
	if err != nil {
		t.Fatalf("Evaluate with no data: expected no error, got %v", err)
	}
	if sigs.VaR95 != 0 || sigs.VaR99 != 0 {
		t.Fatalf("expected zero VaR with no data, got VaR95=%f VaR99=%f", sigs.VaR95, sigs.VaR99)
	}
	if len(sigs.Active) != 0 {
		t.Fatalf("expected no active signals with no data, got %v", sigs.Active)
	}
}

func TestSignalEngineVaRWarningActive(t *testing.T) {
	dir := t.TempDir()
	store := ledger.NewStore(dir).(*ledger.Store)

	var outcomes []domain.RecommendationOutcome
	for range 25 {
		outcomes = append(outcomes, domain.RecommendationOutcome{
			AgentID:       "agent1",
			Symbol:        "2330",
			Side:          domain.SideBuy,
			Layer:         domain.LayerSector,
			Conviction:    1,
			Window:        "1d",
			ForwardReturn: -0.06,
			Hit:           false,
		})
	}
	if err := store.RecordOutcomes(outcomes); err != nil {
		t.Fatalf("RecordOutcomes: %v", err)
	}

	eng, err := NewSignalEngine(dir)
	if err != nil {
		t.Fatalf("NewSignalEngine: %v", err)
	}
	sigs, err := eng.Evaluate()
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}

	found := slices.Contains(sigs.Active, SignalVaRWarning)
	if !found {
		t.Errorf("expected SignalVaRWarning when VaR95 < -0.05, got active=%v", sigs.Active)
	}
}

func TestSignalEngineVaRNoWarning(t *testing.T) {
	dir := t.TempDir()
	store := ledger.NewStore(dir).(*ledger.Store)

	var outcomes []domain.RecommendationOutcome
	for range 25 {
		outcomes = append(outcomes, domain.RecommendationOutcome{
			AgentID:       "agent1",
			Symbol:        "2330",
			Side:          domain.SideBuy,
			Layer:         domain.LayerSector,
			Conviction:    1,
			Window:        "1d",
			ForwardReturn: 0.01,
			Hit:           true,
		})
	}
	if err := store.RecordOutcomes(outcomes); err != nil {
		t.Fatalf("RecordOutcomes: %v", err)
	}

	eng, err := NewSignalEngine(dir)
	if err != nil {
		t.Fatalf("NewSignalEngine: %v", err)
	}
	sigs, err := eng.Evaluate()
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}

	for _, s := range sigs.Active {
		if s == SignalVaRWarning {
			t.Fatalf("expected no VaRWarning when returns are positive, got active=%v", sigs.Active)
		}
	}
}

func TestSignalEngineCircuitBreakerActive(t *testing.T) {
	dir := t.TempDir()
	store := ledger.NewStore(dir).(*ledger.Store)

	var allOutcomes []domain.RecommendationOutcome
	for i := range 25 {
		session := domain.ReplaySession{ID: "sess-cb-" + string(rune('a'+i))}
		summary := domain.SessionSummary{
			SessionID:      session.ID,
			Regime:         domain.RegimeRiskOn,
			PortfolioValue: 100000.0 - float64(i)*10000.0,
			EndingCash:     50000.0,
		}
		if err := store.RecordSessionSummary(session, summary); err != nil {
			t.Fatalf("RecordSessionSummary[%d]: %v", i, err)
		}
		var outs []domain.RecommendationOutcome
		for range 5 {
			out := domain.RecommendationOutcome{
				AgentID:       "agent1",
				Symbol:        "2330",
				Side:          domain.SideBuy,
				Layer:         domain.LayerSector,
				Conviction:    1,
				Window:        "1d",
				ForwardReturn: 0.001,
				Hit:           true,
			}
			outs = append(outs, out)
			allOutcomes = append(allOutcomes, out)
		}
		if err := store.RecordSessionOutcomes(session, outs); err != nil {
			t.Fatalf("RecordSessionOutcomes[%d]: %v", i, err)
		}
	}
	if err := store.RecordOutcomes(allOutcomes); err != nil {
		t.Fatalf("RecordOutcomes: %v", err)
	}

	eng, err := NewSignalEngine(dir)
	if err != nil {
		t.Fatalf("NewSignalEngine: %v", err)
	}
	sigs, err := eng.Evaluate()
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}

	found := slices.Contains(sigs.Active, SignalCircuitBreaker)
	if !found {
		t.Errorf("expected SignalCircuitBreaker for drawdown < -0.15, got active=%v", sigs.Active)
	}
}

func TestSignalEngineCircuitBreakerInactive(t *testing.T) {
	dir := t.TempDir()
	store := ledger.NewStore(dir).(*ledger.Store)

	var allOutcomes []domain.RecommendationOutcome
	for i := range 5 {
		session := domain.ReplaySession{ID: "sess-ok-" + string(rune('0'+i))}
		summary := domain.SessionSummary{
			SessionID:      session.ID,
			Regime:         domain.RegimeRiskOn,
			PortfolioValue: 100000.0,
			EndingCash:     50000.0,
		}
		if err := store.RecordSessionSummary(session, summary); err != nil {
			t.Fatalf("RecordSessionSummary: %v", err)
		}
		var outs []domain.RecommendationOutcome
		for range 5 {
			out := domain.RecommendationOutcome{
				AgentID:       "agent1",
				Symbol:        "2330",
				Side:          domain.SideBuy,
				Layer:         domain.LayerSector,
				Conviction:    1,
				Window:        "1d",
				ForwardReturn: 0.001,
				Hit:           true,
			}
			outs = append(outs, out)
			allOutcomes = append(allOutcomes, out)
		}
		if err := store.RecordSessionOutcomes(session, outs); err != nil {
			t.Fatalf("RecordSessionOutcomes: %v", err)
		}
	}
	if err := store.RecordOutcomes(allOutcomes); err != nil {
		t.Fatalf("RecordOutcomes: %v", err)
	}

	eng, err := NewSignalEngine(dir)
	if err != nil {
		t.Fatalf("NewSignalEngine: %v", err)
	}
	sigs, err := eng.Evaluate()
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}

	for _, s := range sigs.Active {
		if s == SignalCircuitBreaker {
			t.Fatalf("expected no CircuitBreaker when drawdown >= -0.15, got active=%v", sigs.Active)
		}
	}
}
