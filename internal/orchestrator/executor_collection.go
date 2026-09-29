package orchestrator

import (
	"context"
	"fmt"
	"math"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/internal/constants"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/logging"
	"github.com/kaecer68/atlas-go/internal/methodology"
	"github.com/kaecer68/atlas-go/internal/narrative"
	"github.com/kaecer68/atlas-go/internal/portfolio"
	"github.com/kaecer68/atlas-go/internal/retail"
)

// filterRecommendationsByPeriod is the CharterMode (Phase C2) optional gate
// applied to raw recommendations after collection. Each recommendation's agent
// skill is mapped to a charter strategy category (methodology.SkillToStrategyCategory);
// recs whose category is not in the period's allowed strategy list are dropped.
//
// Unknown periods (advisor.AllowedStrategies returns nil) pass through
// unfiltered; unmapped skills default to all_weather (conservative keep).
func filterRecommendationsByPeriod(
	period domain.MarketPeriod,
	recs []domain.Recommendation,
	registry domain.AgentRegistry,
	advisor *methodology.Advisor,
) []domain.Recommendation {
	allowed := advisor.AllowedStrategies(period)
	if allowed == nil || len(recs) == 0 {
		return recs
	}
	allowedSet := make(map[string]bool, len(allowed))
	for _, id := range allowed {
		allowedSet[id] = true
	}
	skillByAgent := make(map[string]string, len(registry.Agents))
	for _, a := range registry.Agents {
		skillByAgent[a.ID] = a.Skill
	}
	filtered := make([]domain.Recommendation, 0, len(recs))
	for _, rec := range recs {
		if allowedSet[methodology.SkillToStrategyCategory(skillByAgent[rec.Agent])] {
			filtered = append(filtered, rec)
		}
	}
	if len(filtered) != len(recs) {
		logging.Info("charter", "period_strategy_filter",
			"period", string(period),
			"in", len(recs),
			"out", len(filtered))
	}
	return filtered
}

