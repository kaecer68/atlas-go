package orchestrator

import (
	"math"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/baseline"
	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/eventbus"
	"github.com/kaecer68/atlas-go/internal/janus"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/prism"
	"github.com/kaecer68/atlas-go/internal/replay"
	"github.com/kaecer68/atlas-go/internal/spawning"
)

type mockServiceRegistry struct {
	replay       *replay.Dataset
	registry     domain.AgentRegistry
	policy       baseline.Policy
	lastOutcomes []domain.RecommendationOutcome
	ledger       ledger.OutcomeStore
	eventBus     *eventbus.ChannelEventBus
}

func (m *mockServiceRegistry) Replay() *replay.Dataset                         { return m.replay }
func (m *mockServiceRegistry) GetRegistry() domain.AgentRegistry               { return m.registry }
func (m *mockServiceRegistry) GetPolicy() baseline.Policy                      { return m.policy }
func (m *mockServiceRegistry) GetLastOutcomes() []domain.RecommendationOutcome { return m.lastOutcomes }
func (m *mockServiceRegistry) Ledger() ledger.OutcomeStore                     { return m.ledger }
func (m *mockServiceRegistry) EventBus() *eventbus.ChannelEventBus             { return m.eventBus }

type prismTrainingExecutorStub struct {
	result prism.TrainingResult
	err    error
}

func (s *prismTrainingExecutorStub) Run(task prism.TrainingTask) (prism.TrainingResult, error) {
	if s.err != nil {
		return prism.TrainingResult{}, s.err
	}
	return s.result, nil
}

func productionSystemConfig(t *testing.T) config.Config {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test file path")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	cfg := config.Load()
	cfg.WorkDir = root
	cfg = config.Normalize(cfg)
	cfg.AgentRegistryPath = filepath.Join(root, "configs", "agents.json")
	cfg.BaselinePolicyPath = filepath.Join(root, "data", "state", "baseline_policy.json")
	cfg.ParametersConfigPath = filepath.Join(root, "configs", "parameters.json")
	cfg.ReplayDataPath = config.GetReplayDataPath(root)
	cfg.LedgerDir = t.TempDir()
	// Hermetic: JSONL backend in temp dir regardless of machine-global
	// ATLAS_STORE_BACKEND (~/.config/atlas-go/.env may default to postgres).
	cfg.StoreBackend = "jsonl"
	return cfg
}

func productionPRISMAndJANUSPlugins(t *testing.T, sys *System) (*prismPlugin, *janusPlugin) {
	t.Helper()
	if sys == nil || sys.host == nil {
		t.Fatal("expected production system with plugin host")
	}
	var pp *prismPlugin
	var jp *janusPlugin
	for _, plugin := range sys.host.plugins {
		switch p := plugin.(type) {
		case *prismPlugin:
			pp = p
		case *janusPlugin:
			jp = p
		}
	}
	if pp == nil {
		t.Fatal("expected PRISM plugin in production system")
	}
	if jp == nil {
		t.Fatal("expected JANUS plugin in production system")
	}
	return pp, jp
}

func waitForCondition(t *testing.T, timeout time.Duration, message string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal(message)
}

func TestJanusPlugin_ProcessRecommendations_NilEngine(t *testing.T) {
	p := &janusPlugin{}
	recs := []domain.Recommendation{{Symbol: "2330", Conviction: 50}}
	got := p.ProcessRecommendations(domain.RegimeRiskOn, recs)
	if len(got) != 1 || got[0].Conviction != 50 {
		t.Error("expected recs unchanged when engine is nil")
	}
}

func TestJanusPlugin_PostSimulation_NilEngine(t *testing.T) {
	p := &janusPlugin{}
	p.PostSimulation(nil, domain.RegimeRiskOn, time.Now())
}

func TestPrismPlugin_ProcessRecommendations_NilController(t *testing.T) {
	p := &prismPlugin{}
	recs := []domain.Recommendation{{Symbol: "2330", Conviction: 50}}
	got := p.ProcessRecommendations(domain.RegimeRiskOn, recs)
	if len(got) != 1 || got[0].Conviction != 50 {
		t.Error("expected recs unchanged when controller is nil")
	}
}

