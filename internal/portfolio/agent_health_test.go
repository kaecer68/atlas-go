package portfolio

import (
	"testing"
	"time"
)

func TestRecordOutcomeUpdatesStreaks(t *testing.T) {
	m := NewAgentHealthManagerWithConfig(DefaultAgentHealthConfig())
	agentID := "test-agent-1"

	m.RecordOutcome(agentID, true, 1.0, 0.6)
	h := m.GetHealth(agentID)
	if h.ConsecutiveWins != 1 {
		t.Errorf("expected ConsecutiveWins=1 after first win, got %d", h.ConsecutiveWins)
	}
	if h.ConsecutiveLosses != 0 {
		t.Errorf("expected ConsecutiveLosses=0 after win, got %d", h.ConsecutiveLosses)
	}

	m.RecordOutcome(agentID, true, 1.2, 0.65)
	h = m.GetHealth(agentID)
	if h.ConsecutiveWins != 2 {
		t.Errorf("expected ConsecutiveWins=2 after second win, got %d", h.ConsecutiveWins)
	}

	m.RecordOutcome(agentID, false, 0.8, 0.55)
	h = m.GetHealth(agentID)
	if h.ConsecutiveLosses != 1 {
		t.Errorf("expected ConsecutiveLosses=1 after loss, got %d", h.ConsecutiveLosses)
	}
	if h.ConsecutiveWins != 0 {
		t.Errorf("expected ConsecutiveWins=0 after loss, got %d", h.ConsecutiveWins)
	}
}

func TestMuteAfterConsecutiveLosses(t *testing.T) {
	m := NewAgentHealthManagerWithConfig(DefaultAgentHealthConfig())
	agentID := "test-agent-2"

	for i := 0; i < 4; i++ {
		m.RecordOutcome(agentID, false, 0.5, 0.5)
	}
	h := m.GetHealth(agentID)
	if h.Status != HealthStatusHealthy {
		t.Errorf("expected status=healthy after 4 losses, got %s", h.Status)
	}

	m.RecordOutcome(agentID, false, 0.5, 0.5)
	h = m.GetHealth(agentID)
	if h.Status != HealthStatusMuted {
		t.Errorf("expected status=muted after 5 losses, got %s", h.Status)
	}
	if h.MutedAt == nil {
		t.Error("expected MutedAt to be set after muting")
	}
}

func TestMuteAfterNegativeSharpe(t *testing.T) {
	m := NewAgentHealthManagerWithConfig(DefaultAgentHealthConfig())
	agentID := "test-agent-3"

	m.RecordOutcome(agentID, true, -0.6, 0.5)
	h := m.GetHealth(agentID)
	if h.Status != HealthStatusMuted {
		t.Errorf("expected status=muted after negative Sharpe, got %s", h.Status)
	}
}

func TestAutoUnmuteAfterConsecutiveWins(t *testing.T) {
	m := NewAgentHealthManagerWithConfig(DefaultAgentHealthConfig())
	agentID := "test-agent-4"

	for i := 0; i < 5; i++ {
		m.RecordOutcome(agentID, false, 0.5, 0.5)
	}
	h := m.GetHealth(agentID)
	if h.Status != HealthStatusMuted {
		t.Fatalf("precondition: expected status=muted after 5 losses, got %s", h.Status)
	}

	for i := 0; i < 2; i++ {
		m.RecordOutcome(agentID, true, 0.5, 0.5)
	}
	h = m.GetHealth(agentID)
	if h.Status != HealthStatusMuted {
		t.Errorf("expected status=muted after 2 wins (threshold=3), got %s", h.Status)
	}

	m.RecordOutcome(agentID, true, 0.5, 0.5)
	h = m.GetHealth(agentID)
	if h.Status != HealthStatusRecovering {
		t.Errorf("expected status=recovering after 3 consecutive wins, got %s", h.Status)
	}
}

func TestAutoUnmuteAfterTimeBasedRecovery(t *testing.T) {
	m := NewAgentHealthManagerWithConfig(DefaultAgentHealthConfig())
	agentID := "test-agent-5"

	for i := 0; i < 5; i++ {
		m.RecordOutcome(agentID, false, 0.5, 0.5)
	}
	h := m.GetHealth(agentID)
	if h.Status != HealthStatusMuted {
		t.Fatalf("precondition: expected status=muted after 5 losses, got %s", h.Status)
	}

	mutedAt := time.Now().Add(-8 * 24 * time.Hour)
	h.MutedAt = &mutedAt

	m.EvaluateAgentBreakers()
	h = m.GetHealth(agentID)
	if h.Status != HealthStatusRecovering {
		t.Errorf("expected status=recovering after time-based recovery (7 days), got %s", h.Status)
	}
}

