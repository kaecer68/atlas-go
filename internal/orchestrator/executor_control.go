package orchestrator

import (
	"fmt"
	"math"
	"slices"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/macroflow"
)

func applyMacroConvictionScaling(recs []domain.Recommendation, adj *macroflow.AdjustmentResult) []domain.Recommendation {
	if adj == nil || len(recs) == 0 {
		return recs
	}
	a := adj.Adjustment
	conservative := math.Max(a.Defensive, 0) + math.Max(a.Cash, 0) + math.Max(-a.Aggressive, 0)
	riskOn := math.Max(a.Aggressive, 0)
	net := riskOn - conservative
	scale := 1.0 + net/100*0.3
	if scale < 0.5 {
		scale = 0.5
	}
	if scale > 1.5 {
		scale = 1.5
	}
	out := make([]domain.Recommendation, len(recs))
	for i, rec := range recs {
		adjRec := rec
		adjRec.Conviction = clampConvictionInt(int(float64(rec.Conviction) * scale))
		out[i] = adjRec
	}
	return out
}

func clampConvictionInt(v int) int {
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return v
}

func applyControlLayerWithOutcomes(registry domain.AgentRegistry, plugins *PluginRegistry, recs []domain.Recommendation, policy domain.ExecutionPolicy, regime domain.Regime, scratchpad *Scratchpad, sessionID string, macroAdjustment *macroflow.AdjustmentResult) ([]domain.Recommendation, []domain.GuardOutcome) {
	if !policy.RequireCROPass {
		return recs, []domain.GuardOutcome{{
			GuardID:     "control-bypass",
			GuardSkill:  "control_bypass",
			Severity:    domain.GuardSeveritySoft,
			Passed:      true,
			Reason:      "控制層已略過（未啟用 CRO 檢查）",
			InputCount:  len(recs),
			OutputCount: len(recs),
		}}
	}

	current := applyMacroConvictionScaling(recs, macroAdjustment)
	outcomes := make([]domain.GuardOutcome, 0)
	for _, agent := range registry.Agents {
		if !agent.Enabled || (agent.Layer != domain.LayerControl && agent.Layer != domain.LayerSuperinvestor) {
			continue
		}
		before := len(current)
		next := plugins.ApplyControl(agent, current, policy, regime)
		after := len(next)
		severity := severityForControlAgent(agent)
		blocked := before > 0 && after == 0 && severity == domain.GuardSeverityHard
		reason := "未過濾任何推薦，全部放行"
		if after < before {
			reason = fmt.Sprintf("過濾了 %d 筆推薦，僅保留符合條件的標的", before-after)
		}
		if blocked {
			reason = "強制阻擋全部推薦，當日不進場"
		}
		outcomes = append(outcomes, domain.GuardOutcome{
			GuardID:     agent.ID,
			GuardSkill:  agent.Skill,
			Severity:    severity,
			Passed:      !blocked,
			Reason:      reason,
			InputCount:  before,
			OutputCount: after,
		})
		current = next

		if scratchpad != nil {
			if agent.Skill == "cro_risk" {
				scratchpad.Record(ReasoningTrace{
					SessionID: sessionID,
					Timestamp: time.Now().UTC(),
					Phase:     PhaseControlFilter,
					Step:      3,
					Component: "cro_filter",
					Action:    "apply_cro_filter",
					Reasoning: fmt.Sprintf("CRO filter: %d in -> %d out, conviction floor: %d (regime %s)", before, after, effectiveConvictionFloor(policy, regime), regime),
					Data: map[string]any{
						"input_count":                before,
						"output_count":               after,
						"conviction_floor":           policy.ConvictionFloor,
						"effective_conviction_floor": effectiveConvictionFloor(policy, regime),
						"regime":                     string(regime),
						"z_score_enabled":            policy.EnableConvictionNormalization,
						"momentum_crash_protection":  policy.MomentumCrashProtection,
					},
					Confidence: passRatio(before, after),
				})
			}
			if agent.Skill == "cio_portfolio" {
				symbolAgents := make(map[string][]map[string]any)
				for _, rec := range recs[:before] {
					symbolAgents[rec.Symbol] = append(symbolAgents[rec.Symbol], map[string]any{
						"agent":      rec.Agent,
						"conviction": rec.Conviction,
					})
				}
				symbolData := make([]map[string]any, 0, len(next))
				for _, rec := range next {
					agents := symbolAgents[rec.Symbol]
					symbolData = append(symbolData, map[string]any{
						"symbol":              rec.Symbol,
						"agent_count":         len(agents),
						"weighted_conviction": rec.Conviction,
						"agents":              agents,
					})
				}
				scratchpad.Record(ReasoningTrace{
					SessionID: sessionID,
					Timestamp: time.Now().UTC(),
					Phase:     PhaseControlFilter,
					Step:      4,
					Component: "cio_aggregator",
					Action:    "apply_cio_aggregation",
					Reasoning: fmt.Sprintf("CIO aggregation: %d recommendations -> %d unique symbols", before, len(next)),
					Data: map[string]any{
						"input_count":  before,
						"output_count": len(next),
						"symbols":      symbolData,
					},
					Confidence: passRatio(before, len(next)),
				})
			}
		}
	}

	current = applyCrowdingPenalty(current)
	current = applyAntiCorrelationLayer(current, 0)

	// Do NOT overwrite the last guard's OutputCount/Reason here.
	// Each guard already records its own input/output count during control
	// execution (see the loop above). Crowding penalty and anti-correlation
	// filtering are post-guard stages whose effect is reflected in `current`,
	// not in any individual guard's outcome. Overwriting here would conflate
	// "what the guard passed" with "what survived all post-processing", which
	// breaks the audit trail (PassedGuards can no longer attribute filtering
	// to the correct stage). Downstream readers should derive the final count
	// from len(current) and treat GuardOutcome counts as per-guard.

	return current, outcomes
}

