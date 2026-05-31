package narrative

import (
	"testing"

	"github.com/kaecer68/atlas-go/internal/industry"
)

func TestCorrelationMultiplier(t *testing.T) {
	sb := NewSeasonalBridge(nil)

	tests := []struct {
		name      string
		theme     string
		industryA string
		industryB string
		want      float64
	}{
		{
			name:      "oil_price_shock energy+shipping",
			theme:     "oil_price_shock",
			industryA: "energy",
			industryB: "shipping",
			want:      1.15,
		},
		{
			name:      "oil_price_shock shipping+energy reversed",
			theme:     "oil_price_shock",
			industryA: "shipping",
			industryB: "energy",
			want:      1.15,
		},
		{
			name:      "oil_price_shock energy+industrial",
			theme:     "oil_price_shock",
			industryA: "energy",
			industryB: "industrial",
			want:      1.15,
		},
		{
			name:      "oil_price_shock shipping+industrial dampened",
			theme:     "oil_price_shock",
			industryA: "shipping",
			industryB: "industrial",
			want:      0.92,
		},
		{
			name:      "oil_price_shock semiconductor+ai_supply_chain no effect",
			theme:     "oil_price_shock",
			industryA: "semiconductor",
			industryB: "ai_supply_chain",
			want:      1.0,
		},
		{
			name:      "AI_capex_surge semiconductor+ai_supply_chain amplified",
			theme:     "AI_capex_surge",
			industryA: "semiconductor",
			industryB: "ai_supply_chain",
			want:      1.12,
		},
		{
			name:      "AI_capex_surge semiconductor+electronics amplified",
			theme:     "AI_capex_surge",
			industryA: "semiconductor",
			industryB: "electronics",
			want:      1.12,
		},
		{
			name:      "AI_capex_surge ai_supply_chain+electronics",
			theme:     "AI_capex_surge",
			industryA: "ai_supply_chain",
			industryB: "electronics",
			want:      1.12,
		},
		{
			name:      "AI_capex_surge shipping+energy no effect",
			theme:     "AI_capex_surge",
			industryA: "shipping",
			industryB: "energy",
			want:      1.0,
		},
		{
			name:      "US_rates_up financials+shipping",
			theme:     "US_rates_up",
			industryA: "financials",
			industryB: "shipping",
			want:      1.10,
		},
		{
			name:      "US_rates_up financials+industrial",
			theme:     "US_rates_up",
			industryA: "financials",
			industryB: "industrial",
			want:      1.08,
		},
		{
			name:      "US_rates_up semiconductor+ai_supply_chain no effect",
			theme:     "US_rates_up",
			industryA: "semiconductor",
			industryB: "ai_supply_chain",
			want:      1.0,
		},
		{
			name:      "JPY_carry_unwind semiconductor+ai_supply_chain dampened",
			theme:     "JPY_carry_unwind",
			industryA: "semiconductor",
			industryB: "ai_supply_chain",
			want:      0.90,
		},
		{
			name:      "JPY_carry_unwind ai_supply_chain+shipping",
			theme:     "JPY_carry_unwind",
			industryA: "ai_supply_chain",
			industryB: "shipping",
			want:      0.90,
		},
		{
			name:      "JPY_carry_unwind financials+consumer no effect",
			theme:     "JPY_carry_unwind",
			industryA: "financials",
			industryB: "consumer",
			want:      1.0,
		},
		{
			name:      "geopolitical_risk_spike semiconductor+shipping",
			theme:     "geopolitical_risk_spike",
			industryA: "semiconductor",
			industryB: "shipping",
			want:      0.88,
		},
		{
			name:      "geopolitical_risk_spike electronics+shipping",
			theme:     "geopolitical_risk_spike",
			industryA: "electronics",
			industryB: "shipping",
			want:      0.88,
		},
		{
			name:      "geopolitical_risk_spike consumer+financials",
			theme:     "geopolitical_risk_spike",
			industryA: "consumer",
			industryB: "financials",
			want:      1.08,
		},
		{
			name:      "geopolitical_risk_spike energy+shipping no effect",
			theme:     "geopolitical_risk_spike",
			industryA: "energy",
			industryB: "shipping",
			want:      1.0,
		},
		{
			name:      "unknown theme returns 1.0",
			theme:     "unknown_theme",
			industryA: "semiconductor",
			industryB: "ai_supply_chain",
			want:      1.0,
		},
		{
			name:      "geopolitical_risk_spike leo_satellite+semiconductor amplified",
			theme:     "geopolitical_risk_spike",
			industryA: "leo_satellite",
			industryB: "semiconductor",
			want:      1.15,
		},
		{
			name:      "geopolitical_risk_spike semiconductor+leo_satellite reversed",
			theme:     "geopolitical_risk_spike",
			industryA: "semiconductor",
			industryB: "leo_satellite",
			want:      1.15,
		},
		{
			name:      "geopolitical_risk_spike leo_satellite+ai_supply_chain amplified",
			theme:     "geopolitical_risk_spike",
			industryA: "leo_satellite",
			industryB: "ai_supply_chain",
			want:      1.12,
		},
		{
			name:      "AI_capex_surge leo_satellite+semiconductor amplified",
			theme:     "AI_capex_surge",
			industryA: "leo_satellite",
			industryB: "semiconductor",
			want:      1.10,
		},
		{
			name:      "AI_capex_surge leo_satellite+ai_supply_chain amplified",
			theme:     "AI_capex_surge",
			industryA: "leo_satellite",
			industryB: "ai_supply_chain",
			want:      1.08,
		},
		{
			name:      "taiwan_political_risk leo_satellite+semiconductor",
			theme:     "taiwan_political_risk",
			industryA: "leo_satellite",
			industryB: "semiconductor",
			want:      1.12,
		},
		{
			name:      "taiwan_political_risk leo_satellite+ai_supply_chain",
			theme:     "taiwan_political_risk",
			industryA: "leo_satellite",
			industryB: "ai_supply_chain",
			want:      1.10,
		},
		{
			name:      "JPY_carry_unwind leo_satellite+electronics dampened",
			theme:     "JPY_carry_unwind",
			industryA: "leo_satellite",
			industryB: "electronics",
			want:      0.92,
		},
		{
			name:      "geopolitical_risk_spike mining+semiconductor amplified",
			theme:     "geopolitical_risk_spike",
			industryA: "mining",
			industryB: "semiconductor",
			want:      1.12,
		},
		{
			name:      "geopolitical_risk_spike mining+electronics amplified",
			theme:     "geopolitical_risk_spike",
			industryA: "mining",
			industryB: "electronics",
			want:      1.10,
		},
		{
			name:      "geopolitical_risk_spike mining+energy amplified",
			theme:     "geopolitical_risk_spike",
			industryA: "mining",
			industryB: "energy",
			want:      1.08,
		},
		{
			name:      "oil_price_shock mining+energy cost pass-through",
			theme:     "oil_price_shock",
			industryA: "mining",
			industryB: "energy",
			want:      1.08,
		},
		{
			name:      "JPY_carry_unwind mining+financials safe-haven",
			theme:     "JPY_carry_unwind",
			industryA: "mining",
			industryB: "financials",
			want:      1.05,
		},
		{
			name:      "AI_capex_surge leo_satellite+shipping no effect",
			theme:     "AI_capex_surge",
			industryA: "leo_satellite",
			industryB: "shipping",
			want:      1.0,
		},
		{
			name:      "oil_price_shock mining+semiconductor no direct effect",
			theme:     "oil_price_shock",
			industryA: "mining",
			industryB: "semiconductor",
			want:      1.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sb.CorrelationMultiplier(tt.theme, tt.industryA, tt.industryB)
			if got != tt.want {
				t.Fatalf("CorrelationMultiplier(%q, %q, %q) = %f, want %f",
					tt.theme, tt.industryA, tt.industryB, got, tt.want)
			}
		})
	}
}

