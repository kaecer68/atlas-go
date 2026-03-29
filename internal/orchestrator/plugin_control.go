package orchestrator

import (
	"slices"

	"github.com/kaecer68/atlas-go/internal/domain"
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
