package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/kaecer68/atlas-go/internal/baseline"
	"github.com/kaecer68/atlas-go/internal/capitalflow"
	"github.com/kaecer68/atlas-go/internal/charter"
	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/domain/shared"
	"github.com/kaecer68/atlas-go/internal/eventbus"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/logging"
	"github.com/kaecer68/atlas-go/internal/macroflow"
	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/methodology"
	"github.com/kaecer68/atlas-go/internal/narrative"
	"github.com/kaecer68/atlas-go/internal/portfolio"
	"github.com/kaecer68/atlas-go/internal/replay"
	"github.com/kaecer68/atlas-go/internal/repository"
	"github.com/kaecer68/atlas-go/internal/retail"
	"github.com/kaecer68/atlas-go/internal/risk"
	"github.com/kaecer68/atlas-go/internal/sectorallocation"
	"github.com/kaecer68/atlas-go/internal/sim"
	"github.com/kaecer68/atlas-go/internal/strategy"
	"github.com/kaecer68/atlas-go/internal/stress"
)

// SystemCore holds the essential simulation state and services.
type SimulationCore struct {
	cfg             config.Config
	provider        marketdata.Provider
	factorEngine    *portfolio.FactorEngine
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

func (sc *SystemCore) Sim() *SimulationCore { return &sc.sim }

// WithSimDecisionRecorder attaches an external sink for the simulation
// engine's pre-trade gate decisions (#1785-D). main.go wires this to
// RiskGate.RecordDecision so the 風控長評語 surface reflects simulation runs.
func (sc *SystemCore) WithSimDecisionRecorder(fn func(risk.RiskDecision)) {
	if sc == nil || sc.sim.engine == nil {
		return
	}
	sc.sim.engine.WithDecisionRecorder(fn)
}
func (sc *SystemCore) Port() *PortfolioManager { return &sc.port }
func (sc *SystemCore) Risk() *RiskOps          { return &sc.risk }
func (sc *SystemCore) Strat() *StrategyLayer   { return &sc.strat }

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

// charterConfig holds the CharterMode (Phase C2/C3) components: the 7-period
// detector, the macroflow engine, and the methodology advisor. It is
// initialized when cfg.CharterMode is true (NewSystemWithEventBus) or when
// WithCharterMode enables at least one switch; nil otherwise, preserving
// Phase A behavior exactly. options selects which charter layers are active
// (Phase C3 stepwise A/B): cfg.CharterMode=true without WithCharterMode
// defaults to AllOn (Phase C2 behavior).
type charterConfig struct {
	periodDetector *portfolio.PeriodDetector
	macroflow      *macroflow.Engine
	advisor        *methodology.Advisor
	options        charter.Options
}

// System orchestrates the full simulation loop via a SystemCore and a PluginHost.
type System struct {
	*SystemCore
	host             *PluginHost
	macroSnapshot    *marketdata.MacroDataSnapshot
	charter          *charterConfig  // nil unless charter wiring active (Phase C2/C3)
	lastResearch     *ResearchResult // most recent ExecuteWithContext output (C3 attribution)
	drawdownMu       sync.RWMutex
	drawdownReporter func(portfolio.DrawdownResult)
	traceVerbose     bool // when true, SimTraceWriter emits color-coded terminal output
	phase3Ctrl       *Phase3Controller

	maturityTracker *domain.MaturityTracker

	sectorL1Mapper     portfolio.L1SymbolResolver
	sectorCalc         *portfolio.SectorExposureCalculator
	sectorWeightEngine sectorallocation.WeightEngine

	// F04: event-driven prediction for simulation tilt.
	eventPredictor EventFlowPredictor

	// C4 P1: E07 capital-flow assessment for sector rotation.
	cfAssessor CapitalFlowAssessmentProvider
}

// Phase3Controller returns the Phase 3 optimization controller, if attached.
func (s *System) Phase3Controller() *Phase3Controller { return s.phase3Ctrl }

// SetVerboseTrace enables or disables color-coded verbose trace output.
func (s *System) SetVerboseTrace(v bool) { s.traceVerbose = v }

// MaturityTracker returns the system's maturity tracker (nil if not attached).
func (s *System) MaturityTracker() *domain.MaturityTracker { return s.maturityTracker }

// WithMaturityTracker attaches a maturity tracker to the system.
func (s *System) WithMaturityTracker(mt *domain.MaturityTracker) *System {
	s.maturityTracker = mt
	return s
}

// WithSectorL1Mapper injects the symbol→L1-sector resolver used by
// currentSectorAllocations for computing real simulation-closing exposure.
func (s *System) WithSectorL1Mapper(m portfolio.L1SymbolResolver) *System {
	s.sectorL1Mapper = m
	return s
}

// WithSectorExposureCalculator injects the SectorExposureCalculator used
// by currentSectorAllocations.
func (s *System) WithSectorExposureCalculator(c *portfolio.SectorExposureCalculator) *System {
	s.sectorCalc = c
	return s
}

// WithSectorWeightEngine injects the sector WeightEngine into both the System
// and its StrategyEvolver (SA06→SA08 wiring). The StrategyEvolver uses it in
// ApplySectorRotation to compute projected targets from the weight engine.
func (s *System) WithSectorWeightEngine(eng sectorallocation.WeightEngine) *System {
	s.sectorWeightEngine = eng
	if evolver := s.GetStrategyEvolver(); evolver != nil {
		evolver.WithSectorWeightEngine(eng)
	}
	return s
}

// EventFlowPredictor is the interface consumed by the orchestrator to apply
// event-driven capital flow predictions as simulation tilts (F04).
// Implementations must be safe for concurrent use during simulation runs.
type EventFlowPredictor interface {
	// PredictToday returns the first day prediction (direction + confidence).
	PredictToday() (direction string, confidence float64)
}

// WithEventPredictor injects an event-driven flow predictor for F04
// simulation tilt. When nil or when ATLAS_EVENT_PREDICTION_ENABLED is
// false, prediction tilt is skipped.
func (s *System) WithEventPredictor(p EventFlowPredictor) *System {
	s.eventPredictor = p
	return s
}

// CapitalFlowAssessmentProvider supplies the E07 4-layer capital-flow assessment
// to the orchestrator's sector rotation path (C4 P1). When nil, the legacy
// PrimaryFlow fallback is used — no assessment → no institutional+behavioral consensus.
//
// NOTE: infrastructure-only in this PR. Production wiring (injecting *capitalflow.Service
// via WithCapitalFlowAssessmentProvider) is deferred to a follow-up PR because the
// capitalflow service is constructed in a different goroutine scope (HTTP server) than
// the simulation loop (RunDailySimulation). See docs/ATLAS_CONSTITUTION_AUDIT.md 附錄D.
type CapitalFlowAssessmentProvider interface {
	LatestAssessment(ctx context.Context) (*capitalflow.CapitalFlowAssessment, error)
}

// WithCapitalFlowAssessmentProvider injects a capital-flow assessment provider.
// When non-nil and the assessment is eligible for automation, the sector rotation
// plan uses institutional+behavioral consensus to derive CapitalFlowAction (C4 P1).
func (s *System) WithCapitalFlowAssessmentProvider(p CapitalFlowAssessmentProvider) *System {
	s.cfAssessor = p
	return s
}

// WithCapitalFlowService injects the shared *capitalflow.Service into the
// stockpicker-winrate executor so its flow gateway enforces the full
// two-level gate (per-symbol foreign + market-regime institutional/retail)
// via capitalflow.Service.LatestDaily → CheckFromReport (issue #1737).
// A nil service is a no-op: the executor keeps its documented foreign-only
// fallback.
func (s *System) WithCapitalFlowService(svc *capitalflow.Service) *System {
	if s == nil || s.plugins == nil || svc == nil {
		return s
	}
	s.plugins.WithCapitalFlowReportProvider(NewCapitalFlowServiceAdapter(svc))
	return s
}

// CapitalFlowServiceAdapter wraps a *capitalflow.Service to satisfy
// CapitalFlowAssessmentProvider, bridging the value/pointer return mismatch.
type CapitalFlowServiceAdapter struct {
	svc *capitalflow.Service
}

// NewCapitalFlowServiceAdapter creates an adapter from a capitalflow service.
func NewCapitalFlowServiceAdapter(svc *capitalflow.Service) *CapitalFlowServiceAdapter {
	return &CapitalFlowServiceAdapter{svc: svc}
}

// LatestAssessment delegates to the underlying service and returns a pointer.
func (a *CapitalFlowServiceAdapter) LatestAssessment(ctx context.Context) (*capitalflow.CapitalFlowAssessment, error) {
	if a.svc == nil {
		return nil, fmt.Errorf("capitalflow service not available")
	}
	assessment, err := a.svc.LatestAssessment(ctx)
	if err != nil {
		return nil, err
	}
	return &assessment, nil
}

// LatestDaily delegates to the underlying service and returns the value
// report. It satisfies CapitalFlowReportProvider so the same adapter backs
// the stockpicker-winrate executor's two-level flow gate (issue #1737).
func (a *CapitalFlowServiceAdapter) LatestDaily(ctx context.Context) (capitalflow.DailyReport, error) {
	if a.svc == nil {
		return capitalflow.DailyReport{}, fmt.Errorf("capitalflow service not available")
	}
	return a.svc.LatestDaily(ctx)
}

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

	// Calibrate ETF NAV from replay data — use latest close prices as NAV proxy.
	// ETF market prices tightly track NAV (typically <0.5% tracking error), making
	// close prices a reliable fallback when no real-time NAV API is available.
	// Replay CSV uses symbol format "0050" (no .TW suffix); strip suffix for lookup.
	if ds != nil && len(ds.Dates) > 0 {
		etfSymbols := factorEngine.GetETFAnalyzer().AllSymbols()
		if len(etfSymbols) > 0 {
			// Map ETFAnalyzer symbols (0050.TW) to replay symbols (0050)
			replaySymbols := make([]string, 0, len(etfSymbols))
			lookup := make(map[string]string, len(etfSymbols))
			for _, sym := range etfSymbols {
				replaySym := strings.TrimSuffix(sym, ".TW")
				replaySymbols = append(replaySymbols, replaySym)
				lookup[replaySym] = sym
			}
			latestDate := ds.Dates[len(ds.Dates)-1]
			quotes := ds.QuotesForDate(latestDate, replaySymbols)
			// Restore .TW suffix on returned quotes so UpdateNAVFromQuotes matches
			for i := range quotes {
				if orig, ok := lookup[quotes[i].Symbol]; ok {
					quotes[i].Symbol = orig
				}
			}
			if updated := factorEngine.GetETFAnalyzer().UpdateNAVFromQuotes(quotes); updated > 0 {
				logging.Info("orchestrator", "etf_nav_calibrated",
					"date", latestDate.Format("2006-01-02"),
					"updated", updated,
					"total", len(etfSymbols))
			} else {
				logging.Warn("orchestrator", "etf_nav_calibrate_no_data",
					"date", latestDate.Format("2006-01-02"),
					"symbols", len(etfSymbols),
					"hint", "replay data may not contain ETF symbols — extend replay data to include ETFs")
			}
		}
	}

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

	simCore := buildSimulationCore(cfg, registry, policy, ds, optimizer, store)
	simCore.factorEngine = factorEngine

	sys := &System{
		SystemCore: &SystemCore{
			sim:             simCore,
			port:            buildPortfolioManager(runtimeParams, registry, eventBus, factorEngine, cfg.LedgerDir),
			strat:           buildStrategyLayer(cfg.LedgerDir, thresholdEngine),
			risk:            buildRiskOps(cfg, eventBus, macroRiskEngine, structuralTrendEngine, macroDrawdownEngine, sectorDataProvider),
			plugins:         plugins,
			narrativeEngine: narrative.NewNarrativeEngine(),
		},
		macroSnapshot: macroSnapshot,
	}

	// Phase C2: when charter mode is enabled, wire the period detector, the
	// macroflow engine, and the methodology advisor into the system. These are
	// injected into every ExecutionContext (RunDailySimulation /
	// runReplaySimulation) and drive period→strategy filtering + period cash
	// reserve. nil charter keeps Phase A behavior.
	if cfg.CharterMode {
		sys.charter = &charterConfig{
			periodDetector: portfolio.NewPeriodDetectorWithDefaults(),
			macroflow:      macroflow.NewEngine(0),
			advisor:        methodology.NewAdvisor(nil),
			options:        charter.AllOn(),
		}
		logging.Info("orchestrator", "charter_mode_enabled",
			"period_detector", "defaults",
			"macroflow", "engine",
			"advisor", "methodology_rules")
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
					ID:                   p.EventID,
					Theme:                p.Theme,
					Confidence:           p.Confidence,
					HitRate:              p.HitRate,
					Timestamp:            time.Now(),
					Status:               "active",
					Duration:             7 * 24 * time.Hour,
					Explanation:          p.Explanation,
					SentimentExplanation: p.SentimentExplanation,
				}
				port.factorWeightEngine.AddEvent(ev)
			}
			return nil
		})
	}

	return sys, nil
}

