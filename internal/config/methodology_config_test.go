package config

import (
	"slices"
	"testing"
)

func TestGetStrategiesWithCategory_AllPeriods(t *testing.T) {
	rules := TryLoadMethodologyRules("../../configs/methodology_rules.yaml")

	// Expected order and category/priority match configs/methodology_rules.yaml.
	want := map[string][]StrategyBrief{
		"downturn": {
			{ID: "all_weather", Name: "全天候防禦", Category: "defensive", Priority: "primary"},
			{ID: "value", Name: "價值投資", Category: "defensive", Priority: "secondary"},
		},
		"turnaround_up": {
			{ID: "growth", Name: "成長動能", Category: "aggressive", Priority: "primary"},
			{ID: "momentum", Name: "動能追蹤", Category: "aggressive", Priority: "secondary"},
			{ID: "event_arbitrage", Name: "事件套利", Category: "tactical", Priority: "secondary"},
		},
		"bull": {
			{ID: "momentum", Name: "動能追蹤", Category: "aggressive", Priority: "primary"},
			{ID: "growth", Name: "成長動能", Category: "aggressive", Priority: "secondary"},
			{ID: "event_arbitrage", Name: "事件套利", Category: "tactical", Priority: "secondary"},
		},
		"plateau": {
			{ID: "event_arbitrage", Name: "事件套利", Category: "tactical", Priority: "primary"},
			{ID: "value", Name: "價值投資", Category: "defensive", Priority: "secondary"},
			{ID: "all_weather", Name: "全天候防禦", Category: "defensive", Priority: "secondary"},
		},
		"consolidation": {
			{ID: "event_arbitrage", Name: "事件套利", Category: "tactical", Priority: "primary"},
			{ID: "all_weather", Name: "全天候防禦", Category: "defensive", Priority: "secondary"},
		},
		"turnaround_down": {
			{ID: "all_weather", Name: "全天候防禦", Category: "defensive", Priority: "primary"},
		},
		"black_swan": {
			{ID: "all_weather", Name: "全天候防禦", Category: "defensive", Priority: "primary"},
			{ID: "cash_only", Name: "現金為主", Category: "defensive", Priority: "secondary"},
		},
	}

	for periodID, expected := range want {
		t.Run(periodID, func(t *testing.T) {
			got := rules.GetStrategiesWithCategory(periodID)
			if len(got) != len(expected) {
				t.Fatalf("GetStrategiesWithCategory(%q) len=%d, want %d; got=%v",
					periodID, len(got), len(expected), got)
			}
			for i := range expected {
				if got[i] != expected[i] {
					t.Errorf("GetStrategiesWithCategory(%q)[%d] = %+v, want %+v",
						periodID, i, got[i], expected[i])
				}
			}
		})
	}
}

func TestGetStrategiesWithCategory_UnknownPeriod(t *testing.T) {
	rules := TryLoadMethodologyRules("../../configs/methodology_rules.yaml")
	got := rules.GetStrategiesWithCategory("not_a_period")
	if len(got) != 0 {
		t.Errorf("unknown period should return empty slice, got %v", got)
	}
}

func TestGetStrategiesWithCategory_CashOnlyOnlyInBlackSwan(t *testing.T) {
	rules := TryLoadMethodologyRules("../../configs/methodology_rules.yaml")

	periods := []string{"downturn", "turnaround_up", "bull", "plateau", "consolidation", "turnaround_down", "black_swan"}
	for _, p := range periods {
		strats := rules.GetStrategiesWithCategory(p)
		hasCashOnly := slices.ContainsFunc(strats, func(s StrategyBrief) bool { return s.ID == "cash_only" })
		if p == "black_swan" {
			if !hasCashOnly {
				t.Errorf("black_swan must include cash_only")
			}
			continue
		}
		if hasCashOnly {
			t.Errorf("cash_only must NOT appear in %s", p)
		}
	}
}
