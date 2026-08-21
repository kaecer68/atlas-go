package orchestrator

import (
	"fmt"
	"math"
	"slices"
	"sort"
	"strings"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/portfolio"
)

type ControlExecutor interface {
	Supports(agent domain.AgentSpec) bool
	Apply(agent domain.AgentSpec, recs []domain.Recommendation, policy domain.ExecutionPolicy, regime domain.Regime) []domain.Recommendation
}

type CRORiskExecutor struct {
	convictionNormalizer *portfolio.ConvictionNormalizer
}

func NewCRORiskExecutor() *CRORiskExecutor {
	return &CRORiskExecutor{
		convictionNormalizer: portfolio.NewConvictionNormalizer(),
	}
}

func (CRORiskExecutor) Supports(agent domain.AgentSpec) bool {
	return agent.Skill == "cro_risk"
}

func (e CRORiskExecutor) Apply(agent domain.AgentSpec, recs []domain.Recommendation, policy domain.ExecutionPolicy, regime domain.Regime) []domain.Recommendation {
	filtered := make([]domain.Recommendation, 0, len(recs))
	params := config.GetParametersConfig().Orchestrator
	// A6 (perf audit 2026-08-21): during RISK_OFF the effective conviction
	// floor is raised to 70 (max with the policy floor) so low-conviction
	// recommendations are not pushed into a risk-off market. RISK_ON/Neutral
	// keep the policy floor unchanged.
	floor := effectiveConvictionFloor(policy, regime)

	if policy.EnableConvictionNormalization && e.convictionNormalizer != nil {
		for _, rec := range recs {
			e.convictionNormalizer.RecordConviction(rec.Agent, rec.Conviction)
		}
		for _, rec := range recs {
			zScore := e.convictionNormalizer.Normalize(rec.Agent, rec.Conviction, portfolio.ZScore)
			if zScore <= params.CROZScoreThreshold.Value {
				continue
			}
			// A6: even with z-score normalization enabled, RISK_OFF still gates
			// on absolute conviction (effective floor >= 70) so low-conviction
			// recommendations never enter a risk-off market.
			if regime == domain.RegimeRiskOff && rec.Conviction < floor {
				continue
			}
			if rec.StopLossPrice > 0 && rec.TargetPrice > 0 && rec.Side == domain.SideBuy {
				if rec.StopLossPrice >= rec.TargetPrice {
					continue
				}
			}
			filtered = append(filtered, rec)
		}
	} else {
		for _, rec := range recs {
			if rec.Conviction < floor {
				continue
			}
			if rec.StopLossPrice > 0 && rec.TargetPrice > 0 && rec.Side == domain.SideBuy {
				if rec.StopLossPrice >= rec.TargetPrice {
					continue
				}
			}
			filtered = append(filtered, rec)
		}
	}

	sectorCount := map[string]int{}
	for _, rec := range filtered {
		sector := skillToSector(rec.Skill)
		sectorCount[sector]++
	}

	if len(filtered) > 3 {
		result := make([]domain.Recommendation, 0, len(filtered))
		for _, rec := range filtered {
			sector := skillToSector(rec.Skill)
			concentrationThreshold := params.SectorConcentrationThreshold.Value
			if len(filtered) >= 10 {
				concentrationThreshold = params.SectorConcentrationThresholdHigh.Value
			}
			if float64(sectorCount[sector])/float64(len(filtered)) > concentrationThreshold {
				rec.Reason = fmt.Sprintf("[CRO:產業集中 %.0f%%] ", float64(sectorCount[sector])/float64(len(filtered))*100) + rec.Reason
				rec.Conviction = int(float64(rec.Conviction) * params.SectorConvictionMultiplier.Value)
				if rec.Conviction < floor {
					continue
				}
			}
			result = append(result, rec)
		}
		return result
	}

	return filtered
}

