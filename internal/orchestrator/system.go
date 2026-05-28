package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/kaecer68/atlas-go/internal/baseline"
	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/domain/shared"
	"github.com/kaecer68/atlas-go/internal/eventbus"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/logging"
	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/narrative"
	"github.com/kaecer68/atlas-go/internal/portfolio"
	"github.com/kaecer68/atlas-go/internal/replay"
	"github.com/kaecer68/atlas-go/internal/repository"
	"github.com/kaecer68/atlas-go/internal/risk"
	"github.com/kaecer68/atlas-go/internal/sim"
	"github.com/kaecer68/atlas-go/internal/strategy"
	"github.com/kaecer68/atlas-go/internal/stress"
)

// SystemCore holds the essential simulation state and services.
type SimulationCore struct {
	cfg             config.Config
	provider        marketdata.Provider
	engine          *sim.Engine
	registry        domain.AgentRegistry
	policy          baseline.Policy
	ledger          ledger.OutcomeStore
	replay          *replay.Dataset
	session         domain.ReplaySession
	persistentState *domain.SimulationState
	ctx             context.Context
	scratchpad      *Scratchpad

	lastOutcomes     []domain.RecommendationOutcome
	portfolioHistory []float64
	returnHistory    []float64
	lastQuotes       []domain.Quote
}

type PortfolioManager struct {
	alphaDiscovery     *AlphaDiscoveryEngine
	darwinian          *portfolio.DarwinianWeightManager
	capitalAllocator   *portfolio.CapitalAllocator
	factorWeightEngine *portfolio.FactorWeightEngine
}

func (pm *PortfolioManager) FactorWeightEngine() *portfolio.FactorWeightEngine {
	return pm.factorWeightEngine
}

type StrategyLayer struct {
	strategyRegistry  *strategy.Registry
	strategySelector  *strategy.Selector
	comparisonEngine  *strategy.ComparisonEngine
	thresholdEngine   *sim.DynamicThresholdEngine
	strategyAllocator *strategy.StrategyAllocator // P2: nil-safe multi-strategy allocator
	strategyEvolver   *StrategyEvolver            // nil-safe: no evolution when nil
}

type RiskOps struct {
	capitalController     *risk.CapitalPhaseController
	approvalWorkflow      *risk.ApprovalWorkflow
	metricsCollector      interface{ RecordScreening(passed, rejected int64) }
	eventBus              *eventbus.ChannelEventBus
	clampingLogger        *clampingLogger
	repo                  repository.OutcomeRepository
	macroRiskEngine       *narrative.MacroRiskAssessmentEngine
	structuralTrendEngine *narrative.StructuralTrendEngine
	macroDrawdownEngine   *risk.MacroAwareDrawdownEngine
	sectorDataProvider    *marketdata.SectorDataProvider
}

type SystemCore struct {
	sim   SimulationCore
	port  PortfolioManager
	strat StrategyLayer
	risk  RiskOps

	plugins         *PluginRegistry
	narrativeEngine *narrative.NarrativeEngine
}

func (sc *SystemCore) Sim() *SimulationCore    { return &sc.sim }
func (sc *SystemCore) Port() *PortfolioManager { return &sc.port }
func (sc *SystemCore) Risk() *RiskOps          { return &sc.risk }

// ServiceRegistry interface implementation for SystemCore
func (s *SystemCore) Replay() *replay.Dataset                         { return s.Sim().replay }
func (s *SystemCore) GetRegistry() domain.AgentRegistry               { return s.Sim().registry }
func (s *SystemCore) GetPolicy() baseline.Policy                      { return s.Sim().policy }
func (s *SystemCore) GetLastOutcomes() []domain.RecommendationOutcome { return s.Sim().lastOutcomes }
func (s *SystemCore) Ledger() ledger.OutcomeStore                     { return s.Sim().ledger }
func (s *SystemCore) EventBus() *eventbus.ChannelEventBus             { return s.Risk().eventBus }

func (s *System) SetEventBus(eventBus *eventbus.ChannelEventBus) {
	s.Risk().eventBus = eventBus
}

// SetDrawdownReporter registers a callback for drawdown simulation results.
func (s *System) SetDrawdownReporter(fn func(portfolio.DrawdownResult)) {
	s.drawdownMu.Lock()
	defer s.drawdownMu.Unlock()
	s.drawdownReporter = fn
}

// System orchestrates the full simulation loop via a SystemCore and a PluginHost.
type System struct {
	*SystemCore
	host             *PluginHost
	macroSnapshot    *marketdata.MacroDataSnapshot
	drawdownMu       sync.RWMutex
	drawdownReporter func(portfolio.DrawdownResult)
	traceVerbose     bool // when true, SimTraceWriter emits color-coded terminal output
	phase3Ctrl       *Phase3Controller
}

// Phase3Controller returns the Phase 3 optimization controller, if attached.
func (s *System) Phase3Controller() *Phase3Controller { return s.phase3Ctrl }

// SetVerboseTrace enables or disables color-coded verbose trace output.
func (s *System) SetVerboseTrace(v bool) { s.traceVerbose = v }

// SystemOption configures optional subsystems during System construction.
// Use the With* functions to create options.
type SystemOption func(*systemOptions)

type systemOptions struct {
	executorLoader ExecutorLoader
}

// WithExecutorLoader injects a custom ExecutorLoader for loading executor
// implementations. When nil (default), NewPluginRegistry uses StaticLoader{}
// for full backward compatibility.
func WithExecutorLoader(loader ExecutorLoader) SystemOption {
	return func(o *systemOptions) { o.executorLoader = loader }
}

// NewSystem builds a fully-wired System with an internally-created EventBus.
func NewSystem(cfg config.Config, opts ...SystemOption) (*System, error) {
	return NewSystemWithEventBus(cfg, nil, opts...)
}

// NewSystemWithEventBus builds a fully-wired System using the provided EventBus.
// If eventBus is nil, a new internal EventBus is created (backward-compatible).
func NewSystemWithEventBus(cfg config.Config, eventBus *eventbus.ChannelEventBus, opts ...SystemOption) (*System, error) {
	var o systemOptions
	for _, opt := range opts {
		opt(&o)
	}
	var registry domain.AgentRegistry
	var err error
	if len(cfg.AgentRegistryExtraPaths) > 0 {
		allPaths := append([]string{cfg.AgentRegistryPath}, cfg.AgentRegistryExtraPaths...)
		registry, err = LoadRegistryMulti(allPaths...)
	} else {
		registry, err = LoadRegistry(cfg.AgentRegistryPath)
	}
	if err != nil {
		registry = SeedRegistry()
	}
	policy, err := baseline.Load(cfg.BaselinePolicyPath)
	if err != nil {
		policy = baseline.DefaultPolicy()
	}
	ds, _ := replay.LoadTWSEOpenDataCSV(cfg.ReplayDataPath)

	runtimeParams := loadRuntimeParamsOrDefault(cfg.ParametersConfigPath)
	macroSnapshot := &marketdata.MacroDataSnapshot{}
	factorEngine, hp, fp := buildFactorEngine(runtimeParams, macroSnapshot, cfg.ReplayDataPath)
	if eventBus == nil {
		eventBus = eventbus.NewChannelEventBus(256)
	}
	plugins := buildPluginRegistry(factorEngine, fp, o.executorLoader)

	optimizer := portfolio.NewOptimizer()
	optimizer.WithHistoricalPrices(hp).WithFundamentalProvider(fp).WithFactorEngine(factorEngine)
	thresholdEngine := sim.NewDynamicThresholdEngine()
	store, err := ledger.NewOutcomeStore(cfg)
	if err != nil {
		return nil, fmt.Errorf("create store: %w", err)
	}

	macroRiskEngine, structuralTrendEngine, macroDrawdownEngine, sectorDataProvider := buildMacroEngines(cfg.LedgerDir)

	sys := &System{
		SystemCore: &SystemCore{
			sim:             buildSimulationCore(cfg, registry, policy, ds, optimizer, store),
			port:            buildPortfolioManager(runtimeParams, registry, eventBus, factorEngine),
			strat:           buildStrategyLayer(thresholdEngine),
			risk:            buildRiskOps(cfg, eventBus, macroRiskEngine, structuralTrendEngine, macroDrawdownEngine, sectorDataProvider),
			plugins:         plugins,
			narrativeEngine: narrative.NewNarrativeEngine(),
		},
		macroSnapshot: macroSnapshot,
	}

	sys.Sim().scratchpad = NewScratchpad(sys.Sim().session.ID, cfg.LedgerDir)

	// Wire factor weight engine to event bus for self-evolution.
	if port := sys.port; port.factorWeightEngine != nil && eventBus != nil {
		eventBus.Subscribe(eventbus.EventRegimeChange, func(_ context.Context, e eventbus.BusEvent) error {
			if p, ok := e.Payload.(eventbus.RegimeEventPayload); ok {
				port.factorWeightEngine.SetRegime(string(p.NewRegime))
				port.factorWeightEngine.OnRegimeChange(string(p.OldRegime), string(p.NewRegime), p.Confidence)
			}
			return nil
		})
		eventBus.Subscribe(eventbus.EventNarrative, func(_ context.Context, e eventbus.BusEvent) error {
			if p, ok := e.Payload.(eventbus.NarrativeEventPayload); ok {
				ev := &narrative.NarrativeEvent{
					ID:         p.EventID,
					Theme:      p.Theme,
					Confidence: p.Confidence,
					HitRate:    p.HitRate,
					Timestamp:  time.Now(),
					Status:     "active",
					Duration:   7 * 24 * time.Hour,
				}
				port.factorWeightEngine.AddEvent(ev)
			}
			return nil
		})
	}

	return sys, nil
}