func TestSeasonalMultiplier(t *testing.T) {
	sb := NewSeasonalBridge(nil)

	tests := []struct {
		name       string
		theme      string
		industryID string
		direction  float64
		want       float64
	}{
		{
			name:       "oil_price_shock energy +1.0",
			theme:      "oil_price_shock",
			industryID: "energy",
			direction:  1.0,
			want:       1.12,
		},
		{
			name:       "oil_price_shock energy -1.0",
			theme:      "oil_price_shock",
			industryID: "energy",
			direction:  -1.0,
			want:       0.88,
		},
		{
			name:       "oil_price_shock shipping +1.0",
			theme:      "oil_price_shock",
			industryID: "shipping",
			direction:  1.0,
			want:       0.92,
		},
		{
			name:       "oil_price_shock shipping -1.0",
			theme:      "oil_price_shock",
			industryID: "shipping",
			direction:  -1.0,
			want:       1.08,
		},
		{
			name:       "oil_price_shock semiconductor default",
			theme:      "oil_price_shock",
			industryID: "semiconductor",
			direction:  1.0,
			want:       1.0,
		},
		{
			name:       "AI_capex_surge semiconductor +1.0",
			theme:      "AI_capex_surge",
			industryID: "semiconductor",
			direction:  1.0,
			want:       1.15,
		},
		{
			name:       "AI_capex_surge financials default",
			theme:      "AI_capex_surge",
			industryID: "financials",
			direction:  1.0,
			want:       1.0,
		},
		{
			name:       "US_rates_up financials +1.0",
			theme:      "US_rates_up",
			industryID: "financials",
			direction:  1.0,
			want:       1.08,
		},
		{
			name:       "US_rates_up consumer +1.0",
			theme:      "US_rates_up",
			industryID: "consumer",
			direction:  1.0,
			want:       0.95,
		},
		{
			name:       "JPY_carry_unwind semiconductor +1.0",
			theme:      "JPY_carry_unwind",
			industryID: "semiconductor",
			direction:  1.0,
			want:       0.90,
		},
		{
			name:       "JPY_carry_unwind financials +1.0",
			theme:      "JPY_carry_unwind",
			industryID: "financials",
			direction:  1.0,
			want:       1.05,
		},
		{
			name:       "geopolitical_risk_spike consumer +1.0",
			theme:      "geopolitical_risk_spike",
			industryID: "consumer",
			direction:  1.0,
			want:       1.06,
		},
		{
			name:       "geopolitical_risk_spike semiconductor +1.0",
			theme:      "geopolitical_risk_spike",
			industryID: "semiconductor",
			direction:  1.0,
			want:       0.94,
		},
		{
			name:       "taiwan_political_risk semiconductor",
			theme:      "taiwan_political_risk",
			industryID: "semiconductor",
			direction:  0,
			want:       0.92,
		},
		{
			name:       "taiwan_political_risk financials",
			theme:      "taiwan_political_risk",
			industryID: "financials",
			direction:  0,
			want:       1.08,
		},
		{
			name:       "election_cycle semiconductor",
			theme:      "election_cycle",
			industryID: "semiconductor",
			direction:  0,
			want:       0.97,
		},
		{
			name:       "election_cycle consumer",
			theme:      "election_cycle",
			industryID: "consumer",
			direction:  0,
			want:       1.03,
		},
		{
			name:       "spring_festival_season consumer",
			theme:      "spring_festival_season",
			industryID: "consumer",
			direction:  0,
			want:       1.05,
		},
		{
			name:       "spring_festival_season semiconductor",
			theme:      "spring_festival_season",
			industryID: "semiconductor",
			direction:  0,
			want:       0.95,
		},
		{
			name:       "USD_TWD_volatility semiconductor",
			theme:      "USD_TWD_volatility",
			industryID: "semiconductor",
			direction:  0,
			want:       1.05,
		},
		{
			name:       "USD_TWD_volatility consumer",
			theme:      "USD_TWD_volatility",
			industryID: "consumer",
			direction:  0,
			want:       0.97,
		},
		{
			name:       "unknown_theme semiconductor default",
			theme:      "unknown_theme",
			industryID: "semiconductor",
			direction:  0,
			want:       1.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sb.SeasonalMultiplier(tt.theme, tt.industryID, tt.direction)
			if got != tt.want {
				t.Fatalf("SeasonalMultiplier(%q, %q, %f) = %f, want %f",
					tt.theme, tt.industryID, tt.direction, got, tt.want)
			}
		})
	}
}