func skillToSector(skill string) string {
	switch skill {
	case "semiconductor", "ai_supply_chain", "pcb", "thermal":
		return "technology"
	case "financials", "value_yield":
		return "financials"
	case "shipping":
		return "shipping"
	case "consumer", "tourism":
		return "consumer"
	case "leo_satellite":
		return "technology"
	default:
		return "other"
	}
}

type CIOPortfolioExecutor struct{}

func (CIOPortfolioExecutor) Supports(agent domain.AgentSpec) bool {
	return agent.Skill == "cio_portfolio"
}

func (CIOPortfolioExecutor) Apply(agent domain.AgentSpec, recs []domain.Recommendation, policy domain.ExecutionPolicy, regime domain.Regime) []domain.Recommendation {
	type agg struct {
		count          int
		conviction     int
		bestConviction int
		reason         string
		skill          string
		targetPrice    float64
		stopLossPrice  float64
		bestAgent      string
		bestSide       domain.Side
	}

	params := config.GetParametersConfig().Orchestrator
	bySymbol := map[string]*agg{}
	for _, rec := range recs {
		entry, ok := bySymbol[rec.Symbol]
		if !ok {
			entry = &agg{reason: rec.Reason, skill: rec.Skill, targetPrice: rec.TargetPrice, stopLossPrice: rec.StopLossPrice, bestConviction: rec.Conviction, bestAgent: rec.Agent, bestSide: rec.Side}
			bySymbol[rec.Symbol] = entry
		}
		entry.count++
		entry.conviction += rec.Conviction
		if rec.Conviction > entry.bestConviction {
			entry.bestConviction = rec.Conviction
			entry.targetPrice = rec.TargetPrice
			entry.stopLossPrice = rec.StopLossPrice
			entry.bestAgent = rec.Agent
			entry.bestSide = rec.Side
		}
	}

	out := make([]domain.Recommendation, 0, len(bySymbol))
	symbols := make([]string, 0, len(bySymbol))
	for symbol := range bySymbol {
		symbols = append(symbols, symbol)
	}
	sort.Strings(symbols)

	for _, symbol := range symbols {
		entry := bySymbol[symbol]
		avgConviction := int(float64(entry.conviction) / float64(entry.count))
		if entry.count >= 3 {
			avgConviction = int(float64(avgConviction) * params.CrowdedConvictionMultiplier.Value)
		}
		reason := entry.reason
		if entry.count >= 3 {
			reason = fmt.Sprintf("[crowded:%d agents] ", entry.count) + reason
		}
		out = append(out, domain.Recommendation{
			Agent:         entry.bestAgent,
			Skill:         agent.Skill,
			Layer:         agent.Layer,
			Symbol:        symbol,
			Side:          entry.bestSide,
			Conviction:    avgConviction,
			Reason:        reason,
			TargetPrice:   entry.targetPrice,
			StopLossPrice: entry.stopLossPrice,
		})
	}

	// Apply diversity bonus: penalize crowded symbols, reward unique high-conviction picks.
	for i := range out {
		entry := bySymbol[out[i].Symbol]
		if entry.count > 1 {
			penalty := math.Min(0.20, float64(entry.count-1)*0.05)
			out[i].Conviction = int(float64(out[i].Conviction) * (1 - penalty))
		} else if out[i].Conviction >= 50 {
			out[i].Conviction += 5
			if out[i].Conviction > 100 {
				out[i].Conviction = 100
			}
		}
	}

	slices.SortFunc(out, func(a, b domain.Recommendation) int {
		switch {
		case a.Conviction > b.Conviction:
			return -1
		case a.Conviction < b.Conviction:
			return 1
		default:
			switch {
			case a.Symbol < b.Symbol:
				return -1
			case a.Symbol > b.Symbol:
				return 1
			case a.Reason < b.Reason:
				return -1
			case a.Reason > b.Reason:
				return 1
			default:
				return 0
			}
		}
	})
	return out
}

