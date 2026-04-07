package orchestrator

import (
	"fmt"
	"slices"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/portfolio"
)

type ControlExecutor interface {
	Supports(agent domain.AgentSpec) bool
	Apply(agent domain.AgentSpec, recs []domain.Recommendation, policy domain.ExecutionPolicy) []domain.Recommendation
}

type CRORiskExecutor struct{}

func (CRORiskExecutor) Supports(agent domain.AgentSpec) bool {
	return agent.Skill == "cro_risk"
}

func (CRORiskExecutor) Apply(agent domain.AgentSpec, recs []domain.Recommendation, policy domain.ExecutionPolicy) []domain.Recommendation {
	filtered := make([]domain.Recommendation, 0, len(recs))
	floor := policy.ConvictionFloor
	if floor <= 0 {
		floor = 50
	}
	for _, rec := range recs {
		if rec.Conviction < floor {
			continue
		}
		filtered = append(filtered, rec)
	}
	return filtered
}

type CIOPortfolioExecutor struct{}

func (CIOPortfolioExecutor) Supports(agent domain.AgentSpec) bool {
	return agent.Skill == "cio_portfolio"
}

func (CIOPortfolioExecutor) Apply(agent domain.AgentSpec, recs []domain.Recommendation, policy domain.ExecutionPolicy) []domain.Recommendation {
	type agg struct {
		count      int
		conviction int
		reason     string
		skill      string
	}

	bySymbol := map[string]*agg{}
	for _, rec := range recs {
		entry, ok := bySymbol[rec.Symbol]
		if !ok {
			entry = &agg{reason: rec.Reason, skill: rec.Skill}
			bySymbol[rec.Symbol] = entry
		}
		entry.count++
		entry.conviction += rec.Conviction
	}

	out := make([]domain.Recommendation, 0, len(bySymbol))
	for symbol, entry := range bySymbol {
		out = append(out, domain.Recommendation{
			Agent:      agent.ID,
			Skill:      agent.Skill,
			Symbol:     symbol,
			Side:       domain.SideBuy,
			Conviction: entry.conviction / entry.count,
			Reason:     entry.reason,
		})
	}

	slices.SortFunc(out, func(a, b domain.Recommendation) int {
		switch {
		case a.Conviction > b.Conviction:
			return -1
		case a.Conviction < b.Conviction:
			return 1
		default:
			return 0
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
	}

	bySymbol := map[string]*agg{}

	for _, rec := range recs {
		entry, ok := bySymbol[rec.Symbol]
		if !ok {
			entry = &agg{reason: rec.Reason, skill: rec.Skill}
			bySymbol[rec.Symbol] = entry
		}

		// Get agent weight from Darwinian manager (default 1.0)
		agentWeight := 1.0
		if e.WeightManager != nil {
			agentWeight = e.WeightManager.GetWeight(rec.Agent)
		}

		// Weighted contribution: conviction * agent_weight
		entry.weightedConviction += float64(rec.Conviction) * agentWeight
		entry.totalWeight += agentWeight
		entry.count++
	}

	out := make([]domain.Recommendation, 0, len(bySymbol))
	for symbol, entry := range bySymbol {
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

		out = append(out, domain.Recommendation{
			Agent:      agent.ID,
			Skill:      agent.Skill,
			Symbol:     symbol,
			Side:       domain.SideBuy,
			Conviction: weightedConviction,
			Reason:     reasonPrefix + entry.reason,
		})
	}

	slices.SortFunc(out, func(a, b domain.Recommendation) int {
		switch {
		case a.Conviction > b.Conviction:
			return -1
		case a.Conviction < b.Conviction:
			return 1
		default:
			return 0
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
	// Superinvestor agents typically have higher conviction thresholds
	minConviction := 65 // Higher bar for superinvestor recommendations
	if policy.ConvictionFloor > minConviction {
		minConviction = policy.ConvictionFloor
	}

	filtered := make([]domain.Recommendation, 0, len(recs))
	for _, rec := range recs {
		if rec.Conviction >= minConviction {
			// Mark as superinvestor-sourced
			rec.Reason = "[Superinvestor:" + agent.Skill + "] " + rec.Reason
			filtered = append(filtered, rec)
		}
	}
	return filtered
}
