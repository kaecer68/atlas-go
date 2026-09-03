package monitoring

import (
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/repository"
)

// slimLedgerStore embeds a JSONL ledger.Store (full ledger.OutcomeStore) and
// adds the optional slim projection with a distinct outcome set so tests can
// tell which loader was used.
type slimLedgerStore struct {
	*ledger.Store
	slimOutcomes []domain.RecommendationOutcome
	slimCalls    int
}

func (s *slimLedgerStore) LoadScorecardOutcomes() ([]domain.RecommendationOutcome, error) {
	s.slimCalls++
	return s.slimOutcomes, nil
}

func scorecardAdapterFixture() []domain.RecommendationOutcome {
	return []domain.RecommendationOutcome{
		{
			AgentID:       "adapter-slim-agent",
			Skill:         "value",
			Window:        "2026-06-01",
			ForwardReturn: 0.01,
			Hit:           true,
			RecordedAt:    time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		},
	}
}

// TestOutcomeStoreAdapter_LoadScorecardOutcomes_UsesInnerSlim proves the
// adapter delegates to the wrapped ledger store's optional slim loader.
func TestOutcomeStoreAdapter_LoadScorecardOutcomes_UsesInnerSlim(t *testing.T) {
	inner := &slimLedgerStore{Store: ledger.NewStore(t.TempDir()).(*ledger.Store), slimOutcomes: scorecardAdapterFixture()}
	adapter := NewOutcomeStoreAdapter(inner)

	before := ScorecardSlimAdapterFallbackTotal()
	got, err := adapter.LoadScorecardOutcomes()
	if err != nil {
		t.Fatalf("LoadScorecardOutcomes: %v", err)
	}
	if len(got) != 1 || got[0].AgentID != "adapter-slim-agent" {
		t.Fatalf("expected slim outcomes, got %+v", got)
	}
	if inner.slimCalls != 1 {
		t.Errorf("expected 1 inner slim call, got %d", inner.slimCalls)
	}
	if after := ScorecardSlimAdapterFallbackTotal(); after != before {
		t.Errorf("adapter fallback counter must stay flat on the slim path: before=%d after=%d", before, after)
	}
}

// TestOutcomeStoreAdapter_LoadScorecardOutcomes_FallsBack proves jsonl/sqlite
// ledger stores (no optional loader) keep the pre-#1780 full read and the
// fallback is counted (B1).
func TestOutcomeStoreAdapter_LoadScorecardOutcomes_FallsBack(t *testing.T) {
	inner := ledger.NewStore(t.TempDir()).(*ledger.Store)
	fixture := scorecardAdapterFixture()
	// jsonl LoadOutcomesFromSessions reads per-session files under sessions/,
	// so seed through RecordSessionOutcomes.
	if err := inner.RecordSessionOutcomes(domain.ReplaySession{ID: "session-20260601-slim"}, fixture); err != nil {
		t.Fatalf("seed jsonl store: %v", err)
	}
	adapter := NewOutcomeStoreAdapter(inner)

	before := ScorecardSlimAdapterFallbackTotal()
	got, err := adapter.LoadScorecardOutcomes()
	if err != nil {
		t.Fatalf("LoadScorecardOutcomes fallback: %v", err)
	}
	if len(got) != 1 || got[0].AgentID != "adapter-slim-agent" {
		t.Fatalf("expected full-read fallback outcomes, got %+v", got)
	}
	if after := ScorecardSlimAdapterFallbackTotal(); after != before+1 {
		t.Errorf("adapter fallback counter delta = %d, want 1", after-before)
	}
}

// TestDualWriteOutcomeStoreAdapter_LoadScorecardOutcomes_Delegates proves the
// DualWriteOutcomeStoreAdapter routes the slim call into the repository's
// QueryScorecardOutcomes, which reaches the slim-capable JSONL outcome store
// (monitoring.OutcomeStoreAdapter wrapping a slim ledger store) end to end.
func TestDualWriteOutcomeStoreAdapter_LoadScorecardOutcomes_Delegates(t *testing.T) {
	inner := &slimLedgerStore{Store: ledger.NewStore(t.TempDir()).(*ledger.Store), slimOutcomes: scorecardAdapterFixture()}
	repo := repository.NewDualWriteRepository(nil, nil, nil, NewOutcomeStoreAdapter(inner), nil, nil, nil)
	adapter := NewDualWriteOutcomeStoreAdapter(repo)

	got, err := adapter.LoadScorecardOutcomes()
	if err != nil {
		t.Fatalf("DualWriteOutcomeStoreAdapter.LoadScorecardOutcomes: %v", err)
	}
	if len(got) != 1 || got[0].AgentID != "adapter-slim-agent" {
		t.Fatalf("expected slim outcomes end to end, got %+v", got)
	}
	if inner.slimCalls != 1 {
		t.Errorf("expected 1 inner slim call through the dual-write chain, got %d", inner.slimCalls)
	}
}
