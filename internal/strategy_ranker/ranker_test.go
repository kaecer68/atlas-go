package strategy_ranker

import (
	"testing"
)

func TestRankAndTier(t *testing.T) {
	r := New()
	reports := []*StrategyReport{
		{StrategyID: "momentum", StrategyName: "純動能", SharpeRatio: new(0.8), WinRate: 0.55, AlphaScore: new(5.0), MaxDrawdown: new(25.0)},
		{StrategyID: "defensive", StrategyName: "防禦型", SharpeRatio: new(1.2), WinRate: 0.65, AlphaScore: new(-2.0), MaxDrawdown: new(10.0)},
	}

	ranked := r.RankAndTier(reports)
	if len(ranked) != 2 {
		t.Fatalf("expected 2 ranked reports, got %d", len(ranked))
	}
	if ranked[0].Rank != 1 || ranked[1].Rank != 2 {
		t.Errorf("unexpected ranks: %d, %d", ranked[0].Rank, ranked[1].Rank)
	}
}

func TestFreePremiumFilters(t *testing.T) {
	r := New()
	reports := []*StrategyReport{
		{StrategyID: "a", StrategyName: "A", SharpeRatio: new(1.5), WinRate: 0.7, AlphaScore: new(10.0), MaxDrawdown: new(5.0)},
		{StrategyID: "b", StrategyName: "B", SharpeRatio: new(1.0), WinRate: 0.6, AlphaScore: new(5.0), MaxDrawdown: new(10.0)},
		{StrategyID: "c", StrategyName: "C", SharpeRatio: new(0.8), WinRate: 0.55, AlphaScore: new(3.0), MaxDrawdown: new(15.0)},
		{StrategyID: "d", StrategyName: "D", SharpeRatio: new(0.5), WinRate: 0.5, AlphaScore: new(0.0), MaxDrawdown: new(20.0)},
		{StrategyID: "e", StrategyName: "E", SharpeRatio: new(0.3), WinRate: 0.45, AlphaScore: new(-5.0), MaxDrawdown: new(30.0)},
	}

	ranked := r.RankAndTier(reports)

	premium := r.PremiumReports(ranked)
	if len(premium) != 2 {
		t.Errorf("expected 2 premium reports, got %d", len(premium))
	}

	free := r.FreeReports(ranked)
	if len(free) != 1 {
		t.Errorf("expected 1 free report, got %d", len(free))
	}
	if free[0].Tier != "free" {
		t.Errorf("expected 'free' tier, got %q", free[0].Tier)
	}
}
