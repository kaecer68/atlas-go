package orchestrator

import (
	"testing"
)

// TestConvictionBuilderAddWithProvenance tests the provenance-aware add method.
func TestConvictionBuilderAddWithProvenance(t *testing.T) {
	b := newConvictionBuilder(50, 20)

	// Use addWithProvenance with source and param info
	b.addWithProvenance("rule1", 10, "positive signal", "config:叙事命中率", "ThemeHitRates.tech", "0.75")

	if b.final != 60 {
		t.Errorf("after addWithProvenance +10: final = %d, want 60", b.final)
	}
	if len(b.steps) != 1 {
		t.Fatalf("steps len = %d, want 1", len(b.steps))
	}

	step := b.steps[0]
	if step.Rule != "rule1" {
		t.Errorf("step.Rule = %q, want 'rule1'", step.Rule)
	}
	if step.Delta != 10 {
		t.Errorf("step.Delta = %d, want 10", step.Delta)
	}
	if step.Reason != "positive signal" {
		t.Errorf("step.Reason = %q, want 'positive signal'", step.Reason)
	}
	if step.Source != "config:叙事命中率" {
		t.Errorf("step.Source = %q, want 'config:叙事命中率'", step.Source)
	}
	if step.ParamRef != "ThemeHitRates.tech" {
		t.Errorf("step.ParamRef = %q, want 'ThemeHitRates.tech'", step.ParamRef)
	}
	if step.ParamValue != "0.75" {
		t.Errorf("step.ParamValue = %q, want '0.75'", step.ParamValue)
	}
}

// TestConvictionBuilderAddWithProvenance_NegativeDelta tests that negative deltas work.
func TestConvictionBuilderAddWithProvenance_NegativeDelta(t *testing.T) {
	b := newConvictionBuilder(50, 20)

	b.addWithProvenance("rule_neg", -15, "negative signal", "config:fallback", "default", "0.0")

	if b.final != 35 {
		t.Errorf("after addWithProvenance -15: final = %d, want 35", b.final)
	}

	step := b.steps[0]
	if step.Delta != -15 {
		t.Errorf("step.Delta = %d, want -15", step.Delta)
	}
}

// TestConvictionBuilderAddWithProvenance_BuildReturnsCorrectBreakdown tests that build works after addWithProvenance.
func TestConvictionBuilderAddWithProvenance_BuildReturnsCorrectBreakdown(t *testing.T) {
	b := newConvictionBuilder(50, 30)

	b.addWithProvenance("rule_pos", 15, "first positive", "src1", "ref1", "val1")
	b.addWithProvenance("rule_neg", -5, "first negative", "src2", "ref2", "val2")

	final, breakdown := b.build()

	if final != 60 {
		t.Errorf("final = %d, want 60", final)
	}
	if breakdown.Base != 50 {
		t.Errorf("breakdown.Base = %d, want 50", breakdown.Base)
	}
	if breakdown.Floor != 30 {
		t.Errorf("breakdown.Floor = %d, want 30", breakdown.Floor)
	}
	if breakdown.Final != 60 {
		t.Errorf("breakdown.Final = %d, want 60", breakdown.Final)
	}
	if len(breakdown.Steps) != 2 {
		t.Errorf("breakdown.Steps len = %d, want 2", len(breakdown.Steps))
	}

	// Verify provenance is preserved in breakdown
	if breakdown.Steps[0].Source != "src1" {
		t.Errorf("step[0].Source = %q, want 'src1'", breakdown.Steps[0].Source)
	}
	if breakdown.Steps[1].ParamRef != "ref2" {
		t.Errorf("step[1].ParamRef = %q, want 'ref2'", breakdown.Steps[1].ParamRef)
	}
}

// TestConvictionBuilderAddWithProvenance_MultipleSteps tests multiple provenance adds.
func TestConvictionBuilderAddWithProvenance_MultipleSteps(t *testing.T) {
	b := newConvictionBuilder(50, 20)

	b.addWithProvenance("rule1", 10, "r1", "s1", "p1", "v1")
	b.addWithProvenance("rule2", 5, "r2", "s2", "p2", "v2")
	b.addWithProvenance("rule3", -3, "r3", "s3", "p3", "v3")

	if b.final != 62 {
		t.Errorf("final = %d, want 62", b.final)
	}
	if len(b.steps) != 3 {
		t.Errorf("steps len = %d, want 3", len(b.steps))
	}

	// Verify all provenance fields
	for i, step := range b.steps {
		expectedSource := "s" + string(rune('1'+i))
		if step.Source != expectedSource {
			t.Errorf("step[%d].Source = %q, want %q", i, step.Source, expectedSource)
		}
	}
}

// TestConvictionBuilderMixedAddAndAddWithProvenance tests mixing add() and addWithProvenance().
func TestConvictionBuilderMixedAddAndAddWithProvenance(t *testing.T) {
	b := newConvictionBuilder(50, 20)

	b.add("legacy_rule", 10, "legacy reason")
	b.addWithProvenance("new_rule", 5, "new reason", "new_source", "new_ref", "new_val")

	if b.final != 65 {
		t.Errorf("final = %d, want 65", b.final)
	}
	if len(b.steps) != 2 {
		t.Errorf("steps len = %d, want 2", len(b.steps))
	}

	// First step (add) has no provenance fields
	if b.steps[0].Source != "" {
		t.Errorf("step[0].Source = %q, want empty for legacy add()", b.steps[0].Source)
	}

	// Second step (addWithProvenance) has provenance
	if b.steps[1].Source != "new_source" {
		t.Errorf("step[1].Source = %q, want 'new_source'", b.steps[1].Source)
	}
}
