package config

import (
	"slices"
	"testing"
)

func TestIsDemotedStockpickerCondition(t *testing.T) {
	demoted := []string{
		StockpickerConditionForeign3DNetBuy,
		StockpickerConditionMomentum20DPositive,
		// Source form must be accepted too: callers hold the ledger key.
		StockpickerSourcePrefix + StockpickerConditionForeign3DNetBuy,
	}
	for _, id := range demoted {
		if !IsDemotedStockpickerCondition(id) {
			t.Errorf("IsDemotedStockpickerCondition(%q) = false, want true", id)
		}
		if !IsDemotedStockpickerSource(id) {
			t.Errorf("IsDemotedStockpickerSource(%q) = false, want true", id)
		}
		if DemotedStockpickerConditionReason(id) == "" {
			t.Errorf("DemotedStockpickerConditionReason(%q) empty, want the measurement", id)
		}
	}

	// Retained families and unknown ids stay allowed — the predicate adds a
	// refusal, it is never an allow-list.
	kept := []string{
		"price-volume-bottom-divergence",
		StockpickerSourcePrefix + "price-volume-bottom-divergence",
		"price-volume-top-divergence",
		"",
		"no-such-condition",
	}
	for _, id := range kept {
		if IsDemotedStockpickerCondition(id) {
			t.Errorf("IsDemotedStockpickerCondition(%q) = true, want false", id)
		}
		if got := DemotedStockpickerConditionReason(id); got != "" {
			t.Errorf("DemotedStockpickerConditionReason(%q) = %q, want empty", id, got)
		}
	}
}

func TestDemotedStockpickerConditionIDsSorted(t *testing.T) {
	got := DemotedStockpickerConditionIDs()
	want := []string{StockpickerConditionForeign3DNetBuy, StockpickerConditionMomentum20DPositive}
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("DemotedStockpickerConditionIDs() = %v, want %v", got, want)
	}
}
