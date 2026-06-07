package narrative

import (
	"slices"
	"testing"
)

// TestMacroRiskAssessmentEngine_HistoricalScenarios validates the engine against
// three key historical events that shaped the macro-aware drawdown strategy:
// 1. 2022 Russia-Ukraine war (Feb 2022) - Should trigger RED risk level
// 2. 2024 Aug 5 Carry Trade unwind - Should trigger RED risk level
// 3. 2026 Iran-Israel war (simulated) - Should trigger ORANGE/RED with sector rotation
func TestMacroRiskAssessmentEngine_HistoricalScenarios(t *testing.T) {
	engine := NewMacroRiskAssessmentEngine()

	tests := []struct {
		name        string
		data        MacroDataSnapshot
		wantLevel   MacroRiskLevel
		wantMinProb float64
		wantFlow    string
		wantFavored []string
		wantAvoided []string
	}{
		{
			name: "2022 Russia-Ukraine War - Classic Geopolitical Risk",
			data: MacroDataSnapshot{
				US10Y:   MacroDataPoint{Value: 2.0, ChangePct: 0.1},
				DXY:     MacroDataPoint{Value: 96.5, ChangePct: 0.5},
				VIX:     MacroDataPoint{Value: 35.0, ChangePct: 45.0},
				USD_TWD: MacroDataPoint{Value: 27.8, ChangePct: 0.3},
				Oil:     MacroDataPoint{Value: 95.0, ChangePct: 8.5},
				Gold:    MacroDataPoint{Value: 1950, ChangePct: 3.5},
				JPY:     MacroDataPoint{Value: 115.0, ChangePct: -0.2},
			},
			wantLevel:   MacroRiskRed, // VIX≥35 triggers crisis detection → force RED
			wantMinProb: 25.0,
			wantFlow:    "carry_trade_unwind",
			wantFavored: []string{"cash", "short_term_bonds", "jpy"},
			wantAvoided: []string{"all_equities", "tech", "financials"},
		},
		{
			name: "2024 Aug 5 Carry Trade Unwind - JPY Spike Crisis",
			data: MacroDataSnapshot{
				US10Y:   MacroDataPoint{Value: 3.8, ChangePct: -0.2},
				DXY:     MacroDataPoint{Value: 102.0, ChangePct: -0.5},
				VIX:     MacroDataPoint{Value: 65.0, ChangePct: 150.0},
				USD_TWD: MacroDataPoint{Value: 32.5, ChangePct: 1.8},
				Oil:     MacroDataPoint{Value: 72.0, ChangePct: -2.0},
				Gold:    MacroDataPoint{Value: 2400, ChangePct: 1.5},
				JPY:     MacroDataPoint{Value: 138.0, ChangePct: 5.0},
			},
			wantLevel:   MacroRiskRed,
			wantMinProb: 53.0, // Config-based threshold yields ~53.62%
			wantFlow:    "carry_trade_unwind",
			wantFavored: []string{"cash", "short_term_bonds", "jpy"},
			wantAvoided: []string{"all_equities", "tech", "financials"},
		},
		{
			name: "2026 Iran-Israel War - Energy Crisis with Sector Rotation",
			data: MacroDataSnapshot{
				US10Y:   MacroDataPoint{Value: 4.2, ChangePct: 0.1},
				DXY:     MacroDataPoint{Value: 104.0, ChangePct: 0.3},
				VIX:     MacroDataPoint{Value: 32.0, ChangePct: 25.0},
				USD_TWD: MacroDataPoint{Value: 33.0, ChangePct: 0.8},
				Oil:     MacroDataPoint{Value: 95.0, ChangePct: 15.0},
				Gold:    MacroDataPoint{Value: 2800, ChangePct: 1.0},
				JPY:     MacroDataPoint{Value: 148.0, ChangePct: 0.5},
			},
			wantLevel:   MacroRiskOrange,
			wantMinProb: 35.0,
			wantFlow:    "sector_rotation",
			wantFavored: []string{"energy", "oil_services", "alternative_energy", "shipping"},
			wantAvoided: []string{"high_valuation_tech", "rate_sensitive"},
		},
		{
			name: "Normal Market - Green Level",
			data: MacroDataSnapshot{
				US10Y:   MacroDataPoint{Value: 4.0, ChangePct: 0.0},
				DXY:     MacroDataPoint{Value: 103.0, ChangePct: 0.1},
				VIX:     MacroDataPoint{Value: 18.0, ChangePct: 5.0},
				USD_TWD: MacroDataPoint{Value: 32.0, ChangePct: 0.2},
				Oil:     MacroDataPoint{Value: 75.0, ChangePct: 1.0},
				Gold:    MacroDataPoint{Value: 2350, ChangePct: 0.5},
				JPY:     MacroDataPoint{Value: 150.0, ChangePct: -0.1}, // USD/JPY above threshold, stable
			},
			wantLevel:   MacroRiskGreen,
			wantMinProb: 10.0,
			wantFlow:    "mixed",
			wantFavored: []string{"diversified"},
			wantAvoided: []string{"concentrated"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := engine.Assess(tt.data)

			if result.Level != tt.wantLevel {
				t.Errorf("Risk level = %v, want %v", result.Level, tt.wantLevel)
			}

			if result.ForeignOutflowProb < tt.wantMinProb {
				t.Errorf("Foreign outflow probability = %.2f, want at least %.2f",
					result.ForeignOutflowProb, tt.wantMinProb)
			}

			if result.PrimaryFlow != tt.wantFlow {
				t.Errorf("Primary flow = %s, want %s", result.PrimaryFlow, tt.wantFlow)
			}

			// Check favored sectors
			if len(tt.wantFavored) > 0 {
				for _, want := range tt.wantFavored {
					found := slices.Contains(result.FavoredSectors, want)
					if !found {
						t.Errorf("Expected favored sector %s not found in %v", want, result.FavoredSectors)
					}
				}
			}

			// Check avoided sectors
			if len(tt.wantAvoided) > 0 {
				for _, want := range tt.wantAvoided {
					found := slices.Contains(result.AvoidedSectors, want)
					if !found {
						t.Errorf("Expected avoided sector %s not found in %v", want, result.AvoidedSectors)
					}
				}
			}

			t.Logf("Test: %s", tt.name)
			t.Logf("  Risk Level: %s", result.Level.String())
			t.Logf("  Outflow Prob: %.2f%%", result.ForeignOutflowProb)
			t.Logf("  Primary Flow: %s", result.PrimaryFlow)
			t.Logf("  Favored: %v", result.FavoredSectors)
			t.Logf("  Avoided: %v", result.AvoidedSectors)
			t.Logf("  Rationale: %s", result.Rationale)
		})
	}
}

