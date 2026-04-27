package orchestrator

import (
	"context"
	"fmt"
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
	"github.com/kaecer68/atlas-go/internal/reflexivity"
	"github.com/kaecer68/atlas-go/internal/replay"
	"github.com/kaecer68/atlas-go/internal/risk"
	"github.com/kaecer68/atlas-go/internal/screener"
	"github.com/kaecer68/atlas-go/internal/sim"
)

// SystemCore holds the essential simulation state and services.
type SystemCore struct {
	cfg              config.Config
	provider         marketdata.Provider
	engine           *sim.Engine
	registry         domain.AgentRegistry
	policy           baseline.Policy
	ledger           *ledger.Store
	replay           *replay.Dataset
	session          domain.ReplaySession
	alphaDiscovery   *AlphaDiscoveryEngine
	optimizer        *portfolio.Optimizer
	plugins          *PluginRegistry
	narrativeEngine  *narrative.NarrativeEngine
	persistentState  *domain.SimulationState
	ctx              context.Context
	lastOutcomes     []domain.RecommendationOutcome
	portfolioHistory []float64
	returnHistory    []float64
	darwinian        *portfolio.DarwinianWeightManager

	capitalController *risk.CapitalPhaseController
	capitalAllocator  *portfolio.CapitalAllocator
	approvalWorkflow  *risk.ApprovalWorkflow
	metricsCollector  interface{ RecordScreening(passed, rejected int64) }
	eventBus          *eventbus.ChannelEventBus
}

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
	session := newSession(cfg, ds)
	optimizer := portfolio.NewOptimizer()
	hp := portfolio.NewHistoricalPrices()
	if err := hp.LoadFromExtendedJSONL("data/replay/tw_extended_90days.jsonl"); err != nil {
		fmt.Printf("[System] warn: failed to load historical prices: %v\n", err)
	}
	fp := portfolio.NewFundamentalProvider()
	if err := fp.LoadFromJSON("data/fundamentals.json"); err != nil {
		fmt.Printf("[System] warn: failed to load fundamentals: %v\n", err)
	}
	factorEngine := portfolio.NewFactorEngine().
		WithHistoricalPrices(hp).
		WithFundamentalProvider(fp)
	optimizer.WithHistoricalPrices(hp).WithFundamentalProvider(fp).WithFactorEngine(factorEngine)
	screenerEngine := screener.NewEngine(factorEngine, fp)
	plugins := NewPluginRegistry().WithScreener(screenerEngine).WithFactorEngine(factorEngine)

	darwinian := portfolio.NewDarwinianWeightManager("data/state/darwinian_weights.json")
	darwinian.InitializeFromRegistry(registry)
	_ = darwinian.Load()

	engine := sim.NewEngine(policy.Constraints).
		WithOptimizer(optimizer).
		WithReflexivityRules(
			reflexivity.PriceToFundamentalsRule{},
			reflexivity.PnLBehaviorRule{},
			reflexivity.NarrativeFlowsRule{Threshold: 3},
			reflexivity.MarketPolicyRule{Threshold: 0.03},
			reflexivity.NewReversalDetectionRule(),
		)
	return &System{
		SystemCore: &SystemCore{
			cfg:             cfg,
			provider:        selectProvider(cfg),
			engine:          engine.WithContext(context.Background()),
			registry:        registry,
			policy:          policy,
			ledger:          ledger.NewStore(cfg.LedgerDir),
			replay:          ds,
			session:         session,
			optimizer:       optimizer,
			plugins:         plugins,
			alphaDiscovery:  NewAlphaDiscoveryEngine(factorEngine),
			narrativeEngine: narrative.NewNarrativeEngine(),
			ctx:             context.Background(),
			darwinian:       darwinian,
			eventBus:        eventbus.NewChannelEventBus(256),
		},
	}
}