// CIOPortfolioExecutorWithWeights extends CIOPortfolioExecutor with Darwinian weight support
// This aggregator weights recommendations by agent performance (Atlas-GIC style)
type CIOPortfolioExecutorWithWeights struct {
	WeightManager *portfolio.DarwinianWeightManager
}

func (e CIOPortfolioExecutorWithWeights) Supports(agent domain.AgentSpec) bool {
	return agent.Skill == "cio_portfolio"
}

func (e CIOPortfolioExecutorWithWeights) Apply(agent domain.AgentSpec, recs []domain.Recommendation, policy domain.ExecutionPolicy, regime domain.Regime) []domain.Recommendation {
	type agg struct {
		weightedConviction float64
		totalWeight        float64
		count              int
		reason             string
		skill              string
		targetPrice        float64
		stopLossPrice      float64
		bestScore          float64
		bestAgent          string
		bestSide           domain.Side
	}

	bySymbol := map[string]*agg{}

	for _, rec := range recs {
		entry, ok := bySymbol[rec.Symbol]
		if !ok {
			entry = &agg{reason: rec.Reason, skill: rec.Skill, targetPrice: rec.TargetPrice, stopLossPrice: rec.StopLossPrice, bestScore: -1, bestAgent: rec.Agent}
			bySymbol[rec.Symbol] = entry
		}

		// Get agent weight from Darwinian manager (default 1.0)
		agentWeight := 1.0
		if e.WeightManager != nil {
			agentWeight = e.WeightManager.GetWeight(rec.Agent)
		}

		// Weighted contribution: conviction * agent_weight
		score := float64(rec.Conviction) * agentWeight
		entry.weightedConviction += score
		entry.totalWeight += agentWeight
		entry.count++
		if score > entry.bestScore {
			entry.bestScore = score
			entry.targetPrice = rec.TargetPrice
			entry.stopLossPrice = rec.StopLossPrice
			entry.bestAgent = rec.Agent
			entry.bestSide = rec.Side
		}
	}

	out := make([]domain.Recommendation, 0, len(bySymbol))
	symbols := make([]string, 0, len(bySymbol))
	for symbol := range bySymbol {
		symbols = append(symbols, symbol)
	}
	sort.Strings(symbols)

	for _, symbol := range symbols {
		entry := bySymbol[symbol]
		// Calculate weighted average conviction
		var weightedConviction int
		if entry.totalWeight > 0 {
			weightedConviction = int(entry.weightedConviction / entry.totalWeight)
		}

		// Add metadata to reason about weighting
		reasonPrefix := ""
		if e.WeightManager != nil && entry.count > 0 {
			reasonPrefix = fmt.Sprintf("[Weighted:%d agents] ", entry.count)
		}
		if entry.count >= 3 {
			reasonPrefix = fmt.Sprintf("[crowded:%d agents] ", entry.count) + reasonPrefix
			weightedConviction = int(float64(weightedConviction) * 0.7)
		}

		out = append(out, domain.Recommendation{
			Agent:         entry.bestAgent,
			Skill:         agent.Skill,
			Layer:         agent.Layer,
			Symbol:        symbol,
			Side:          domain.SideBuy,
			Conviction:    weightedConviction,
			Reason:        reasonPrefix + entry.reason,
			TargetPrice:   entry.targetPrice,
			StopLossPrice: entry.stopLossPrice,
		})
	}

	slices.SortFunc(out, func(a, b domain.Recommendation) int {
		switch {
		case a.Conviction > b.Conviction:
			return -1
		case a.Conviction < b.Conviction:
			return 1
		default:
			switch {
			case a.Symbol < b.Symbol:
				return -1
			case a.Symbol > b.Symbol:
				return 1
			case a.Reason < b.Reason:
				return -1
			case a.Reason > b.Reason:
				return 1
			default:
				return 0
			}
		}
	})
	return out
}