func (s *System) RunDailySimulation(asOf time.Time) (domain.SimulationResult, error) {
	if s.Risk().eventBus != nil {
		go s.Risk().eventBus.PublishSimulationStart(s.Sim().session.ID, asOf)
	}

	if sessionDate, ok := s.resolveReplayDate(); ok && s.Sim().replay != nil {
		return s.runReplaySimulation(sessionDate)
	}

	// SimTraceWriter for pipeline layer transparency audit trail.
	tw := NewSimTraceWriter(s.Sim().cfg.WorkDir, asOf.Format("20060102"), s.traceVerbose)
	defer func() { _, _ = tw.ExportJSONL() }()
	// Wire trace writer to engine and screener for internal trace events.
	s.Sim().engine.WithTraceWriter(tw)
	if s.plugins != nil {
		s.plugins.WireScreenerTraceWriter(tw)
	}

	symbols := RegistrySymbols(s.Sim().registry)
	tw.Record(1, "data_fetch", "START", nil)
	quotes, err := s.Sim().provider.GetQuotes(s.Sim().ctx, asOf, symbols)
	if err != nil {
		tw.Record(1, "data_fetch", "FAIL", map[string]any{"error": err.Error()})
		return domain.SimulationResult{}, err
	}
	if len(quotes) == 0 {
		tw.Record(1, "data_fetch", "WARN", map[string]any{"symbols": 0})
		logging.Warn("system", "no_quotes_available",
			"session", s.Sim().session.ID,
			"as_of", asOf.Format("2006-01-02"),
			"symbols_requested", len(symbols))
	} else {
		tw.Record(1, "data_fetch", "OK", map[string]any{"symbols": len(quotes)})
	}

	if s.macroSnapshot != nil {
		*s.macroSnapshot = QuotesToMacroDataSnapshot(quotes)
		if opt := s.Sim().engine.Optimizer(); opt != nil {
			fb := portfolio.NewFactorBridge()
			opt.WithBridgeInput(fb.Convert(*s.macroSnapshot))
		}
	}

	// Pipeline trace: START for layers inside ExecuteWithContext.
	tw.Record(2, "regime_detect", "START", nil)
	tw.Record(3, "screening", "START", nil)
	tw.Record(4, "recommend", "START", nil)
	tw.Record(5, "guard_filter", "START", nil)

	events := s.detectNarrativeEvents(quotes)
	researchResult := ExecuteWithContext(ExecutionContext{
		Registry:        s.Sim().registry,
		Quotes:          quotes,
		Overrides:       s.Sim().policy.PromptOverrides,
		Policy:          s.Sim().policy.ExecutionPolicy,
		Plugins:         s.plugins,
		SessionID:       s.Sim().session.ID,
		WeightManager:   s.Port().darwinian,
		Context:         s.Sim().ctx,
		NarrativeEvents: events,
		ConvictionClampingCallback: func(evts []portfolio.ConvictionClampingEvent) {
			if s.Risk().clampingLogger != nil {
				s.Risk().clampingLogger.AppendConvictionEvents(evts)
			}
		},
		Scratchpad: s.Sim().scratchpad,
	})
	regime := researchResult.Regime
	rawRecs := researchResult.RawRecommendations
	finalRecs := researchResult.FinalRecommendations
	guardOutcomes := researchResult.GuardOutcomes
	rejects := researchResult.ScreeningRejects

	// Pipeline trace: OK/WARN for regime_detect, screening, recommend, guard_filter.
	tw.Record(2, "regime_detect", "OK", map[string]any{"regime": string(regime), "narrative_events": len(events)})
	totalScreened := len(rawRecs) + len(rejects)
	if totalScreened == 0 {
		tw.Record(3, "screening", "WARN", map[string]any{"candidates": 0})
	} else {
		tw.Record(3, "screening", "OK", map[string]any{"candidates": totalScreened, "rejected": len(rejects)})
	}
	if len(rawRecs) == 0 {
		tw.Record(4, "recommend", "WARN", map[string]any{"raw_recs": 0})
	} else {
		tw.Record(4, "recommend", "OK", map[string]any{"raw_recs": len(rawRecs), "final_recs": len(finalRecs)})
	}
	passedCnt := 0
	blockedCnt := 0
	for _, g := range guardOutcomes {
		passedCnt += g.OutputCount
		blockedCnt += g.InputCount - g.OutputCount
	}
	tw.Record(5, "guard_filter", "OK", map[string]any{"passed": passedCnt, "blocked": blockedCnt})

	// Preserve original recs for outcome building so GuardOutcomes align with outcomes.
	outcomeRawRecs := append([]domain.Recommendation(nil), rawRecs...)
	outcomeFinalRecs := append([]domain.Recommendation(nil), finalRecs...)
	oldRegime := regime
	regime = AdjustRegimeFromNarrative(regime, events)
	if s.Risk().eventBus != nil {
		go s.Risk().eventBus.PublishRegimeChange(oldRegime, regime, 0.0, "orchestrator")
		// Sync regime to factor weight engine (event subscriber handles async path).
		if s.Port().factorWeightEngine != nil {
			s.Port().factorWeightEngine.SetRegime(string(regime))
			s.Port().factorWeightEngine.OnRegimeChange(string(oldRegime), string(regime), 0.0)
		}
	}

	vix := vixFromQuotes(quotes)
	if s.strat.strategyAllocator != nil {
		mix := s.strat.strategyAllocator.Allocate(regime, vix)
		if s.Port().factorWeightEngine != nil {
			s.Port().factorWeightEngine.ApplyStrategyMix(mix, s.strat.strategyRegistry)
		}
		var topStrategy *strategy.Strategy
		var topWeight float64
		for name, w := range mix {
			if w > topWeight {
				if st, ok := s.strat.strategyRegistry.Get(name); ok {
					topStrategy = st
					topWeight = w
				}
			}
		}
		if topStrategy != nil && s.strat.thresholdEngine != nil {
			s.strat.thresholdEngine.SetRiskAppetite(sim.RiskAppetite(topStrategy.RiskAppetite))
		}
	} else if s.strat.strategySelector != nil {
		selectedStrategy, err := s.strat.strategySelector.Select(
			s.Sim().ctx,
			vix,
			regime,
		)
		if err == nil && selectedStrategy != nil {
			if s.Port().factorWeightEngine != nil {
				s.Port().factorWeightEngine.ApplyStrategy(selectedStrategy)
			}
			if s.strat.thresholdEngine != nil {
				s.strat.thresholdEngine.SetRiskAppetite(sim.RiskAppetite(selectedStrategy.RiskAppetite))
			}
		}
	}

	rawRecs = s.applyNarrativeContextWithEvents(rawRecs, events)
	finalRecs = s.applyNarrativeContextWithEvents(finalRecs, events)
	rawRecs = s.applyHumanOverrides(rawRecs)
	finalRecs = s.applyHumanOverrides(finalRecs)
	alphaRecs := s.applyAlphaDiscovery(quotes, rawRecs)
	finalRecs = append(finalRecs, alphaRecs...)
	finalRecs = s.host.ProcessRecommendations(regime, finalRecs)
	if s.Risk().eventBus != nil {
		go s.Risk().eventBus.PublishRecommendation("orchestrator", finalRecs)
	}
	if err := s.ensurePersistentStateLoaded(); err != nil {
		return domain.SimulationResult{}, fmt.Errorf("load persistent state: %w", err)
	}
	tw.Record(6, "sim_exec", "START", nil)
	var result domain.SimulationResult
	if s.Sim().persistentState != nil {
		result = s.Sim().engine.RunWithState(s.Sim().persistentState, regime, quotes, finalRecs)
	} else {
		result = s.Sim().engine.Run(regime, quotes, finalRecs)
	}
	tw.Record(6, "sim_exec", "OK", map[string]any{"orders": len(result.Orders), "positions": len(result.Positions)})
	// P3-4: Monte Carlo drawdown simulation for monitoring dashboard.
	if opt := s.Sim().engine.Optimizer(); opt != nil {
		ddResult := opt.SimulateDrawdownForMonitoring(result.Positions, result.PortfolioValue)
		logging.Info("system", "drawdown_simulation",
			logging.FFloat64("max_drawdown", ddResult.MaxDrawdown),
			logging.FFloat64("var_95", ddResult.VaR95),
			logging.FStr("session", s.Sim().session.ID))
		s.drawdownMu.RLock()
		if s.drawdownReporter != nil {
			s.drawdownReporter(ddResult)
		}
		s.drawdownMu.RUnlock()
	}
	result.GuardOutcomes = guardOutcomes
	if s.Risk().eventBus != nil {
		go s.Risk().eventBus.PublishGuardOutcomes(s.Sim().session.ID, guardOutcomes)
	}

	s.Sim().portfolioHistory = append(s.Sim().portfolioHistory, result.PortfolioValue)
	if len(s.Sim().portfolioHistory) > 1 {
		prev := s.Sim().portfolioHistory[len(s.Sim().portfolioHistory)-2]
		if prev > 0 {
			dailyReturn := (result.PortfolioValue - prev) / prev
			s.Sim().returnHistory = append(s.Sim().returnHistory, dailyReturn)
		}
	}
	if len(s.Sim().returnHistory) >= 30 {
		snap := risk.ComputeRiskSnapshot(s.Sim().returnHistory, s.Sim().portfolioHistory)
		result.RiskSnapshot = &snap
	}
	if err := s.persistPersistentState(); err != nil {
		logging.Warn("System", "failed to persist simulation state", "session_id", s.Sim().session.ID, "err", err)
	}
	s.Sim().lastQuotes = quotes
	s.updateCapitalMetrics(s.Sim().ctx, result)

	tw.Record(7, "ledger_write", "START", nil)
	outcomes := buildSyntheticOutcomes(outcomeRawRecs, outcomeFinalRecs, quotes, asOf)
	// Write outcomes to ALL stores: PostgreSQL (if available), global file, and per-session file.
	// The XOR pattern was removed because DualWriteRepository already handles DB ↔ file sync.
	if s.Risk().repo != nil {
		_ = s.Risk().repo.RecordOutcomes(s.Sim().ctx, outcomes)
	}
	_ = s.Sim().ledger.RecordOutcomes(outcomes)
	_ = s.Sim().ledger.RecordSessionOutcomes(s.Sim().session, outcomes)
	_ = s.Sim().ledger.RecordSessionScreeningRejects(s.Sim().session.ID, rejects)
	if s.Risk().metricsCollector != nil {
		s.Risk().metricsCollector.RecordScreening(int64(len(rawRecs)), int64(len(rejects)))
	}
	s.Sim().lastOutcomes = outcomes
	tw.Record(7, "ledger_write", "OK", map[string]any{"outcomes": len(outcomes)})

	if s.Port().darwinian != nil {
		for _, outcome := range outcomes {
			s.Port().darwinian.RecordOutcome(outcome.AgentID, outcome.ForwardReturn, outcome.Hit)
		}
		_, clampingEvents := s.Port().darwinian.PerformDailyAdjustment()
		_ = s.Port().darwinian.Save()
		_ = s.Port().darwinian.AppendSnapshot()
		// Publish clamping events for monitoring and audit trail
		if len(clampingEvents) > 0 && s.Risk().eventBus != nil {
			payloads := make([]eventbus.ClampingEventPayload, len(clampingEvents))
			for i, e := range clampingEvents {
				payloads[i] = eventbus.ClampingEventPayload{
					AgentID:     e.AgentID,
					RawWeight:   e.RawWeight,
					FinalWeight: e.FinalWeight,
					Boundary:    e.Boundary,
					Timestamp:   e.Timestamp,
				}
			}
			go s.Risk().eventBus.PublishDarwinianClamping(payloads)
			if s.Risk().clampingLogger != nil {
				for _, p := range payloads {
					s.Risk().clampingLogger.Append(p)
				}
			}
		}
	}

	s.host.PostSimulation(quotes, regime, asOf)

	if s.Risk().eventBus != nil {
		go s.Risk().eventBus.PublishSimulationComplete(s.Sim().session.ID, result.PortfolioValue, len(result.Orders), len(result.Positions))
	}

	if s.Sim().scratchpad != nil {
		// Add portfolio summary trace showing current holdings + P&L
		posData := make([]map[string]any, 0, len(result.Positions))
		for _, p := range result.Positions {
			posData = append(posData, map[string]any{
				"symbol":         p.Symbol,
				"quantity":       p.Quantity,
				"market_value":   p.MarketValue,
				"unrealized_pnl": p.UnrealizedPnL,
				"average_cost":   p.AverageCost,
			})
		}
		s.Sim().scratchpad.Record(ReasoningTrace{
			SessionID: s.Sim().session.ID,
			Timestamp: time.Now().UTC(),
			Phase:     PhasePortfolioBuild,
			Step:      5,
			Component: "portfolio_summary",
			Action:    "simulation_complete",
			Reasoning: fmt.Sprintf("組合摘要: %d 持倉, %d 訂單, 價值 %.0f, 現金 %.0f, 稅後盈虧 %.0f",
				len(result.Positions), len(result.Orders), result.PortfolioValue, result.EndingCash, result.AfterTaxPnL),
			Data: map[string]any{
				"order_count":     len(result.Orders),
				"position_count":  len(result.Positions),
				"portfolio_value": result.PortfolioValue,
				"ending_cash":     result.EndingCash,
				"after_tax_pnl":   result.AfterTaxPnL,
				"positions":       posData,
			},
			Confidence: -1,
		})
		s.Sim().scratchpad.MarkAllAsFallback()
		_, _ = s.Sim().scratchpad.ExportJSONL()
	}

	return result, nil
}

