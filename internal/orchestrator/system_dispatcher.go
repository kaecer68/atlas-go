package orchestrator

import (
	"fmt"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/logging"
	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/portfolio"
	"github.com/kaecer68/atlas-go/internal/replay"
	"github.com/kaecer68/atlas-go/internal/sim"
	"github.com/kaecer68/atlas-go/internal/strategy"
)

// PR2 — dispatcher logic extracted from system.go (Issue #611 sub-issue-4):
//   - runReplaySimulation: main per-day replay pipeline (regime → screening →
//     recommend → guard filter → sim_exec → ledger_write → portfolio persist).
//   - selectProvider: routes MarketDataProvider config → concrete provider.
//   - resolveReplayDate: picks a session date with measurable forward returns.
//   - newSession: builds a domain.ReplaySession with config-derived metadata.
//
// These functions share a common concern — orchestrating the per-session
// dispatch path — but were historically scattered across system.go.

func (s *System) runReplaySimulation(sessionDate time.Time) (domain.SimulationResult, error) {
	s.publishSimulationStart(s.Sim().session.ID, sessionDate)

	tw := NewSimTraceWriter(s.Sim().cfg.LedgerDir, sessionDate.Format("20060102"), s.traceVerbose)
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
	var result domain.SimulationResult
	if s.Sim().persistentState != nil {
		result = s.Sim().engine.RunWithState(s.Sim().persistentState, regime, quotes, finalRecs)
	} else {
		result = s.Sim().engine.Run(regime, quotes, finalRecs)
	}
	tw.Record(6, "sim_exec", "OK", map[string]any{"orders": len(result.Orders), "positions": len(result.Positions)})
	result.GuardOutcomes = guardOutcomes
	tw.Record(7, "ledger_write", "START", nil)
	outcomes := buildReplayOutcomes(outcomeRawRecs, outcomeFinalRecs, quotes, sessionDate, string(regime), s.Sim().replay)
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

	var clampingEvents []portfolio.ClampingEvent
	if s.Port().darwinian != nil && len(outcomes) > 0 {
		for _, outcome := range outcomes {
			s.Port().darwinian.RecordOutcomeAt(outcome.AgentID, outcome.ForwardReturn, outcome.Hit, outcome.RecordedAt)
		}
		_, clampingEvents = s.Port().darwinian.PerformDailyAdjustment()
		_ = s.Port().darwinian.Save()
		_ = s.Port().darwinian.AppendSnapshot()
	}

	s.host.PostSimulation(quotes, regime, sessionDate)

	s.publishSessionClose(s.Sim().session.ID, guardOutcomes,
		result.PortfolioValue, len(result.Orders), len(result.Positions),
		clampingEvents)

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
			// Shared singleton client: one rate limiter across all Fugle call
			// sites (hybrid provider, stocktools, gateway channel, warmup).
			return marketdata.NewFugleProviderWithClient(marketdata.GetSharedFugleClient(cfg.FugleAPIKey))
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
	if len(s.Sim().replay.Dates) > 1 {
		// Walk backward from last date to find one with measurable forward returns.
		// The last dates may be flat (data generation gap), so skip to dates with
		// actual price movement.
		for i := len(s.Sim().replay.Dates) - 2; i >= 0; i-- {
			date := s.Sim().replay.Dates[i]
			// Check if at least one stock has non-zero ForwardReturn on this date
			testSymbols := []string{"2330.TW", "2317.TW", "2881.TW"}
			for _, sym := range testSymbols {
				if fr, ok := s.Sim().replay.ForwardReturn(sym, date, 1); ok && fr != 0 {
					return date, true
				}
			}
		}
		// Fallback: use second-to-last date even if flat
		return s.Sim().replay.Dates[len(s.Sim().replay.Dates)-2], true
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