// SuperinvestorExecutor implements BOTH AgentExecutor (signal generation via Recommend)
// and ControlExecutor (quality gate via Apply) for superinvestor-layer agents.
//
// DUAL ROLE — Portfolio Manager analogy:
//   - AgentExecutor.Recommend(): Generates themed, high-conviction recommendations.
//     Each superinvestor acts as a PM with concentrated universe and philosophy-specific
//     conviction adjustments. Base conviction starts at SuperinvestorConvictionBase (70),
//     higher than sector agents (~55-60), reflecting a PM's quality bar.
//   - ControlExecutor.Apply(): Filters ALL recommendations by SuperinvestorMinConviction (65).
//     This quality gate runs AFTER Darwinian weighting but BEFORE final output.
//
// The same struct appears in BOTH builtinAgentExecutors() and builtinControlExecutors()
// in loader.go. Do NOT split into separate structs — the dual role is intentional.
//
// Darwinian tracking: LayerSuperinvestor agents are automatically tracked by
// DarwinianWeightManager (see darwinian_weights.go InitializeFromRegistry).
// Performance-based weight adjustments flow back into conviction via
// ApplyDarwinianWeightsWithEvents().
type SuperinvestorExecutor struct{}

func (SuperinvestorExecutor) Supports(agent domain.AgentSpec) bool {
	return agent.Layer == domain.LayerSuperinvestor
}

// Recommend generates a themed, high-conviction recommendation for superinvestor agents.
// This is the Portfolio Manager role: concentrated, thesis-driven picks from a curated universe.
//
// Conviction logic:
//  1. Base = SuperinvestorConvictionBase parameter (default 70, higher than sector's ~55)
//  2. Theme-specific keyword boosts (driven by agent prompt content + quote conditions)
//  3. Momentum and liquidity factor adjustments
//  4. Floor check against SuperinvestorMinConviction — PMs don't recommend unless conviction
//     meets the quality bar
func (SuperinvestorExecutor) Recommend(agent domain.AgentSpec, quote domain.Quote, prompt string, regime domain.Regime, fq FactorQuery) (domain.Recommendation, bool) {
	params := config.GetParametersConfig().Orchestrator
	minConv := params.SuperinvestorMinConviction.Value

	// Use dynamicSignalStrength as base (55-75 range) to match sector agent behavior.
	// Add superinvestor conviction premium (+5) to maintain PM quality bar.
	b := newConvictionBuilder(dynamicSignalStrength(quote, signalParamsFromAgent(agent))+5, minConv)

	// Theme-specific conviction adjustments based on agent skill
	switch agent.Skill {
	case "druckenmiller_macro":
		if strings.Contains(prompt, "momentum") && quote.Last > quote.Open {
			b.add("macro_momentum_boost", 8, "momentum keyword + uptrend matches Druckenmiller thesis")
		}
		if strings.Contains(prompt, "asymmetric") && quote.Volume > 10_000_000 {
			b.add("asymmetric_volume", 5, "asymmetric keyword + high volume")
		}
		if strings.Contains(prompt, "macro") && quote.Last > quote.High*0.98 {
			b.add("macro_near_high", 5, "macro keyword + near session high")
		}

	case "aschenbrenner_ai_compute":
		if strings.Contains(prompt, "ai_capex") && quote.Last > quote.Open {
			b.add("ai_capex_boost", 8, "ai_capex keyword + uptrend matches compute cycle thesis")
		}
		if strings.Contains(prompt, "compute") && quote.Last > quote.High*0.98 {
			b.add("compute_near_high", 5, "compute keyword + near session high")
		}
		if strings.Contains(prompt, "datacenter") {
			b.add("datacenter_boost", 6, "datacenter keyword indicates infrastructure demand")
		}

	case "baker_deep_tech":
		if strings.Contains(prompt, "ip_moat") && quote.Last > quote.Open {
			b.add("ip_moat_boost", 8, "ip_moat keyword + uptrend indicates moat recognition")
		}
		if strings.Contains(prompt, "patent") {
			b.add("patent_boost", 5, "patent keyword indicates IP catalyst")
		}
		if strings.Contains(prompt, "differentiation") && quote.Last > quote.High*0.98 {
			b.add("differentiation_near_high", 5, "differentiation keyword + strong close")
		}

	case "ackman_quality":
		if strings.Contains(prompt, "quality") && quote.Last > quote.Open {
			b.add("quality_boost", 8, "quality keyword + uptrend matches compounder thesis")
		}
		if strings.Contains(prompt, "catalyst") {
			b.add("catalyst_boost", 7, "catalyst keyword indicates near-term activation")
		}
		if strings.Contains(prompt, "compounder") && quote.Last > quote.High*0.99 {
			b.add("compounder_near_high", 5, "compounder keyword + near session high")
		}
	}

	// PMs require price confirmation — penalize weak closes
	if quote.Last < quote.Open {
		b.add("weak_close_penalty", -10, "last < open — PM requires price confirmation")
	}
	if quote.Last < quote.High*0.95 {
		b.add("far_from_high_penalty", -8, "last < 95% of high — weak session")
	}

	fc := loadFactorConfig()
	addMomentumAdjustment(b, fq, quote.Symbol, fc)
	addLiquidityAdjustment(b, fq, quote.Symbol, fc)

	if !b.floorCheck() {
		return domain.Recommendation{}, false
	}

	// Tighter price targets for PMs (10% upside, 8% downside vs sector's 7%/6%)
	tp, slp := priceTargets(quote, 1.10, 0.92)
	conv, cb := b.build()

	reason := "superinvestor thematic conviction"
	switch agent.Skill {
	case "druckenmiller_macro":
		reason = "macro momentum asymmetric thesis"
	case "aschenbrenner_ai_compute":
		reason = "AI compute cycle durable demand thesis"
	case "baker_deep_tech":
		reason = "deep tech IP moat differentiation thesis"
	case "ackman_quality":
		reason = "quality compounder catalyst thesis"
	}

	return domain.Recommendation{
		Agent:               agent.ID,
		Skill:               agent.Skill,
		Layer:               agent.Layer,
		Symbol:              quote.Symbol,
		Side:                domain.SideBuy,
		Conviction:          conv,
		Reason:              reason,
		TargetPrice:         tp,
		StopLossPrice:       slp,
		ConvictionBreakdown: cb,
	}, true
}