func (s *System) runReplaySimulation(sessionDate time.Time) (domain.SimulationResult, error) {
	if s.Risk().eventBus != nil {
		go s.Risk().eventBus.PublishSimulationStart(s.Sim().session.ID, sessionDate)
	}

	tw := NewSimTraceWriter(s.Sim().cfg.WorkDir, sessionDate.Format("20060102"), s.traceVerbose)
	defer func() { _, _ = tw.ExportJSONL() }()
	s.Sim().engine.WithTraceWriter(tw)
	if s.plugins != nil {
		s.plugins.WireScreenerTraceWriter(tw)
	}

	symbols := RegistrySymbols(s.Sim().registry)
	tw.Record(1, "data_fetch", "START", nil)
	quotes := s.Sim().replay.QuotesForDate(sessionDate, symbols)
	if len(quotes) == 0 {
		tw.Record(1, "data_fetch", "WARN", map[string]any{"symbols": 0})
	} else {
		tw.Record(1, "data_fetch", "OK", map[string]any{"symbols": len(quotes)})
	}

	// P3-1: Update macro snapshot for PM factor scoring.
	if s.macroSnapshot != nil {
		*s.macroSnapshot = QuotesToMacroDataSnapshot(quotes)
	}

	tw.Record(2, "regime_detect", "START", nil)
	tw.Record(3, "screening", "START", nil)
	tw.Record(4, "recommend", "START", nil)
	tw.Record(5, "guard_filter", "START", nil)

	events := s.detectNarrativeEvents(quotes)
	researchResult := ExecuteWithContext(ExecutionContext{
		Registry:        s.Sim().registry,
		Quotes:          quotes,
		Overrides:       s.Sim().policy.PromptOverrides,
		Policy:          s.Sim().policy.ExecutionPolicy,
		Plugins:         s.plugins,
		SessionID:       s.Sim().session.ID,
		WeightManager:   s.Port().darwinian,
		Context:         s.Sim().ctx,
		NarrativeEvents: events,
		ConvictionClampingCallback: func(evts []portfolio.ConvictionClampingEvent) {
			if s.Risk().clampingLogger != nil {
				s.Risk().clampingLogger.AppendConvictionEvents(evts)
			}
		},
		Scratchpad: s.Sim().scratchpad,
	})
	regime := researchResult.Regime
	rawRecs := researchResult.RawRecommendations
	finalRecs := researchResult.FinalRecommendations
	guardOutcomes := researchResult.GuardOutcomes
	rejects := researchResult.ScreeningRejects

	tw.Record(2, "regime_detect", "OK", map[string]any{"regime": string(regime), "narrative_events": len(events)})
	totalScreened := len(rawRecs) + len(rejects)
	if totalScreened == 0 {
		tw.Record(3, "screening", "WARN", map[string]any{"candidates": 0})
	} else {
		tw.Record(3, "screening", "OK", map[string]any{"candidates": totalScreened, "rejected": len(rejects)})
	}
	if len(rawRecs) == 0 {
		tw.Record(4, "recommend", "WARN", map[string]any{"raw_recs": 0})
	} else {
		tw.Record(4, "recommend", "OK", map[string]any{"raw_recs": len(rawRecs), "final_recs": len(finalRecs)})
	}
	passedCnt := 0
	blockedCnt := 0
	for _, g := range guardOutcomes {
		passedCnt += g.OutputCount
		blockedCnt += g.InputCount - g.OutputCount
	}
	tw.Record(5, "guard_filter", "OK", map[string]any{"passed": passedCnt, "blocked": blockedCnt})

	// Preserve original recs for outcome building so GuardOutcomes align with outcomes.
	outcomeRawRecs := append([]domain.Recommendation(nil), rawRecs...)
	outcomeFinalRecs := append([]domain.Recommendation(nil), finalRecs...)
	oldRegime := regime
	regime = AdjustRegimeFromNarrative(regime, events)
	if s.Risk().eventBus != nil {
		go s.Risk().eventBus.PublishRegimeChange(oldRegime, regime, 0.0, "orchestrator")
		// Sync regime to factor weight engine (event subscriber handles async path).
		if s.Port().factorWeightEngine != nil {
			s.Port().factorWeightEngine.SetRegime(string(regime))
			s.Port().factorWeightEngine.OnRegimeChange(string(oldRegime), string(regime), 0.0)
		}
	}

	vix := vixFromQuotes(quotes)
	if s.strat.strategyAllocator != nil {
		mix := s.strat.strategyAllocator.Allocate(regime, vix)
		if s.Port().factorWeightEngine != nil {
			s.Port().factorWeightEngine.ApplyStrategyMix(mix, s.strat.strategyRegistry)
		}
		var topStrategy *strategy.Strategy
		var topWeight float64
		for name, w := range mix {
			if w > topWeight {
				if st, ok := s.strat.strategyRegistry.Get(name); ok {
					topStrategy = st
					topWeight = w
				}
			}
		}
		if topStrategy != nil && s.strat.thresholdEngine != nil {
			s.strat.thresholdEngine.SetRiskAppetite(sim.RiskAppetite(topStrategy.RiskAppetite))
		}
	} else if s.strat.strategySelector != nil {
		selectedStrategy, err := s.strat.strategySelector.Select(
			s.Sim().ctx,
			vix,
			regime,
		)
		if err == nil && selectedStrategy != nil {
			if s.Port().factorWeightEngine != nil {
				s.Port().factorWeightEngine.ApplyStrategy(selectedStrategy)
			}
			if s.strat.thresholdEngine != nil {
				s.strat.thresholdEngine.SetRiskAppetite(sim.RiskAppetite(selectedStrategy.RiskAppetite))
			}
		}
	}

	rawRecs = s.applyNarrativeContextWithEvents(rawRecs, events)
	finalRecs = s.applyNarrativeContextWithEvents(finalRecs, events)
	rawRecs = s.applyHumanOverrides(rawRecs)
	finalRecs = s.applyHumanOverrides(finalRecs)
	alphaRecs := s.applyAlphaDiscovery(quotes, rawRecs)
	finalRecs = append(finalRecs, alphaRecs...)
	finalRecs = s.host.ProcessRecommendations(regime, finalRecs)
	if s.Risk().eventBus != nil {
		go s.Risk().eventBus.PublishRecommendation("orchestrator", finalRecs)
	}
	if err := s.ensurePersistentStateLoaded(); err != nil {
		return domain.SimulationResult{}, fmt.Errorf("load persistent state: %w", err)
	}
	tw.Record(6, "sim_exec", "START", nil)
	var result domain.SimulationResult
	if s.Sim().persistentState != nil {
		result = s.Sim().engine.RunWithState(s.Sim().persistentState, regime, quotes, finalRecs)
	} else {
		result = s.Sim().engine.Run(regime, quotes, finalRecs)
	}
	tw.Record(6, "sim_exec", "OK", map[string]any{"orders": len(result.Orders), "positions": len(result.Positions)})
	result.GuardOutcomes = guardOutcomes
	if s.Risk().eventBus != nil {
		go s.Risk().eventBus.PublishGuardOutcomes(s.Sim().session.ID, guardOutcomes)
	}
	tw.Record(7, "ledger_write", "START", nil)
	outcomes := buildReplayOutcomes(outcomeRawRecs, outcomeFinalRecs, quotes, sessionDate, s.Sim().replay)
	if s.Risk().repo != nil {
		_ = s.Risk().repo.RecordOutcomes(s.Sim().ctx, outcomes)
	}
	_ = s.Sim().ledger.RecordOutcomes(outcomes)
	_ = s.Sim().ledger.RecordSessionOutcomes(s.Sim().session, outcomes)
	_ = s.Sim().ledger.RecordSessionScreeningRejects(s.Sim().session.ID, rejects)
	if s.Risk().metricsCollector != nil {
		s.Risk().metricsCollector.RecordScreening(int64(len(rawRecs)), int64(len(rejects)))
	}
	s.Sim().lastOutcomes = outcomes
	tw.Record(7, "ledger_write", "OK", map[string]any{"outcomes": len(outcomes)})

	s.Sim().portfolioHistory = append(s.Sim().portfolioHistory, result.PortfolioValue)
	if len(s.Sim().portfolioHistory) > 1 {
		prev := s.Sim().portfolioHistory[len(s.Sim().portfolioHistory)-2]
		if prev > 0 {
			dailyReturn := (result.PortfolioValue - prev) / prev
			s.Sim().returnHistory = append(s.Sim().returnHistory, dailyReturn)
		}
	}
	if err := s.persistPersistentState(); err != nil {
		logging.Warn("System", "failed to persist simulation state", "session_id", s.Sim().session.ID, "err", err)
	}
	s.Sim().lastQuotes = quotes
	s.updateCapitalMetrics(s.Sim().ctx, result)

	if s.Port().darwinian != nil {
		for _, outcome := range outcomes {
			s.Port().darwinian.RecordOutcome(outcome.AgentID, outcome.ForwardReturn, outcome.Hit)
		}
		_, clampingEvents := s.Port().darwinian.PerformDailyAdjustment()
		_ = s.Port().darwinian.Save()
		if len(clampingEvents) > 0 && s.Risk().eventBus != nil {
			payloads := make([]eventbus.ClampingEventPayload, len(clampingEvents))
			for i, e := range clampingEvents {
				payloads[i] = eventbus.ClampingEventPayload{
					AgentID:     e.AgentID,
					RawWeight:   e.RawWeight,
					FinalWeight: e.FinalWeight,
					Boundary:    e.Boundary,
					Timestamp:   e.Timestamp,
				}
			}
			go s.Risk().eventBus.PublishDarwinianClamping(payloads)
			if s.Risk().clampingLogger != nil {
				for _, p := range payloads {
					s.Risk().clampingLogger.Append(p)
				}
			}
		}
	}

	s.host.PostSimulation(quotes, regime, sessionDate)

	if s.Risk().eventBus != nil {
		go s.Risk().eventBus.PublishSimulationComplete(s.Sim().session.ID, result.PortfolioValue, len(result.Orders), len(result.Positions))
	}

	if s.Sim().scratchpad != nil {
		posData := make([]map[string]any, 0, len(result.Positions))
		for _, p := range result.Positions {
			posData = append(posData, map[string]any{
				"symbol":         p.Symbol,
				"quantity":       p.Quantity,
				"market_value":   p.MarketValue,
				"unrealized_pnl": p.UnrealizedPnL,
				"average_cost":   p.AverageCost,
			})
		}
		s.Sim().scratchpad.Record(ReasoningTrace{
			SessionID: s.Sim().session.ID,
			Timestamp: time.Now().UTC(),
			Phase:     PhasePortfolioBuild,
			Step:      5,
			Component: "portfolio_summary",
			Action:    "simulation_complete",
			Reasoning: fmt.Sprintf("組合摘要: %d 持倉, %d 訂單, 價值 %.0f, 現金 %.0f, 稅後盈虧 %.0f",
				len(result.Positions), len(result.Orders), result.PortfolioValue, result.EndingCash, result.AfterTaxPnL),
			Data: map[string]any{
				"order_count":     len(result.Orders),
				"position_count":  len(result.Positions),
				"portfolio_value": result.PortfolioValue,
				"ending_cash":     result.EndingCash,
				"after_tax_pnl":   result.AfterTaxPnL,
				"positions":       posData,
			},
			Confidence: -1,
		})
		_, _ = s.Sim().scratchpad.ExportJSONL()
	}

	return result, nil
}

