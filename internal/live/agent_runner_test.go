package live

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	recommendation "github.com/kaecer68/atlas-go/internal/domain/recommendation"
	"github.com/kaecer68/atlas-go/internal/domain/shared"
	livestore "github.com/kaecer68/atlas-go/internal/live/store"
	"github.com/kaecer68/atlas-go/internal/orchestrator"
)

// mockMarketData implements the marketData interface for AgentRunner.
type mockMarketData struct {
	quotes []domain.Quote
	err    error
}

func (m *mockMarketData) GetQuotes(ctx context.Context, t time.Time, symbols []string) ([]domain.Quote, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.quotes, nil
}

// mockSystem implements the system interface for AgentRunner.
type mockSystem struct {
	registry        domain.AgentRegistry
	plugins         *orchestrator.PluginRegistry
	executionPolicy domain.ExecutionPolicy
}

func (m *mockSystem) Registry() domain.AgentRegistry {
	return m.registry
}

func (m *mockSystem) GetPlugins() *orchestrator.PluginRegistry {
	return m.plugins
}

func (m *mockSystem) GetExecutionPolicy() domain.ExecutionPolicy {
	return m.executionPolicy
}

func newTestMarketData() *mockMarketData {
	return &mockMarketData{
		quotes: []domain.Quote{
			{Symbol: "2330.TW", Last: 550, IsTradable: true},
			{Symbol: "0050.TW", Last: 150, IsTradable: true},
			{Symbol: "2317.TW", Last: 110, IsTradable: true},
		},
	}
}

func newTestAgentRunner(t *testing.T) (*AgentRunner, *livestore.StateStore, *mockMarketData, *mockSystem) {
	t.Helper()
	st := livestore.NewStateStore(t.TempDir())
	md := newTestMarketData()
	sys := &mockSystem{
		registry: domain.AgentRegistry{
			Agents: []recommendation.AgentSpec{
				{ID: "ctx-1", Name: "ContextAgent", Layer: shared.LayerContext, Skill: "context", Enabled: true, Universe: nil},
				{ID: "sec-1", Name: "SectorAgent", Layer: shared.LayerSector, Skill: "sector", Enabled: true, Universe: []string{"2330.TW"}},
				{ID: "cro-1", Name: "CROFilter", Layer: shared.LayerControl, Skill: "control", Enabled: true, Universe: nil},
			},
		},
		plugins: orchestrator.NewPluginRegistry(),
	}
	runner := NewAgentRunner(st, md, sys, "paper")
	return runner, st, md, sys
}

func TestAgentRunner_NewAgentRunner(t *testing.T) {
	st := livestore.NewStateStore(t.TempDir())
	md := newTestMarketData()
	sys := &mockSystem{}

	runner := NewAgentRunner(st, md, sys, "paper")
	if runner == nil {
		t.Fatal("expected non-nil AgentRunner")
	}
	if runner.stateStore != st {
		t.Error("stateStore not set")
	}
	if runner.effectiveBrokerMode != "paper" {
		t.Errorf("expected effectiveBrokerMode paper, got %s", runner.effectiveBrokerMode)
	}
}

func TestAgentRunner_SetEventBus(t *testing.T) {
	runner := &AgentRunner{}
	eb := NewChannelEventBus(8)
	defer eb.Close()
	runner.SetEventBus(eb)
	if runner.eventBus != eb {
		t.Error("eventBus not set")
	}
}

func TestAgentRunner_SetMetrics(t *testing.T) {
	runner := &AgentRunner{}
	m := &noopMetrics{}
	runner.SetMetrics(m)
	if runner.metrics != m {
		t.Error("metrics not set")
	}
}

