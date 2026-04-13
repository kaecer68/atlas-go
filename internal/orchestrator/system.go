package orchestrator

import (
	"context"
	"fmt"
	"time"

	"github.com/kaecer68/atlas-go/internal/baseline"
	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/evolution"
	"github.com/kaecer68/atlas-go/internal/janus"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/narrative"
	"github.com/kaecer68/atlas-go/internal/portfolio"
	"github.com/kaecer68/atlas-go/internal/reflexivity"
	"github.com/kaecer68/atlas-go/internal/replay"
	"github.com/kaecer68/atlas-go/internal/sim"
)

type System struct {
	cfg             config.Config
	provider        marketdata.Provider
	engine          *sim.Engine
	registry        domain.AgentRegistry
	policy          baseline.Policy
	ledger          *ledger.Store
	replay          *replay.Dataset
	session         domain.ReplaySession
	janusEngine     *janus.Engine
	alphaDiscovery  *AlphaDiscoveryEngine
	optimizer       *portfolio.Optimizer
	narrativeEngine *narrative.NarrativeEngine
	persistentState *domain.SimulationState
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
	_ = hp.LoadFromExtendedJSONL("data/replay/tw_extended_90days.jsonl")
	fp := portfolio.NewFundamentalProvider()
	_ = fp.LoadFromJSON("data/fundamentals.json")
	optimizer.WithHistoricalPrices(hp).WithFundamentalProvider(fp)

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
		cfg:             cfg,
		provider:        selectProvider(cfg),
		engine:          engine,
		registry:        registry,
		policy:          policy,
		ledger:          ledger.NewStore(cfg.LedgerDir),
		replay:          ds,
		session:         session,
		optimizer:       optimizer,
		alphaDiscovery:  NewAlphaDiscoveryEngine(optimizer),
		narrativeEngine: narrative.NewNarrativeEngine(),
	}
}

func (s *System) RunDailySimulation(asOf time.Time) (domain.SimulationResult, error) {
	if sessionDate, ok := s.resolveReplayDate(); ok && s.replay != nil {
		return s.runReplaySimulation(sessionDate)
	}

	symbols := RegistrySymbols(s.registry)
	quotes, err := s.provider.GetQuotes(context.Background(), asOf, symbols)
	if err != nil {
		return domain.SimulationResult{}, err
	}

	events := s.detectNarrativeEvents(quotes)
	regime, rawRecs, finalRecs, guardOutcomes := ExecuteRegistryResearchDetailedWithPolicyAndGuards(s.registry, quotes, s.policy.PromptOverrides, s.policy.ExecutionPolicy)
	regime = AdjustRegimeFromNarrative(regime, events)
	rawRecs = s.applyNarrativeContextWithEvents(rawRecs, events)
	finalRecs = s.applyNarrativeContextWithEvents(finalRecs, events)
	rawRecs = s.applyHumanOverrides(rawRecs)
	finalRecs = s.applyHumanOverrides(finalRecs)
	alphaRecs := s.applyAlphaDiscovery(quotes, rawRecs)
	rawRecs = append(rawRecs, alphaRecs...)
	finalRecs = append(finalRecs, alphaRecs...)
	finalRecs = s.applyJANUS(regime, finalRecs)
	var result domain.SimulationResult
	if s.persistentState != nil {
		result = s.engine.RunWithState(s.persistentState, regime, quotes, finalRecs)
	} else {
		result = s.engine.Run(regime, quotes, finalRecs)
	}
	result.GuardOutcomes = guardOutcomes
	outcomes := buildSyntheticOutcomes(rawRecs, quotes, asOf)
	_ = s.ledger.RecordOutcomes(outcomes)
	_ = s.ledger.RecordSessionOutcomes(s.session, outcomes)
	return result, nil
}

func (s *System) runReplaySimulation(sessionDate time.Time) (domain.SimulationResult, error) {
	symbols := RegistrySymbols(s.registry)
	quotes := s.replay.QuotesForDate(sessionDate, symbols)
	events := s.detectNarrativeEvents(quotes)
	regime, rawRecs, finalRecs, guardOutcomes := ExecuteRegistryResearchDetailedWithPolicyAndGuards(s.registry, quotes, s.policy.PromptOverrides, s.policy.ExecutionPolicy)
	regime = AdjustRegimeFromNarrative(regime, events)
	rawRecs = s.applyNarrativeContextWithEvents(rawRecs, events)
	finalRecs = s.applyNarrativeContextWithEvents(finalRecs, events)
	rawRecs = s.applyHumanOverrides(rawRecs)
	finalRecs = s.applyHumanOverrides(finalRecs)
	alphaRecs := s.applyAlphaDiscovery(quotes, rawRecs)
	rawRecs = append(rawRecs, alphaRecs...)
	finalRecs = append(finalRecs, alphaRecs...)
	finalRecs = s.applyJANUS(regime, finalRecs)
	var result domain.SimulationResult
	if s.persistentState != nil {
		result = s.engine.RunWithState(s.persistentState, regime, quotes, finalRecs)
	} else {
		result = s.engine.Run(regime, quotes, finalRecs)
	}
	result.GuardOutcomes = guardOutcomes
	outcomes := buildReplayOutcomes(rawRecs, sessionDate, s.replay)
	_ = s.ledger.RecordOutcomes(outcomes)
	_ = s.ledger.RecordSessionOutcomes(s.session, outcomes)
	return result, nil
}