func selectProvider(cfg config.Config) marketdata.Provider {
	switch cfg.MarketDataProvider {
	case "fugle":
		if cfg.FugleAPIKey != "" {
			return marketdata.NewFugleProviderWithAPIKey(cfg.FugleAPIKey)
		}
		logging.Warn("system", "Fugle API key not configured, falling back to mock provider. DO NOT USE IN PRODUCTION.")
		return marketdata.NewMockProvider()
	case "twse":
		return marketdata.NewTWSEOpenAPIProvider()
	case "hybrid", "":
		return marketdata.NewHybridProvider(cfg.FinMindAPIKey, cfg.FugleAPIKey)
	default:
		return marketdata.NewHybridProvider(cfg.FinMindAPIKey, cfg.FugleAPIKey)
	}
}

func (s *System) Registry() domain.AgentRegistry {
	return s.Sim().registry
}

func (s *System) GetPlugins() *PluginRegistry {
	return s.plugins
}

func (s *System) GetExecutionPolicy() domain.ExecutionPolicy {
	return s.Sim().policy.ExecutionPolicy
}

func (s *System) GetCurrentStrategy() *strategy.Strategy {
	if s.strat.strategySelector == nil {
		return nil
	}
	return s.strat.strategySelector.GetCurrentStrategy()
}