func TestCompositeScoreCalculation(t *testing.T) {
	m := NewAgentHealthManagerWithConfig(DefaultAgentHealthConfig())

	tests := []struct {
		name    string
		sharpe  float64
		hitRate float64
		wins    int
		losses  int
		wantMin float64
		wantMax float64
	}{
		{"perfect Sharpe, perfect hitRate, max wins", 5.0, 1.0, 10, 0, 95, 100},
		{"negative Sharpe, zero hitRate, max losses", -5.0, 0.0, 0, 10, 0, 5},
		{"neutral Sharpe, 50% hitRate, no streak", 0.0, 0.5, 0, 0, 30, 40},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			score := m.calculateCompositeScore(tc.sharpe, tc.hitRate, tc.wins, tc.losses)
			if score < tc.wantMin || score > tc.wantMax {
				t.Errorf("calculateCompositeScore(sharpe=%.1f, hitRate=%.1f, wins=%d, losses=%d) = %.2f; want [%.1f, %.1f]",
					tc.sharpe, tc.hitRate, tc.wins, tc.losses, score, tc.wantMin, tc.wantMax)
			}
		})
	}
}

func TestIsAgentHealthy(t *testing.T) {
	m := NewAgentHealthManagerWithConfig(DefaultAgentHealthConfig())
	agentID := "test-agent-6"

	if !m.IsAgentHealthy(agentID) {
		t.Error("expected new agent to be healthy")
	}

	m.RecordOutcome(agentID, false, 0.5, 0.5)
	if !m.IsAgentHealthy(agentID) {
		t.Error("expected agent with healthy status to be healthy")
	}

	for i := 0; i < 5; i++ {
		m.RecordOutcome(agentID, false, 0.5, 0.5)
	}
	if m.IsAgentHealthy(agentID) {
		t.Error("expected muted agent to NOT be healthy")
	}
}

func TestGetMutedAgents(t *testing.T) {
	m := NewAgentHealthManagerWithConfig(DefaultAgentHealthConfig())

	m.RecordOutcome("agent-a", false, 0.5, 0.5)
	for i := 0; i < 5; i++ {
		m.RecordOutcome("agent-b", false, 0.5, 0.5)
	}
	m.RecordOutcome("agent-c", true, 1.0, 0.7)

	muted := m.GetMutedAgents()
	if len(muted) != 1 {
		t.Errorf("expected 1 muted agent, got %d", len(muted))
	}
	if len(muted) > 0 && muted[0].AgentID != "agent-b" {
		t.Errorf("expected muted agent to be agent-b, got %s", muted[0].AgentID)
	}
}

func TestAgentHealthStore_SaveAndLoadAll(t *testing.T) {
	dir := t.TempDir()
	store, err := NewAgentHealthStore(dir)
	if err != nil {
		t.Fatalf("NewAgentHealthStore: %v", err)
	}

	h1 := &AgentHealth{AgentID: "agent-a", Status: HealthStatusHealthy, AnnualizedSharpe: 1.5, HitRate: 0.6, ConsecutiveLosses: 0, ConsecutiveWins: 3, CompositeScore: 75.0}
	h2 := &AgentHealth{AgentID: "agent-b", Status: HealthStatusMuted, AnnualizedSharpe: -1.0, HitRate: 0.3, ConsecutiveLosses: 6, ConsecutiveWins: 0, CompositeScore: 20.0}

	if err := store.Save(h1); err != nil {
		t.Fatalf("Save h1: %v", err)
	}
	if err := store.Save(h2); err != nil {
		t.Fatalf("Save h2: %v", err)
	}

	loaded, err := store.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	if len(loaded) != 2 {
		t.Errorf("LoadAll len = %d, want 2", len(loaded))
	}

	if a, ok := loaded["agent-a"]; !ok {
		t.Errorf("missing agent-a")
	} else {
		if a.AnnualizedSharpe != 1.5 {
			t.Errorf("agent-a Sharpe = %v, want 1.5", a.AnnualizedSharpe)
		}
		if a.ConsecutiveWins != 3 {
			t.Errorf("agent-a ConsecutiveWins = %v, want 3", a.ConsecutiveWins)
		}
	}

	if b, ok := loaded["agent-b"]; !ok {
		t.Errorf("missing agent-b")
	} else {
		if b.Status != HealthStatusMuted {
			t.Errorf("agent-b Status = %v, want muted", b.Status)
		}
	}
}