func TestAgentRunner_ApplyExecutionInput(t *testing.T) {
	st := livestore.NewStateStore(t.TempDir())
	runner := &AgentRunner{
		stateStore: st,
		marketData: nil,
		system:     nil,
	}

	input := ExecutionInput{
		Regime:               domain.RegimeRiskOn,
		RawRecommendations:   []domain.Recommendation{{Agent: "sector_semiconductor", Symbol: "2330.TW"}},
		FinalRecommendations: []domain.Recommendation{{Agent: "sector_semiconductor", Symbol: "2330.TW"}},
		DeterminedBy:         "orchestrator-pipeline-v1",
	}

	err := runner.ApplyExecutionInput(context.Background(), input)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if got := st.GetCurrentRegime(); got != domain.RegimeRiskOn {
		t.Errorf("expected regime RiskOn, got %v", got)
	}

	pending := st.GetPendingRecommendations()
	if len(pending) != 1 || pending[0].Symbol != "2330.TW" {
		t.Errorf("expected 1 pending rec for 2330.TW, got %v", pending)
	}

	filtered := st.GetFilteredRecommendations()
	if len(filtered) != 1 || filtered[0].Symbol != "2330.TW" {
		t.Errorf("expected 1 filtered rec for 2330.TW, got %v", filtered)
	}
}

func TestAgentRunner_ApplyExecutionInput_EmptyRecommendations(t *testing.T) {
	st := livestore.NewStateStore(t.TempDir())
	runner := &AgentRunner{stateStore: st}

	input := ExecutionInput{
		Regime:               domain.RegimeNeutral,
		RawRecommendations:   nil,
		FinalRecommendations: nil,
		DeterminedBy:         "test",
	}

	err := runner.ApplyExecutionInput(context.Background(), input)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if got := st.GetCurrentRegime(); got != domain.RegimeNeutral {
		t.Errorf("expected regime Neutral, got %v", got)
	}

	if pending := st.GetPendingRecommendations(); len(pending) != 0 {
		t.Errorf("expected no pending recs, got %v", pending)
	}
}

func TestAgentRunner_RunContextAgent_NilSystem(t *testing.T) {
	st := livestore.NewStateStore(t.TempDir())
	runner := &AgentRunner{stateStore: st, system: nil}

	err := runner.RunContextAgent(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error for nil system")
	}
	if err.Error() != "system not initialized" {
		t.Errorf("expected 'system not initialized', got %q", err.Error())
	}
}

func TestAgentRunner_RunContextAgent_EmptyWatchlist(t *testing.T) {
	runner, st, _, _ := newTestAgentRunner(t)

	err := runner.RunContextAgent(context.Background(), nil)
	if err != nil {
		t.Fatalf("RunContextAgent failed: %v", err)
	}

	regime := st.GetCurrentRegime()
	if regime != domain.RegimeNeutral {
		t.Errorf("expected Neutral regime (no executors), got %v", regime)
	}
}

func TestAgentRunner_RunContextAgent_MarketDataError(t *testing.T) {
	st := livestore.NewStateStore(t.TempDir())
	md := &mockMarketData{err: errors.New("connection refused")}
	sys := &mockSystem{plugins: orchestrator.NewPluginRegistry()}
	runner := NewAgentRunner(st, md, sys, "paper")

	err := runner.RunContextAgent(context.Background(), []string{"2330.TW"})
	if err == nil {
		t.Fatal("expected error from market data")
	}
}

func TestAgentRunner_RunContextAgent_WithQuotes(t *testing.T) {
	runner, st, _, _ := newTestAgentRunner(t)

	err := runner.RunContextAgent(context.Background(), []string{"2330.TW", "0050.TW"})
	if err != nil {
		t.Fatalf("RunContextAgent failed: %v", err)
	}

	regime := st.GetCurrentRegime()
	if regime != domain.RegimeNeutral {
		t.Errorf("expected Neutral regime (score=0 from empty plugins), got %v", regime)
	}
}

func TestAgentRunner_RunStyleAndSectorAgents_NilSystem(t *testing.T) {
	st := livestore.NewStateStore(t.TempDir())
	runner := &AgentRunner{stateStore: st, system: nil}

	err := runner.RunStyleAndSectorAgents(context.Background(), []string{"2330.TW"})
	if err == nil {
		t.Fatal("expected error for nil system")
	}
}

func TestAgentRunner_RunStyleAndSectorAgents_EmptyWatchlist(t *testing.T) {
	runner, _, _, _ := newTestAgentRunner(t)

	err := runner.RunStyleAndSectorAgents(context.Background(), nil)
	if err != nil {
		t.Fatalf("expected no error for empty watchlist, got %v", err)
	}
}

