package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/kaecer68/atlas-go/internal/baseline"
	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/eventbus"
	"github.com/kaecer68/atlas-go/internal/evolution"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/narrative"
	"github.com/kaecer68/atlas-go/internal/portfolio"
	"github.com/kaecer68/atlas-go/internal/replay"
	"github.com/kaecer68/atlas-go/internal/repository"
	"github.com/kaecer68/atlas-go/internal/risk"
	"github.com/kaecer68/atlas-go/internal/sim"
	"github.com/kaecer68/atlas-go/internal/strategy"
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

	lastOutcomes     []domain.RecommendationOutcome
	portfolioHistory []float64
	returnHistory    []float64
}

type PortfolioManager struct {
	alphaDiscovery     *AlphaDiscoveryEngine
	optimizer          *portfolio.Optimizer
	darwinian          *portfolio.DarwinianWeightManager
	capitalAllocator   *portfolio.CapitalAllocator
	factorWeightEngine *portfolio.FactorWeightEngine
}

type StrategyLayer struct {
	strategyRegistry *strategy.Registry
	strategySelector *strategy.Selector
	comparisonEngine *strategy.ComparisonEngine
	thresholdEngine  *sim.DynamicThresholdEngine
}

type RiskOps struct {
	capitalController *risk.CapitalPhaseController
	approvalWorkflow  *risk.ApprovalWorkflow
	metricsCollector  interface{ RecordScreening(passed, rejected int64) }
	eventBus          *eventbus.ChannelEventBus
	clampingLogger    *clampingLogger
	repo              repository.OutcomeRepository
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

// System orchestrates the full simulation loop via a SystemCore and a PluginHost.
type System struct {
	*SystemCore
	host *PluginHost
}

func NewSystem(cfg config.Config) *System {
	registry, err := LoadRegistry(cfg.AgentRegistryPath)
	if err != nil {
		registry = SeedRegistry()
	}
	policy, err := baseline.Load(cfg.BaselinePolicyPath)
	if err != nil {
		policy = baseline.DefaultPolicy()
	}
	ds, _ := replay.LoadTWSEOpenDataCSV(cfg.ReplayDataPath)

	runtimeParams := loadRuntimeParamsOrDefault()
	factorEngine, hp, fp := buildFactorEngine(runtimeParams)
	eventBus := eventbus.NewChannelEventBus(256)
	plugins := buildPluginRegistry(factorEngine, fp)

	optimizer := portfolio.NewOptimizer()
	optimizer.WithHistoricalPrices(hp).WithFundamentalProvider(fp).WithFactorEngine(factorEngine)
	thresholdEngine := sim.NewDynamicThresholdEngine()

	return &System{
		SystemCore: &SystemCore{
			sim:             buildSimulationCore(cfg, registry, policy, ds, optimizer),
			port:            buildPortfolioManager(runtimeParams, registry, eventBus, factorEngine),
			strat:           buildStrategyLayer(thresholdEngine),
			risk:            buildRiskOps(cfg, eventBus),
			plugins:         plugins,
			narrativeEngine: narrative.NewNarrativeEngine(),
		},
	}
}

func (s *System) RunDailySimulation(asOf time.Time) (domain.SimulationResult, error) {
	if sessionDate, ok := s.resolveReplayDate(); ok && s.Sim().replay != nil {
		return s.runReplaySimulation(sessionDate)
	}

	symbols := RegistrySymbols(s.Sim().registry)
	quotes, err := s.Sim().provider.GetQuotes(s.Sim().ctx, asOf, symbols)
	if err != nil {
		return domain.SimulationResult{}, err
	}

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
	})
	regime := researchResult.Regime
	rawRecs := researchResult.RawRecommendations
	finalRecs := researchResult.FinalRecommendations
	guardOutcomes := researchResult.GuardOutcomes
	rejects := researchResult.ScreeningRejects
	// Preserve original recs for outcome building so GuardOutcomes align with outcomes.
	outcomeRawRecs := append([]domain.Recommendation(nil), rawRecs...)
	outcomeFinalRecs := append([]domain.Recommendation(nil), finalRecs...)
	oldRegime := regime
	regime = AdjustRegimeFromNarrative(regime, events)
	if s.Risk().eventBus != nil {
		go s.Risk().eventBus.PublishRegimeChange(oldRegime, regime, 0.0, "orchestrator")
	}

	if s.strat.strategySelector != nil {
		selectedStrategy, err := s.strat.strategySelector.Select(
			s.Sim().ctx,
			vixFromQuotes(quotes),
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
	var result domain.SimulationResult
	if s.Sim().persistentState != nil {
		result = s.Sim().engine.RunWithState(s.Sim().persistentState, regime, quotes, finalRecs)
	} else {
		result = s.Sim().engine.Run(regime, quotes, finalRecs)
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
	s.updateCapitalMetrics(result)

	outcomes := buildSyntheticOutcomes(outcomeRawRecs, outcomeFinalRecs, quotes, asOf)
	if s.Risk().repo != nil {
		_ = s.Risk().repo.RecordOutcomes(s.Sim().ctx, outcomes)
	} else {
		_ = s.Sim().ledger.RecordOutcomes(outcomes)
	}
	_ = s.Sim().ledger.RecordSessionOutcomes(s.Sim().session, outcomes)
	_ = s.Sim().ledger.RecordSessionScreeningRejects(s.Sim().session.ID, rejects)
	if s.Risk().metricsCollector != nil {
		s.Risk().metricsCollector.RecordScreening(int64(len(rawRecs)), int64(len(rejects)))
	}
	s.Sim().lastOutcomes = outcomes

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
	return result, nil
}

func (s *System) runReplaySimulation(sessionDate time.Time) (domain.SimulationResult, error) {
	symbols := RegistrySymbols(s.Sim().registry)
	quotes := s.Sim().replay.QuotesForDate(sessionDate, symbols)
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
	})
	regime := researchResult.Regime
	rawRecs := researchResult.RawRecommendations
	finalRecs := researchResult.FinalRecommendations
	guardOutcomes := researchResult.GuardOutcomes
	rejects := researchResult.ScreeningRejects
	// Preserve original recs for outcome building so GuardOutcomes align with outcomes.
	outcomeRawRecs := append([]domain.Recommendation(nil), rawRecs...)
	outcomeFinalRecs := append([]domain.Recommendation(nil), finalRecs...)
	oldRegime := regime
	regime = AdjustRegimeFromNarrative(regime, events)
	if s.Risk().eventBus != nil {
		go s.Risk().eventBus.PublishRegimeChange(oldRegime, regime, 0.0, "orchestrator")
	}

	if s.strat.strategySelector != nil {
		selectedStrategy, err := s.strat.strategySelector.Select(
			s.Sim().ctx,
			vixFromQuotes(quotes),
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
	var result domain.SimulationResult
	if s.Sim().persistentState != nil {
		result = s.Sim().engine.RunWithState(s.Sim().persistentState, regime, quotes, finalRecs)
	} else {
		result = s.Sim().engine.Run(regime, quotes, finalRecs)
	}
	result.GuardOutcomes = guardOutcomes
	if s.Risk().eventBus != nil {
		go s.Risk().eventBus.PublishGuardOutcomes(s.Sim().session.ID, guardOutcomes)
	}
	outcomes := buildReplayOutcomes(outcomeRawRecs, outcomeFinalRecs, quotes, sessionDate, s.Sim().replay)
	if s.Risk().repo != nil {
		_ = s.Risk().repo.RecordOutcomes(s.Sim().ctx, outcomes)
	} else {
		_ = s.Sim().ledger.RecordOutcomes(outcomes)
	}
	_ = s.Sim().ledger.RecordSessionOutcomes(s.Sim().session, outcomes)
	_ = s.Sim().ledger.RecordSessionScreeningRejects(s.Sim().session.ID, rejects)
	if s.Risk().metricsCollector != nil {
		s.Risk().metricsCollector.RecordScreening(int64(len(rawRecs)), int64(len(rejects)))
	}
	s.Sim().lastOutcomes = outcomes

	s.Sim().portfolioHistory = append(s.Sim().portfolioHistory, result.PortfolioValue)
	if len(s.Sim().portfolioHistory) > 1 {
		prev := s.Sim().portfolioHistory[len(s.Sim().portfolioHistory)-2]
		if prev > 0 {
			dailyReturn := (result.PortfolioValue - prev) / prev
			s.Sim().returnHistory = append(s.Sim().returnHistory, dailyReturn)
		}
	}
	s.updateCapitalMetrics(result)

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
	return result, nil
}

func selectProvider(cfg config.Config) marketdata.Provider {
	switch cfg.MarketDataProvider {
	case "fugle":
		// 纯 Fugle 模式（需有效 API key）
		if cfg.FugleAPIKey != "" {
			return marketdata.NewFugleProviderWithAPIKey(cfg.FugleAPIKey)
		}
		fmt.Println("[WARNING] Fugle API key not configured, falling back to mock provider. DO NOT USE IN PRODUCTION.")
		return marketdata.NewMockProvider()
	case "twse":
		// 纯 TWSE 模式（免费，rate limited）
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
	for _, iv := range interventions {
		switch iv.Type {
		case "pause_agent":
			pausedAgents[iv.TargetAgentID] = true
		case "resume_agent":
			delete(pausedAgents, iv.TargetAgentID)
		case "sector_ban":
			bannedSectors[iv.TargetSector] = true
		case "sector_unban":
			delete(bannedSectors, iv.TargetSector)
		default:
			// Ignore unknown intervention types.
		}
	}

	filtered := make([]domain.Recommendation, 0, len(recs))
	for _, rec := range recs {
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
	mappings := map[string][]string{
		"semiconductor_desk":   {"semiconductor", "foundry"},
		"ai_supply_chain_desk": {"ai_supply_chain", "pcb", "thermal"},
		"financials_desk":      {"financials"},
		"shipping_desk":        {"shipping"},
		"etf_rotation_desk":    {"high_dividend", "etf_rotation"},
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

func (s *System) NextExperimentCandidate() (*evolution.Candidate, error) {
	outcomes, err := s.Sim().ledger.LoadOutcomes()
	if err != nil {
		return nil, err
	}
	scorecards := ledger.BuildScorecards(outcomes)
	candidate := evolution.SelectWeakestAgent(s.Sim().registry, scorecards)
	if candidate != nil {
		_ = s.Sim().ledger.RecordExperiment(candidate.Experiment)
		_ = s.Sim().ledger.RecordSessionExperiment(s.Sim().session, candidate.Experiment)
	}
	return candidate, nil
}

func (s *System) Session() domain.ReplaySession {
	return s.Sim().session
}

func (s *System) RecordSessionSummary(result domain.SimulationResult, candidate *evolution.Candidate) error {
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
		log.Printf("[System] warn: failed to marshal positions for %s: %v", sessionID, err)
		return
	}
	if err := os.WriteFile(path, bytes, 0o644); err != nil {
		log.Printf("[System] warn: failed to write positions for %s: %v", sessionID, err)
	}
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

func buildSyntheticOutcomes(rawRecs, finalRecs []domain.Recommendation, quotes []domain.Quote, asOf time.Time) []domain.RecommendationOutcome {
	if len(rawRecs) == 0 {
		return nil
	}
	quoteMap := quoteBySymbolMap(quotes)
	finalKey := buildFinalRecKey(finalRecs)
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
	outcomes := make([]domain.RecommendationOutcome, 0, len(rawRecs))
	for _, rec := range rawRecs {
		forwardReturn, ok := ds.ForwardReturn(rec.Symbol, asOf, 1)
		if !ok {
			forwardReturn = 0
		}
		_, passed := finalKey[rec.Symbol+"|"+rec.Agent]
		guardReason := ""
		if !passed {
			guardReason = "未通過控制層過濾"
		}
		quote := quoteMap[rec.Symbol]
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
		})
	}
	return outcomes
}

func (s *System) resolveReplayDate() (time.Time, bool) {
	if s.Sim().replay == nil || s.Sim().cfg.ReplaySessionDate == "" {
		return time.Time{}, false
	}
	date, err := time.Parse("2006-01-02", s.Sim().cfg.ReplaySessionDate)
	if err != nil {
		return time.Time{}, false
	}
	return date, true
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

func (s *System) updateCapitalMetrics(result domain.SimulationResult) {
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
}

func vixFromQuotes(quotes []domain.Quote) float64 {
	for _, q := range quotes {
		if q.Symbol == "VIX" || q.Symbol == "^VIX" {
			return q.Last
		}
	}
	return 20.0
}