func TestAgentHealthStore_Deduplication(t *testing.T) {
	dir := t.TempDir()
	store, err := NewAgentHealthStore(dir)
	if err != nil {
		t.Fatalf("NewAgentHealthStore: %v", err)
	}

	h1 := &AgentHealth{AgentID: "agent-x", Status: HealthStatusHealthy, AnnualizedSharpe: 1.0, HitRate: 0.5, ConsecutiveWins: 1, CompositeScore: 50.0}
	h2 := &AgentHealth{AgentID: "agent-x", Status: HealthStatusMuted, AnnualizedSharpe: -0.5, HitRate: 0.4, ConsecutiveLosses: 5, CompositeScore: 30.0}
	h3 := &AgentHealth{AgentID: "agent-x", Status: HealthStatusRecovering, AnnualizedSharpe: 0.8, HitRate: 0.55, ConsecutiveWins: 3, CompositeScore: 60.0}

	store.Save(h1)
	store.Save(h2)
	store.Save(h3)

	loaded, err := store.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	if len(loaded) != 1 {
		t.Errorf("LoadAll len = %d, want 1 (dedup by AgentID)", len(loaded))
	}

	x := loaded["agent-x"]
	if x.Status != HealthStatusRecovering {
		t.Errorf("agent-x Status = %v, want recovering (latest record)", x.Status)
	}
	if x.ConsecutiveLosses != 0 {
		t.Errorf("agent-x ConsecutiveLosses = %v, want 0 (last record h3 sets it to 0)", x.ConsecutiveLosses)
	}
}

func TestAgentHealthStore_NonExistentFile(t *testing.T) {
	dir := t.TempDir()
	store, err := NewAgentHealthStore(dir)
	if err != nil {
		t.Fatalf("NewAgentHealthStore: %v", err)
	}

	loaded, err := store.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll on nonexistent: %v", err)
	}
	if loaded != nil {
		t.Errorf("LoadAll on nonexistent = %v, want nil", loaded)
	}
}

func TestNewAgentHealthManagerWithStore_PersistsAndRestores(t *testing.T) {
	dir := t.TempDir()
	store, err := NewAgentHealthStore(dir)
	if err != nil {
		t.Fatalf("NewAgentHealthStore: %v", err)
	}

	mgr := NewAgentHealthManagerWithStore(DefaultAgentHealthConfig(), store)
	mgr.RecordOutcome("agent-p", false, -1.0, 0.3)
	mgr.RecordOutcome("agent-q", true, 2.0, 0.8)

	mgr2 := NewAgentHealthManagerWithStore(DefaultAgentHealthConfig(), store)

	p := mgr2.GetHealth("agent-p")
	if p == nil {
		t.Fatalf("GetHealth agent-p: got nil")
	}
	if p.ConsecutiveLosses != 1 {
		t.Errorf("agent-p ConsecutiveLosses = %d, want 1", p.ConsecutiveLosses)
	}
	if p.Status != HealthStatusMuted {
		t.Errorf("agent-p Status = %v, want muted", p.Status)
	}

	q := mgr2.GetHealth("agent-q")
	if q == nil {
		t.Fatalf("GetHealth agent-q: got nil")
	}
	if q.ConsecutiveWins != 1 {
		t.Errorf("agent-q ConsecutiveWins = %d, want 1", q.ConsecutiveWins)
	}
}

func TestNewAgentHealthManager(t *testing.T) {
	m := NewAgentHealthManager()
	if m == nil {
		t.Fatal("expected non-nil manager")
	}
	if m.health == nil {
		t.Error("expected health map to be initialized")
	}
	if m.config.DefaultMuteThreshold != 5 {
		t.Errorf("expected default mute threshold 5, got %d", m.config.DefaultMuteThreshold)
	}
}

func TestAgentHealthManagerWithParameters(t *testing.T) {
	m := NewAgentHealthManagerWithConfig(DefaultAgentHealthConfig())
	params := DefaultRuntimeParameters()
	m.WithParameters(params)
	if m.runtimeParams == nil {
		t.Error("expected runtime params to be set")
	}
}