// ResolveReplayContext loads the core replay execution context: agent registry,
// baseline policy, and replay dataset. Falls back to seeds/defaults when files
// are missing. This provides a single authority for replay context resolution,
// used by both NewSystem and backtest.Runner.
func ResolveReplayContext(cfg config.Config) (domain.AgentRegistry, baseline.Policy, *replay.Dataset) {
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
	return registry, policy, ds
}

// publishSessionClose publishes session close lifecycle events.
// Handles GuardOutcomes, DarwinianClamping, and SimulationComplete
// — the three publish blocks that cluster at the end of every simulation run.
func (s *System) publishSessionClose(
	sessionID string,
	guardOutcomes []domain.GuardOutcome,
	portfolioValue float64,
	orderCount int,
	positionCount int,
	clampingEvents []portfolio.ClampingEvent,
) {
	if s.Risk().eventBus == nil {
		if len(clampingEvents) > 0 && s.Risk().clampingLogger != nil {
			// Still log clamping events even without event bus.
			for _, e := range clampingEvents {
				s.Risk().clampingLogger.Append(eventbus.ClampingEventPayload{
					AgentID: e.AgentID, RawWeight: e.RawWeight,
					FinalWeight: e.FinalWeight, Boundary: e.Boundary,
					Timestamp: e.Timestamp,
				})
			}
		}
		return
	}

	go s.Risk().eventBus.PublishGuardOutcomes(sessionID, guardOutcomes)

	if len(clampingEvents) > 0 {
		payloads := make([]eventbus.ClampingEventPayload, len(clampingEvents))
		for i, e := range clampingEvents {
			payloads[i] = eventbus.ClampingEventPayload{
				AgentID: e.AgentID, RawWeight: e.RawWeight,
				FinalWeight: e.FinalWeight, Boundary: e.Boundary,
				Timestamp: e.Timestamp,
			}
		}
		go s.Risk().eventBus.PublishDarwinianClamping(payloads)
		if s.Risk().clampingLogger != nil {
			for _, p := range payloads {
				s.Risk().clampingLogger.Append(p)
			}
		}
	}

	go s.Risk().eventBus.PublishSimulationComplete(sessionID, portfolioValue, orderCount, positionCount)
}

