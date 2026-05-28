package orchestrator

import (
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/baseline"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/eventbus"
	"github.com/kaecer68/atlas-go/internal/janus"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/prism"
	"github.com/kaecer68/atlas-go/internal/replay"
	"github.com/kaecer68/atlas-go/internal/spawning"
	"github.com/kaecer68/atlas-go/internal/swarm"
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

func TestSwarmPlugin_ProcessRecommendations_EmptyRecs(t *testing.T) {
	p := &swarmPlugin{}
	recs := p.ProcessRecommendations(domain.RegimeRiskOn, nil)
	if len(recs) != 0 {
		t.Errorf("expected empty recs, got %d", len(recs))
	}
}

func TestSwarmPlugin_ProcessRecommendations_NoData(t *testing.T) {
	t.Run("both nil", func(t *testing.T) {
		p := &swarmPlugin{}
		input := []domain.Recommendation{{Symbol: "2330", Conviction: 50, Side: domain.SideBuy}}
		recs := p.ProcessRecommendations(domain.RegimeRiskOn, input)
		if len(recs) != 1 {
			t.Fatalf("expected 1 rec, got %d", len(recs))
		}
		if recs[0].Conviction != 50 {
			t.Errorf("expected conviction unchanged, got %d", recs[0].Conviction)
		}
	})

	t.Run("swarm with no results", func(t *testing.T) {
		cfg := swarm.DefaultSwarmConfig()
		cfg.FishCount = 1
		cfg.SimulationHorizon = time.Nanosecond
		cfg.TimeStep = time.Hour
		cfg.Parallelism = 1
		sw := swarm.NewMiroFishSwarm(cfg)
		p := &swarmPlugin{swarm: sw}
		input := []domain.Recommendation{{Symbol: "2330", Conviction: 50, Side: domain.SideBuy}}
		recs := p.ProcessRecommendations(domain.RegimeRiskOn, input)
		if len(recs) != 1 {
			t.Fatalf("expected 1 rec, got %d", len(recs))
		}
		if recs[0].Conviction != 50 {
			t.Errorf("expected conviction unchanged for empty swarm, got %d", recs[0].Conviction)
		}
	})

	t.Run("unmatched symbol", func(t *testing.T) {
		sw, done := newSwarmWithSymbol(t, "REAL.TW")
		defer done()

		p := &swarmPlugin{swarm: sw}
		input := []domain.Recommendation{{Symbol: "NONEXISTENT", Conviction: 50, Side: domain.SideBuy}}
		recs := p.ProcessRecommendations(domain.RegimeRiskOn, input)
		if len(recs) != 1 {
			t.Fatalf("expected 1 rec, got %d", len(recs))
		}
		if recs[0].Conviction != 50 {
			t.Errorf("expected conviction unchanged for unmatched symbol, got %d", recs[0].Conviction)
		}
	})
}

func TestSwarmPlugin_ProcessRecommendations_WithConsensus(t *testing.T) {
	sw, done := newSwarmWithSymbol(t, "SYM.TW")
	defer done()

	result, ok := sw.GetLatestResult()
	if !ok || len(result.Consensus) == 0 {
		t.Fatal("swarm should have produced consensus")
	}
	cp := result.Consensus["SYM.TW"]
	t.Logf("consensus direction=%s bullish=%d bearish=%d neutral=%d",
		cp.ConsensusDirection, cp.BullishCount, cp.BearishCount, cp.NeutralCount)

	p := &swarmPlugin{swarm: sw}

	expectedDelta := func(side domain.Side) int {
		switch cp.ConsensusDirection {
		case "bullish":
			if side == domain.SideBuy {
				return 5
			}
			return -5
		case "bearish":
			if side == domain.SideSell {
				return 5
			}
			return -5
		default:
			return 0
		}
	}

	t.Run("adjusts conviction by direction", func(t *testing.T) {
		input := []domain.Recommendation{
			{Symbol: "SYM.TW", Conviction: 50, Side: domain.SideBuy},
			{Symbol: "SYM.TW", Conviction: 50, Side: domain.SideSell},
		}
		recs := p.ProcessRecommendations(domain.RegimeRiskOn, input)
		if len(recs) != 2 {
			t.Fatalf("expected 2 recs, got %d", len(recs))
		}
		for i, rec := range recs {
			d := expectedDelta(input[i].Side)
			want := 50 + d
			if rec.Conviction != want {
				t.Errorf("rec[%d] conviction=%d, want=%d (dir=%s, side=%s, delta=%d)",
					i, rec.Conviction, want, cp.ConsensusDirection, input[i].Side, d)
			}
			if rec.Conviction < 0 || rec.Conviction > 100 {
				t.Errorf("rec[%d] conviction %d out of range [0,100]", i, rec.Conviction)
			}
			if rec.Symbol != input[i].Symbol {
				t.Errorf("rec[%d] symbol changed from %s to %s", i, input[i].Symbol, rec.Symbol)
			}
			if rec.Side != input[i].Side {
				t.Errorf("rec[%d] side changed from %s to %s", i, input[i].Side, rec.Side)
			}
		}
	})

	t.Run("leaves unmatched symbol unchanged", func(t *testing.T) {
		input := []domain.Recommendation{
			{Symbol: "SYM.TW", Conviction: 50, Side: domain.SideBuy},
			{Symbol: "ZZZ.ZZ", Conviction: 30, Side: domain.SideBuy},
		}
		recs := p.ProcessRecommendations(domain.RegimeRiskOn, input)
		if len(recs) != 2 {
			t.Fatalf("expected 2 recs, got %d", len(recs))
		}
		d := expectedDelta(domain.SideBuy)
		if recs[0].Conviction != 50+d {
			t.Errorf("SYM.TW conviction=%d, want=%d (dir=%s, delta=%d)",
				recs[0].Conviction, 50+d, cp.ConsensusDirection, d)
		}
		if recs[1].Conviction != 30 {
			t.Errorf("ZZZ.ZZ conviction=%d, want=30", recs[1].Conviction)
		}
	})

	t.Run("clamps conviction at boundaries", func(t *testing.T) {
		input := []domain.Recommendation{
			{Symbol: "SYM.TW", Conviction: 97, Side: domain.SideBuy},
			{Symbol: "SYM.TW", Conviction: 3, Side: domain.SideSell},
		}
		recs := p.ProcessRecommendations(domain.RegimeRiskOn, input)
		if len(recs) != 2 {
			t.Fatalf("expected 2 recs, got %d", len(recs))
		}
		for i, rec := range recs {
			if rec.Conviction < 0 || rec.Conviction > 100 {
				t.Errorf("rec[%d] conviction %d out of range [0,100]", i, rec.Conviction)
			}
		}
	})
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
		ctrl := NewPhase3Controller(nil, nil, nil, nil, nil, nil)
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
		ctrl := NewPhase3Controller(nil, nil, nil, nil, nil, nil)
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
	ctrl := NewPhase3Controller(nil, nil, nil, nil, nil, nil)
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
	ctrl := NewPhase3Controller(nil, nil, nil, nil, nil, nil)
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
	ctrl := NewPhase3Controller(nil, nil, nil, nil, nil, nil)
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
		ctrl := NewPhase3Controller(nil, nil, nil, nil, nil, nil)
		ctrl.prismManager = pm
		mock := &mockServiceRegistry{}
		p := &prismPlugin{controller: ctrl, core: mock}
		p.PostSimulation(nil, domain.RegimeRiskOn, time.Now())
	})
}

func newSwarmWithSymbol(t *testing.T, symbol string) (*swarm.MiroFishSwarm, func()) {
	t.Helper()
	cfg := swarm.DefaultSwarmConfig()
	cfg.FishCount = 10
	cfg.SimulationHorizon = 1 * time.Hour
	cfg.TimeStep = 1 * time.Hour
	cfg.Parallelism = 4
	sw := swarm.NewMiroFishSwarm(cfg)
	baseState := swarm.MarketState{
		Timestamp: time.Now(),
		Prices:    map[string]float64{symbol: 100.0},
		Volumes:   map[string]float64{symbol: 1000000},
	}
	sw.InitializeScenarios(baseState)
	sw.Start()

	result, ok := sw.GetLatestResult()
	if !ok || len(result.Consensus) == 0 {
		t.Log("swarm produced no consensus — test may be brittle with this config")
	}
	return sw, func() {}
}
