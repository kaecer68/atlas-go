package orchestrator

import (
	"encoding/json"
	"testing"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/narrative"
)

// TestNarrativeConvictionModulator_NoProvenance validates that the narrative
// modulator produces correct ConvictionSteps with provenance fields populated.
func TestNarrativeConvictionModulator_NoProvenance(t *testing.T) {
	mod := NewNarrativeConvictionModulator()

	recs := []domain.Recommendation{
		{Agent: "agent1", Conviction: 50, ConvictionBreakdown: &domain.ConvictionBreakdown{Base: 50, Floor: 40, Final: 50, Steps: []domain.ConvictionStep{}}},
	}
	registry := domain.AgentRegistry{
		Agents: []domain.AgentSpec{{ID: "agent1", Skill: "semiconductor_desk"}},
	}
	events := []narrative.NarrativeEvent{
		{Theme: "AI_capex_surge", Status: "active", Confidence: 0.8, HitRate: 0.81},
	}

	steps := mod.CollectModulationSteps(recs, registry, events)
	for _, ms := range steps {
		if ms.RecIndex >= len(recs) {
			continue
		}
		for _, step := range ms.Steps {
			recs[ms.RecIndex].Conviction += step.Delta
			if recs[ms.RecIndex].ConvictionBreakdown != nil {
				recs[ms.RecIndex].ConvictionBreakdown.Steps = append(recs[ms.RecIndex].ConvictionBreakdown.Steps, step)
				recs[ms.RecIndex].ConvictionBreakdown.Final = recs[ms.RecIndex].Conviction
			}
		}
	}

	if len(recs[0].ConvictionBreakdown.Steps) == 0 {
		t.Fatal("expected at least one conviction step after modulation")
	}

	step := recs[0].ConvictionBreakdown.Steps[len(recs[0].ConvictionBreakdown.Steps)-1]
	if step.Rule != "modulator:narrative:narrative_boost" {
		t.Fatalf("expected rule 'modulator:narrative:narrative_boost', got %q", step.Rule)
	}

	if step.Source == "" {
		t.Error("expected Source to be populated")
	}
	if step.ParamRef == "" {
		t.Error("expected ParamRef to be populated")
	}
	if step.ParamValue == "" {
		t.Error("expected ParamValue to be populated")
	}
}

// TestNarrativeConvictionModulator_JSONRoundTrip verifies JSON serialization
// of ConvictionSteps including the new omitempty fields.
func TestNarrativeConvictionModulator_JSONRoundTrip(t *testing.T) {
	step := domain.ConvictionStep{
		Rule:   "narrative_boost",
		Delta:  8,
		Reason: "AI_capex_surge (hit_rate: 81%, confidence: 80%)",
	}

	data, err := json.Marshal(step)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	// With omitempty, empty provenance fields should not appear
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal to map: %v", err)
	}

	if _, hasSource := raw["source"]; hasSource {
		t.Error("expected 'source' to be omitted when empty")
	}
	if _, hasParamRef := raw["param_ref"]; hasParamRef {
		t.Error("expected 'param_ref' to be omitted when empty")
	}
	if _, hasParamValue := raw["param_value"]; hasParamValue {
		t.Error("expected 'param_value' to be omitted when empty")
	}

	// Now test with provenance fields populated
	stepWithProv := domain.ConvictionStep{
		Rule:       "narrative_boost",
		Delta:      8,
		Reason:     "test",
		Source:     "config",
		ParamRef:   "NarrativeConviction.ThemeHitRates.AI_capex_surge",
		ParamValue: "0.81",
	}

	data2, err := json.Marshal(stepWithProv)
	if err != nil {
		t.Fatalf("marshal with provenance: %v", err)
	}

	var raw2 map[string]any
	if err := json.Unmarshal(data2, &raw2); err != nil {
		t.Fatalf("unmarshal to map: %v", err)
	}

	if raw2["source"] != "config" {
		t.Errorf("expected source='config', got %v", raw2["source"])
	}
	if raw2["param_ref"] != "NarrativeConviction.ThemeHitRates.AI_capex_surge" {
		t.Errorf("expected param_ref mismatch, got %v", raw2["param_ref"])
	}
}

// TestNarrativeConvictionModulator_SensitivityField verifies that Sensitivity
// is populated on steps returned by CollectModulationSteps.
func TestNarrativeConvictionModulator_SensitivityField(t *testing.T) {
	mod := NewNarrativeConvictionModulator()

	recs := []domain.Recommendation{
		{Agent: "agent1", Conviction: 50, ConvictionBreakdown: &domain.ConvictionBreakdown{Base: 50, Floor: 40, Final: 50, Steps: []domain.ConvictionStep{}}},
	}
	registry := domain.AgentRegistry{
		Agents: []domain.AgentSpec{{ID: "agent1", Skill: "semiconductor_desk"}},
	}
	events := []narrative.NarrativeEvent{
		{Theme: "AI_capex_surge", Status: "active", Confidence: 0.8, HitRate: 0.81},
	}

	steps := mod.CollectModulationSteps(recs, registry, events)
	if len(steps) == 0 {
		t.Fatal("expected at least one modulation step")
	}
	for _, ms := range steps {
		for _, step := range ms.Steps {
			if step.Sensitivity == nil {
				t.Errorf("expected Sensitivity to be non-nil for step rule=%q, ParamValue=%q", step.Rule, step.ParamValue)
			} else {
				if *step.Sensitivity <= 0 {
					t.Errorf("expected positive Sensitivity, got %f", *step.Sensitivity)
				}
			}
		}
	}
}