// publishSimulationStart emits the simulation-start lifecycle event.
func (s *System) publishSimulationStart(sessionID string, asOf time.Time) {
	if s.Risk().eventBus == nil {
		return
	}
	go s.Risk().eventBus.PublishSimulationStart(sessionID, asOf)
}

// publishRegimeChange emits the regime-change event and syncs the factor weight
// engine so it observes the new regime even if the async subscriber path is not
// yet wired.
func (s *System) publishRegimeChange(oldRegime, newRegime domain.Regime, confidence float64, source string) {
	if s.Risk().eventBus == nil {
		return
	}
	go s.Risk().eventBus.PublishRegimeChange(oldRegime, newRegime, confidence, source)
	// Sync regime to factor weight engine (event subscriber handles async path).
	if s.Port().factorWeightEngine != nil {
		s.Port().factorWeightEngine.SetRegime(string(newRegime))
		s.Port().factorWeightEngine.OnRegimeChange(string(oldRegime), string(newRegime), confidence)
	}
}

// publishRecommendation emits the final recommendations after guard filters
// and host processing.
func (s *System) publishRecommendation(source string, recs []domain.Recommendation) {
	if s.Risk().eventBus == nil {
		return
	}
	go s.Risk().eventBus.PublishRecommendation(source, recs)
}

