package orchestrator

import "testing"

// TestProductionRegistryLeavesConvictionModulatorsUnwired pins the Batch 2
// decision (issue #1944 Q6 I10/I11/I33): the production registry built by
// buildPluginRegistry installs no conviction modulator, so the industry cycle /
// narrative modulation code in executor_collection.go is unreachable in
// production. The constant is the machine-readable half of that decision.
//
// If this test fails, someone wired a modulator: update ModulatorWiringActive,
// the audit spec (docs/specs/industry-allocation-inert-audit-20260924.md) and
// docs/reference/inert-registry.md in the same change.
func TestProductionRegistryLeavesConvictionModulatorsUnwired(t *testing.T) {
	reg := buildPluginRegistry(nil, nil, nil)
	if reg == nil {
		t.Fatal("buildPluginRegistry returned nil")
	}
	if reg.cycleModulator != nil {
		t.Error("production registry now wires IndustryCycleModulator")
	}
	if reg.narrativeModulator != nil {
		t.Error("production registry now wires NarrativeConvictionModulator")
	}
	if ModulatorWiringActive {
		t.Error("ModulatorWiringActive = true but buildPluginRegistry installs no modulator")
	}
}

// TestSetCycleCardHasNoProductionCaller documents the I11 half: the cycle card
// is only ever set from tests, so GetCycleCard stays nil in the wiring path
// that executor_collection.go would read.
func TestSetCycleCardIsNilByDefaultInProductionShape(t *testing.T) {
	mod := NewIndustryCycleModulator(nil)
	if mod.GetCycleCard() != nil {
		t.Fatal("GetCycleCard must be nil until SetCycleCard is called")
	}
}