func TestPrismPlugin_PostSimulation_NilDeps(t *testing.T) {
	t.Run("nil manager and nil controller", func(t *testing.T) {
		p := &prismPlugin{}
		p.PostSimulation(nil, domain.RegimeRiskOn, time.Now())
	})

	t.Run("manager nil, controller has nil prismManager", func(t *testing.T) {
		ctrl := NewPhase3Controller(nil, nil, nil, nil, nil)
		p := &prismPlugin{controller: ctrl}
		p.PostSimulation(nil, domain.RegimeRiskOn, time.Now())
	})
}

func TestPrismPlugin_Attach_NilCore(t *testing.T) {
	p := &prismPlugin{}
	p.Attach(nil)
	if p.core != nil {
		t.Error("expected core to remain nil when nil is passed")
	}
}

func TestSpawningPlugin_PostSimulation_NilDeps(t *testing.T) {
	t.Run("nil manager and nil controller", func(t *testing.T) {
		p := &spawningPlugin{}
		p.PostSimulation(nil, domain.RegimeRiskOn, time.Now())
	})

	t.Run("controller fallback with nil spawningManager", func(t *testing.T) {
		ctrl := NewPhase3Controller(nil, nil, nil, nil, nil)
		p := &spawningPlugin{controller: ctrl}
		p.PostSimulation(nil, domain.RegimeRiskOn, time.Now())
	})
}

func TestPhase3Plugin_Attach_NilDeps(t *testing.T) {
	t.Run("nil controller", func(t *testing.T) {
		p := &phase3Plugin{}
		p.Attach(nil)
	})

	t.Run("controller nil core", func(t *testing.T) {
		p := &phase3Plugin{controller: &Phase3Controller{}}
		p.Attach(nil)
	})
}

func TestPhase3Plugin_PostSimulation_NilController(t *testing.T) {
	p := &phase3Plugin{}
	p.PostSimulation(nil, domain.RegimeRiskOn, time.Now())
}

func TestPhase3Plugin_PostSimulation_WithController(t *testing.T) {
	ctrl := NewPhase3Controller(nil, nil, nil, nil, nil)
	p := &phase3Plugin{controller: ctrl}

	quotes := []domain.Quote{
		{Symbol: "2330.TW", Last: 580.0, Volume: 5000000},
		{Symbol: "2317.TW", Last: 120.0, Volume: 3000000},
	}
	p.PostSimulation(quotes, domain.RegimeRiskOn, time.Now())
}

func TestJanusPlugin_PostSimulation_RegimeRouting(t *testing.T) {
	t.Run("RiskOn routes to RiskOn", func(t *testing.T) {
		eng := janus.NewEngine()
		outcomes := []domain.RecommendationOutcome{
			{Hit: true, ForwardReturn: 0.05},
			{Hit: false, ForwardReturn: -0.02},
			{Hit: true, ForwardReturn: 0.03},
		}
		mock := &mockServiceRegistry{lastOutcomes: outcomes}
		p := &janusPlugin{engine: eng, core: mock}
		p.PostSimulation(nil, domain.RegimeRiskOn, time.Now())
	})

	t.Run("RiskOff routes to RiskOff", func(t *testing.T) {
		eng := janus.NewEngine()
		mock := &mockServiceRegistry{lastOutcomes: []domain.RecommendationOutcome{
			{Hit: true, ForwardReturn: 0.01},
		}}
		p := &janusPlugin{engine: eng, core: mock}
		p.PostSimulation(nil, domain.RegimeRiskOff, time.Now())
	})

	t.Run("Neutral routes to LowVolatility", func(t *testing.T) {
		eng := janus.NewEngine()
		mock := &mockServiceRegistry{}
		p := &janusPlugin{engine: eng, core: mock}
		p.PostSimulation(nil, domain.RegimeNeutral, time.Now())
	})

	t.Run("unknown regime routes to Transition", func(t *testing.T) {
		eng := janus.NewEngine()
		mock := &mockServiceRegistry{}
		p := &janusPlugin{engine: eng, core: mock}
		p.PostSimulation(nil, "unknown_regime", time.Now())
	})
}