func (s *System) GetStrategySelector() *strategy.Selector {
	return s.strat.strategySelector
}

// GetStrategyAllocator returns the multi-strategy allocator (nil if not attached).
func (s *System) GetStrategyAllocator() *strategy.StrategyAllocator {
	return s.strat.strategyAllocator
}

// WithStrategyAllocator attaches a risk-parity strategy allocator (P2).
// When attached, sessions can use multi-strategy allocation instead of single-strategy selection.
// nil-safe: if nil, Selector path is used (backward compatible).
func (s *System) WithStrategyAllocator(sa *strategy.StrategyAllocator) *System {
	s.strat.strategyAllocator = sa
	return s
}

// GetStrategyEvolver returns the strategy evolver (nil if not attached).
func (s *System) GetStrategyEvolver() *StrategyEvolver {
	return s.strat.strategyEvolver
}

// WithStrategyEvolver attaches a strategy evolver for macro-driven state transitions.
// When attached, the macro pipeline evaluates strategy evolution after drawdown assessment.
// nil-safe: if nil, no strategy evolution occurs (backward compatible).
func (s *System) WithStrategyEvolver(ev *StrategyEvolver) *System {
	s.strat.strategyEvolver = ev
	return s
}

func (s *System) GetThresholdEngine() *sim.DynamicThresholdEngine {
	return s.strat.thresholdEngine
}

func (s *System) detectNarrativeEvents(quotes []domain.Quote) []narrative.NarrativeEvent {
	if s.narrativeEngine == nil {
		return nil
	}
	data := QuotesToNarrativeData(quotes)
	return s.narrativeEngine.DetectEvents(data)
}

func (s *System) applyNarrativeContextWithEvents(recs []domain.Recommendation, events []narrative.NarrativeEvent) []domain.Recommendation {
	if s.narrativeEngine == nil || len(events) == 0 {
		return recs
	}
	chains := s.narrativeEngine.MatchChains(events)

	enriched := make([]domain.Recommendation, len(recs))
	for i, rec := range recs {
		enriched[i] = rec
		// Attach narrative context to context and superinvestor layers.
		var agentLayer string
		for _, agent := range s.Sim().registry.Agents {
			if agent.ID == rec.Agent {
				agentLayer = string(agent.Layer)
				break
			}
		}
		if agentLayer == "context" || agentLayer == "superinvestor" {
			enriched[i].SupportingEvents = make([]string, len(events))
			for j, e := range events {
				enriched[i].SupportingEvents[j] = e.ID
			}
			enriched[i].ReasoningChain = []string{}
			for _, e := range events {
				enriched[i].ReasoningChain = append(enriched[i].ReasoningChain, fmt.Sprintf("%s (%s, confidence %.2f)", e.Theme, e.Region, e.Confidence))
			}
			for _, c := range chains {
				if len(c.Steps) > 0 {
					enriched[i].ReasoningChain = append(enriched[i].ReasoningChain, fmt.Sprintf("Chain %s: %s", c.TemplateID, c.Steps[0].Description))
				}
			}
			if enriched[i].Reason != "" {
				enriched[i].Reason = fmt.Sprintf("%s | Narrative: %d event(s)", enriched[i].Reason, len(events))
			}
		}
	}
	return enriched
}

func AdjustRegimeFromNarrative(base domain.Regime, events []narrative.NarrativeEvent) domain.Regime {
	if len(events) == 0 {
		return base
	}

	riskOffScore := 0
	riskOnScore := 0
	for _, e := range events {
		switch e.Theme {
		case "US_rates_up", "geopolitical_risk_spike", "oil_price_shock", "JPY_carry_unwind":
			riskOffScore++
		case "AI_capex_surge":
			riskOnScore++
		}
	}

	switch {
	case riskOffScore >= 1:
		return domain.RegimeRiskOff
	case riskOnScore >= 1 && base == domain.RegimeNeutral:
		return domain.RegimeRiskOn
	case riskOnScore >= 1 && base == domain.RegimeRiskOff:
		return domain.RegimeNeutral
	default:
		return base
	}
}

func (s *System) applyHumanOverrides(recs []domain.Recommendation) []domain.Recommendation {
	if s.Sim().ledger == nil {
		return recs
	}
	interventions, err := s.Sim().ledger.LoadHumanInterventions()
	if err != nil {
		return recs
	}

	pausedAgents := make(map[string]bool)
	bannedSectors := make(map[string]bool)
	type approvedKey struct{ agentID, symbol string }
	approved := make(map[approvedKey]bool)
	rejected := make(map[approvedKey]bool)
	for _, iv := range interventions {
		if iv.IsExpired() {
			continue
		}
		switch iv.Type {
		case "pause_agent":
			pausedAgents[iv.TargetAgentID] = true
		case "resume_agent":
			delete(pausedAgents, iv.TargetAgentID)
		case "sector_ban":
			bannedSectors[iv.TargetSector] = true
		case "sector_unban":
			delete(bannedSectors, iv.TargetSector)
		case "approve_rec":
			approved[approvedKey{iv.TargetAgentID, iv.TargetSymbol}] = true
		case "reject_rec":
			rejected[approvedKey{iv.TargetAgentID, iv.TargetSymbol}] = true
		case "set_model_weight":
			if s.Port() != nil && s.Port().darwinian != nil && iv.TargetModelID != "" {
				s.Port().darwinian.SetWeight(iv.TargetModelID, iv.Value)
			}
		default:
			// Ignore unknown intervention types.
		}
	}

	filtered := make([]domain.Recommendation, 0, len(recs))
	for _, rec := range recs {
		key := approvedKey{rec.Agent, rec.Symbol}
		if rejected[key] {
			continue
		}
		if pausedAgents[rec.Agent] {
			continue
		}
		if isRecommendationInBannedSector(rec, s.Sim().registry, bannedSectors) {
			continue
		}
		filtered = append(filtered, rec)
	}
	return filtered
}

