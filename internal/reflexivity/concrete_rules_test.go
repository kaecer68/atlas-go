package reflexivity

import (
	"testing"

	"github.com/kaecer68/atlas-go/internal/domain"
)

func TestPriceToFundamentalsRuleReducesConvictionOnCrash(t *testing.T) {
	rule := PriceToFundamentalsRule{}
	recs := []domain.Recommendation{
		{Agent: "a", Symbol: "2330.TW", Side: domain.SideBuy, Conviction: 80},
		{Agent: "b", Symbol: "2317.TW", Side: domain.SideBuy, Conviction: 70},
	}
	quotes := map[string]domain.Quote{
		"2330.TW": {Symbol: "2330.TW", Open: 100, Last: 84, IsTradable: true}, // -16%
		"2317.TW": {Symbol: "2317.TW", Open: 100, Last: 99, IsTradable: true},
	}
	adjusted := rule.Apply(recs, domain.SimulationState{}, quotes)
	if adjusted[0].Conviction >= 80 {
		t.Error("expected conviction reduction after price crash")
	}
	if adjusted[1].Conviction >= 70 {
		t.Error("expected conviction reduction for all recs when crash detected")
	}
}

func TestPnLBehaviorRuleReducesConvictionOnDrawdown(t *testing.T) {
	rule := PnLBehaviorRule{}
	recs := []domain.Recommendation{
		{Agent: "a", Symbol: "2330.TW", Side: domain.SideBuy, Conviction: 80},
	}
	state := domain.SimulationState{CurrentDrawdown: 0.12}
	adjusted := rule.Apply(recs, state, nil)
	if adjusted[0].Conviction >= 80 {
		t.Error("expected conviction reduction when drawdown > 10%")
	}
	if adjusted[0].Conviction != 72 {
		t.Errorf("expected conviction 72, got %d", adjusted[0].Conviction)
	}
}

func TestNarrativeFlowsRuleReducesCrowdedSymbols(t *testing.T) {
	rule := NarrativeFlowsRule{Threshold: 3}
	recs := []domain.Recommendation{
		{Agent: "a", Symbol: "2330.TW", Side: domain.SideBuy, Conviction: 80},
		{Agent: "b", Symbol: "2330.TW", Side: domain.SideBuy, Conviction: 75},
		{Agent: "c", Symbol: "2330.TW", Side: domain.SideBuy, Conviction: 70},
		{Agent: "d", Symbol: "2317.TW", Side: domain.SideBuy, Conviction: 60},
	}
	adjusted := rule.Apply(recs, domain.SimulationState{}, nil)
	if adjusted[0].Conviction >= 80 || adjusted[1].Conviction >= 75 || adjusted[2].Conviction >= 70 {
		t.Error("expected conviction reduction for crowded symbol")
	}
	if adjusted[3].Conviction != 60 {
		t.Error("expected no change for non-crowded symbol")
	}
}

func TestMarketPolicyRuleBoostsOnBroadDecline(t *testing.T) {
	rule := MarketPolicyRule{Threshold: 0.03}
	recs := []domain.Recommendation{
		{Agent: "a", Symbol: "2330.TW", Side: domain.SideBuy, Conviction: 80},
	}
	quotes := map[string]domain.Quote{
		"A": {Symbol: "A", Open: 100, Last: 96, IsTradable: true},
		"B": {Symbol: "B", Open: 100, Last: 95, IsTradable: true},
	}
	adjusted := rule.Apply(recs, domain.SimulationState{}, quotes)
	if adjusted[0].Conviction <= 80 {
		t.Error("expected conviction boost during broad market decline")
	}
	if adjusted[0].Conviction != 84 {
		t.Errorf("expected conviction 84, got %d", adjusted[0].Conviction)
	}
}

func TestReversalDetectionRuleReducesAfterFiveDays(t *testing.T) {
	rule := NewReversalDetectionRule()
	recs := make([]domain.Recommendation, 1)
	for day := 1; day <= 6; day++ {
		recs[0] = domain.Recommendation{Agent: "a", Symbol: "2330.TW", Side: domain.SideBuy, Conviction: 80}
		recs = rule.Apply(recs, domain.SimulationState{}, nil)
	}
	if recs[0].Conviction >= 80 {
		t.Error("expected conviction reduction after 5 consecutive days")
	}
	if recs[0].Conviction != 68 {
		t.Errorf("expected conviction 68, got %d", recs[0].Conviction)
	}
}