func (s *System) RunDailySimulation(asOf time.Time) (domain.SimulationResult, error) {
	if sessionDate, ok := s.resolveReplayDate(); ok && s.replay != nil {
		return s.runReplaySimulation(sessionDate)
	}

	symbols := RegistrySymbols(s.registry)
	quotes, err := s.provider.GetQuotes(s.ctx, asOf, symbols)
	if err != nil {
		return domain.SimulationResult{}, err
	}

	events := s.detectNarrativeEvents(quotes)
	researchResult := ExecuteWithContext(ExecutionContext{
		Registry:      s.registry,
		Quotes:        quotes,
		Overrides:     s.policy.PromptOverrides,
		Policy:        s.policy.ExecutionPolicy,
		Plugins:       s.plugins,
		SessionID:     s.session.ID,
		WeightManager: s.darwinian,
		Context:       s.ctx,
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
	if s.eventBus != nil {
		go s.eventBus.PublishRegimeChange(oldRegime, regime, 0.0, "orchestrator")
	}
	rawRecs = s.applyNarrativeContextWithEvents(rawRecs, events)
	finalRecs = s.applyNarrativeContextWithEvents(finalRecs, events)
	rawRecs = s.applyHumanOverrides(rawRecs)
	finalRecs = s.applyHumanOverrides(finalRecs)
	alphaRecs := s.applyAlphaDiscovery(quotes, rawRecs)
	finalRecs = append(finalRecs, alphaRecs...)
	finalRecs = s.host.ProcessRecommendations(regime, finalRecs)
	if s.eventBus != nil {
		go s.eventBus.PublishRecommendation("orchestrator", finalRecs)
	}
	var result domain.SimulationResult
	if s.persistentState != nil {
		result = s.engine.RunWithState(s.persistentState, regime, quotes, finalRecs)
	} else {
		result = s.engine.Run(regime, quotes, finalRecs)
	}
	result.GuardOutcomes = guardOutcomes
	if s.eventBus != nil {
		go s.eventBus.PublishGuardOutcomes(s.session.ID, guardOutcomes)
	}

	s.portfolioHistory = append(s.portfolioHistory, result.PortfolioValue)
	if len(s.portfolioHistory) > 1 {
		prev := s.portfolioHistory[len(s.portfolioHistory)-2]
		if prev > 0 {
			dailyReturn := (result.PortfolioValue - prev) / prev
			s.returnHistory = append(s.returnHistory, dailyReturn)
		}
	}
	if len(s.returnHistory) >= 30 {
		snap := risk.ComputeRiskSnapshot(s.returnHistory, s.portfolioHistory)
		result.RiskSnapshot = &snap
	}
	s.updateCapitalMetrics(result)

	outcomes := buildSyntheticOutcomes(outcomeRawRecs, outcomeFinalRecs, quotes, asOf)
	_ = s.ledger.RecordOutcomes(outcomes)
	_ = s.ledger.RecordSessionOutcomes(s.session, outcomes)
	_ = s.ledger.RecordSessionScreeningRejects(s.session.ID, rejects)
	if s.metricsCollector != nil {
		s.metricsCollector.RecordScreening(int64(len(rawRecs)), int64(len(rejects)))
	}
	s.lastOutcomes = outcomes

	if s.darwinian != nil {
		for _, outcome := range outcomes {
			s.darwinian.RecordOutcome(outcome.AgentID, outcome.ForwardReturn, outcome.Hit)
		}
		s.darwinian.PerformDailyAdjustment()
		_ = s.darwinian.Save()
	}

	s.host.PostSimulation(quotes, regime, asOf)
	return result, nil
}

func (s *System) runReplaySimulation(sessionDate time.Time) (domain.SimulationResult, error) {
	symbols := RegistrySymbols(s.registry)
	quotes := s.replay.QuotesForDate(sessionDate, symbols)
	events := s.detectNarrativeEvents(quotes)
	researchResult := ExecuteWithContext(ExecutionContext{
		Registry:      s.registry,
		Quotes:        quotes,
		Overrides:     s.policy.PromptOverrides,
		Policy:        s.policy.ExecutionPolicy,
		Plugins:       s.plugins,
		SessionID:     s.session.ID,
		WeightManager: s.darwinian,
		Context:       s.ctx,
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
	if s.eventBus != nil {
		go s.eventBus.PublishRegimeChange(oldRegime, regime, 0.0, "orchestrator")
	}
	rawRecs = s.applyNarrativeContextWithEvents(rawRecs, events)
	finalRecs = s.applyNarrativeContextWithEvents(finalRecs, events)
	rawRecs = s.applyHumanOverrides(rawRecs)
	finalRecs = s.applyHumanOverrides(finalRecs)
	alphaRecs := s.applyAlphaDiscovery(quotes, rawRecs)
	finalRecs = append(finalRecs, alphaRecs...)
	finalRecs = s.host.ProcessRecommendations(regime, finalRecs)
	if s.eventBus != nil {
		go s.eventBus.PublishRecommendation("orchestrator", finalRecs)
	}
	var result domain.SimulationResult
	if s.persistentState != nil {
		result = s.engine.RunWithState(s.persistentState, regime, quotes, finalRecs)
	} else {
		result = s.engine.Run(regime, quotes, finalRecs)
	}
	result.GuardOutcomes = guardOutcomes
	if s.eventBus != nil {
		go s.eventBus.PublishGuardOutcomes(s.session.ID, guardOutcomes)
	}
	outcomes := buildReplayOutcomes(outcomeRawRecs, outcomeFinalRecs, quotes, sessionDate, s.replay)
	_ = s.ledger.RecordOutcomes(outcomes)
	_ = s.ledger.RecordSessionOutcomes(s.session, outcomes)
	_ = s.ledger.RecordSessionScreeningRejects(s.session.ID, rejects)
	if s.metricsCollector != nil {
		s.metricsCollector.RecordScreening(int64(len(rawRecs)), int64(len(rejects)))
	}
	s.lastOutcomes = outcomes

	s.portfolioHistory = append(s.portfolioHistory, result.PortfolioValue)
	if len(s.portfolioHistory) > 1 {
		prev := s.portfolioHistory[len(s.portfolioHistory)-2]
		if prev > 0 {
			dailyReturn := (result.PortfolioValue - prev) / prev
			s.returnHistory = append(s.returnHistory, dailyReturn)
		}
	}
	s.updateCapitalMetrics(result)

	if s.darwinian != nil {
		for _, outcome := range outcomes {
			s.darwinian.RecordOutcome(outcome.AgentID, outcome.ForwardReturn, outcome.Hit)
		}
		s.darwinian.PerformDailyAdjustment()
		_ = s.darwinian.Save()
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
		// 默认：Hybrid 模式（优先 Fugle，失败回退 TWSE）
		return marketdata.NewHybridProvider(cfg.FugleAPIKey)
	default:
		return marketdata.NewHybridProvider(cfg.FugleAPIKey)
	}
}

func (s *System) Registry() domain.AgentRegistry {
	return s.registry
}

func (s *System) GetPlugins() *PluginRegistry {
	return s.plugins
}

func (s *System) GetExecutionPolicy() domain.ExecutionPolicy {
	return s.policy.ExecutionPolicy
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
		for _, agent := range s.registry.Agents {
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
	if s.ledger == nil {
		return recs
	}
	interventions, err := s.ledger.LoadHumanInterventions()
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
		if isRecommendationInBannedSector(rec, s.registry, bannedSectors) {
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
	if s.alphaDiscovery == nil {
		return nil
	}
	symbols := RegistrySymbols(s.registry)
	quoteMap := make(map[string]domain.Quote, len(quotes))
	for _, q := range quotes {
		quoteMap[q.Symbol] = q
	}
	return s.alphaDiscovery.Discover(s.ctx, symbols, quoteMap, recs)
}

func (s *System) NextExperimentCandidate() (*evolution.Candidate, error) {
	outcomes, err := s.ledger.LoadOutcomes()
	if err != nil {
		return nil, err
	}
	scorecards := ledger.BuildScorecards(outcomes)
	candidate := evolution.SelectWeakestAgent(s.registry, scorecards)
	if candidate != nil {
		_ = s.ledger.RecordExperiment(candidate.Experiment)
		_ = s.ledger.RecordSessionExperiment(s.session, candidate.Experiment)
	}
	return candidate, nil
}

func (s *System) Session() domain.ReplaySession {
	return s.session
}

func (s *System) RecordSessionSummary(result domain.SimulationResult, candidate *evolution.Candidate) error {
	summary := domain.SessionSummary{
		SessionID:      s.session.ID,
		Regime:         result.Regime,
		OrderCount:     len(result.Orders),
		PositionCount:  len(result.Positions),
		EndingCash:     result.EndingCash,
		PortfolioValue: result.PortfolioValue,
		OutcomeCount:   len(s.lastOutcomes),
		BrokerRuntime: domain.BrokerRuntimeAudit{
			Mode:             s.cfg.BrokerMode,
			Adapter:          s.cfg.BrokerAdapter,
			Signer:           s.cfg.BrokerSigner,
			SignerVersion:    "v1",
			KeyID:            s.cfg.BrokerKeyID,
			MaxRetries:       s.cfg.BrokerMaxRetries,
			HTTPTimeoutSec:   s.cfg.BrokerHTTPTimeoutS,
			HTTPAttempts:     s.cfg.BrokerHTTPAttempts,
			RetryStatusCodes: append([]int(nil), s.cfg.BrokerHTTPRetryStatusCodes...),
			MaxClockSkewSec:  s.cfg.BrokerMaxClockSkewS,
			NonceTTLSec:      s.cfg.BrokerNonceTTLS,
			NonceStore:       s.cfg.BrokerNonceStore,
			NonceStorePath:   s.cfg.BrokerNonceStorePath,
			NonceRedisPrefix: s.cfg.BrokerNonceRedisKeyPrefix,
		},
		GuardOutcomes: append([]domain.GuardOutcome(nil), result.GuardOutcomes...),
		RecordedAt:    time.Now(),
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

	return s.ledger.RecordSessionSummary(s.session, summary)
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
	if s.replay == nil || s.cfg.ReplaySessionDate == "" {
		return time.Time{}, false
	}
	date, err := time.Parse("2006-01-02", s.cfg.ReplaySessionDate)
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
	s.capitalController = controller
	s.capitalAllocator = allocator
	s.approvalWorkflow = workflow
}

func (s *System) WithMetricsCollector(mc interface{ RecordScreening(passed, rejected int64) }) {
	s.metricsCollector = mc
}

func (s *System) checkCapitalPhase() (bool, string) {
	if s.capitalController == nil {
		return false, "capital controller not initialized"
	}

	canAdvance, reason := s.capitalController.CanAdvance()
	if !canAdvance {
		return false, reason
	}

	if s.capitalController.GetSnapshot().Phase == domain.PhaseLive {
		if s.approvalWorkflow != nil {
			_, err := s.approvalWorkflow.RequestApproval(
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
	if s.capitalController == nil {
		return
	}

	if len(s.returnHistory) < 2 {
		return
	}

	sharpe := risk.CalculateSharpeRatio(s.returnHistory)
	maxDD := risk.CalculateMaxDrawdown(s.portfolioHistory)

	s.capitalController.UpdateMetrics(sharpe, maxDD)

	if result.AfterTaxPnL < 0 {
		s.capitalController.RecordLoss()
	} else {
		s.capitalController.RecordWin()
	}
}
