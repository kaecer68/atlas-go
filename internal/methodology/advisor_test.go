package methodology

import (
	"testing"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
)

func TestAdvisor_AllowedStrategies(t *testing.T) {
	rules := config.TryLoadMethodologyRules("../../configs/methodology_rules.yaml")
	advisor := NewAdvisor(rules)

	tests := []struct {
		period domain.MarketPeriod
		want   []string
	}{
		{domain.PeriodBull, []string{"momentum", "growth", "event_arbitrage"}},
		{domain.PeriodDownturn, []string{"all_weather", "value"}},
		{domain.PeriodBlackSwan, []string{"all_weather", "cash_only"}},
		{domain.PeriodTurnaroundUp, []string{"growth", "momentum", "event_arbitrage"}},
	}
	for _, tt := range tests {
		t.Run(string(tt.period), func(t *testing.T) {
			got := advisor.AllowedStrategies(tt.period)
			if len(got) != len(tt.want) {
				t.Errorf("AllowedStrategies(%v) = %v (len=%d), want %v (len=%d)",
					tt.period, got, len(got), tt.want, len(tt.want))
				return
			}
			for i, id := range tt.want {
				if got[i] != id {
					t.Errorf("AllowedStrategies(%v)[%d] = %q, want %q", tt.period, i, got[i], id)
				}
			}
		})
	}
}

func TestAdvisor_IsStrategyAllowed(t *testing.T) {
	rules := config.TryLoadMethodologyRules("../../configs/methodology_rules.yaml")
	advisor := NewAdvisor(rules)

	if !advisor.IsStrategyAllowed(domain.PeriodBull, "momentum") {
		t.Error("momentum should be allowed in bull")
	}
	if !advisor.IsStrategyAllowed(domain.PeriodBull, "growth") {
		t.Error("growth should be allowed in bull")
	}
	if advisor.IsStrategyAllowed(domain.PeriodDownturn, "momentum") {
		t.Error("momentum should NOT be allowed in downturn")
	}
	if advisor.IsStrategyAllowed(domain.PeriodBlackSwan, "cash_only") {
		t.Log("cash_only is secondary in black_swan — allowed")
	}
}

func TestAdvisor_CashReserve(t *testing.T) {
	rules := config.TryLoadMethodologyRules("../../configs/methodology_rules.yaml")
	advisor := NewAdvisor(rules)

	if got := advisor.CashReserve(domain.PeriodBull); got != 7 {
		t.Errorf("bull cash reserve = %f, want 7", got)
	}
	if got := advisor.CashReserve(domain.PeriodBlackSwan); got != 90 {
		t.Errorf("black_swan cash reserve = %f, want 90", got)
	}
	if got := advisor.CashReserve(domain.PeriodDownturn); got != 45 {
		t.Errorf("downturn cash reserve = %f, want 45", got)
	}
}

func TestAdvisor_FilterStrategies(t *testing.T) {
	rules := config.TryLoadMethodologyRules("../../configs/methodology_rules.yaml")
	advisor := NewAdvisor(rules)

	ranked := []string{"momentum", "growth", "event_arbitrage", "all_weather", "value"}

	// In downturn, only all_weather and value allowed
	got := advisor.FilterStrategies(domain.PeriodDownturn, ranked)
	if len(got) != 2 || got[0] != "all_weather" || got[1] != "value" {
		t.Errorf("FilterStrategies(downturn, %v) = %v, want [all_weather value]", ranked, got)
	}

	// In bull, momentum, growth, event_arbitrage allowed
	got = advisor.FilterStrategies(domain.PeriodBull, ranked)
	if len(got) != 3 {
		t.Errorf("FilterStrategies(bull, %v) = %v (len=%d), want 3 items", ranked, got, len(got))
	}

	// Unknown period → pass through
	got = advisor.FilterStrategies("unknown", ranked)
	if len(got) != len(ranked) {
		t.Errorf("FilterStrategies(unknown) should pass through: got %v, want %v", got, ranked)
	}
}

func TestAdvisor_MacroflowRiskLevel(t *testing.T) {
	rules := config.TryLoadMethodologyRules("../../configs/methodology_rules.yaml")
	advisor := NewAdvisor(rules)

	if got := advisor.MacroflowRiskLevel(domain.PeriodBull); got != "yellow" {
		t.Errorf("bull risk level = %q, want yellow", got)
	}
	if got := advisor.MacroflowRiskLevel(domain.PeriodDownturn); got != "orange" {
		t.Errorf("downturn risk level = %q, want orange", got)
	}
	if got := advisor.MacroflowRiskLevel(domain.PeriodBlackSwan); got != "red" {
		t.Errorf("black_swan risk level = %q, want red", got)
	}
}

func TestRegimeToPeriod(t *testing.T) {
	tests := []struct {
		regime domain.Regime
		want   domain.MarketPeriod
	}{
		{domain.RegimeRiskOn, domain.PeriodBull},
		{domain.RegimeRiskOff, domain.PeriodDownturn},
		{domain.RegimeNeutral, domain.PeriodConsolidation},
	}
	for _, tt := range tests {
		t.Run(string(tt.regime), func(t *testing.T) {
			got := RegimeToPeriod(tt.regime)
			if got != tt.want {
				t.Errorf("RegimeToPeriod(%v) = %v, want %v", tt.regime, got, tt.want)
			}
		})
	}
}

func TestDefaultMethodologyRules(t *testing.T) {
	rules := config.TryLoadMethodologyRules("nonexistent.yaml")
	if rules == nil {
		t.Fatal("TryLoadMethodologyRules should return fallback, not nil")
	}
	if len(rules.Regimes) < 3 {
		t.Errorf("default rules should have at least 3 regimes, got %d", len(rules.Regimes))
	}
	if len(rules.Strategies) < 3 {
		t.Errorf("default rules should have at least 3 strategies, got %d", len(rules.Strategies))
	}

	advisor := NewAdvisor(rules)
	if strategies := advisor.AllowedStrategies(domain.PeriodBull); len(strategies) == 0 {
		t.Error("bull period should have allowed strategies in default config")
	}
}