func TestAgentRunner_RunStyleAndSectorAgents_MarketDataError(t *testing.T) {
	st := livestore.NewStateStore(t.TempDir())
	md := &mockMarketData{err: errors.New("timeout")}
	sys := &mockSystem{plugins: orchestrator.NewPluginRegistry()}
	runner := NewAgentRunner(st, md, sys, "paper")

	err := runner.RunStyleAndSectorAgents(context.Background(), []string{"2330.TW"})
	if err == nil {
		t.Fatal("expected error from market data")
	}
}

func TestAgentRunner_RunStyleAndSectorAgents_HappyPath(t *testing.T) {
	runner, st, _, _ := newTestAgentRunner(t)

	err := runner.RunStyleAndSectorAgents(context.Background(), []string{"2330.TW", "0050.TW"})
	if err != nil {
		t.Fatalf("RunStyleAndSectorAgents failed: %v", err)
	}

	// With no executors in the registry, no recommendations should be generated
	recs := st.GetPendingRecommendations()
	t.Logf("generated %d recommendations", len(recs))
}

func TestAgentRunner_ApplyRiskFilters_NilSystem(t *testing.T) {
	st := livestore.NewStateStore(t.TempDir())
	runner := &AgentRunner{stateStore: st, system: nil}

	err := runner.ApplyRiskFilters(context.Background())
	if err == nil {
		t.Fatal("expected error for nil system")
	}
}

func TestAgentRunner_ApplyRiskFilters_EmptyPendingRecs(t *testing.T) {
	runner, _, _, _ := newTestAgentRunner(t)

	err := runner.ApplyRiskFilters(context.Background())
	if err != nil {
		t.Fatalf("expected no error for empty pending recs, got %v", err)
	}
}

func TestAgentRunner_ApplyRiskFilters_WithRecs(t *testing.T) {
	runner, st, _, _ := newTestAgentRunner(t)

	// Set pending recommendations
	st.SetPendingRecommendations([]domain.Recommendation{
		{Agent: "sec-1", Symbol: "2330.TW", Conviction: 80, Side: "buy"},
		{Agent: "sec-1", Symbol: "0050.TW", Conviction: 40, Side: "sell"},
	})

	err := runner.ApplyRiskFilters(context.Background())
	if err != nil {
		t.Fatalf("ApplyRiskFilters failed: %v", err)
	}

	filtered := st.GetFilteredRecommendations()
	t.Logf("filtered to %d recommendations", len(filtered))
}

func TestAgentRunner_ApplyRiskFilters_BlockedEvent(t *testing.T) {
	runner, st, _, _ := newTestAgentRunner(t)

	// Register an event bus to capture the risk alert
	eb := NewChannelEventBus(8)
	defer eb.Close()
	runner.SetEventBus(eb)

	// Set up mock system with control-layer agent that blocks all recs
	sys := &mockSystem{
		plugins: orchestrator.NewPluginRegistry(),
	}
	runner.system = sys

	// Seed pending recs
	st.SetPendingRecommendations([]domain.Recommendation{
		{Agent: "sec-1", Symbol: "2330.TW", Conviction: 80},
	})

	// Subscribe to risk alerts
	var eventReceived bool
	sub := eb.Subscribe(EventRiskAlert, func(ctx context.Context, event BusEvent) error {
		eventReceived = true
		return nil
	})
	defer sub.Cancel()

	err := runner.ApplyRiskFilters(context.Background())
	if err != nil {
		t.Fatalf("ApplyRiskFilters failed: %v", err)
	}

	// With no control executors, no events are published (nothing gets blocked)
	t.Logf("event received: %v", eventReceived)
}

func TestAgentRunner_PublishEvent_NilEventBus(t *testing.T) {
	runner := &AgentRunner{eventBus: nil}

	// Should not panic
	runner.publishEvent(BusEvent{ID: "evt-1", Type: EventSystemStart})
}