func collectRecommendations(ctx context.Context, registry domain.AgentRegistry, quotes map[string]domain.Quote, plugins *PluginRegistry, overrides map[string]string, regime domain.Regime, narrativeEvents []narrative.NarrativeEvent, sessionID string, scratchpad *Scratchpad) ([]domain.Recommendation, []domain.ScreeningReject) {
	recs := make([]domain.Recommendation, 0)
	rejects := make([]domain.ScreeningReject, 0)
	now := time.Now().UTC()

	// Pre-compute factor scores once for all symbols before the agent loop.
	var factorSnapshot FactorQuery
	if plugins != nil && plugins.factorEngine != nil {
		factorSnapshot = NewFactorSnapshot(quotes, plugins.factorEngine)
	}

	// Skip accounting (issue #1944 T1). The three `continue`s below used to be
	// completely silent: a symbol that never produced a recommendation left
	// neither a ScreeningReject row nor a metric, so "which gate removed this
	// agent's candidates" was unanswerable from production data (that is exactly
	// how B-group agents in the I36 audit became unattributable). The counters are
	// per (agent, reason) — bounded by 21 agents × 3 reasons — and are emitted
	// once per session, never per skip.
	//
	// Label mapping (see skipReasons below):
	//   - skips_no_tradable_quote   : no quote at all, or quote.IsTradable == false
	//   - skips_factor_quality_gate : factor scores EXIST but their average is < 0.40
	//   - skips_executor_declined   : the executor claimed the spec and returned false
	// A symbol with NO factor scores at all (preCount == 0) does not stop at the
	// quality gate — it falls through to skips_executor_declined on purpose, so the
	// label keeps meaning "we had factor evidence and it was too weak".
	skips := newSessionSkipCounts()

	for _, agent := range registry.Agents {
		if !agent.Enabled {
			continue
		}
		if agent.Layer != domain.LayerSector && agent.Layer != domain.LayerStyle && agent.Layer != domain.LayerSuperinvestor {
			continue
		}

		prompt := plugins.ResolvePrompt(agent, overrides)
		symbols := agent.Universe
		if len(symbols) == 0 {
			symbols = slices.Collect(symbolIterator(DefaultSymbols()))
		} else {
			// Auto-expand agent universe from CSV data
			expanded := ExpandUniverse(constants.ReplayCSVPath, nil)
			if len(expanded) > 0 {
				seen := make(map[string]bool)
				for _, s := range symbols {
					seen[s] = true
				}
				for _, s := range expanded {
					if !seen[s] {
						symbols = append(symbols, s)
						seen[s] = true
					}
				}
			}
		}

		for _, symbol := range symbols {
			quote, ok := quotes[symbol]
			if !ok || !quote.IsTradable {
				skips.record(agent.ID, skipReasonNoTradableQuote)
				continue
			}
			screenRes, err := plugins.ScreenDetailed(ctx, agent, symbol, quotes)
			if err != nil || !screenRes.Passed {
				if !screenRes.Passed {
					logging.Debug("screener", "screen_reject",
						logging.Symbol(symbol),
						logging.AgentID(agent.ID),
						logging.FStr("criterion", screenRes.Criterion),
						logging.FStr("reason", screenRes.Reason))
					rejects = append(rejects, domain.ScreeningReject{
						SessionID:      sessionID,
						Symbol:         symbol,
						AgentID:        agent.ID,
						Skill:          agent.Skill,
						Criterion:      screenRes.Criterion,
						CriterionLabel: screenRes.Label,
						Threshold:      screenRes.Threshold,
						ActualValue:    screenRes.Actual,
						RecordedAt:     now,
					})
				}
				continue
			}
			// Factor quality gate: skip symbols with low pre-computed factor scores
			if factorSnapshot != nil {
				var preTotal float64
				var preCount int
				if s, ok := factorSnapshot.GetScore(symbol, portfolio.FactorMomentum); ok {
					preTotal += s
					preCount++
				}
				if s, ok := factorSnapshot.GetScore(symbol, portfolio.FactorValue); ok {
					preTotal += s
					preCount++
				}
				if s, ok := factorSnapshot.GetScore(symbol, portfolio.FactorQuality); ok {
					preTotal += s
					preCount++
				}
				if s, ok := factorSnapshot.GetScore(symbol, portfolio.FactorLiquidity); ok {
					preTotal += s
					preCount++
				}
				// Factor scores are clamped to [-1, 1] (see portfolio/factor_engine.go);
				// skip symbols whose average factor score is below the 0.40 quality bar.
				if preCount > 0 && preTotal/float64(preCount) < 0.40 {
					skips.record(agent.ID, skipReasonFactorQualityGate)
					continue
				}
			}
			rec, ok := plugins.Recommendation(agent, quote, prompt, regime, factorSnapshot)
			if !ok {
				skips.record(agent.ID, skipReasonExecutorDeclined)
				continue
			}

			// Human-in-the-loop override: force-approve or force-reject
			// specific (agent, symbol) pairs before guard filtering.
			// This is the ONLY path by which approve_rec/reject_rec interventions
			// affect the simulation pipeline. Without it, those interventions
			// are audit-log-only with no runtime effect.
			if plugins != nil && len(plugins.recOverrides) > 0 {
				key := agent.ID + ":" + rec.Symbol
				if action, ok := plugins.recOverrides[key]; ok {
					if action == "rejected" {
						continue // skip entirely — human rejected this recommendation
					}
					if action == "approved" {
						recs = append(recs, rec)
						continue // bypass guard + multi-timeframe for approved recs
					}
				}
			}

			// Multi-timeframe adjustment: use intraday OHLC position as a
			// lightweight proxy for short-to-medium-term momentum.  Stocks
			// trading near the day high suggest strength across timeframes;
			// stocks near the day low suggest weakening momentum.
			if quote.High > 0 && quote.Low > 0 && quote.Last > 0 {
				dayRange := quote.High - quote.Low
				if dayRange > 0 {
					position := (quote.Last - quote.Low) / dayRange // 0=at low, 1=at high
					if position < 0.3 {
						// Near day low: weaker across all timeframes
						rec.Conviction -= 5
					} else if position > 0.7 {
						// Near day high: stronger across all timeframes
						rec.Conviction += 3
					}
				}
			}

			recs = append(recs, rec)
		}
	}

	// Wave 2: Append position-rotation recs (SELL/REDUCE) for held positions whose
	// factor signals have decayed. Auto-rotation per ULTRAWORK rule "machine-first".
	// No-op when heldPositions is empty or rotator has no evaluators.
	if plugins != nil && plugins.rotator != nil && len(plugins.rotator.evaluators) > 0 && len(plugins.heldPositions) > 0 {
		for _, agent := range registry.Agents {
			if !agent.Enabled {
				continue
			}
			if agent.Layer != domain.LayerSector && agent.Layer != domain.LayerStyle && agent.Layer != domain.LayerSuperinvestor {
				continue
			}
			prompt := plugins.ResolvePrompt(agent, overrides)
			rotationRecs := plugins.rotator.Rotate(plugins.heldPositions, quotes, agent, prompt, regime, factorSnapshot)
			if len(rotationRecs) > 0 {
				recs = append(recs, rotationRecs...)
				logging.Info("rotation", "evaluator_fired",
					logging.AgentID(agent.ID),
					"layer", string(agent.Layer),
					"held_positions", len(plugins.heldPositions),
					"rotation_recs", len(rotationRecs))
			}
		}
	}

	// Fill SupportingEvents for all recommendations with narrative event IDs.
	for i := range recs {
		eventIDs := make([]string, len(narrativeEvents))
		for j, e := range narrativeEvents {
			eventIDs[j] = e.ID
		}
		recs[i].SupportingEvents = eventIDs
	}

	// Diversity metrics: track recommendation concentration
	if len(recs) > 0 {
		symbolCounts := make(map[string]int)
		for _, rec := range recs {
			symbolCounts[rec.Symbol]++
		}
		var hhi float64
		var topSymbol string
		var topCount int
		for sym, count := range symbolCounts {
			share := float64(count) / float64(len(recs)) * 100
			hhi += share * share
			if count > topCount {
				topSymbol = sym
				topCount = count
			}
		}
		logging.Info("diversity", "metrics",
			"total_recs", len(recs),
			"unique_symbols", len(symbolCounts),
			"hhi", int(hhi),
			"top_symbol", topSymbol,
			"top_count", topCount)
	}

	agentWeights := make(map[string]float64)
	for i := range recs {
		breakdown, scores := plugins.CalculateFactorScoresWithBreakdown(recs[i].Symbol, quotes, recs, agentWeights)
		if scores != nil {
			recs[i].FactorScores = domain.FactorScores{
				Momentum:               scores[portfolio.FactorMomentum],
				Value:                  scores[portfolio.FactorValue],
				Quality:                scores[portfolio.FactorQuality],
				Agent:                  scores[portfolio.FactorAgent],
				InstitutionalSentiment: scores[portfolio.FactorInstSent],
				Liquidity:              scores[portfolio.FactorLiquidity],
				Total:                  scores["total"],
				Breakdown:              breakdown,
			}
		}
	}
	for i := range rejects {
		breakdown, scores := plugins.CalculateFactorScoresWithBreakdown(rejects[i].Symbol, quotes, recs, agentWeights)
		if scores != nil {
			rejects[i].FactorScores = domain.FactorScores{
				Momentum:               scores[portfolio.FactorMomentum],
				Value:                  scores[portfolio.FactorValue],
				Quality:                scores[portfolio.FactorQuality],
				Agent:                  scores[portfolio.FactorAgent],
				InstitutionalSentiment: scores[portfolio.FactorInstSent],
				Liquidity:              scores[portfolio.FactorLiquidity],
				Total:                  scores["total"],
				Breakdown:              breakdown,
			}
		}
	}

	var modulatorSteps []ModulationStep
	if plugins.cycleModulator != nil {
		steps := plugins.cycleModulator.CollectModulationSteps(recs, registry)
		modulatorSteps = append(modulatorSteps, steps...)
	}
	if plugins.narrativeModulator != nil {
		steps := plugins.narrativeModulator.CollectModulationSteps(recs, registry, narrativeEvents)
		modulatorSteps = append(modulatorSteps, steps...)
	}
	for _, ms := range modulatorSteps {
		if ms.RecIndex >= len(recs) {
			continue
		}
		for _, step := range ms.Steps {
			recs[ms.RecIndex].Conviction += step.Delta
			if recs[ms.RecIndex].ConvictionBreakdown != nil {
				recs[ms.RecIndex].ConvictionBreakdown.Steps = append(recs[ms.RecIndex].ConvictionBreakdown.Steps, step)
				recs[ms.RecIndex].ConvictionBreakdown.Final = recs[ms.RecIndex].Conviction
			}
		}
	}

	// Wave 4: Apply CycleStatusCard composite sentiment as an additional
	// conviction layer for recommendations with known industry mappings.
	if plugins.cycleModulator != nil && plugins.cycleModulator.skillToIndustry != nil {
		card := plugins.cycleModulator.GetCycleCard()
		if card != nil {
			skillLookup := make(map[string]string, len(registry.Agents))
			for _, agent := range registry.Agents {
				skillLookup[agent.ID] = agent.Skill
			}
			for i := range recs {
				if recs[i].ConvictionBreakdown == nil {
					continue
				}
				skill := skillLookup[recs[i].Agent]
				industryID, ok := plugins.cycleModulator.skillToIndustry[skill]
				if !ok {
					continue
				}
				cycleConf := plugins.cycleModulator.CycleConfidenceFromCard(industryID)
				delta := 0
				switch {
				case card.CompositeCoefficient > 1.05:
					delta = int(math.Round(10 * (card.CompositeCoefficient - 1.0)))
				case card.CompositeCoefficient < 0.95:
					delta = int(math.Round(10 * (card.CompositeCoefficient - 1.0)))
				}
				cycleStep := domain.ConvictionStep{
					Rule:        "modulator:cycle_status_card",
					Delta:       delta,
					Reason:      fmt.Sprintf("週期綜合情緒: %s (%.3f, 週期信心:%.0f%%)", card.SentimentLabel, card.CompositeCoefficient, cycleConf*100),
					Source:      "CycleStatusCard",
					ParamRef:    "industry.CycleStatusCard.CompositeCoefficient",
					ParamValue:  fmt.Sprintf("%.3f", card.CompositeCoefficient),
					Sensitivity: paramSensitivity(fmt.Sprintf("%.3f", card.CompositeCoefficient)),
				}
				recs[i].Conviction += delta
				recs[i].ConvictionBreakdown.Steps = append(recs[i].ConvictionBreakdown.Steps, cycleStep)
				recs[i].ConvictionBreakdown.Final = recs[i].Conviction
			}
		}
	}

	if calc := retail.GetCalculator(); calc != nil {
		score := calc.LastScore()
		if absScore := math.Abs(score); absScore >= 0.5 {
			convictionDelta := int(math.Round(-15.0 * absScore))
			for i := range recs {
				if recs[i].ConvictionBreakdown == nil {
					continue
				}
				rsiTwStep := domain.ConvictionStep{
					Rule:        "modulator:rsi_tw_sentiment",
					Delta:       convictionDelta,
					Reason:      fmt.Sprintf("散戶情緒極端 (%.2f)，降低信心", score),
					Source:      "RSITwCalculator",
					ParamRef:    "retail.RSITw.Score",
					ParamValue:  fmt.Sprintf("%.4f", score),
					Sensitivity: paramSensitivity(fmt.Sprintf("%.4f", score)),
				}
				recs[i].Conviction += convictionDelta
				recs[i].ConvictionBreakdown.Steps = append(recs[i].ConvictionBreakdown.Steps, rsiTwStep)
				recs[i].ConvictionBreakdown.Final = recs[i].Conviction
			}
			logging.Info("orchestrator", "rsi_tw conviction adjustment applied",
				"score", score, "delta", convictionDelta, "recs", len(recs))
		}
	}

	if scratchpad != nil {
		recData := make([]map[string]any, 0, len(recs))
		for _, rec := range recs {
			recData = append(recData, map[string]any{
				"symbol":     rec.Symbol,
				"agent":      rec.Agent,
				"conviction": rec.Conviction,
			})
		}
		rejSummary := make([]map[string]string, 0, len(rejects))
		rejReasons := make(map[string]int)
		for _, r := range rejects {
			rejSummary = append(rejSummary, map[string]string{
				"symbol":    r.Symbol,
				"agent":     r.AgentID,
				"reason":    r.Criterion,
				"label":     r.CriterionLabel,
				"actual":    r.ActualValue,
				"threshold": r.Threshold,
			})
			rejReasons[r.Criterion]++
		}
		reasoning := fmt.Sprintf("Collected %d recommendations, %d screening rejects", len(recs), len(rejects))
		if len(recs) == 0 && len(rejects) > 0 {
			var topReasons []string
			for k, v := range rejReasons {
				topReasons = append(topReasons, fmt.Sprintf("%d×%s", v, k))
			}
			reasoning += " | All rejected: " + strings.Join(topReasons, ", ")
		}
		if len(recs) == 0 && len(rejects) == 0 {
			reasoning += " | WARNING: no quotes available — check replay data or market provider"
		}
		scratchpad.Record(ReasoningTrace{
			SessionID: sessionID,
			Timestamp: now,
			Phase:     PhaseAgentRecommendation,
			Step:      2,
			Component: "recommendation_collector",
			Action:    "collect_recommendations",
			Reasoning: reasoning,
			Data: map[string]any{
				"recommendation_count": len(recs),
				"reject_count":         len(rejects),
				"quote_count":          len(quotes),
				"recommendations":      recData,
				"rejects":              rejSummary,
				// Skip accounting (issue #1944 T1) — one per session, see
				// newSessionSkipCounts for the label mapping.
				"skips_no_tradable_quote":   skips.byReason[skipReasonNoTradableQuote],
				"skips_factor_quality_gate": skips.byReason[skipReasonFactorQualityGate],
				"skips_executor_declined":   skips.byReason[skipReasonExecutorDeclined],
				"skips_total":               skips.total(),
				"skips_by_agent":            skips.byAgentTotals(),
			},
			Confidence: avgConvictionScore(recs),
		})
	}

	// Emit WARN trace when all agents muted (zero recommendations)
	if len(recs) == 0 && scratchpad != nil {
		activeAgents := 0
		for _, agent := range registry.Agents {
			if agent.Enabled && (agent.Layer == domain.LayerSector || agent.Layer == domain.LayerStyle || agent.Layer == domain.LayerSuperinvestor) {
				activeAgents++
			}
		}
		scratchpad.Record(ReasoningTrace{
			SessionID: sessionID,
			Timestamp: time.Now().UTC(),
			Phase:     PhaseAgentRecommendation,
			Step:      3,
			Component: "recommendation_collector",
			Action:    "zero_recommendations_warning",
			Reasoning: "All agents muted: no recommendations generated",
			Data: map[string]any{
				"agents_total":  len(registry.Agents),
				"agents_active": activeAgents,
				"regime":        string(regime),
			},
			Confidence: 0.0,
		})
	}

	// One line per session, and only when something was skipped: a session with no
	// skips carries no attribution value, and "no line" is unambiguous because the
	// same session still writes its collect_recommendations trace. Never one line
	// per skip — that would be a second paging channel (issue #1944 T1).
	logRecommendationSkips(sessionID, len(registry.Agents), len(quotes), skips)

	return recs, rejects
}

