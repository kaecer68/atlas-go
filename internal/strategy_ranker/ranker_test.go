package strategy_ranker

import (
	"testing"
)

func TestRankAndTier(t *testing.T) {
	r := New()
	reports := []*StrategyReport{
		{StrategyID: "momentum", StrategyName: "純動能", SharpeRatio: 0.8, WinRate: 0.55, AlphaScore: 5, MaxDrawdown: 25},
		{StrategyID: "defensive", StrategyName: "防禦型", SharpeRatio: 1.2, WinRate: 0.65, AlphaScore: -2, MaxDrawdown: 10},
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
		{StrategyID: "a", StrategyName: "A", SharpeRatio: 1.5, WinRate: 0.7, AlphaScore: 10, MaxDrawdown: 5},
		{StrategyID: "b", StrategyName: "B", SharpeRatio: 1.0, WinRate: 0.6, AlphaScore: 5, MaxDrawdown: 10},
		{StrategyID: "c", StrategyName: "C", SharpeRatio: 0.8, WinRate: 0.55, AlphaScore: 3, MaxDrawdown: 15},
		{StrategyID: "d", StrategyName: "D", SharpeRatio: 0.5, WinRate: 0.5, AlphaScore: 0, MaxDrawdown: 20},
		{StrategyID: "e", StrategyName: "E", SharpeRatio: 0.3, WinRate: 0.45, AlphaScore: -5, MaxDrawdown: 30},
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