// buildExecutionContext assembles the per-day ExecutionContext for the research
// pipeline. When CharterMode is active (s.charter != nil), it additionally
// injects the period detector, macroflow engine, the current macro snapshot,
// and the period→strategy recommendation gate so ExecuteWithContext computes
// the 7-period classification, macro-flow adjustment, and period-filtered recs.
// With a nil charter, the returned context matches Phase A exactly.
func (s *System) buildExecutionContext(quotes []domain.Quote, events []narrative.NarrativeEvent) ExecutionContext {
	execCtx := ExecutionContext{
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
	}
	if s.charter != nil {
		opts := s.charter.options
		// Zero options = full charter (Phase C2 behavior preserved for systems
		// wired with cfg.CharterMode=true or hand-constructed in tests); non-zero
		// options select the per-arm switch set (Phase C3).
		allOn := !opts.Enabled()
		if allOn || opts.PeriodOnly {
			execCtx.PeriodDetector = s.charter.periodDetector
		}
		if allOn || opts.MacroFlow {
			execCtx.MacroFlow = DefaultMacroFlowStrategy{engine: s.charter.macroflow}
		}
		execCtx.MacroDataSnapshot = s.macroSnapshot
		if allOn || opts.StrategyFilter {
			advisor := s.charter.advisor
			execCtx.PeriodStrategyFilter = func(period domain.MarketPeriod, recs []domain.Recommendation, registry domain.AgentRegistry) []domain.Recommendation {
				return filterRecommendationsByPeriod(period, recs, registry, advisor)
			}
		}
	}
	return execCtx
}

