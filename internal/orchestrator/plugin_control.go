package orchestrator

import (
	"fmt"
	"slices"
	"sort"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/portfolio"
)

type ControlExecutor interface {
	Supports(agent domain.AgentSpec) bool
	Apply(agent domain.AgentSpec, recs []domain.Recommendation, policy domain.ExecutionPolicy) []domain.Recommendation
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

func (e CRORiskExecutor) Apply(agent domain.AgentSpec, recs []domain.Recommendation, policy domain.ExecutionPolicy) []domain.Recommendation {
	filtered := make([]domain.Recommendation, 0, len(recs))
	params := config.GetParametersConfig().Orchestrator
	floor := policy.ConvictionFloor
	if floor <= 0 {
		floor = params.ConvictionFloorDefault.Value
	}

	if policy.EnableConvictionNormalization && e.convictionNormalizer != nil {
		for _, rec := range recs {
			e.convictionNormalizer.RecordConviction(rec.Agent, rec.Conviction)
		}
		for _, rec := range recs {
			zScore := e.convictionNormalizer.Normalize(rec.Agent, rec.Conviction, portfolio.ZScore)
			if zScore <= params.CROZScoreThreshold.Value {
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
	default:
		return "other"
	}
}

type CIOPortfolioExecutor struct{}

func (CIOPortfolioExecutor) Supports(agent domain.AgentSpec) bool {
	return agent.Skill == "cio_portfolio"
}

func (CIOPortfolioExecutor) Apply(agent domain.AgentSpec, recs []domain.Recommendation, policy domain.ExecutionPolicy) []domain.Recommendation {
	type agg struct {
		count          int
		conviction     int
		bestConviction int
		reason         string
		skill          string
		targetPrice    float64
		stopLossPrice  float64
		bestAgent      string
	}

	params := config.GetParametersConfig().Orchestrator
	bySymbol := map[string]*agg{}
	for _, rec := range recs {
		entry, ok := bySymbol[rec.Symbol]
		if !ok {
			entry = &agg{reason: rec.Reason, skill: rec.Skill, targetPrice: rec.TargetPrice, stopLossPrice: rec.StopLossPrice, bestConviction: rec.Conviction, bestAgent: rec.Agent}
			bySymbol[rec.Symbol] = entry
		}
		entry.count++
		entry.conviction += rec.Conviction
		if rec.Conviction > entry.bestConviction {
			entry.bestConviction = rec.Conviction
			entry.targetPrice = rec.TargetPrice
			entry.stopLossPrice = rec.StopLossPrice
			entry.bestAgent = rec.Agent
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
		avgConviction := entry.conviction / entry.count
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
			Side:          domain.SideBuy,
			Conviction:    avgConviction,
			Reason:        reason,
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

// CIOPortfolioExecutorWithWeights extends CIOPortfolioExecutor with Darwinian weight support
// This aggregator weights recommendations by agent performance (Atlas-GIC style)
type CIOPortfolioExecutorWithWeights struct {
	WeightManager *portfolio.DarwinianWeightManager
}

func (e CIOPortfolioExecutorWithWeights) Supports(agent domain.AgentSpec) bool {
	return agent.Skill == "cio_portfolio"
}

func (e CIOPortfolioExecutorWithWeights) Apply(agent domain.AgentSpec, recs []domain.Recommendation, policy domain.ExecutionPolicy) []domain.Recommendation {
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

// SuperinvestorExecutor handles superinvestor layer recommendations with quality filtering
type SuperinvestorExecutor struct{}

func (SuperinvestorExecutor) Supports(agent domain.AgentSpec) bool {
	return agent.Layer == domain.LayerSuperinvestor
}

func (SuperinvestorExecutor) Apply(agent domain.AgentSpec, recs []domain.Recommendation, policy domain.ExecutionPolicy) []domain.Recommendation {
	params := config.GetParametersConfig().Orchestrator
	minConviction := params.SuperinvestorMinConviction.Value
	if policy.ConvictionFloor > minConviction {
		minConviction = policy.ConvictionFloor
	}

	filtered := make([]domain.Recommendation, 0, len(recs))
	for _, rec := range recs {
		if rec.Conviction >= minConviction {
			rec.Reason = "[Superinvestor:" + agent.Skill + "] " + rec.Reason
			filtered = append(filtered, rec)
		}
	}
	return filtered
}