// applyCrowdingPenalty reduces conviction when 3+ agents recommend the same symbol.
func applyCrowdingPenalty(recs []domain.Recommendation) []domain.Recommendation {
	if len(recs) == 0 {
		return recs
	}
	symbolAgents := map[string]map[string]struct{}{}
	for _, rec := range recs {
		if _, ok := symbolAgents[rec.Symbol]; !ok {
			symbolAgents[rec.Symbol] = map[string]struct{}{}
		}
		symbolAgents[rec.Symbol][rec.Agent] = struct{}{}
	}

	cfg := config.GetParametersConfig().Engine.Executors
	out := make([]domain.Recommendation, len(recs))
	for i, rec := range recs {
		agents := symbolAgents[rec.Symbol]
		penalty := 1.0
		if len(agents) >= 4 {
			penalty = cfg.CrowdingPenaltyAgents4.Value
		} else if len(agents) >= 3 {
			penalty = cfg.CrowdingPenaltyAgents3.Value
		}
		rec.Conviction = int(float64(rec.Conviction) * penalty)
		out[i] = rec
	}
	return out
}

// applyAntiCorrelationLayer deduplicates by symbol and enforces skill-level diversity.
func applyAntiCorrelationLayer(recs []domain.Recommendation, availableCash float64) []domain.Recommendation {
	if len(recs) == 0 {
		return recs
	}
	bySymbol := map[string]domain.Recommendation{}
	for _, rec := range recs {
		existing, ok := bySymbol[rec.Symbol]
		if !ok || rec.Conviction > existing.Conviction {
			bySymbol[rec.Symbol] = rec
		}
	}

	skillRecs := map[string][]domain.Recommendation{}
	for _, rec := range bySymbol {
		skillRecs[rec.Skill] = append(skillRecs[rec.Skill], rec)
	}
	for skill := range skillRecs {
		slices.SortFunc(skillRecs[skill], func(a, b domain.Recommendation) int {
			if a.Conviction > b.Conviction {
				return -1
			}
			if a.Conviction < b.Conviction {
				return 1
			}
			return 0
		})
	}

	cfg := config.GetParametersConfig().Engine.Executors
	minTrade := cfg.MinTradeAmount.Value
	maxStocks := cfg.MaxStocksDefault.Value
	if availableCash > 0 {
		calculated := min(max(int(availableCash/minTrade), cfg.MaxStocksMin.Value), cfg.MaxStocksMax.Value)
		maxStocks = calculated
	}

	out := make([]domain.Recommendation, 0, len(bySymbol))
	for _, recsForSkill := range skillRecs {
		for i, rec := range recsForSkill {
			if i >= 2 {
				continue
			}
			out = append(out, rec)
		}
	}
	for _, recsForSkill := range skillRecs {
		for i, rec := range recsForSkill {
			if i < 2 {
				continue
			}
			if len(out) < maxStocks {
				out = append(out, rec)
			}
		}
	}

	slices.SortFunc(out, func(a, b domain.Recommendation) int {
		if a.Conviction > b.Conviction {
			return -1
		}
		if a.Conviction < b.Conviction {
			return 1
		}
		if a.Symbol < b.Symbol {
			return -1
		}
		if a.Symbol > b.Symbol {
			return 1
		}
		return 0
	})
	return out
}

func severityForControlAgent(agent domain.AgentSpec) domain.GuardSeverity {
	if agent.Skill == "cro_risk" {
		return domain.GuardSeverityHard
	}
	return domain.GuardSeveritySoft
}

// passRatio returns the ratio of output to input as a 0-1 confidence score.
// Returns 1 when input is 0 (no recommendations to filter).
func passRatio(input, output int) float64 {
	if input <= 0 {
		return 0.0
	}
	ratio := float64(output) / float64(input)
	if ratio > 1.0 {
		return 1.0
	}
	return ratio
}

// effectiveConvictionFloor returns the conviction floor actually applied by the
// CRO filter for the given regime. A6 (perf audit 2026-08-21): during RISK_OFF
// the effective floor is raised to at least 70; otherwise the policy floor is
// used (falling back to the configured default when unset).
func effectiveConvictionFloor(policy domain.ExecutionPolicy, regime domain.Regime) int {
	floor := policy.ConvictionFloor
	if floor <= 0 {
		floor = config.GetParametersConfig().Orchestrator.ConvictionFloorDefault.Value
	}
	if regime == domain.RegimeRiskOff && floor < 70 {
		floor = 70
	}
	return floor
}