// LastResearchResult returns the most recent ExecuteWithContext output
// (raw/final recommendations, detected period). Used by the backtest Runner
// to build the C3 attribution trace. nil before the first run.
func (s *System) LastResearchResult() *ResearchResult {
	return s.lastResearch
}

// applyCharterReserveCash sets the simulation engine's reserve cash fraction
// from the detected market period (CharterMode, Phase C2):
// reserve = advisor.CashReserve(period) / 100. When no period was detected,
// the override is cleared so the base constraint value applies (Phase A).
func (s *System) applyCharterReserveCash(period *domain.MarketPeriod) {
	if s.charter == nil {
		return
	}
	// Zero options = full charter (C2); explicit options gate the switch.
	opts := s.charter.options
	if opts.Enabled() && !opts.CashReserve {
		return
	}
	if period == nil {
		s.Sim().engine.WithReserveCashFraction(-1)
		return
	}
	reserve := s.charter.advisor.CashReserve(*period) / 100.0
	s.Sim().engine.WithReserveCashFraction(reserve)
	logging.Info("charter", "reserve_cash_set",
		"period", string(*period),
		"reserve_fraction", reserve)
}

// applyCharterConvictionFloor sets the simulation engine's periodized
// conviction floor (CharterMode, Phase C3): the base MinRecommendationConviction
// is raised by charter.ConvictionFloorDelta(period) — RISK_OFF +10, black_swan
// +20 percentage points (§C14/C17). When no period was detected (or the switch
// is off), the adjustment is cleared so the base constraint applies.
func (s *System) applyCharterConvictionFloor(period *domain.MarketPeriod) {
	if s.charter == nil {
		return
	}
	// Zero options = full charter (C2); explicit options gate the switch.
	opts := s.charter.options
	if opts.Enabled() && !opts.ConvictionFloor {
		return
	}
	if period == nil {
		s.Sim().engine.WithConvictionFloorAdjustment(0)
		return
	}
	delta := charter.ConvictionFloorDelta(*period)
	s.Sim().engine.WithConvictionFloorAdjustment(delta)
	if delta > 0 {
		logging.Info("charter", "conviction_floor_set",
			"period", string(*period),
			"delta", delta)
	}
}