func TestAgentRunner_PublishEvent_WithEventBus(t *testing.T) {
	eb := NewChannelEventBus(8)
	defer eb.Close()
	runner := &AgentRunner{eventBus: eb}

	var received bool
	sub := eb.Subscribe(EventSystemStart, func(ctx context.Context, event BusEvent) error {
		received = true
		return nil
	})
	defer sub.Cancel()

	runner.publishEvent(BusEvent{ID: "evt-1", Type: EventSystemStart, Timestamp: time.Now()})
	time.Sleep(50 * time.Millisecond)

	if !received {
		t.Error("expected event to be published and received")
	}
}

func TestAgentRunner_InferRegime_NoEnabledContextAgents(t *testing.T) {
	runner, _, _, _ := newTestAgentRunner(t)

	regime := runner.inferRegime(
		domain.AgentRegistry{
			Agents: []recommendation.AgentSpec{
				{ID: "sec-1", Layer: shared.LayerSector, Enabled: true},
			},
		},
		map[string]domain.Quote{},
		orchestrator.NewPluginRegistry(),
	)
	if regime != domain.RegimeNeutral {
		t.Errorf("expected Neutral with no context agents, got %v", regime)
	}
}

func TestAgentRunner_InferRegime_OnlyDisabledContextAgent(t *testing.T) {
	runner, _, _, _ := newTestAgentRunner(t)

	regime := runner.inferRegime(
		domain.AgentRegistry{
			Agents: []recommendation.AgentSpec{
				{ID: "ctx-1", Layer: shared.LayerContext, Skill: "context", Enabled: false},
			},
		},
		map[string]domain.Quote{},
		orchestrator.NewPluginRegistry(),
	)
	if regime != domain.RegimeNeutral {
		t.Errorf("expected Neutral with disabled context agent, got %v", regime)
	}
}

func TestAgentRunner_InferRegime_EnabledContextAgent(t *testing.T) {
	runner, _, _, _ := newTestAgentRunner(t)

	regime := runner.inferRegime(
		domain.AgentRegistry{
			Agents: []recommendation.AgentSpec{
				{ID: "ctx-1", Layer: shared.LayerContext, Skill: "context", Enabled: true},
			},
		},
		map[string]domain.Quote{"2330.TW": {Last: 550}},
		orchestrator.NewPluginRegistry(),
	)
	// Score is 0 (no executors), so regime should be Neutral
	if regime != domain.RegimeNeutral {
		t.Errorf("expected Neutral (score=0), got %v", regime)
	}
}

func TestAgentRunner_ApplyExecutionInput_WithEventBus(t *testing.T) {
	st := livestore.NewStateStore(t.TempDir())
	eb := NewChannelEventBus(8)
	defer eb.Close()

	var eventPayload map[string]any
	sub := eb.Subscribe(EventSystemStart, func(ctx context.Context, event BusEvent) error {
		eventPayload = event.Payload.(map[string]any)
		return nil
	})
	defer sub.Cancel()

	runner := &AgentRunner{stateStore: st, eventBus: eb}
	input := ExecutionInput{
		Regime:               domain.RegimeRiskOn,
		RawRecommendations:   []domain.Recommendation{{Agent: "agent", Symbol: "2330.TW"}},
		FinalRecommendations: []domain.Recommendation{{Agent: "agent", Symbol: "2330.TW"}},
		DeterminedBy:         "test",
	}

	err := runner.ApplyExecutionInput(context.Background(), input)
	if err != nil {
		t.Fatalf("ApplyExecutionInput failed: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	if eventPayload == nil {
		t.Fatal("expected event to be published")
	}
	if tp, ok := eventPayload["type"]; !ok || tp != "execution_input_applied" {
		t.Errorf("expected event type execution_input_applied, got %v", tp)
	}
}

// noopMetrics is a minimal MetricsRecorder for testing.
type noopMetrics struct{}

func (n *noopMetrics) RecordOrder(order domain.Order, status string)                      {}
func (n *noopMetrics) RecordPosition(position domain.Position)                            {}
func (n *noopMetrics) RecordPortfolio(cash, totalValue float64)                           {}
func (n *noopMetrics) RecordCircuitBreakerState(state string)                             {}
func (n *noopMetrics) RecordRiskEvent(eventType, symbol string)                           {}
func (n *noopMetrics) RecordCounter(name string, value float64, labels map[string]string) {}
func (n *noopMetrics) RecordGauge(name string, value float64, labels map[string]string)   {}