func TestJanusPlugin_PostSimulation_EmptyOutcomes(t *testing.T) {
	eng := janus.NewEngine()
	mock := &mockServiceRegistry{lastOutcomes: []domain.RecommendationOutcome{}}
	p := &janusPlugin{engine: eng, core: mock}
	p.PostSimulation(nil, domain.RegimeRiskOn, time.Now())
}

func TestNewProductionSystem_StartsPRISMWorkers(t *testing.T) {
	sys, err := NewProductionSystem(productionSystemConfig(t))
	if err != nil {
		t.Fatalf("NewProductionSystem() error = %v", err)
	}

	pp, _ := productionPRISMAndJANUSPlugins(t, sys)
	defer pp.manager.Stop()

	pp.manager.WithExecutor(&prismTrainingExecutorStub{result: prism.TrainingResult{
		HitRate:      0.75,
		SharpeRatio:  1.8,
		TotalReturn:  0.14,
		SignalsCount: 12,
	}})

	err = pp.manager.ScheduleTraining(domain.AgentSpec{
		ID:      "test-agent",
		Skill:   "test-skill",
		Layer:   domain.LayerSector,
		Enabled: true,
	}, []prism.TrainingWindow{{
		Start:     time.Now().Add(-time.Hour),
		End:       time.Now(),
		Regime:    prism.RegimeRiskOn,
		RegimeSet: true,
	}})
	if err != nil {
		t.Fatalf("ScheduleTraining() error = %v", err)
	}

	waitForCondition(t, 2*time.Second, "expected production PRISM workers to process scheduled task", func() bool {
		return len(pp.manager.GetCompletedResults()) == 1
	})
}

func TestNewProductionSystem_JANUSConsumesCompletedPRISMResults(t *testing.T) {
	sys, err := NewProductionSystem(productionSystemConfig(t))
	if err != nil {
		t.Fatalf("NewProductionSystem() error = %v", err)
	}

	pp, jp := productionPRISMAndJANUSPlugins(t, sys)
	defer pp.manager.Stop()

	jp.engine.EnsureAllRegimes()
	jp.engine.Update()
	before := jp.engine.GetCohortWeights()[prism.RegimeRiskOn].Weight

	pp.manager.WithExecutor(&prismTrainingExecutorStub{result: prism.TrainingResult{
		HitRate:      0.8,
		SharpeRatio:  2.2,
		TotalReturn:  0.19,
		SignalsCount: 16,
	}})

	err = pp.manager.ScheduleTraining(domain.AgentSpec{
		ID:      "test-agent",
		Skill:   "test-skill",
		Layer:   domain.LayerSector,
		Enabled: true,
	}, []prism.TrainingWindow{{
		Start:     time.Now().Add(-time.Hour),
		End:       time.Now(),
		Regime:    prism.RegimeRiskOn,
		RegimeSet: true,
	}})
	if err != nil {
		t.Fatalf("ScheduleTraining() error = %v", err)
	}

	waitForCondition(t, 2*time.Second, "expected completed PRISM result before JANUS update", func() bool {
		return len(pp.manager.GetCompletedResults()) == 1
	})

	jp.PostSimulation(nil, domain.RegimeRiskOn, time.Now())

	after := jp.engine.GetCohortWeights()[prism.RegimeRiskOn].Weight
	if after <= before {
		t.Fatalf("expected JANUS to incorporate completed PRISM results, before=%.4f after=%.4f", before, after)
	}
	jp.PostSimulation(nil, domain.RegimeRiskOn, time.Now())
	again := jp.engine.GetCohortWeights()[prism.RegimeRiskOn].Weight
	if math.Abs(again-after) > 1e-9 {
		t.Fatalf("expected JANUS bridge to avoid double-counting PRISM results, after=%.4f again=%.4f", after, again)
	}
}