func (s *System) RunDailySimulation(asOf time.Time) (domain.SimulationResult, error) {
	s.publishSimulationStart(s.Sim().session.ID, asOf)

	if sessionDate, ok := s.resolveReplayDate(); ok && s.Sim().replay != nil {
		return s.runReplaySimulation(sessionDate)
	}

	// SimTraceWriter for pipeline layer transparency audit trail.
	tw := NewSimTraceWriter(s.Sim().cfg.LedgerDir, asOf.Format("20060102"), s.traceVerbose)
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
			fb.SetCalculator(retail.GetCalculator())
			opt.WithBridgeInput(fb.Convert(*s.macroSnapshot))
		}
	}

	// Pipeline trace: START for layers inside ExecuteWithContext.
	tw.Record(2, "regime_detect", "START", nil)
	tw.Record(3, "screening", "START", nil)
	tw.Record(4, "recommend", "START", nil)
	tw.Record(5, "guard_filter", "START", nil)

	if err := s.ensurePersistentStateLoaded(); err != nil {
		return domain.SimulationResult{}, fmt.Errorf("load persistent state: %w", err)
	}
	events := s.detectNarrativeEvents(quotes)
	if s.plugins != nil {
		var held []domain.Position
		if s.Sim().persistentState != nil {
			held = s.Sim().persistentState.Positions
		}
		s.plugins.WithHeldPositions(held)
		s.plugins.WithRecOverrides(loadRecOverrides(s.Sim().ledger))
	}
	researchResult := ExecuteWithContext(s.buildExecutionContext(quotes, events))
	s.lastResearch = &researchResult
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
	s.publishRegimeChange(oldRegime, regime, 0.0, "orchestrator")

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
	s.publishRecommendation("orchestrator", finalRecs)

	tw.Record(6, "sim_exec", "START", nil)
	// Phase C2: period-driven cash reserve (CharterMode only; no-op otherwise).
	s.applyCharterReserveCash(researchResult.Period)
	// Phase C3: periodized conviction floor (CharterMode ConvictionFloor only).
	s.applyCharterConvictionFloor(researchResult.Period)
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
		result.RiskCommentary = risk.AnnotateSnapshot(s.Sim().ctx, snap)
	}
	if err := s.persistPersistentState(); err != nil {
		logging.Warn("System", "failed to persist simulation state", "session_id", s.Sim().session.ID, "err", err)
	}
	s.Sim().lastQuotes = quotes
	s.updateCapitalMetrics(s.Sim().ctx, result)

	tw.Record(7, "ledger_write", "START", nil)
	outcomes := buildReplayOutcomes(outcomeRawRecs, outcomeFinalRecs, quotes, asOf, string(regime), s.Sim().replay)
	syntheticOutcomes := len(outcomes) == 0
	if syntheticOutcomes {
		outcomes = buildSyntheticOutcomes(outcomeRawRecs, outcomeFinalRecs, quotes, asOf, string(regime))
	}
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

	if s.Strat().comparisonEngine != nil && !syntheticOutcomes {
		strats := s.Strat().strategyRegistry.List()
		vals := make([]strategy.Strategy, len(strats))
		for i, sp := range strats {
			vals[i] = *sp
		}
		eval := strategy.NewShadowStrategyEvaluator(vals)
		stratOutcomes := make([]strategy.RecommendationOutcome, len(outcomes))
		for i, o := range outcomes {
			stratOutcomes[i] = strategy.RecommendationOutcome{
				AgentID: o.AgentID, Skill: o.Skill, Symbol: o.Symbol,
				Conviction: o.Conviction, ForwardReturn: o.ForwardReturn,
				IsSynthetic: o.IsSynthetic, PassedGuards: o.PassedGuards,
			}
		}
		benchmarkReturn := 0.0
		if s.macroSnapshot != nil {
			benchmarkReturn = s.macroSnapshot.TAIEX.ChangePct / 100
		}
		benchmark := strategy.BenchmarkObservation{
			TradingDate: asOf.Format("2006-01-02"),
			SourceID:    "TAIEX",
			Return:      benchmarkReturn,
			Available:   s.macroSnapshot != nil,
		}
		obs := eval.Evaluate(stratOutcomes, asOf, benchmark)
		if len(obs) > 0 {
			day := strategy.ComparisonDay{
				TradingDate:  asOf.Format("2006-01-02"),
				Benchmark:    benchmark,
				Observations: obs,
			}
			_ = s.Strat().comparisonEngine.RecordShadowDay(day)
		}
	}

	tw.Record(7, "ledger_write", "OK", map[string]any{"outcomes": len(outcomes)})

	var clampingEvents []portfolio.ClampingEvent
	if s.Port().darwinian != nil && !syntheticOutcomes {
		for _, outcome := range outcomes {
			s.Port().darwinian.RecordOutcomeAt(outcome.AgentID, outcome.ForwardReturn, outcome.Hit, outcome.RecordedAt)
		}
		_, clampingEvents = s.Port().darwinian.PerformDailyAdjustment()
		_ = s.Port().darwinian.Save()
		_ = s.Port().darwinian.AppendSnapshot()
	}

	s.host.PostSimulation(quotes, regime, asOf)

	s.publishSessionClose(s.Sim().session.ID, guardOutcomes,
		result.PortfolioValue, len(result.Orders), len(result.Positions),
		clampingEvents)

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