// Apply implements ControlExecutor — quality gate that filters recommendations.
//
// B02 fix (perf-report-zero audit): the SuperinvestorMinConviction bar (65)
// must only apply to recommendations that ORIGINATED from a superinvestor-layer
// agent. The CIO aggregator rewrites rec.Layer to "control" and preserves the
// original source in rec.Agent, so source is identified by the `super-` agent
// id prefix (registry convention: super-dru-01 / super-asc-01 / super-bak-01 /
// super-ack-01). Sector/ETF recs (e.g. etf-rotation-01 → 00713.TW at conv 40-60)
// must only clear the baseline ConvictionFloor — gating them at 65 starves the
// daily sim of every non-superinvestor rec, producing perpetual 0-order sessions.
func (SuperinvestorExecutor) Apply(agent domain.AgentSpec, recs []domain.Recommendation, policy domain.ExecutionPolicy, regime domain.Regime) []domain.Recommendation {
	params := config.GetParametersConfig().Orchestrator

	filtered := make([]domain.Recommendation, 0, len(recs))
	for _, rec := range recs {
		minConviction := policy.ConvictionFloor
		if strings.HasPrefix(rec.Agent, "super-") {
			minConviction = max(policy.ConvictionFloor, params.SuperinvestorMinConviction.Value)
		}
		if rec.Conviction >= minConviction {
			rec.Reason = "[Superinvestor:" + agent.Skill + "] " + rec.Reason
			filtered = append(filtered, rec)
		}
	}
	return filtered
}
