package monitoring

import (
	"testing"

	"github.com/kaecer68/atlas-go/internal/domain"
	livestore "github.com/kaecer68/atlas-go/internal/live/store"
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
		state := &livestore.State{
			Portfolio: livestore.PortfolioState{Cash: 1000000, DayPnL: -30000},
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
		state := &livestore.State{
			Portfolio: livestore.PortfolioState{Cash: 1000000, DayPnL: -18000},
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
		state := &livestore.State{
			Portfolio: livestore.PortfolioState{Cash: 1000000, UnrealizedPnL: 0},
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
		state := &livestore.State{
			Portfolio: livestore.PortfolioState{Cash: 1000000},
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

func TestNewRuleEngine(t *testing.T) {
	e := NewRuleEngine(nil)
	if e == nil {
		t.Fatal("NewRuleEngine returned nil")
	}
	if e.rules == nil {
		t.Error("rules should be initialized")
	}
	if e.lastFired == nil {
		t.Error("lastFired should be initialized")
	}
	if e.checkInterval == 0 {
		t.Error("checkInterval should be non-zero")
	}
}

func TestRegisterRule(t *testing.T) {
	e := NewRuleEngine(nil)
	e.RegisterRule(AlertRule{Name: "test-rule"})
	e.RegisterRule(AlertRule{Name: "test-rule-2"})

	e.mu.RLock()
	defer e.mu.RUnlock()
	if len(e.rules) != 2 {
		t.Fatalf("rules len = %d, want 2", len(e.rules))
	}
}

func TestSetCheckInterval(t *testing.T) {
	e := NewRuleEngine(nil)
	e.SetCheckInterval(60)
	if e.checkInterval != 60 {
		t.Errorf("checkInterval = %d, want 60", e.checkInterval)
	}
}

func TestEvaluateRules_NoRules(t *testing.T) {
	e := NewRuleEngine(nil)
	e.EvaluateRules(nil) // no panic on nil state, no rules
}

func TestEvaluateRules_WithRules_NilState(t *testing.T) {
	// DefaultRules conditions all handle nil state via explicit checks.
	e := NewRuleEngine(nil)
	rules := DefaultRules()
	for _, r := range rules {
		e.RegisterRule(r)
	}
	e.EvaluateRules(nil)
}

func TestDefaultRules(t *testing.T) {
	rules := DefaultRules()
	if len(rules) == 0 {
		t.Fatal("expected default rules")
	}
	names := make(map[string]bool)
	for _, r := range rules {
		names[r.Name] = true
	}
	if !names["portfolio_value_drop"] {
		t.Error("missing portfolio_value_drop rule")
	}
	if !names["position_concentration"] {
		t.Error("missing position_concentration rule")
	}
	if !names["system_ready"] {
		t.Error("missing system_ready rule")
	}

	// All conditions should handle nil state.
	for _, r := range rules {
		fired, _ := r.Condition(nil)
		if fired {
			t.Errorf("rule %q should not fire on nil state", r.Name)
		}
	}
}

func findRule(rules []AlertRule, name string) *AlertRule {
	for i := range rules {
		if rules[i].Name == name {
			return &rules[i]
		}
	}
	return nil
}