// ── skip accounting (issue #1944 T1) ───────────────────────────────────────

// Skip reason identifiers. They are stable strings because they travel in the
// trace payload and in the log line.
const (
	skipReasonNoTradableQuote   = "no_tradable_quote"
	skipReasonFactorQualityGate = "factor_quality_gate"
	skipReasonExecutorDeclined  = "executor_declined"
)

// sessionSkipCounts counts, per agent and reason, how many candidates the
// recommendation collector dropped without leaving any other trace.
//
// Semantics (issue #1944 T1 ruling): every skip EVENT increments a counter — no
// de-duplication by (agent, symbol, reason) — because the question being answered
// is "how many candidates did this agent lose in this session". The map is
// bounded by the 21 seeded agents × 3 reasons.
type sessionSkipCounts struct {
	byReason  map[string]int
	byAgent   map[string]map[string]int // agentID → reason → count
	agentsHit map[string]bool
}

func newSessionSkipCounts() *sessionSkipCounts {
	return &sessionSkipCounts{
		byReason:  map[string]int{},
		byAgent:   map[string]map[string]int{},
		agentsHit: map[string]bool{},
	}
}

// record counts one skip event for an agent under a reason.
func (s *sessionSkipCounts) record(agentID, reason string) {
	s.byReason[reason]++
	if s.byAgent[agentID] == nil {
		s.byAgent[agentID] = map[string]int{}
	}
	s.byAgent[agentID][reason]++
	s.agentsHit[agentID] = true
}

