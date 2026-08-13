package narrative

import (
	"testing"
	"time"
)

func TestSectorBias_FavoredPositiveAvoidedNegative(t *testing.T) {
	ne := NewNarrativeEngine()
	events := []NarrativeEvent{
		{
			ID:         "evt-1",
			Theme:      "AI_capex_surge",
			Region:     "Global",
			Confidence: 0.8,
			HitRate:    0.9,
			Timestamp:  time.Now(),
		},
	}

	// ai_supercycle_model (ActiveThemes=[AI_capex_surge]) favors
	// ai_supply_chain/semiconductor/pcb/thermal and avoids consumer.
	positive := ne.SectorBias("semiconductor", events)
	negative := ne.SectorBias("consumer", events)
	uncovered := ne.SectorBias("tourism", events)

	if positive <= 0 {
		t.Fatalf("expected positive bias for favored sector semiconductor, got %f", positive)
	}
	if negative >= 0 {
		t.Fatalf("expected negative bias for avoided sector consumer, got %f", negative)
	}
	if uncovered != 0 {
		t.Fatalf("expected zero bias for uncovered sector tourism, got %f", uncovered)
	}
}

func TestSectorBias_NoEventsReturnsZero(t *testing.T) {
	ne := NewNarrativeEngine()
	if bias := ne.SectorBias("semiconductor", nil); bias != 0 {
		t.Fatalf("expected 0 bias with no events, got %f", bias)
	}
}

func TestSectorBias_UsesBestConfidencePerTheme(t *testing.T) {
	ne := NewNarrativeEngine()
	// Same theme twice with different confidences: the higher confidence×hit-rate must win.
	events := []NarrativeEvent{
		{
			ID:         "weak",
			Theme:      "AI_capex_surge",
			Confidence: 0.2,
			HitRate:    0.5,
		},
		{
			ID:         "strong",
			Theme:      "AI_capex_surge",
			Confidence: 0.9,
			HitRate:    0.8,
		},
	}
	weak := ne.SectorBias("semiconductor", events[:1])
	strong := ne.SectorBias("semiconductor", events)
	if strong <= weak {
		t.Fatalf("expected stronger confidence to produce larger bias (weak=%f, strong=%f)", weak, strong)
	}
}