func (s *System) NextExperimentCandidate() (*domain.Candidate, error) {
	// Use session-dir outcomes (richest data source) instead of sparse global file.
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

	// New agent onboarding: create experiments for agents with outcomes but no
	// prior experiment records. Reads experiments.jsonl directly to check.
	// Backlog cap: stop adding when too many planned experiments remain
	// undigested (fix manifest #D01 — backlog grew to 628 with no cap).
	const maxUnresolvedPlanned = 100
	backlogFull := ledger.CountUnresolvedPlanned(s.Sim().cfg.LedgerDir) >= maxUnresolvedPlanned
	existingIDs := make(map[string]bool)
	if expData, err := os.ReadFile(filepath.Join(s.Sim().cfg.LedgerDir, "experiments.jsonl")); err == nil {
		for line := range strings.SplitSeq(string(expData), "\n") {
			if line == "" {
				continue
			}
			var rec domain.ExperimentRecord
			if json.Unmarshal([]byte(line), &rec) == nil {
				existingIDs[rec.TargetAgentID] = true
			}
		}
	}
	for _, sc := range scorecards {
		if backlogFull {
			break
		}
		if sc.WindowCount == 0 || existingIDs[sc.AgentID] {
			continue
		}
		// Find agent spec from registry
		var ag domain.AgentSpec
		for _, a := range s.Sim().registry.Agents {
			if a.ID == sc.AgentID {
				ag = a
				break
			}
		}
		if ag.ID == "" || !ag.Enabled {
			continue
		}
		eid := fmt.Sprintf("onboard-%s-%d", ag.ID, time.Now().Unix())
		_ = s.Sim().ledger.RecordExperiment(domain.ExperimentRecord{
			ID:               eid,
			TargetAgentID:    ag.ID,
			Skill:            ag.Skill,
			MutationType:     "onboarding",
			Status:           domain.ExperimentPlanned,
			BaselineValue:    sc.SharpeLike,
			AcceptanceMetric: "sharpe_like",
		})
		logging.Info("experiment", "onboarding_created",
			"agent", ag.ID, "skill", ag.Skill)
	}

	return candidate, nil
}

func (s *System) Session() domain.ReplaySession {
	return s.Sim().session
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

func quoteBySymbolMap(quotes []domain.Quote) map[string]domain.Quote {
	m := make(map[string]domain.Quote, len(quotes))
	for _, q := range quotes {
		m[q.Symbol] = q
	}
	return m
}

// buildPassedSymbolKey 回傳通過控制層 (CIO) 篩選的標的集合。改採 Symbol-only
// key 是為了對齊 internal/orchestrator/AGENTS.md「ID 混淆」陷阱：CIO aggregator
// 會以「最佳 agent」覆寫 Agent 欄位，若仍以 Symbol+"|"+Agent 作為 key 進行
// PassedGuards 查核，非最佳 agent 的原始推薦會被誤判為未過濾。
func buildPassedSymbolKey(finalRecs []domain.Recommendation) map[string]struct{} {
	keys := make(map[string]struct{}, len(finalRecs))
	for _, rec := range finalRecs {
		keys[rec.Symbol] = struct{}{}
	}
	return keys
}

func syntheticForwardReturn(symbol, agentID string, quote domain.Quote, asOf time.Time) float64 {
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
	// Deterministic distribution branch: agent-scoped seed so different agents
	// recommending the same symbol draw different synthetic values (A4 L2 —
	// symbol-only seeding made multi-agent windows byte-identical).
	hash := hashString(agentID + "|" + symbol)
	daySeed := int64(asOf.YearDay())
	return (float64((hash+daySeed)%10000)/10000.0)*0.04 - 0.02
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
		maps.Copy(snap.FactorWeights, cfg.FactorWeight.BaseWeights.Value)
	}
	if cfg.NarrativeConviction.ThemeHitRates.Value != nil {
		snap.NarrativeHitRates = make(map[string]float64, len(cfg.NarrativeConviction.ThemeHitRates.Value))
		maps.Copy(snap.NarrativeHitRates, cfg.NarrativeConviction.ThemeHitRates.Value)
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

func buildSyntheticOutcomes(rawRecs, finalRecs []domain.Recommendation, quotes []domain.Quote, asOf time.Time, regime string) []domain.RecommendationOutcome {
	if len(rawRecs) == 0 {
		return nil
	}
	quoteMap := quoteBySymbolMap(quotes)
	passedSymbols := buildPassedSymbolKey(finalRecs)
	snapshot := buildParameterSnapshot()
	outcomes := make([]domain.RecommendationOutcome, 0, len(rawRecs))
	for _, rec := range rawRecs {
		quote := quoteMap[rec.Symbol]
		forwardReturn := syntheticForwardReturn(rec.Symbol, rec.Agent, quote, asOf)
		_, passed := passedSymbols[rec.Symbol]
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
			Regime:              regime,
			IsSynthetic:         true,
		})
	}
	return outcomes
}