func (s *sessionSkipCounts) total() int {
	total := 0
	for _, n := range s.byReason {
		total += n
	}
	return total
}

// byAgentTotals returns agentID → total skips across reasons, for agents that
// skipped at least one candidate (empty when nothing was skipped).
func (s *sessionSkipCounts) byAgentTotals() map[string]int {
	out := make(map[string]int, len(s.byAgent))
	for agentID, reasons := range s.byAgent {
		total := 0
		for _, n := range reasons {
			total += n
		}
		out[agentID] = total
	}
	return out
}

// topAgentSkipSummary renders the busiest agents as "agent:reason=n" pairs,
// highest first, capped at limit. Ordering is deterministic (count desc, then
// agentID, then reason) so two runs over the same session produce the same line.
func (s *sessionSkipCounts) topAgentSkipSummary(limit int) string {
	type entry struct {
		agent  string
		reason string
		count  int
	}
	entries := make([]entry, 0, len(s.byAgent)*3)
	for agentID, reasons := range s.byAgent {
		for reason, count := range reasons {
			entries = append(entries, entry{agent: agentID, reason: reason, count: count})
		}
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].count != entries[j].count {
			return entries[i].count > entries[j].count
		}
		if entries[i].agent != entries[j].agent {
			return entries[i].agent < entries[j].agent
		}
		return entries[i].reason < entries[j].reason
	})
	if len(entries) > limit {
		entries = entries[:limit]
	}
	parts := make([]string, 0, len(entries))
	for _, e := range entries {
		parts = append(parts, fmt.Sprintf("%s:%s=%d", e.agent, e.reason, e.count))
	}
	return strings.Join(parts, ",")
}

// logRecommendationSkips emits the per-session skip summary (at most one line).
func logRecommendationSkips(sessionID string, agents, quotes int, s *sessionSkipCounts) {
	if s == nil || s.total() == 0 {
		return
	}
	logging.Info("recommendation_collector", "recommendation_skips",
		logging.FStr("session_id", sessionID),
		logging.FInt("no_tradable_quote", s.byReason[skipReasonNoTradableQuote]),
		logging.FInt("factor_quality_gate", s.byReason[skipReasonFactorQualityGate]),
		logging.FInt("executor_declined", s.byReason[skipReasonExecutorDeclined]),
		logging.FInt("skips_total", s.total()),
		logging.FInt("agents_iterated", agents),
		logging.FInt("quote_count", quotes),
		logging.FStr("top_agents", s.topAgentSkipSummary(5)),
	)
}

// avgConvictionScore returns the average conviction of recommendations as a
// 0-1 score. Returns 0 when recs is empty.
func avgConvictionScore(recs []domain.Recommendation) float64 {
	if len(recs) == 0 {
		return 0
	}
	var total int
	for _, r := range recs {
		total += r.Conviction
	}
	avg := float64(total) / float64(len(recs))
	if avg > 100 {
		return 1.0
	}
	return avg / 100.0
}
