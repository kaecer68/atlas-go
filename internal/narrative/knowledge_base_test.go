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

// allTriggerThemes enumerates the 24 template trigger themes. Kept in sync
// with templates.go DefaultTemplates() — every theme must have at least one
// InvestmentModel so the causality knowledge base is never strategy-dead.
func allTriggerThemes() []string {
	return []string{
		"AI_capex_surge", "JPY_carry_unwind", "USD_TWD_volatility",
		"US_rates_up", "US_rates_down", "gold_rally", "dollar_surge",
		"inflation_spike", "earnings_surprise", "earnings_blackout",
		"tech_peak_season", "year_end_window_dressing", "dividend_season",
		"shipping_rate_spike", "china_slowdown", "taiwan_export_boom",
		"tariff_shock", "geopolitical_risk_spike", "oil_price_shock",
		"semiconductor_downturn", "retail_institutional_divergence",
		"spring_festival_season", "election_cycle", "taiwan_political_risk",
	}
}

// TestAll24ThemesHaveModel is the coverage gate: every causal template's
// trigger theme must map to at least one InvestmentModel, so detected
// narratives always carry an executable sector bet (models = 表).
func TestAll24ThemesHaveModel(t *testing.T) {
	ne := NewNarrativeEngine()
	themes := allTriggerThemes()
	if len(themes) != 24 {
		t.Fatalf("expected 24 trigger themes, got %d", len(themes))
	}
	for _, theme := range themes {
		models := ne.ActiveModels([]string{theme})
		if len(models) == 0 {
			t.Errorf("theme %s has no InvestmentModel (all 24 themes must be covered)", theme)
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
