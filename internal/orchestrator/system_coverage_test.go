package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/baseline"
	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/eventbus"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/portfolio"
	"github.com/kaecer68/atlas-go/internal/replay"
	"github.com/kaecer68/atlas-go/internal/risk"
	"github.com/kaecer68/atlas-go/internal/sim"
	"github.com/kaecer68/atlas-go/internal/strategy"
)

func newTestSystemFull(t *testing.T) *System {
	t.Helper()
	reg := SeedRegistry()
	return &System{
		SystemCore: &SystemCore{
			sim: SimulationCore{
				cfg:      config.Config{PrimaryMarket: "TW"},
				provider: marketdata.NewMockProvider(),
				engine:   sim.NewEngine(domain.SimulationConstraints{StartingCash: 1_000_000}),
				registry: reg,
				policy:   baseline.Policy{ExecutionPolicy: domain.ExecutionPolicy{RequireCROPass: true}},
				ledger:   ledger.NewStore(t.TempDir()),
				replay:   &replay.Dataset{},
				session:  domain.ReplaySession{ID: "session-test", SessionDate: time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)},
			},
			port: PortfolioManager{
				factorWeightEngine: portfolio.NewFactorWeightEngine(),
			},
			strat: StrategyLayer{
				strategyRegistry:  strategy.NewRegistryWithDefaults(),
				strategySelector:  strategy.NewSelector(strategy.NewRegistryWithDefaults(), strategy.NewComparisonEngine(20, nil)),
				strategyAllocator: strategy.NewStrategyAllocator(strategy.NewRegistryWithDefaults()),
				thresholdEngine:   sim.NewDynamicThresholdEngine(),
				strategyEvolver:   nil,
			},
			risk: RiskOps{
				eventBus: eventbus.NewChannelEventBus(16),
			},
			plugins: NewPluginRegistry(),
		},
	}
}

func TestSystem_Getters(t *testing.T) {
	sys := newTestSystemFull(t)

	if got := sys.Registry(); got.Version == 0 {
		t.Error("Registry() returned empty registry")
	}
	if got := sys.GetPlugins(); got == nil {
		t.Error("GetPlugins() returned nil")
	}
	if got := sys.GetExecutionPolicy(); !got.RequireCROPass {
		t.Error("GetExecutionPolicy() did not return expected policy")
	}
	if got := sys.GetStrategySelector(); got == nil {
		t.Error("GetStrategySelector() returned nil")
	}
	if got := sys.GetStrategyAllocator(); got == nil {
		t.Error("GetStrategyAllocator() returned nil")
	}
	if got := sys.GetStrategyEvolver(); got != nil {
		t.Error("GetStrategyEvolver() expected nil")
	}
	if got := sys.GetThresholdEngine(); got == nil {
		t.Error("GetThresholdEngine() returned nil")
	}
	if got := sys.GetCurrentStrategy(); got != nil {
		t.Error("GetCurrentStrategy() expected nil before selection")
	}
}

func TestSystem_GetCurrentStrategy_AfterSelection(t *testing.T) {
	sys := newTestSystemFull(t)
	_, _ = sys.strat.strategySelector.Select(context.Background(), 20.0, domain.RegimeRiskOn)
	if got := sys.GetCurrentStrategy(); got == nil {
		t.Error("GetCurrentStrategy() returned nil after selection")
	}
}

func TestSystem_Setters(t *testing.T) {
	sys := newTestSystemFull(t)

	eb := eventbus.NewChannelEventBus(8)
	sys.SetEventBus(eb)
	if sys.Risk().eventBus != eb {
		t.Error("SetEventBus did not assign event bus")
	}

	called := false
	sys.SetDrawdownReporter(func(portfolio.DrawdownResult) { called = true })
	if sys.drawdownReporter == nil {
		t.Fatal("SetDrawdownReporter did not assign reporter")
	}
	sys.drawdownReporter(portfolio.DrawdownResult{})
	if !called {
		t.Error("Drawdown reporter was not invoked")
	}

	sys.SetVerboseTrace(true)
	if !sys.traceVerbose {
		t.Error("SetVerboseTrace did not enable trace")
	}
}