func isRecommendationInBannedSector(rec domain.Recommendation, registry domain.AgentRegistry, bannedSectors map[string]bool) bool {
	if len(bannedSectors) == 0 {
		return false
	}
	var skill string
	for _, agent := range registry.Agents {
		if agent.ID == rec.Agent {
			skill = agent.Skill
			break
		}
	}
	mappings := config.GetParametersConfig().Industry.SkillToIndustries.Value
	if mappings == nil {
		return false
	}
	for _, sector := range mappings[skill] {
		if bannedSectors[sector] {
			return true
		}
	}
	return false
}

func QuotesToNarrativeData(quotes []domain.Quote) narrative.MarketNarrativeData {
	data := narrative.MarketNarrativeData{}
	for _, q := range quotes {
		switch q.Symbol {
		case "DXY", "^DXY":
			data.DXYChangePct = (q.Last - q.Open) / q.Open * 100
		case "US10Y", "^TNX":
			data.US10YChangeBps = q.Last
		case "VIX", "^VIX":
			data.VIXLevel = q.Last
		case "OIL", "CL=F":
			data.OilChangePct = (q.Last - q.Open) / q.Open * 100
		case "GOLD", "GC=F":
			data.GoldChangePct = (q.Last - q.Open) / q.Open * 100
		case "JPY=X", "USDJPY=X":
			data.JPY_ChangePct = (q.Last - q.Open) / q.Open * 100
		}
	}
	return data
}

func (s *System) applyAlphaDiscovery(quotes []domain.Quote, recs []domain.Recommendation) []domain.Recommendation {
	if s.Port().alphaDiscovery == nil {
		return nil
	}
	symbols := RegistrySymbols(s.Sim().registry)
	quoteMap := make(map[string]domain.Quote, len(quotes))
	for _, q := range quotes {
		quoteMap[q.Symbol] = q
	}
	return s.Port().alphaDiscovery.Discover(s.Sim().ctx, symbols, quoteMap, recs)
}

func (s *System) NextExperimentCandidate() (*domain.Candidate, error) {
	// Use session-dir outcomes (richest data source) instead of sparse global file.
	// This ensures new agents with any outcomes are visible to SelectWeakestAgent.
	outcomes, err := s.Sim().ledger.LoadOutcomesFromSessions()
	if err != nil || len(outcomes) == 0 {
		outcomes, err = s.Sim().ledger.LoadOutcomes()
		if err != nil {
			return nil, err
		}
	}
	scorecards := ledger.BuildScorecards(outcomes)
	candidate := domain.SelectWeakestAgent(s.Sim().registry, scorecards)
	if candidate != nil {
		_ = s.Sim().ledger.RecordExperiment(candidate.Experiment)
		_ = s.Sim().ledger.RecordSessionExperiment(s.Sim().session, candidate.Experiment)
	}
	return candidate, nil
}

func (s *System) Session() domain.ReplaySession {
	return s.Sim().session
}

func (s *System) RecordSessionSummary(result domain.SimulationResult, candidate *domain.Candidate) error {
	summary := domain.SessionSummary{
		SessionID:      s.Sim().session.ID,
		Regime:         result.Regime,
		OrderCount:     len(result.Orders),
		PositionCount:  len(result.Positions),
		EndingCash:     result.EndingCash,
		PortfolioValue: result.PortfolioValue,
		OutcomeCount:   len(s.Sim().lastOutcomes),
		BrokerRuntime: domain.BrokerRuntimeAudit{
			Mode:             s.Sim().cfg.BrokerMode,
			Adapter:          s.Sim().cfg.BrokerAdapter,
			Signer:           s.Sim().cfg.BrokerSigner,
			SignerVersion:    "v1",
			KeyID:            s.Sim().cfg.BrokerKeyID,
			MaxRetries:       s.Sim().cfg.BrokerMaxRetries,
			HTTPTimeoutSec:   s.Sim().cfg.BrokerHTTPTimeoutS,
			HTTPAttempts:     s.Sim().cfg.BrokerHTTPAttempts,
			RetryStatusCodes: append([]int(nil), s.Sim().cfg.BrokerHTTPRetryStatusCodes...),
			MaxClockSkewSec:  s.Sim().cfg.BrokerMaxClockSkewS,
			NonceTTLSec:      s.Sim().cfg.BrokerNonceTTLS,
			NonceStore:       s.Sim().cfg.BrokerNonceStore,
			NonceStorePath:   s.Sim().cfg.BrokerNonceStorePath,
			NonceRedisPrefix: s.Sim().cfg.BrokerNonceRedisKeyPrefix,
		},
		GuardOutcomes: append([]domain.GuardOutcome(nil), result.GuardOutcomes...),
		RecordedAt:    time.Now(),
		TaxSnapshots:  append([]domain.TaxSnapshot(nil), result.TaxSnapshots...),
		AfterTaxPnL:   result.AfterTaxPnL,
		TotalTaxPaid:  result.TotalTaxPaid,
	}
	if cfg := config.GetParametersConfig(); cfg != nil {
		summary.ParametersVersion = cfg.Version
	}
	if candidate != nil {
		summary.NextExperimentAgentID = candidate.Agent.ID
		summary.ProposalID = candidate.Experiment.ProposalID
		summary.CommitID = candidate.Experiment.CommitID
		summary.ApprovalID = candidate.Experiment.ApprovalID
		if summary.ProposalID == "" {
			summary.ProposalID = candidate.Experiment.ID
		}
	}

	if err := s.Sim().ledger.RecordSessionSummary(s.Sim().session, summary); err != nil {
		return err
	}

	// Save per-session position snapshot for portfolio page
	s.saveSessionPositions(s.Sim().session.ID, result.Positions)

	// Anomaly detection: warn on empty or suspicious sessions
	if summary.OutcomeCount == 0 {
		logging.Warn("system", "empty_session",
			"session_id", summary.SessionID,
			"orders", summary.OrderCount,
			"positions", summary.PositionCount,
		)
	}
	if summary.PortfolioValue == 0 && summary.OrderCount > 0 {
		logging.Warn("system", "zero_portfolio_with_orders",
			"session_id", summary.SessionID,
			"orders", summary.OrderCount,
		)
	}
	s.saveSessionTrades(s.Sim().session.ID, result.Trades)
	return nil
}

func (s *System) saveSessionPositions(sessionID string, positions []domain.Position) {
	if len(positions) == 0 {
		return
	}
	sessionDir := filepath.Join(s.Sim().cfg.LedgerDir, "sessions", sessionID)
	_ = os.MkdirAll(sessionDir, 0o755)
	path := filepath.Join(sessionDir, "positions.json")
	bytes, err := json.MarshalIndent(positions, "", "  ")
	if err != nil {
		logging.Warn("System", "failed to marshal positions", "session_id", sessionID, "err", err)
		return
	}
	if err := os.WriteFile(path, bytes, 0o644); err != nil {
		logging.Warn("System", "failed to write positions", "session_id", sessionID, "err", err)
	}
}

func (s *System) saveSessionTrades(sessionID string, trades []domain.TradeRecord) {
	if len(trades) == 0 {
		return
	}
	now := time.Now()
	for i := range trades {
		trades[i].SessionID = sessionID
		if trades[i].TradeID == "" {
			trades[i].TradeID = fmt.Sprintf("%s-%s-%d", sessionID, trades[i].Symbol, i)
		}
		if trades[i].Timestamp.IsZero() {
			trades[i].Timestamp = now
		}
	}
	if err := s.Sim().ledger.RecordSessionTrades(sessionID, trades); err != nil {
		logging.Warn("System", "failed to record trades", "session_id", sessionID, "err", err)
	}
}

func (s *System) ensurePersistentStateLoaded() error {
	if s.Sim().session.Mode != "daily" || s.Sim().persistentState != nil {
		return nil
	}
	loaded, err := sim.LoadPersistentState(s.Sim().cfg.LedgerDir)
	if err != nil {
		return err
	}
	if loaded == nil {
		state := domain.NewSimulationState(s.Sim().policy.Constraints.StartingCash)
		loaded = &state
	}
	s.Sim().persistentState = loaded
	return nil
}

