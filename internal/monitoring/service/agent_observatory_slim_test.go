package service

import (
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// observatorySlimStore serves the same fixture outcomes through the #1780
// Phase 1 slim projection (LoadScorecardOutcomes) and tracks which loader the
// service actually invoked.
type observatorySlimStore struct {
	*mockOutcomeStore
	mu        sync.Mutex
	outcomes  []domain.RecommendationOutcome
	slimCalls int
	fullCalls int
}

func newObservatorySlimStore(outcomes []domain.RecommendationOutcome) *observatorySlimStore {
	return &observatorySlimStore{mockOutcomeStore: &mockOutcomeStore{}, outcomes: outcomes}
}

func (s *observatorySlimStore) LoadScorecardOutcomes() ([]domain.RecommendationOutcome, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.slimCalls++
	return s.outcomes, nil
}

func (s *observatorySlimStore) LoadOutcomesFromSessions() ([]domain.RecommendationOutcome, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fullCalls++
	return s.outcomes, nil
}

func (s *observatorySlimStore) calls() (int, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.slimCalls, s.fullCalls
}

// fullOnlyOutcomeStore is a ledger.OutcomeStore WITHOUT the optional slim
// loader — the service must fall back to the full read (B1).
type fullOnlyOutcomeStore struct {
	*mockOutcomeStore
	outcomes []domain.RecommendationOutcome
	calls    int
}

func newFullOnlyOutcomeStore(outcomes []domain.RecommendationOutcome) *fullOnlyOutcomeStore {
	return &fullOnlyOutcomeStore{mockOutcomeStore: &mockOutcomeStore{}, outcomes: outcomes}
}

func (s *fullOnlyOutcomeStore) LoadOutcomesFromSessions() ([]domain.RecommendationOutcome, error) {
	s.calls++
	return s.outcomes, nil
}

func observatorySlimFixture() []domain.RecommendationOutcome {
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	var out []domain.RecommendationOutcome
	// agent-a: rising per-window returns (positive trend); agent-b: falling.
	for i := 0; i < 30; i++ {
		win := time.Date(2026, 6, i/6+1, 0, 0, 0, 0, time.UTC)
		ret := 0.01 + 0.001*float64(i%6) + 0.003*float64(i/6)
		out = append(out, domain.RecommendationOutcome{
			AgentID:       "obs-slim-a",
			Skill:         "beta",
			Layer:         domain.LayerMacro,
			Symbol:        "2330.TW",
			Side:          domain.SideBuy,
			Conviction:    75,
			Window:        win.Format("2006-01-02"),
			ForwardReturn: ret,
			Hit:           ret > 0.012,
			RecordedAt:    now.Add(time.Duration(i) * time.Minute),
		})
		retB := 0.02 - 0.001*float64(i%6) - 0.003*float64(i/6)
		out = append(out, domain.RecommendationOutcome{
			AgentID:       "obs-slim-b",
			Skill:         "gamma",
			Layer:         domain.LayerStyle,
			Symbol:        "2317.TW",
			Side:          domain.SideBuy,
			Conviction:    60,
			Window:        win.Format("2006-01-02"),
			ForwardReturn: retB,
			Hit:           retB > 0.012,
			RecordedAt:    now.Add(time.Duration(i)*time.Minute + time.Second),
		})
	}
	return out
}

// TestLoadAgentObservatory_UsesSlimProjection proves the sessionID=="" path
// prefers the 8-field slim loader when the store supports it, and never
// touches the full metadata read.
func TestLoadAgentObservatory_UsesSlimProjection(t *testing.T) {
	fixture := observatorySlimFixture()
	store := newObservatorySlimStore(fixture)
	svc := NewPipelineService(t.TempDir(), t.TempDir(), store)

	before := ScorecardSlimServiceFallbackTotal()
	data, err := svc.loadAgentObservatoryUncached("", 50)
	if err != nil {
		t.Fatalf("loadAgentObservatoryUncached: %v", err)
	}
	if data == nil || len(data.Scorecards) == 0 {
		t.Fatal("expected non-empty scorecards")
	}
	slimCalls, fullCalls := store.calls()
	if slimCalls != 1 {
		t.Errorf("expected exactly 1 slim load, got %d", slimCalls)
	}
	if fullCalls != 0 {
		t.Errorf("slim-capable store must not fall back to the full read, got %d full loads", fullCalls)
	}
	if got := ScorecardSlimServiceFallbackTotal(); got != before {
		t.Errorf("service fallback counter must stay flat on the slim path: before=%d after=%d", before, got)
	}
}

// TestLoadAgentObservatory_FallsBackWhenStoreLacksSlim proves jsonl/sqlite
// backends (and existing mocks) — stores without the optional loader — keep
// working through the pre-#1780 full read, and that the fallback is counted
// (B1 deployment acceptance: counter == 0 on the slim-capable path).
func TestLoadAgentObservatory_FallsBackWhenStoreLacksSlim(t *testing.T) {
	fixture := observatorySlimFixture()
	store := newFullOnlyOutcomeStore(fixture)
	svc := NewPipelineService(t.TempDir(), t.TempDir(), store)

	before := ScorecardSlimServiceFallbackTotal()
	data, err := svc.loadAgentObservatoryUncached("", 50)
	if err != nil {
		t.Fatalf("loadAgentObservatoryUncached: %v", err)
	}
	if data == nil || len(data.Scorecards) == 0 {
		t.Fatal("expected non-empty scorecards via fallback")
	}
	if store.calls != 1 {
		t.Errorf("expected 1 full fallback load, got %d", store.calls)
	}
	if got := ScorecardSlimServiceFallbackTotal(); got != before+1 {
		t.Errorf("fallback counter delta = %d, want 1", got-before)
	}
}

// TestLoadAgentObservatory_SlimAndFullScorecardsIdentical is the acceptance
// equivalence: the same outcomes served through the slim loader vs the full
// loader must produce byte-identical scorecards (all fields except
// LastUpdatedAt) — the observatory response is unchanged by Phase 1 (red
// line 1, k3 review B4: per-field comparison, not a bit-for-bit claim on a
// single BuildScorecards run).
func TestLoadAgentObservatory_SlimAndFullScorecardsIdentical(t *testing.T) {
	fixture := observatorySlimFixture()

	slimStore := newObservatorySlimStore(fixture)
	svcSlim := NewPipelineService(t.TempDir(), t.TempDir(), slimStore)
	dataSlim, err := svcSlim.loadAgentObservatoryUncached("", 50)
	if err != nil {
		t.Fatalf("slim load: %v", err)
	}

	fullStore := newFullOnlyOutcomeStore(fixture)
	svcFull := NewPipelineService(t.TempDir(), t.TempDir(), fullStore)
	dataFull, err := svcFull.loadAgentObservatoryUncached("", 50)
	if err != nil {
		t.Fatalf("full load: %v", err)
	}

	if len(dataSlim.Scorecards) != len(dataFull.Scorecards) {
		t.Fatalf("scorecard count differs: slim=%d full=%d", len(dataSlim.Scorecards), len(dataFull.Scorecards))
	}
	fullByAgent := map[string]domain.Scorecard{}
	for _, sc := range dataFull.Scorecards {
		sc.LastUpdatedAt = time.Time{}
		fullByAgent[sc.AgentID] = sc
	}
	for _, slimSC := range dataSlim.Scorecards {
		slimSC.LastUpdatedAt = time.Time{}
		fullSC, ok := fullByAgent[slimSC.AgentID]
		if !ok {
			t.Fatalf("agent %q missing from full-path scorecards", slimSC.AgentID)
		}
		if !reflect.DeepEqual(slimSC, fullSC) {
			t.Errorf("scorecard mismatch for %q:\n  slim=%+v\n  full=%+v", slimSC.AgentID, slimSC, fullSC)
		}
	}
}

// TestLoadAgentObservatory_EndToEndSlim exercises the exported cached entry
// point on the slim path so the 60s TTL / in-flight dedup (PR #1813) is
// untouched and still returns data.
func TestLoadAgentObservatory_EndToEndSlim(t *testing.T) {
	fixture := observatorySlimFixture()
	store := newObservatorySlimStore(fixture)
	svc := NewPipelineService(t.TempDir(), t.TempDir(), store)

	data, err := svc.LoadAgentObservatory("", 50)
	if err != nil {
		t.Fatalf("LoadAgentObservatory: %v", err)
	}
	if data == nil || len(data.Scorecards) == 0 {
		t.Fatal("expected non-empty scorecards")
	}
	// A second identical call must be served from the in-memory cache
	// (same result, slim loader not invoked again).
	cached, err := svc.LoadAgentObservatory("", 50)
	if err != nil {
		t.Fatalf("cached LoadAgentObservatory: %v", err)
	}
	if cached == nil || len(cached.Scorecards) != len(data.Scorecards) {
		t.Fatal("cached result mismatch")
	}
	slimCalls, _ := store.calls()
	if slimCalls != 1 {
		t.Errorf("expected 1 slim load (2nd call cached), got %d", slimCalls)
	}
}