// TestMacroRiskAssessmentEngine_RiskLevelTransitions tests the risk level
// determination logic with various factor combinations
func TestMacroRiskAssessmentEngine_RiskLevelTransitions(t *testing.T) {
	engine := NewMacroRiskAssessmentEngine()

	tests := []struct {
		name      string
		factors   []RiskFactor
		wantLevel MacroRiskLevel
	}{
		{
			name:      "No factors = Green",
			factors:   []RiskFactor{},
			wantLevel: MacroRiskGreen,
		},
		{
			name: "Single low severity = Yellow",
			factors: []RiskFactor{
				{Type: "market_stress", Severity: 0.5},
			},
			wantLevel: MacroRiskYellow,
		},
		{
			name: "Two moderate factors = Orange",
			factors: []RiskFactor{
				{Type: "market_stress", Severity: 0.6},
				{Type: "rates", Severity: 0.5},
			},
			wantLevel: MacroRiskOrange,
		},
		{
			name: "One high severity = Orange",
			factors: []RiskFactor{
				{Type: "carry_trade", Severity: 0.8},
			},
			wantLevel: MacroRiskOrange,
		},
		{
			name: "Two high severity = Red",
			factors: []RiskFactor{
				{Type: "carry_trade", Severity: 0.85},
				{Type: "geopolitical", Severity: 0.9},
			},
			wantLevel: MacroRiskRed,
		},
		{
			name: "Extreme severity = Red",
			factors: []RiskFactor{
				{Type: "taiwan_stress", Severity: 0.95},
			},
			wantLevel: MacroRiskRed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := engine.determineRiskLevel(tt.factors)
			if got != tt.wantLevel {
				t.Errorf("determineRiskLevel() = %v, want %v", got, tt.wantLevel)
			}
		})
	}
}