func TestPrismPlugin_PostSimulation_WithManager(t *testing.T) {
	pm := prism.NewPRISMManager(prism.DefaultPRISMConfig())
	agents := domain.AgentRegistry{
		Agents: []domain.AgentSpec{
			{ID: "semi-desk-01", Enabled: true},
			{ID: "shipping-desk-01", Enabled: false},
			{ID: "value-desk-01", Enabled: true},
		},
	}
	mock := &mockServiceRegistry{registry: agents}
	p := &prismPlugin{manager: pm, core: mock}
	p.PostSimulation(nil, domain.RegimeRiskOn, time.Now())
}

func TestPrismPlugin_Attach_WithReplay(t *testing.T) {
	ds := &replay.Dataset{
		ByDate: map[string]map[string]domain.DailyBar{},
		Dates:  []time.Time{time.Now()},
	}
	pm := prism.NewPRISMManager(prism.DefaultPRISMConfig())
	mock := &mockServiceRegistry{replay: ds}
	p := &prismPlugin{manager: pm}
	p.Attach(mock)
	if p.core == nil {
		t.Error("expected core to be set")
	}
}

func TestPhase3Plugin_Attach_WithReplay(t *testing.T) {
	ds := &replay.Dataset{
		ByDate: map[string]map[string]domain.DailyBar{},
		Dates:  []time.Time{time.Now()},
	}
	ctrl := NewPhase3Controller(nil, nil, nil, nil, nil)
	mock := &mockServiceRegistry{replay: ds}
	p := &phase3Plugin{controller: ctrl}
	p.Attach(mock)
}

func TestSpawningPlugin_PostSimulation_WithManager(t *testing.T) {
	reg := &domain.AgentRegistry{}
	sm := spawning.NewSpawningManager(reg, spawning.DefaultSpawningConfig())
	p := &spawningPlugin{manager: sm}
	p.PostSimulation(nil, domain.RegimeRiskOn, time.Now())
}

func TestPrismPlugin_ProcessRecommendations_WithController(t *testing.T) {
	ctrl := NewPhase3Controller(nil, nil, nil, nil, nil)
	p := &prismPlugin{controller: ctrl}
	recs := []domain.Recommendation{{Symbol: "2330", Conviction: 50}}
	got := p.ProcessRecommendations(domain.RegimeRiskOn, recs)
	if len(got) != 1 || got[0].Conviction != 50 {
		t.Error("expected recs unchanged when prismManager is nil")
	}
}

func TestPrismPlugin_PostSimulation_RegimeCoverage(t *testing.T) {
	t.Run("RiskOff regime with manager", func(t *testing.T) {
		pm := prism.NewPRISMManager(prism.DefaultPRISMConfig())
		agents := domain.AgentRegistry{
			Agents: []domain.AgentSpec{
				{ID: "semi-desk-01", Enabled: true},
				{ID: "shipping-desk-01", Enabled: false},
				{ID: "value-desk-01", Enabled: true},
			},
		}
		mock := &mockServiceRegistry{registry: agents}
		p := &prismPlugin{manager: pm, core: mock}
		p.PostSimulation(nil, domain.RegimeRiskOff, time.Now())
	})

	t.Run("Neutral regime with manager", func(t *testing.T) {
		pm := prism.NewPRISMManager(prism.DefaultPRISMConfig())
		mock := &mockServiceRegistry{}
		p := &prismPlugin{manager: pm, core: mock}
		p.PostSimulation(nil, domain.RegimeNeutral, time.Now())
	})

	t.Run("controller fallback with non-nil prismManager", func(t *testing.T) {
		pm := prism.NewPRISMManager(prism.DefaultPRISMConfig())
		ctrl := NewPhase3Controller(nil, nil, nil, nil, nil)
		ctrl.prismManager = pm
		mock := &mockServiceRegistry{}
		p := &prismPlugin{controller: ctrl, core: mock}
		p.PostSimulation(nil, domain.RegimeRiskOn, time.Now())
	})
}
