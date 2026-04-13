package monitoring

import (
	"testing"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/live"
)

func TestLiveTradingRules(t *testing.T) {
	rules := LiveTradingRules()
	if len(rules) == 0 {
		t.Fatal("expected live trading rules")
	}

	t.Run("circuit_breaker_triggered fires on large loss", func(t *testing.T) {
		rule := findRule(rules, "circuit_breaker_triggered")
		if rule == nil {
			t.Fatal("rule not found")
		}
		state := &live.State{
			Portfolio: live.PortfolioState{Cash: 1000000, DayPnL: -30000},
		}
		fired, msg := rule.Condition(state)
		if !fired {
			t.Fatalf("expected rule to fire, got: %s", msg)
		}
	})

	t.Run("daily_loss_warning fires on moderate loss", func(t *testing.T) {
		rule := findRule(rules, "daily_loss_warning")
		if rule == nil {
			t.Fatal("rule not found")
		}
		state := &live.State{
			Portfolio: live.PortfolioState{Cash: 1000000, DayPnL: -18000},
		}
		fired, msg := rule.Condition(state)
		if !fired {
			t.Fatalf("expected rule to fire, got: %s", msg)
		}
	})

	t.Run("high_position_concentration fires on large position", func(t *testing.T) {
		rule := findRule(rules, "high_position_concentration")
		if rule == nil {
			t.Fatal("rule not found")
		}
		state := &live.State{
			Portfolio: live.PortfolioState{Cash: 1000000, UnrealizedPnL: 0},
			Positions: []domain.Position{
				{Symbol: "2330", MarketValue: 200000, AverageCost: 100},
			},
		}
		fired, msg := rule.Condition(state)
		if !fired {
			t.Fatalf("expected rule to fire, got: %s", msg)
		}
	})

	t.Run("unrealized_loss_position fires on deep loss", func(t *testing.T) {
		rule := findRule(rules, "unrealized_loss_position")
		if rule == nil {
			t.Fatal("rule not found")
		}
		state := &live.State{
			Portfolio: live.PortfolioState{Cash: 1000000},
			Positions: []domain.Position{
				{Symbol: "2330", MarketValue: 94, AverageCost: 100},
			},
		}
		fired, msg := rule.Condition(state)
		if !fired {
			t.Fatalf("expected rule to fire, got: %s", msg)
		}
	})
}

func findRule(rules []AlertRule, name string) *AlertRule {
	for i := range rules {
		if rules[i].Name == name {
			return &rules[i]
		}
	}
	return nil
}
