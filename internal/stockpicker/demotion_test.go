package stockpicker

import (
	"testing"

	"github.com/kaecer68/atlas-go/internal/config"
)

// TestDemotedConditionIDsAreRegisteredConditions is the drift guard for the
// demotion list in internal/config/stockpicker_edge.go: every id it demotes
// must be a condition this package actually registers. A rename here would
// otherwise turn the demotion into a silent no-op (the predicate would match
// nothing) while the recommendation paths kept serving the family.
func TestDemotedConditionIDsAreRegisteredConditions(t *testing.T) {
	registered := map[string]bool{}
	for _, c := range DefaultConditions() {
		registered[c.ID] = true
	}
	if len(registered) == 0 {
		t.Fatal("DefaultConditions() is empty — the registry moved or broke")
	}
	for _, id := range config.DemotedStockpickerConditionIDs() {
		if !registered[id] {
			t.Errorf("demoted condition %q is not a registered default condition (registered: %v)", id, DefaultConditions())
		}
	}
}

// TestDemotedConditionIDsMatchCanonicalConstants pins the demoted ids to the
// canonical DemoConditionID constants so neither copy can drift alone.
func TestDemotedConditionIDsMatchCanonicalConstants(t *testing.T) {
	if !config.IsDemotedStockpickerCondition(string(ConditionForeign3DNetBuy)) {
		t.Errorf("config demotes %q but the canonical constant changed", ConditionForeign3DNetBuy)
	}
	if !config.IsDemotedStockpickerCondition(string(ConditionMomentum20D)) {
		t.Errorf("config demotes %q but the canonical constant changed", ConditionMomentum20D)
	}
	if !config.IsDemotedStockpickerSource("stockpicker-" + string(ConditionForeign3DNetBuy)) {
		t.Error("the stockpicker- source form of a demoted condition must be refused too")
	}

	// The two retained families must stay allowed: the demotion is a targeted
	// verdict, not a blanket one.
	for _, id := range []string{
		string(ConditionPriceVolumeTopDivergence),
		string(ConditionPriceVolumeBottomDivergence),
	} {
		if config.IsDemotedStockpickerCondition(id) {
			t.Errorf("retained condition %q is demoted, want allowed", id)
		}
	}
}
