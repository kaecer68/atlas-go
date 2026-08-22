package methodology

import (
	"testing"

	"github.com/kaecer68/atlas-go/internal/config"
)

func TestSkillToStrategyCategory(t *testing.T) {
	tests := []struct {
		skill string
		want  string
	}{
		// style
		{"growth_momentum", "growth"},
		{"technical_breakout", "momentum"},
		{"value_yield", "value"},
		{"earnings_quality", "value"},
		// sector desks
		{"semiconductor_desk", "momentum"},
		{"ai_supply_chain_desk", "growth"},
		{"etf_rotation_desk", "all_weather"},
		{"financials_desk", "value"},
		{"shipping_desk", "momentum"},
		{"leo_satellite_desk", "growth"},
		{"mining_desk", "value"},
		{"energy_desk", "value"},
		{"electronics_desk", "momentum"},
		{"consumer_desk", "value"},
		{"industrial_desk", "value"},
		{"robotics_desk", "momentum"},
		// superinvestor
		{"druckenmiller_macro", "momentum"},
		{"aschenbrenner_ai_compute", "growth"},
		{"baker_deep_tech", "growth"},
		{"ackman_quality", "value"},
	}
	for _, tt := range tests {
		t.Run(tt.skill, func(t *testing.T) {
			if got := SkillToStrategyCategory(tt.skill); got != tt.want {
				t.Errorf("SkillToStrategyCategory(%q) = %q, want %q", tt.skill, got, tt.want)
			}
		})
	}
}

func TestSkillToStrategyCategory_UnknownDefaultsToAllWeather(t *testing.T) {
	for _, skill := range []string{"", "unknown_skill", "cio_portfolio", "cro_risk", "taiwan_macro"} {
		if got := SkillToStrategyCategory(skill); got != "all_weather" {
			t.Errorf("SkillToStrategyCategory(%q) = %q, want all_weather (conservative keep)", skill, got)
		}
	}
}

// TestSkillStrategyCategoriesMatchMethodology verifies every mapped category
// is a real charter strategy ID declared in the methodology rules, so the
// advisor's AllowedStrategies gating can actually match.
func TestSkillStrategyCategoriesMatchMethodology(t *testing.T) {
	rules := config.TryLoadMethodologyRules("../../configs/methodology_rules.yaml")
	if rules == nil {
		t.Fatal("failed to load methodology rules")
	}
	known := make(map[string]bool)
	for _, s := range rules.Strategies {
		known[s.ID] = true
	}
	for skill, cat := range skillStrategyCategories {
		if !known[cat] {
			t.Errorf("skill %q maps to %q, which is not a declared charter strategy", skill, cat)
		}
	}
}
