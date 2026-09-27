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

// allTriggerThemes enumerates every trigger theme in DefaultTemplates().
// Derived, never hand-written: this used to be a hand-kept count literal that
// silently stopped covering the templates added later (five themes were
// missing) yet still passed, because the old assertion compared the literal
// against its own length instead of against the registry.
func allTriggerThemes() []string {
	templates := DefaultTemplates()
	themes := make([]string, 0, len(templates))
	for _, tmpl := range templates {
		themes = append(themes, tmpl.TriggerTheme)
	}
	return themes
}

// knownModelGaps lists the trigger themes that still have no InvestmentModel.
// The coverage gate below is bidirectional: a theme that is missing a model but
// not listed here fails, and a listed theme that gains a model fails too — so a
// real gap cannot hide and a fixed gap cannot linger as a stale exemption.
var knownModelGaps = map[string]bool{
	"conflict_deescalation": true,
	"dollar_softening":      true,
	"inflation_cool":        true,
	"inflation_moderate":    true,
	"us_earnings_boom":      true,
}

// TestAllThemesHaveModel is the coverage gate: every causal template's trigger
// theme must map to at least one InvestmentModel, so detected narratives always
// carry an executable sector bet (models = 表). Themes that genuinely have no
// model yet are listed in knownModelGaps, which keeps the gap explicit instead
// of hidden behind a stale theme list.
func TestAllThemesHaveModel(t *testing.T) {
	ne := NewNarrativeEngine()
	themes := allTriggerThemes()

	if got, want := len(themes), len(NewDefaultDetectorRegistry().List()); got != want {
		t.Fatalf("DefaultTemplates() exposes %d trigger themes but the registry has %d detectors — templates and detectors must move together", got, want)
	}

	inTemplates := make(map[string]bool, len(themes))
	for _, theme := range themes {
		inTemplates[theme] = true

		models := ne.ActiveModels([]string{theme})
		switch {
		case len(models) == 0 && !knownModelGaps[theme]:
			t.Errorf("theme %s has no InvestmentModel and is not listed in knownModelGaps (every theme must be covered or explicitly exempted)", theme)
		case len(models) > 0 && knownModelGaps[theme]:
			t.Errorf("theme %s now has an InvestmentModel — delete it from knownModelGaps", theme)
		}
	}

	for theme := range knownModelGaps {
		if !inTemplates[theme] {
			t.Errorf("knownModelGaps lists theme %s, which DefaultTemplates() no longer exposes — stale exemption", theme)
		}
	}
}

// TestListModelsCount asserts the total InvestmentModel count (9 original +
// 12 added in the closure plan = 21) so count regressions are caught.
func TestListModelsCount(t *testing.T) {
	ne := NewNarrativeEngine()
	if got := len(ne.ListModels()); got != 21 {
		t.Fatalf("expected 21 InvestmentModels, got %d", got)
	}
}
