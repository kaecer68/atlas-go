package reflexivity

import (
	"github.com/kaecer68/atlas-go/internal/domain"
)

// Rule is a reflexivity feedback rule that adjusts recommendations.
type Rule interface {
	Apply(recs []domain.Recommendation, state domain.SimulationState, quotes map[string]domain.Quote) []domain.Recommendation
}

// PriceToFundamentalsRule triggers a "credit risk premium" when any symbol drops >15%
// intra-day, reducing conviction for all other symbols.
type PriceToFundamentalsRule struct{}

func (r PriceToFundamentalsRule) Apply(recs []domain.Recommendation, state domain.SimulationState, quotes map[string]domain.Quote) []domain.Recommendation {
	crashDetected := false
	for _, q := range quotes {
		if q.Open == 0 {
			continue
		}
		if (q.Last-q.Open)/q.Open <= -0.15 {
			crashDetected = true
			break
		}
	}
	if !crashDetected {
		return recs
	}
	adjusted := make([]domain.Recommendation, len(recs))
	for i, rec := range recs {
		adjusted[i] = rec
		adjusted[i].Conviction = max(int(float64(rec.Conviction)*0.95), 0)
	}
	return adjusted
}

// PnLBehaviorRule scales down new position convictions when portfolio drawdown >10%.
type PnLBehaviorRule struct{}

func (r PnLBehaviorRule) Apply(recs []domain.Recommendation, state domain.SimulationState, quotes map[string]domain.Quote) []domain.Recommendation {
	if state.CurrentDrawdown <= 0.10 {
		return recs
	}
	adjusted := make([]domain.Recommendation, len(recs))
	for i, rec := range recs {
		adjusted[i] = rec
		adjusted[i].Conviction = max(int(float64(rec.Conviction)*0.90), 0)
	}
	return adjusted
}

// NarrativeFlowsRule treats ≥3 agents recommending the same symbol as consensus crowding.
type NarrativeFlowsRule struct {
	Threshold int
}

func (r NarrativeFlowsRule) Apply(recs []domain.Recommendation, state domain.SimulationState, quotes map[string]domain.Quote) []domain.Recommendation {
	threshold := r.Threshold
	if threshold == 0 {
		threshold = 3
	}
	coverage := make(map[string]int)
	for _, rec := range recs {
		if rec.Side == domain.SideBuy {
			coverage[rec.Symbol]++
		}
	}
	crowded := make(map[string]bool)
	for sym, count := range coverage {
		if count >= threshold {
			crowded[sym] = true
		}
	}
	if len(crowded) == 0 {
		return recs
	}
	adjusted := make([]domain.Recommendation, len(recs))
	for i, rec := range recs {
		adjusted[i] = rec
		if crowded[rec.Symbol] {
			adjusted[i].Conviction = max(int(float64(rec.Conviction)*0.90), 0)
		}
	}
	return adjusted
}

// MarketPolicyRule boosts defensive convictions when broad market drops >3% intra-day.
type MarketPolicyRule struct {
	Threshold float64
}

func (r MarketPolicyRule) Apply(recs []domain.Recommendation, state domain.SimulationState, quotes map[string]domain.Quote) []domain.Recommendation {
	threshold := r.Threshold
	if threshold == 0 {
		threshold = 0.03
	}
	if len(quotes) == 0 {
		return recs
	}
	var totalReturn float64
	count := 0
	for _, q := range quotes {
		if q.Open == 0 {
			continue
		}
		totalReturn += (q.Last - q.Open) / q.Open
		count++
	}
	if count == 0 {
		return recs
	}
	avgReturn := totalReturn / float64(count)
	if avgReturn > -threshold {
		return recs
	}
	adjusted := make([]domain.Recommendation, len(recs))
	for i, rec := range recs {
		adjusted[i] = rec
		adjusted[i].Conviction = min(int(float64(rec.Conviction)*1.05), 100)
	}
	return adjusted
}

// ReversalDetectionRule marks symbols with 5+ consecutive days of same-direction
// reinforcement as reflexive extremes, reducing conviction.
type ReversalDetectionRule struct {
	streaks map[string]int // symbol -> consecutive days with BUY recommendation
}

// NewReversalDetectionRule creates a new reversal detection rule.
func NewReversalDetectionRule() *ReversalDetectionRule {
	return &ReversalDetectionRule{streaks: make(map[string]int)}
}

func (r *ReversalDetectionRule) Apply(recs []domain.Recommendation, state domain.SimulationState, quotes map[string]domain.Quote) []domain.Recommendation {
	// Reset symbols not seen today
	seen := make(map[string]bool)
	for _, rec := range recs {
		if rec.Side == domain.SideBuy {
			seen[rec.Symbol] = true
			r.streaks[rec.Symbol]++
		}
	}
	for sym := range r.streaks {
		if !seen[sym] {
			delete(r.streaks, sym)
		}
	}

	extreme := make(map[string]bool)
	for sym, days := range r.streaks {
		if days >= 5 {
			extreme[sym] = true
		}
	}
	if len(extreme) == 0 {
		return recs
	}

	adjusted := make([]domain.Recommendation, len(recs))
	for i, rec := range recs {
		adjusted[i] = rec
		if extreme[rec.Symbol] {
			adjusted[i].Conviction = max(int(float64(rec.Conviction)*0.85), 0)
		}
	}
	return adjusted
}
