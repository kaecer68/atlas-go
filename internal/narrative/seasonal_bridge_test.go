package narrative

import "testing"

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
				t.Errorf("CorrelationMultiplier(%q, %q, %q) = %f, want %f",
					tt.theme, tt.industryA, tt.industryB, got, tt.want)
			}
		})
	}
}