func selectProvider(cfg config.Config) marketdata.Provider {
	switch cfg.MarketDataProvider {
	case "fugle":
		// 纯 Fugle 模式（需有效 API key）
		if cfg.FugleAPIKey != "" {
			return marketdata.NewFugleProviderWithAPIKey(cfg.FugleAPIKey)
		}
		return marketdata.NewTWSEProvider()
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

// WithJANUS attaches a JANUS engine to the system for backtest validation.
func (s *System) WithJANUS(j *janus.Engine) *System {
	s.janusEngine = j
	return s
}

// WithPersistentState enables cross-day simulation state carry-over for backtests.
func (s *System) WithPersistentState(state *domain.SimulationState) *System {
	s.persistentState = state
	return s
}

func (s *System) applyJANUS(regime domain.Regime, recs []domain.Recommendation) []domain.Recommendation {
	if s.janusEngine == nil {
		return recs
	}
	return s.janusEngine.ApplyAdjustment(recs, regime)
}

func (s *System) detectNarrativeEvents(quotes []domain.Quote) []narrative.NarrativeEvent {
	if s.narrativeEngine == nil {
		return nil
	}
	data := quotesToNarrativeData(quotes)
	return s.narrativeEngine.DetectEvents(data)
}

func (s *System) applyNarrativeContext(recs []domain.Recommendation, quotes []domain.Quote) []domain.Recommendation {
	return s.applyNarrativeContextWithEvents(recs, s.detectNarrativeEvents(quotes))
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
		"semiconductor_desk":     {"semiconductor", "foundry"},
		"ai_supply_chain_desk":   {"ai_supply_chain", "pcb", "thermal"},
		"financials_desk":        {"financials"},
		"shipping_desk":          {"shipping"},
		"etf_rotation_desk":      {"high_dividend", "etf_rotation"},
	}
	for _, sector := range mappings[skill] {
		if bannedSectors[sector] {
			return true
		}
	}
	return false
}

func quotesToNarrativeData(quotes []domain.Quote) narrative.MarketNarrativeData {
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
	return s.alphaDiscovery.Discover(symbols, quoteMap, recs)
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
	outcomes, err := s.ledger.LoadOutcomes()
	if err != nil {
		return err
	}

	summary := domain.SessionSummary{
		SessionID:      s.session.ID,
		Regime:         result.Regime,
		OrderCount:     len(result.Orders),
		PositionCount:  len(result.Positions),
		EndingCash:     result.EndingCash,
		PortfolioValue: result.PortfolioValue,
		OutcomeCount:   len(outcomes),
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

func buildSyntheticOutcomes(recs []domain.Recommendation, quotes []domain.Quote, asOf time.Time) []domain.RecommendationOutcome {
	if len(recs) == 0 {
		return nil
	}

	outcomes := make([]domain.RecommendationOutcome, 0, len(recs))
	for _, rec := range recs {
		forwardReturn := syntheticForwardReturn(rec.Symbol)
		outcomes = append(outcomes, domain.RecommendationOutcome{
			AgentID:        rec.Agent,
			Skill:          rec.Skill,
			Symbol:         rec.Symbol,
			Window:         asOf.Format("2006-01-02"),
			ForwardReturn:  forwardReturn,
			BenchmarkDelta: forwardReturn - 0.005,
			Hit:            forwardReturn > 0,
			RecordedAt:     asOf,
		})
	}
	return outcomes
}

func buildReplayOutcomes(recs []domain.Recommendation, asOf time.Time, ds *replay.Dataset) []domain.RecommendationOutcome {
	if ds == nil || len(recs) == 0 {
		return nil
	}

	outcomes := make([]domain.RecommendationOutcome, 0, len(recs))
	for _, rec := range recs {
		forwardReturn, ok := ds.ForwardReturn(rec.Symbol, asOf, 1)
		if !ok {
			continue
		}
		outcomes = append(outcomes, domain.RecommendationOutcome{
			AgentID:        rec.Agent,
			Skill:          rec.Skill,
			Symbol:         rec.Symbol,
			Window:         asOf.Format("2006-01-02"),
			ForwardReturn:  forwardReturn,
			BenchmarkDelta: forwardReturn - 0.003,
			Hit:            forwardReturn > 0,
			RecordedAt:     asOf,
		})
	}

	return outcomes
}

func syntheticForwardReturn(symbol string) float64 {
	switch symbol {
	case "2330.TW":
		return 0.021
	case "2382.TW":
		return 0.014
	case "2317.TW":
		return -0.006
	case "2603.TW":
		return -0.018
	case "0050.TW":
		return 0.008
	default:
		return 0
	}
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
