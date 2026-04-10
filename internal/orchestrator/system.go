package orchestrator

import (
	"context"
	"time"

	"github.com/kaecer68/atlas-go/internal/baseline"
	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/evolution"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/replay"
	"github.com/kaecer68/atlas-go/internal/sim"
)

type System struct {
	cfg      config.Config
	provider marketdata.Provider
	engine   *sim.Engine
	registry domain.AgentRegistry
	policy   baseline.Policy
	ledger   *ledger.Store
	replay   *replay.Dataset
	session  domain.ReplaySession
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
	return &System{
		cfg:      cfg,
		provider: selectProvider(cfg),
		engine:   sim.NewEngine(policy.Constraints),
		registry: registry,
		policy:   policy,
		ledger:   ledger.NewStore(cfg.LedgerDir),
		replay:   ds,
		session:  session,
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

	regime, rawRecs, finalRecs, guardOutcomes := ExecuteRegistryResearchDetailedWithPolicyAndGuards(s.registry, quotes, s.policy.PromptOverrides, s.policy.ExecutionPolicy)
	result := s.engine.Run(regime, quotes, finalRecs)
	result.GuardOutcomes = guardOutcomes
	outcomes := buildSyntheticOutcomes(rawRecs, quotes, asOf)
	_ = s.ledger.RecordOutcomes(outcomes)
	_ = s.ledger.RecordSessionOutcomes(s.session, outcomes)
	return result, nil
}

func (s *System) runReplaySimulation(sessionDate time.Time) (domain.SimulationResult, error) {
	symbols := RegistrySymbols(s.registry)
	quotes := s.replay.QuotesForDate(sessionDate, symbols)
	regime, rawRecs, finalRecs, guardOutcomes := ExecuteRegistryResearchDetailedWithPolicyAndGuards(s.registry, quotes, s.policy.PromptOverrides, s.policy.ExecutionPolicy)
	result := s.engine.Run(regime, quotes, finalRecs)
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
		SessionID:     s.session.ID,
		Regime:        result.Regime,
		OrderCount:    len(result.Orders),
		PositionCount: len(result.Positions),
		EndingCash:    result.EndingCash,
		OutcomeCount:  len(outcomes),
		BrokerRuntime: domain.BrokerRuntimeAudit{
			Mode:             s.cfg.BrokerMode,
			Adapter:          s.cfg.BrokerAdapter,
			Signer:           s.cfg.BrokerSigner,
			KeyID:            s.cfg.BrokerKeyID,
			MaxRetries:       s.cfg.BrokerMaxRetries,
			HTTPTimeoutSec:   s.cfg.BrokerHTTPTimeoutS,
			HTTPAttempts:     s.cfg.BrokerHTTPAttempts,
			RetryStatusCodes: append([]int(nil), s.cfg.BrokerHTTPRetryStatusCodes...),
			MaxClockSkewSec:  s.cfg.BrokerMaxClockSkewS,
			NonceTTLSec:      s.cfg.BrokerNonceTTLS,
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
			Window:         "5d",
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
			Window:         "1d",
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