func TestActiveThemes(t *testing.T) {
	t.Run("returns themes from SetActiveEvents", func(t *testing.T) {
		sb := NewSeasonalBridge(nil)
		sb.SetActiveEvents([]NarrativeEvent{
			{Theme: "US_rates_up"},
			{Theme: "AI_capex_surge"},
			{Theme: "oil_price_shock"},
		})

		got := sb.ActiveThemes()
		if len(got) != 3 {
			t.Fatalf("ActiveThemes length = %d, want 3", len(got))
		}
		exp := map[string]bool{
			"US_rates_up":     false,
			"AI_capex_surge":  false,
			"oil_price_shock": false,
		}
		for _, theme := range got {
			if _, ok := exp[theme]; ok {
				exp[theme] = true
			} else {
				t.Fatalf("unexpected theme %q in ActiveThemes", theme)
			}
		}
		for theme, found := range exp {
			if !found {
				t.Fatalf("expected theme %q not found in ActiveThemes", theme)
			}
		}
	})

	t.Run("returns empty list when no events set and engine is nil", func(t *testing.T) {
		sb := NewSeasonalBridge(nil)
		got := sb.ActiveThemes()
		if len(got) != 0 {
			t.Fatalf("ActiveThemes length = %d, want 0", len(got))
		}
	})

	t.Run("deduplicates themes across multiple events", func(t *testing.T) {
		sb := NewSeasonalBridge(nil)
		sb.SetActiveEvents([]NarrativeEvent{
			{Theme: "US_rates_up"},
			{Theme: "AI_capex_surge"},
			{Theme: "US_rates_up"},
		})

		got := sb.ActiveThemes()
		if len(got) != 2 {
			t.Fatalf("ActiveThemes length = %d, want 2 (deduplicated)", len(got))
		}
		exp := map[string]bool{"US_rates_up": false, "AI_capex_surge": false}
		for _, theme := range got {
			if _, ok := exp[theme]; ok {
				exp[theme] = true
			} else {
				t.Fatalf("unexpected theme %q in ActiveThemes", theme)
			}
		}
		for theme, found := range exp {
			if !found {
				t.Fatalf("expected theme %q not found in ActiveThemes", theme)
			}
		}
	})
}

