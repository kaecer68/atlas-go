package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/logging"
	"github.com/kaecer68/atlas-go/internal/narrative"
	"github.com/kaecer68/atlas-go/internal/portfolio"
	"github.com/kaecer68/atlas-go/internal/risk"
	"github.com/kaecer68/atlas-go/internal/sim"
)

// PR4 — risk + session persistence extracted from system.go (Issue #611 sub-issue-4):
//   - RecordSessionSummary: write session outcomes to disk and repository.
//   - saveSessionPositions: persist per-session position snapshot.
//   - ensurePersistentStateLoaded: lazy-load replay-derived persistent state.
//   - persistPersistentState: write cross-session state (positions, capital).
//   - assessMacroRisk + assessStructuralTrends + evaluateDrawdown: macro-aware
//     risk pipeline that bridges narrative snapshot → drawdown decision.
//   - updateCapitalMetrics: aggregate capital-phase metrics post-simulation.
//   - AdjustRegimeFromNarrative: regime shift driven by narrative events.
//   - applyHumanOverrides + isRecommendationInBannedSector: human override
//     + banned-sector gating of recommendations.

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
		GuardOutcomes:  append([]domain.GuardOutcome(nil), result.GuardOutcomes...),
		RecordedAt:     time.Now(),
		TaxSnapshots:   append([]domain.TaxSnapshot(nil), result.TaxSnapshots...),
		AfterTaxPnL:    result.AfterTaxPnL,
		TotalTaxPaid:   result.TotalTaxPaid,
		RiskCommentary: result.RiskCommentary,
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

	// Retry guard: a single transient I/O failure here would leave an orphan
	// session directory (outcomes.jsonl present, summary.json missing) and
	// break the pipeline page's recommendation count trust contract.
	if err := recordSummaryWithRetry(
		s.Sim().ledger,
		s.Sim().session,
		summary,
		100*time.Millisecond,
	); err != nil {
		return err
	}

	// Save per-session position snapshot for portfolio page
	s.saveSessionPositions(s.Sim().session.ID, result.Positions)

	// Anomaly detection: warn on empty or suspicious sessions
	if summary.OutcomeCount == 0 {
		logging.Info(
			"system", "session_no_outcomes",
			"session_id", summary.SessionID,
			"orders", summary.OrderCount,
			"positions", summary.PositionCount,
		)
	}
	if summary.PortfolioValue == 0 && summary.OrderCount > 0 {
		logging.Warn(
			"system", "zero_portfolio_with_orders",
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

func (s *System) ensurePersistentStateLoaded() error {
	mode := s.Sim().session.Mode
	if (mode != "daily" && mode != "replay") || s.Sim().persistentState != nil {
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
	mode := s.Sim().session.Mode
	if (mode != "daily" && mode != "replay") || s.Sim().persistentState == nil {
		return nil
	}
	return sim.SavePersistentState(s.Sim().cfg.LedgerDir, s.Sim().persistentState)
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
			sessionDate := domain.SessionDateFromID(s.Sim().session.ID)
			currentAllocs := s.currentSectorAllocations(result.Positions, s.Sim().lastQuotes, sessionDate)
			plan := rotator.GeneratePlan(macroAssessment, currentAllocs)

			// F04: apply event-driven prediction tilt when enabled and predictor is wired.
			if s.eventPredictor != nil {
				dir, conf := s.eventPredictor.PredictToday()
				if dir != "" && dir != "neutral" && conf > 0 {
					plan = applyPredictionTilt(plan, dir, conf)
				}
			}

			receipt, applied, rationale := s.strat.strategyEvolver.ApplySectorRotation(plan, sessionDate, currentAllocs)
			if applied {
				logging.Info("sector_rotation", "applied",
					logging.FStr("primary_flow", plan.PrimaryFlow),
					logging.FStr("rationale", rationale),
					logging.FStr("receipt", receipt.ReceiptID))
			} else {
				logging.Info("sector_rotation", "not_applied",
					logging.FStr("reason", rationale))
			}
		}
	}
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

// applyPredictionTilt adjusts a SectorRotationPlan based on event-driven
// capital flow predictions (F04). The tilt scales with confidence, capped
// at ±5% per sector allocation.
func applyPredictionTilt(plan *portfolio.SectorRotationPlan, direction string, confidence float64) *portfolio.SectorRotationPlan {
	const maxTilt = 0.05

	defensive := map[string]bool{
		"utilities": true, "healthcare": true, "consumer_staples": true,
	}

	tilt := confidence * maxTilt
	if direction == "outflow" {
		tilt = -tilt
	}

	for i := range plan.Allocations {
		sec := plan.Allocations[i].Sector
		if defensive[sec] {
			plan.Allocations[i].TargetPct -= tilt
		} else {
			plan.Allocations[i].TargetPct += tilt
		}
		if plan.Allocations[i].TargetPct < 0 {
			plan.Allocations[i].TargetPct = 0
		}
	}

	// Re-normalize to sum=1.
	var sum float64
	for _, a := range plan.Allocations {
		sum += a.TargetPct
	}
	if sum > 0 {
		for i := range plan.Allocations {
			plan.Allocations[i].TargetPct /= sum
		}
	}

	return plan
}