func (s *System) persistPersistentState() error {
	if s.Sim().session.Mode != "daily" || s.Sim().persistentState == nil {
		return nil
	}
	return sim.SavePersistentState(s.Sim().cfg.LedgerDir, s.Sim().persistentState)
}

func quoteBySymbolMap(quotes []domain.Quote) map[string]domain.Quote {
	m := make(map[string]domain.Quote, len(quotes))
	for _, q := range quotes {
		m[q.Symbol] = q
	}
	return m
}

func buildFinalRecKey(finalRecs []domain.Recommendation) map[string]struct{} {
	keys := make(map[string]struct{}, len(finalRecs))
	for _, rec := range finalRecs {
		keys[rec.Symbol+"|"+rec.Agent] = struct{}{}
	}
	return keys
}

func syntheticForwardReturn(symbol string, quote domain.Quote) float64 {
	if quote.Open > 0 {
		intraday := (quote.Last - quote.Open) / quote.Open
		fr := intraday * 0.8
		if fr > 0.05 {
			fr = 0.05
		}
		if fr < -0.05 {
			fr = -0.05
		}
		// Neutral fallback: no artificial bias introduced
		if fr == 0 {
			fr = 0.0
		}
		return fr
	}
	var sum int64
	for _, r := range symbol {
		sum += int64(r)
	}
	return (float64(sum%100)/100.0)*0.04 - 0.02
}

func buildParameterSnapshot() *shared.ParameterSnapshot {
	cfg := config.GetParametersConfig()
	if cfg == nil {
		return nil
	}
	snap := &shared.ParameterSnapshot{
		CapturedAt:    time.Now(),
		ConfigVersion: cfg.Version,
	}
	if cfg.FactorWeight.BaseWeights.Value != nil {
		snap.FactorWeights = make(map[string]float64, len(cfg.FactorWeight.BaseWeights.Value))
		for k, v := range cfg.FactorWeight.BaseWeights.Value {
			snap.FactorWeights[k] = v
		}
	}
	if cfg.NarrativeConviction.ThemeHitRates.Value != nil {
		snap.NarrativeHitRates = make(map[string]float64, len(cfg.NarrativeConviction.ThemeHitRates.Value))
		for k, v := range cfg.NarrativeConviction.ThemeHitRates.Value {
			snap.NarrativeHitRates[k] = v
		}
	}
	if cfg.Industry.PhaseScores.Value.ScoreExpansion != 0 || cfg.Industry.PhaseScores.Value.ScoreRecovery != 0 {
		snap.IndustryPhaseScores = map[string]float64{
			"expansion": cfg.Industry.PhaseScores.Value.ScoreExpansion,
			"recovery":  cfg.Industry.PhaseScores.Value.ScoreRecovery,
			"mature":    cfg.Industry.PhaseScores.Value.ScoreMature,
			"recession": cfg.Industry.PhaseScores.Value.ScoreRecession,
		}
	}
	return snap
}

func buildSyntheticOutcomes(rawRecs, finalRecs []domain.Recommendation, quotes []domain.Quote, asOf time.Time) []domain.RecommendationOutcome {
	if len(rawRecs) == 0 {
		return nil
	}
	quoteMap := quoteBySymbolMap(quotes)
	finalKey := buildFinalRecKey(finalRecs)
	snapshot := buildParameterSnapshot()
	outcomes := make([]domain.RecommendationOutcome, 0, len(rawRecs))
	for _, rec := range rawRecs {
		quote := quoteMap[rec.Symbol]
		forwardReturn := syntheticForwardReturn(rec.Symbol, quote)
		_, passed := finalKey[rec.Symbol+"|"+rec.Agent]
		guardReason := ""
		if !passed {
			guardReason = "未通過控制層過濾"
		}
		outcomes = append(outcomes, domain.RecommendationOutcome{
			AgentID:             rec.Agent,
			Skill:               rec.Skill,
			Layer:               rec.Layer,
			Symbol:              rec.Symbol,
			Side:                rec.Side,
			Conviction:          rec.Conviction,
			TargetPrice:         rec.TargetPrice,
			StopLossPrice:       rec.StopLossPrice,
			Window:              asOf.Format("2006-01-02"),
			ForwardReturn:       forwardReturn,
			BenchmarkDelta:      forwardReturn - 0.005,
			Hit:                 forwardReturn > 0,
			Reason:              rec.Reason,
			Price:               quote.Last,
			PassedGuards:        passed,
			GuardReason:         guardReason,
			RecordedAt:          asOf,
			FactorScores:        rec.FactorScores,
			ConvictionBreakdown: rec.ConvictionBreakdown,
			SupportingEvents:    rec.SupportingEvents,
			ParameterSnapshot:   snapshot,
			IsSynthetic:         true,
		})
	}
	return outcomes
}

func buildReplayOutcomes(rawRecs, finalRecs []domain.Recommendation, quotes []domain.Quote, asOf time.Time, ds *replay.Dataset) []domain.RecommendationOutcome {
	if ds == nil || len(rawRecs) == 0 {
		return nil
	}
	quoteMap := quoteBySymbolMap(quotes)
	finalKey := buildFinalRecKey(finalRecs)
	snapshot := buildParameterSnapshot()
	outcomes := make([]domain.RecommendationOutcome, 0, len(rawRecs))
	for _, rec := range rawRecs {
		quote := quoteMap[rec.Symbol]
		synthetic := false
		forwardReturn, ok := ds.ForwardReturn(rec.Symbol, asOf, 1)
		if !ok || forwardReturn == 0 {
			forwardReturn = syntheticForwardReturn(rec.Symbol, quote)
			synthetic = true
		}
		_, passed := finalKey[rec.Symbol+"|"+rec.Agent]
		guardReason := ""
		if !passed {
			guardReason = "未通過控制層過濾"
		}
		outcomes = append(outcomes, domain.RecommendationOutcome{
			AgentID:             rec.Agent,
			Skill:               rec.Skill,
			Layer:               rec.Layer,
			Symbol:              rec.Symbol,
			Side:                rec.Side,
			Conviction:          rec.Conviction,
			TargetPrice:         rec.TargetPrice,
			StopLossPrice:       rec.StopLossPrice,
			Window:              asOf.Format("2006-01-02"),
			ForwardReturn:       forwardReturn,
			BenchmarkDelta:      forwardReturn - 0.003,
			Hit:                 forwardReturn > 0,
			Reason:              rec.Reason,
			Price:               quote.Last,
			PassedGuards:        passed,
			GuardReason:         guardReason,
			RecordedAt:          asOf,
			FactorScores:        rec.FactorScores,
			ConvictionBreakdown: rec.ConvictionBreakdown,
			SupportingEvents:    rec.SupportingEvents,
			ParameterSnapshot:   snapshot,
			IsSynthetic:         synthetic,
		})
	}
	return outcomes
}

func (s *System) resolveReplayDate() (time.Time, bool) {
	if s.Sim().replay == nil {
		return time.Time{}, false
	}
	if s.Sim().cfg.ReplaySessionDate != "" {
		date, err := time.Parse("2006-01-02", s.Sim().cfg.ReplaySessionDate)
		if err == nil {
			return date, true
		}
	}
	if len(s.Sim().replay.Dates) > 0 {
		return s.Sim().replay.Dates[len(s.Sim().replay.Dates)-1], true
	}
	return time.Time{}, false
}

func newSession(cfg config.Config, ds *replay.Dataset) domain.ReplaySession {
	sessionDate := time.Now()
	if cfg.ReplaySessionDate != "" {
		if parsed, err := time.Parse("2006-01-02", cfg.ReplaySessionDate); err == nil {
			sessionDate = parsed
		}
	}
	dataSource := cfg.ReplayDataPath
	if ds == nil {
		dataSource = cfg.MarketDataProvider
	}

	return domain.ReplaySession{
		ID:          "session-" + sessionDate.Format("20060102") + "-" + cfg.ReplayMode,
		Mode:        cfg.ReplayMode,
		Market:      cfg.PrimaryMarket,
		SessionDate: sessionDate,
		DataSource:  dataSource,
		StartedAt:   time.Now(),
	}
}