func TestSystem_LifecycleSetters(t *testing.T) {
	sys := newTestSystemFull(t)

	mt := &domain.MaturityTracker{}
	sys.WithMaturityTracker(mt)
	if sys.MaturityTracker() != mt {
		t.Error("WithMaturityTracker did not assign tracker")
	}

	sys.WithStrategyEvolver(&StrategyEvolver{})
	if sys.GetStrategyEvolver() == nil {
		t.Error("WithStrategyEvolver did not assign evolver")
	}

	newAlloc := strategy.NewStrategyAllocator(strategy.NewRegistryWithDefaults())
	sys.WithStrategyAllocator(newAlloc)
	if sys.GetStrategyAllocator() != newAlloc {
		t.Error("WithStrategyAllocator did not replace allocator")
	}

	cfg := domain.CapitalPhaseConfig{CurrentPhase: domain.PhaseSimulation}
	sys.WithCapitalManagement(
		risk.NewCapitalPhaseController(cfg),
		portfolio.NewCapitalAllocator(),
		func() *risk.ApprovalWorkflow { w, _ := risk.NewApprovalWorkflow(t.TempDir()); return w }(),
	)
	if sys.Risk().capitalController == nil {
		t.Error("WithCapitalManagement did not assign capital controller")
	}

	mc := &screeningMetricsCollector{}
	sys.WithMetricsCollector(mc)
	if sys.Risk().metricsCollector != mc {
		t.Error("WithMetricsCollector did not assign collector")
	}

	sys.SetRepository(nil)
}

func TestSystemCore_ServiceRegistry(t *testing.T) {
	sys := newTestSystemFull(t)

	if got := sys.Replay(); got == nil {
		t.Error("Replay() returned nil")
	}
	if got := sys.GetRegistry(); got.Version == 0 {
		t.Error("GetRegistry() returned empty registry")
	}
	if got := sys.GetPolicy(); !got.ExecutionPolicy.RequireCROPass {
		t.Error("GetPolicy() did not return expected policy")
	}
	if got := sys.GetLastOutcomes(); got != nil {
		t.Error("GetLastOutcomes() expected nil")
	}
	if got := sys.Ledger(); got == nil {
		t.Error("Ledger() returned nil")
	}
	if got := sys.EventBus(); got == nil {
		t.Error("EventBus() returned nil")
	}
	if got := sys.Sim(); got == nil {
		t.Error("Sim() returned nil")
	}
	if got := sys.Port(); got == nil {
		t.Error("Port() returned nil")
	}
	if got := sys.Risk(); got == nil {
		t.Error("Risk() returned nil")
	}
}

func TestPortfolioManager_FactorWeightEngine(t *testing.T) {
	pm := &PortfolioManager{factorWeightEngine: portfolio.NewFactorWeightEngine()}
	if got := pm.FactorWeightEngine(); got == nil {
		t.Error("FactorWeightEngine() returned nil")
	}
}

func TestSystem_resolveReplayDate(t *testing.T) {
	sys := newTestSystemFull(t)
	// No replay data
	if _, ok := sys.resolveReplayDate(); ok {
		t.Error("resolveReplayDate should return false when replay is nil")
	}

	// With ReplaySessionDate configured
	sys.Sim().replay = &replay.Dataset{Dates: []time.Time{time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC)}}
	sys.Sim().cfg.ReplaySessionDate = "2026-06-10"
	if date, ok := sys.resolveReplayDate(); !ok || date.Format("2006-01-02") != "2026-06-10" {
		t.Errorf("resolveReplayDate with explicit date = %v, ok=%v", date, ok)
	}

	// Invalid explicit date falls through to dataset logic
	sys.Sim().cfg.ReplaySessionDate = "invalid"
	if date, ok := sys.resolveReplayDate(); !ok || date.Format("2006-01-02") != "2026-06-10" {
		t.Errorf("resolveReplayDate fallback = %v, ok=%v", date, ok)
	}
}

func TestQuotesToMacroDataSnapshot_Coverage(t *testing.T) {
	quotes := []domain.Quote{
		{Symbol: "US10Y", Open: 4.0, Last: 4.2},
		{Symbol: "DXY", Open: 100.0, Last: 101.0},
		{Symbol: "VIX", Open: 15.0, Last: 18.0},
		{Symbol: "USD/TWD", Open: 31.5, Last: 31.8},
		{Symbol: "OIL", Open: 70.0, Last: 72.0},
		{Symbol: "GOLD", Open: 2000.0, Last: 2050.0},
		{Symbol: "JPY=X", Open: 150.0, Last: 145.0},
	}
	snap := QuotesToMacroDataSnapshot(quotes)
	if snap.US10Y.Value == 0 {
		t.Error("US10Y not captured")
	}
	if snap.DXY.Value == 0 {
		t.Error("DXY not captured")
	}
	if snap.VIX.Value == 0 {
		t.Error("VIX not captured")
	}
	if snap.USD_TWD.Value == 0 {
		t.Error("USD_TWD not captured")
	}
	if snap.Oil.Value == 0 {
		t.Error("Oil not captured")
	}
	if snap.Gold.Value == 0 {
		t.Error("Gold not captured")
	}
	if snap.JPY.Value == 0 {
		t.Error("JPY not captured")
	}
}

