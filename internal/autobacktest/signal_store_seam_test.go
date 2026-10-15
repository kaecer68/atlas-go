package autobacktest

import (
	"context"
	"slices"
	"testing"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/ledger"
)

// fakeFullStore is an in-memory ledger store for the seam tests. It embeds a
// real JSONL *ledger.Store (so it satisfies ledger.FullStore and every write
// method) but is built on an empty directory, so any read that bypasses the
// injected outcomes/summaries returns nothing. A non-zero evaluation result
// therefore proves the signal path read the injected store, not a directory.
type fakeFullStore struct {
	*ledger.Store
	outcomes  []domain.RecommendationOutcome
	summaries []domain.SessionSummary
}

func newFakeFullStore(t *testing.T) *fakeFullStore {
	t.Helper()
	return &fakeFullStore{Store: ledger.NewStore(t.TempDir()).(*ledger.Store)}
}

func (f *fakeFullStore) LoadOutcomes() ([]domain.RecommendationOutcome, error) {
	return f.outcomes, nil
}

func (f *fakeFullStore) LoadOutcomesFromSessions() ([]domain.RecommendationOutcome, error) {
	return f.outcomes, nil
}

func (f *fakeFullStore) LoadSessionSummaries() ([]domain.SessionSummary, error) {
	return f.summaries, nil
}

var _ ledger.FullStore = (*fakeFullStore)(nil)

// seedBreakerData fills the store with the minimum a CIRCUIT_BREAKER needs:
// a monotonically declining portfolio (>15% drawdown) plus losing outcomes
// from 20 agents (>=20 scorecards, so the short AND long Sharpe windows, the
// 20-sample VaR gate and the drawdown series are all non-zero).
func (f *fakeFullStore) seedBreakerData() {
	for i := range 25 {
		f.summaries = append(f.summaries, domain.SessionSummary{
			SessionID:      "sess-seam-" + string(rune('a'+i)),
			Regime:         domain.RegimeRiskOff,
			PortfolioValue: 100000.0 - float64(i)*2000.0,
			EndingCash:     50000.0,
		})
	}
	for i := range 20 {
		for j := range 3 {
			f.outcomes = append(f.outcomes, domain.RecommendationOutcome{
				AgentID:    "agent-seam-" + string(rune('a'+i)),
				Symbol:     "2330",
				Side:       domain.SideBuy,
				Layer:      domain.LayerSector,
				Conviction: 1,
				Window:     "1d",
				// Slightly varying losses: an identical-return series has a
				// zero variance, which ComputeSharpe reports as SharpeLike 0.
				ForwardReturn: -0.06 + 0.001*float64(j),
				Hit:           false,
			})
		}
	}
}

// TestSignalEngineReadsInjectedStore is the positive half of the T1 proof:
// the engine must evaluate whatever store production wired in, and a decline
// deep enough to trip the breaker must reach Signals.Active.
func TestSignalEngineReadsInjectedStore(t *testing.T) {
	store := newFakeFullStore(t)
	store.seedBreakerData()

	eng := NewSignalEngineWithStore(store)
	sigs, err := eng.Evaluate()
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}

	if !slices.Contains(sigs.Active, SignalCircuitBreaker) {
		t.Fatalf("expected CIRCUIT_BREAKER from injected store, got active=%v", sigs.Active)
	}
	if sigs.DrawdownPct <= 0.15 {
		t.Errorf("DrawdownPct = %f, want > 0.15 from the injected summaries", sigs.DrawdownPct)
	}
	if sigs.VaR95 >= 0 {
		t.Errorf("VaR95 = %f, want < 0 from the injected outcomes", sigs.VaR95)
	}
	if sigs.SharpeShort == 0 || sigs.SharpeLong == 0 {
		t.Errorf("Sharpes should be non-zero from the injected outcomes: short=%f long=%f",
			sigs.SharpeShort, sigs.SharpeLong)
	}
}

// TestSignalEngineFromDirReproducesAllZeroNoError is the negative half of the
// T1 proof. It pins the exact production symptom: a directory-based (JSONL)
// engine whose file is missing answers "no active signals, zero metrics,
// nil error" — indistinguishable from a healthy, risk-free portfolio, which is
// why the dead breaker went unnoticed. If this test ever fails because the
// JSONL store started reporting an error, the seam check script and the
// fail-loud handler can be simplified.
func TestSignalEngineFromDirReproducesAllZeroNoError(t *testing.T) {
	dir := t.TempDir()
	eng, err := NewSignalEngineFromDir(dir)
	if err != nil {
		t.Fatalf("NewSignalEngineFromDir: %v", err)
	}

	sigs, err := eng.Evaluate()
	if err != nil {
		t.Fatalf("expected the missing-file read to be silent (nil error), got %v", err)
	}
	if len(sigs.Active) != 0 {
		t.Errorf("expected no active signals, got %v", sigs.Active)
	}
	if sigs.VaR95 != 0 || sigs.VaR99 != 0 || sigs.SharpeShort != 0 ||
		sigs.SharpeLong != 0 || sigs.DrawdownPct != 0 {
		t.Errorf("expected all-zero metrics (the defect), got %+v", sigs)
	}
}

// TestSignalApplyForcesChannelOpenWithInjectedStore proves the consumer end of
// T1: with data present the breaker actually reaches Gateway.ForceOpenChannel.
func TestSignalApplyForcesChannelOpenWithInjectedStore(t *testing.T) {
	store := newFakeFullStore(t)
	store.seedBreakerData()
	opener := &fakeOpener{}

	if err := SignalApplyWithStore(context.Background(), store, opener); err != nil {
		t.Fatalf("SignalApplyWithStore: %v", err)
	}

	got := opener.snapshot()
	want := []string{"fugle", "fubon", "finmind"}
	if !slices.Equal(got, want) {
		t.Errorf("force-opened channels = %v, want %v", got, want)
	}
}

// TestSignalApplyMissingJsonlLedgerNeverOpensChannels is the negative half at
// the consumer end: the old directory-based read produced no error and no
// opens, i.e. a silently dead breaker.
func TestSignalApplyMissingJsonlLedgerNeverOpensChannels(t *testing.T) {
	opener := &fakeOpener{}
	if err := SignalApplyWithStore(context.Background(), jsonlTestStore(t, t.TempDir()), opener); err != nil {
		t.Fatalf("expected silent no-op on a missing JSONL ledger, got %v", err)
	}
	if got := opener.snapshot(); len(got) != 0 {
		t.Errorf("a missing JSONL ledger must not open channels, got %v", got)
	}
}

// TestNewSignalEnginePostgresWithoutPoolFailsLoud pins the seam itself: an
// unbuildable backend-aware store must return an error instead of degrading to
// the JSONL directory (the failure mode that made production answer 200/0).
func TestNewSignalEnginePostgresWithoutPoolFailsLoud(t *testing.T) {
	if _, err := NewSignalEngine(config.Config{StoreBackend: "postgres", LedgerDir: t.TempDir()}); err == nil {
		t.Fatal("expected an error when the postgres backend has no pool; a nil error would mean a silent JSONL fallback")
	}
}