func (s *System) WithCapitalManagement(
	controller *risk.CapitalPhaseController,
	allocator *portfolio.CapitalAllocator,
	workflow *risk.ApprovalWorkflow,
) {
	s.Risk().capitalController = controller
	s.Port().capitalAllocator = allocator
	s.Risk().approvalWorkflow = workflow
}

func (s *System) WithMetricsCollector(mc interface{ RecordScreening(passed, rejected int64) }) {
	s.Risk().metricsCollector = mc
}

// SetRepository injects an optional repository for dual-write persistence.
// When set, outcomes are written to both PostgreSQL and JSONL via the repository.
func (s *System) SetRepository(repo repository.OutcomeRepository) {
	s.Risk().repo = repo
}

func QuotesToMacroDataSnapshot(quotes []domain.Quote) narrative.MacroDataSnapshot {
	data := narrative.MacroDataSnapshot{}
	for _, q := range quotes {
		switch q.Symbol {
		case "US10Y", "^TNX":
			data.US10Y = narrative.MacroDataPoint{Value: q.Last, ChangePct: (q.Last - q.Open) / q.Open * 100}
		case "DXY", "^DXY":
			data.DXY = narrative.MacroDataPoint{Value: q.Last, ChangePct: (q.Last - q.Open) / q.Open * 100}
		case "VIX", "^VIX":
			data.VIX = narrative.MacroDataPoint{Value: q.Last, ChangePct: (q.Last - q.Open) / q.Open * 100}
		case "USD/TWD", "USDTWD=X":
			data.USD_TWD = narrative.MacroDataPoint{Value: q.Last, ChangePct: (q.Last - q.Open) / q.Open * 100}
		case "OIL", "CL=F":
			data.Oil = narrative.MacroDataPoint{Value: q.Last, ChangePct: (q.Last - q.Open) / q.Open * 100}
		case "GOLD", "GC=F":
			data.Gold = narrative.MacroDataPoint{Value: q.Last, ChangePct: (q.Last - q.Open) / q.Open * 100}
		case "JPY=X", "USDJPY=X":
			data.JPY = narrative.MacroDataPoint{Value: q.Last, ChangePct: (q.Last - q.Open) / q.Open * 100}
		}
	}
	return data
}

func (s *System) assessMacroRisk(quotes []domain.Quote) *narrative.MacroRiskAssessment {
	if s.Risk().macroRiskEngine == nil {
		return nil
	}
	macroData := QuotesToMacroDataSnapshot(quotes)
	return s.Risk().macroRiskEngine.Assess(macroData)
}

func (s *System) assessStructuralTrends(ctx context.Context, macroData narrative.MacroDataSnapshot) (*narrative.StructuralTrendAssessment, narrative.SectorDataSnapshot) {
	if s.Risk().structuralTrendEngine == nil || s.Risk().sectorDataProvider == nil {
		return nil, narrative.SectorDataSnapshot{}
	}
	sectorSnap, _ := s.Risk().sectorDataProvider.FetchSnapshot(ctx)
	sectorData := narrative.SectorDataSnapshot{
		AIRevenueGrowth:    sectorSnap.TSMCRevenue.Value,
		CoWoSUtilization:   sectorSnap.CoWoSUtilization.Value,
		CapexGrowth:        sectorSnap.CapexGrowth.Value,
		SemiconductorIndex: sectorSnap.SOXIndex.Value,
	}
	return s.Risk().structuralTrendEngine.Assess(macroData, sectorData), sectorData
}

func (s *System) evaluateDrawdown(macroAssessment *narrative.MacroRiskAssessment, structuralAssessment *narrative.StructuralTrendAssessment) *risk.MacroAwareDrawdownDecision {
	if s.Risk().macroDrawdownEngine == nil || macroAssessment == nil {
		return nil
	}
	return s.Risk().macroDrawdownEngine.Evaluate(macroAssessment, structuralAssessment)
}

func (s *System) checkCapitalPhase() (bool, string) {
	if s.Risk().capitalController == nil {
		return false, "capital controller not initialized"
	}

	canAdvance, reason := s.Risk().capitalController.CanAdvance()
	if !canAdvance {
		return false, reason
	}

	if s.Risk().capitalController.GetSnapshot().Phase == domain.PhaseLive {
		if s.Risk().approvalWorkflow != nil {
			_, err := s.Risk().approvalWorkflow.RequestApproval(
				"phase_advance_to_full",
				"system",
				"criteria met for transition from live to full capital",
			)
			if err != nil {
				return false, fmt.Errorf("request approval: %w", err).Error()
			}
			return false, "approval requested for live→full transition"
		}
	}

	return true, "ready to advance"
}

func (s *System) updateCapitalMetrics(ctx context.Context, result domain.SimulationResult) {
	if s.Risk().capitalController == nil {
		return
	}

	if len(s.Sim().returnHistory) < 2 {
		return
	}

	sharpe := risk.CalculateSharpeRatio(s.Sim().returnHistory)
	maxDD := risk.CalculateMaxDrawdown(s.Sim().portfolioHistory)

	s.Risk().capitalController.UpdateMetrics(sharpe, maxDD)

	if result.AfterTaxPnL < 0 {
		s.Risk().capitalController.RecordLoss()
	} else {
		s.Risk().capitalController.RecordWin()
	}

	// Macro pipeline: assess macro risk, structural trends, and drawdown
	macroAssessment := s.assessMacroRisk(s.Sim().lastQuotes)
	if macroAssessment != nil {
		structuralAssessment, _ := s.assessStructuralTrends(ctx, QuotesToMacroDataSnapshot(s.Sim().lastQuotes))
		drawdownDecision := s.evaluateDrawdown(macroAssessment, structuralAssessment)
		if s.strat.strategyEvolver != nil {
			if ev := s.strat.strategyEvolver.Evaluate(macroAssessment, structuralAssessment, drawdownDecision); ev != nil {
				logging.Info("strategy", "evolved",
					logging.FStr("from", fmt.Sprintf("%d", ev.FromState)),
					logging.FStr("to", fmt.Sprintf("%d", ev.ToState)),
					logging.FStr("reason", ev.Reason))
			}

			rotator := portfolio.NewSectorRotator()
			currentAllocs := s.currentSectorAllocations()
			plan := rotator.GeneratePlan(macroAssessment, currentAllocs)
			if modified, rationale := s.strat.strategyEvolver.ApplySectorRotation(plan); modified {
				logging.Info("sector_rotation", "applied",
					logging.FStr("primary_flow", plan.PrimaryFlow),
					logging.FStr("rationale", rationale))
			}
		}
	}
}

func vixFromQuotes(quotes []domain.Quote) float64 {
	for _, q := range quotes {
		if q.Symbol == "VIX" || q.Symbol == "^VIX" {
			return q.Last
		}
	}
	return 20.0
}

func (s *System) currentSectorAllocations() map[string]float64 {
	return nil
}

// RunDailyStressTests executes all built-in stress scenarios against the current
// portfolio and logs a summary report. Designed for the stress_test_daily background
// task (P3-5). Requires lastQuotes from a prior simulation run.
func (s *System) RunDailyStressTests() error {
	quotes := s.Sim().lastQuotes
	if len(quotes) == 0 {
		return fmt.Errorf("stress_test: no quotes available — run a simulation first")
	}
	runner := stress.NewRunner(s.Sim().registry, s.Sim().policy.ExecutionPolicy)
	scenarios := stress.AllScenarios()
	report := stress.Report{ScenarioResults: make([]stress.ScenarioResult, 0, len(scenarios))}

	for _, sc := range scenarios {
		result := runner.RunScenario(sc, quotes, nil)
		report.ScenarioResults = append(report.ScenarioResults, result)
		if result.MaxDrawdown > report.WorstDrawdown {
			report.WorstDrawdown = result.MaxDrawdown
		}
		if result.VaR95 < report.WorstVaR {
			report.WorstVaR = result.VaR95
		}
		report.AvgReturn += result.TotalReturn
	}
	if n := len(report.ScenarioResults); n > 0 {
		report.AvgReturn /= float64(n)
	}

	logging.Info("system", "stress_test_daily_completed",
		logging.FInt("scenarios", len(report.ScenarioResults)),
		logging.FFloat64("worst_drawdown", report.WorstDrawdown),
		logging.FFloat64("worst_var95", report.WorstVaR),
		logging.FFloat64("avg_return", report.AvgReturn))

	return nil
}