func TestSimulationCore_SetProvider(t *testing.T) {
	sc := &SimulationCore{}
	p := marketdata.NewMockProvider()
	sc.SetProvider(p)
	if sc.provider != p {
		t.Error("SetProvider did not assign provider")
	}
}

func TestSimulationCore_RefreshETFNAV_NilDeps(t *testing.T) {
	sc := &SimulationCore{}
	if got := sc.RefreshETFNAV(context.TODO()); got != 0 {
		t.Errorf("RefreshETFNAV with nil deps = %d, want 0", got)
	}
}

func TestLoadRecOverrides(t *testing.T) {
	if got := loadRecOverrides(nil); got != nil {
		t.Error("loadRecOverrides(nil) expected nil")
	}

	store := &humanInterventionStore{
		interventions: []domain.HumanIntervention{
			{Type: "approve_rec", TargetAgentID: "a1", TargetSymbol: "2330.TW"},
			{Type: "reject_rec", TargetAgentID: "a2", TargetSymbol: "2317.TW"},
			{Type: "unknown", TargetAgentID: "a3", TargetSymbol: "2881.TW"},
		},
	}
	overrides := loadRecOverrides(store)
	if overrides["a1:2330.TW"] != "approved" {
		t.Error("expected approved override")
	}
	if overrides["a2:2317.TW"] != "rejected" {
		t.Error("expected rejected override")
	}
	if _, ok := overrides["a3:2881.TW"]; ok {
		t.Error("unknown intervention type should not produce override")
	}
}

type screeningMetricsCollector struct {
	passed, rejected int64
}

func (c *screeningMetricsCollector) RecordScreening(passed, rejected int64) {
	c.passed = passed
	c.rejected = rejected
}

type humanInterventionStore struct {
	interventions []domain.HumanIntervention
	err           error
}

func (s *humanInterventionStore) LoadHumanInterventions() ([]domain.HumanIntervention, error) {
	return s.interventions, s.err
}

func (s *humanInterventionStore) RecordHumanIntervention(domain.HumanIntervention) error { return nil }

func (s *humanInterventionStore) RecordOutcomes([]domain.RecommendationOutcome) error { return nil }

func (s *humanInterventionStore) RecordSessionOutcomes(domain.ReplaySession, []domain.RecommendationOutcome) error {
	return nil
}

func (s *humanInterventionStore) LoadOutcomes() ([]domain.RecommendationOutcome, error) {
	return nil, nil
}

func (s *humanInterventionStore) LoadSessionOutcomes(string) ([]domain.RecommendationOutcome, error) {
	return nil, nil
}

func (s *humanInterventionStore) LoadOutcomesFromSessions() ([]domain.RecommendationOutcome, error) {
	return nil, nil
}

func (s *humanInterventionStore) RecordSessionScreeningRejects(string, []domain.ScreeningReject) error {
	return nil
}

func (s *humanInterventionStore) LoadSessionScreeningRejects(string) ([]domain.ScreeningReject, error) {
	return nil, nil
}

func (s *humanInterventionStore) RecordSessionTrades(string, []domain.TradeRecord) error { return nil }

func (s *humanInterventionStore) LoadSessionTrades(string) ([]domain.TradeRecord, error) {
	return nil, nil
}

func (s *humanInterventionStore) LoadAllSessionTrades() ([]domain.TradeRecord, error) {
	return nil, nil
}
func (s *humanInterventionStore) RecordExperiment(domain.ExperimentRecord) error { return nil }
func (s *humanInterventionStore) RecordSessionExperiment(domain.ReplaySession, domain.ExperimentRecord) error {
	return nil
}

func (s *humanInterventionStore) RecordSessionSummary(domain.ReplaySession, domain.SessionSummary) error {
	return nil
}

func (s *humanInterventionStore) LoadSessionSummaries() ([]domain.SessionSummary, error) {
	return nil, nil
}

func (s *humanInterventionStore) LoadAllSessionScorecards() ([]domain.Scorecard, []domain.RecommendationOutcome, error) {
	return nil, nil, nil
}