func buildReplayOutcomes(rawRecs, finalRecs []domain.Recommendation, quotes []domain.Quote, asOf time.Time, regime string, ds *replay.Dataset) []domain.RecommendationOutcome {
	if ds == nil || len(rawRecs) == 0 {
		return nil
	}
	quoteMap := quoteBySymbolMap(quotes)
	passedSymbols := buildPassedSymbolKey(finalRecs)
	snapshot := buildParameterSnapshot()
	outcomes := make([]domain.RecommendationOutcome, 0, len(rawRecs))
	for _, rec := range rawRecs {
		quote := quoteMap[rec.Symbol]
		synthetic := false
		forwardReturn, ok := ds.ForwardReturn(rec.Symbol, asOf, 1)
		if !ok {
			forwardReturn = syntheticForwardReturn(rec.Symbol, rec.Agent, quote, asOf)
			synthetic = true
		}
		_, passed := passedSymbols[rec.Symbol]
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
			Regime:              regime,
			IsSynthetic:         synthetic,
		})
	}
	return outcomes
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
	// Stamp the snapshot as freshly computed. Without this, macroflow.Engine
	// treats RecordedAt=0 (1970) as stale and returns nil adjustments, which
	// would silently disable the CharterMode macro-flow path (Phase C2).
	data.RecordedAt = time.Now().Unix()
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

func vixFromQuotes(quotes []domain.Quote) float64 {
	for _, q := range quotes {
		if q.Symbol == "VIX" || q.Symbol == "^VIX" {
			return q.Last
		}
	}
	return 20.0
}

func (s *System) currentSectorAllocations(positions []domain.Position, quotes []domain.Quote, asOf time.Time) map[string]float64 {
	if s.sectorCalc == nil || s.sectorL1Mapper == nil {
		return nil
	}
	exp := s.sectorCalc.Calculate(positions, quotes, asOf, s.sectorL1Mapper)
	// Normalize from map[industry.SectorID]float64 to map[string]float64 for
	// backward compatibility with callers that consume string-keyed maps.
	out := make(map[string]float64, len(exp.Weights))
	for id, w := range exp.Weights {
		out[string(id)] = w
	}
	return out
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

	if opt := s.Sim().engine.Optimizer(); opt != nil && len(quotes) > 5 {
		symbols := make([]string, 0, len(quotes))
		for _, q := range quotes {
			if q.IsTradable {
				symbols = append(symbols, q.Symbol)
			}
		}
		covMatrix, covSymbols := opt.GetCovarianceMatrix(symbols)
		if covMatrix != nil && len(covSymbols) > 0 {
			runner.SetCovariance(covMatrix, covSymbols)
			weights := make(map[string]float64, len(covSymbols))
			for _, sym := range covSymbols {
				weights[sym] = 1.0
			}
			runner.SetPortfolioWeights(weights)
		}
	}

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

	// SK-29: forward the worst-case drawdown / VaR to the registered
	// reporter so the dashboard's /api/dashboard/drawdown endpoint reflects
	// stress_test_daily results (mirrors the RunDailySimulation reporter
	// pattern at system.go:664-668).
	s.drawdownMu.RLock()
	if s.drawdownReporter != nil {
		s.drawdownReporter(portfolio.DrawdownResult{
			MaxDrawdown: report.WorstDrawdown,
			VaR95:       report.WorstVaR,
		})
	}
	s.drawdownMu.RUnlock()

	return nil
}

// PR4 candidates — narrative regime adjustment + human override + banned-sector filter.
// Restored after PR3 sed over-deletion; will be moved to system_risk_session.go in PR4.