func TestSetActiveEvents(t *testing.T) {
	sb := NewSeasonalBridge(nil)
	events := []NarrativeEvent{
		{Theme: "JPY_carry_unwind"},
		{Theme: "geopolitical_risk_spike"},
	}
	sb.SetActiveEvents(events)

	themes := sb.ActiveThemes()
	if len(themes) != 2 {
		t.Fatalf("ActiveThemes length = %d, want 2", len(themes))
	}
	exp := map[string]bool{
		"JPY_carry_unwind":        false,
		"geopolitical_risk_spike": false,
	}
	for _, theme := range themes {
		if _, ok := exp[theme]; ok {
			exp[theme] = true
		} else {
			t.Fatalf("unexpected theme %q in ActiveThemes", theme)
		}
	}
	for theme, found := range exp {
		if !found {
			t.Fatalf("expected theme %q not found in ActiveThemes", theme)
		}
	}
}

func TestCycleAmplifiedMultiplier_ExpansionWithPatterns(t *testing.T) {
	sb := NewSeasonalBridge(nil)
	card := &industry.CycleStatusCard{
		BusinessCycle:        "expansion",
		CompositeCoefficient: 1.10,
		ActivePatterns:       []industry.SeasonalPatternSnapshot{{ID: "q1_rally", Name: "Q1 Rally"}},
	}
	sb.SetCycleCard(card)
	result := sb.CycleAmplifiedMultiplier(1.0, "AI_capex_surge", "semiconductor", 1.0)
	expected := 1.15 * (1.0 + (1.10-1.0)*0.5)
	if result != expected {
		t.Fatalf("expected amplified multiplier %.4f, got %.4f", expected, result)
	}
}

func TestCycleAmplifiedMultiplier_NilCard(t *testing.T) {
	sb := NewSeasonalBridge(nil)
	sb.SetCycleCard(nil)
	result := sb.CycleAmplifiedMultiplier(1.0, "AI_capex_surge", "semiconductor", 1.0)
	if result != 1.15 {
		t.Fatalf("expected base multiplier 1.15 when no card, got %.4f", result)
	}
}

func TestCycleAmplifiedMultiplier_NotExpansion(t *testing.T) {
	sb := NewSeasonalBridge(nil)
	card := &industry.CycleStatusCard{
		BusinessCycle:        "recession",
		CompositeCoefficient: 0.90,
		ActivePatterns:       []industry.SeasonalPatternSnapshot{{ID: "x"}},
	}
	sb.SetCycleCard(card)
	result := sb.CycleAmplifiedMultiplier(1.0, "AI_capex_surge", "semiconductor", 1.0)
	if result != 1.15 {
		t.Fatalf("expected base multiplier 1.15 when not expansion, got %.4f", result)
	}
}

func TestCycleAmplifiedMultiplier_NoActivePatterns(t *testing.T) {
	sb := NewSeasonalBridge(nil)
	card := &industry.CycleStatusCard{
		BusinessCycle:        "expansion",
		CompositeCoefficient: 1.10,
		ActivePatterns:       []industry.SeasonalPatternSnapshot{},
	}
	sb.SetCycleCard(card)
	result := sb.CycleAmplifiedMultiplier(1.0, "AI_capex_surge", "semiconductor", 1.0)
	if result != 1.15 {
		t.Fatalf("expected base multiplier when no active patterns, got %.4f", result)
	}
}
