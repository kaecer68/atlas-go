package narrative

import (
	"testing"
)

func TestStructuralTrendEngine_Assess(t *testing.T) {
	engine := NewStructuralTrendEngine()

	tests := []struct {
		name             string
		macroData        MacroDataSnapshot
		sectorData       SectorDataSnapshot
		wantTrendCount   int
		wantOverride     bool
		wantCanWithstand bool
	}{
		{
			name: "Strong AI Trend Overrides Macro Risk",
			macroData: MacroDataSnapshot{
				VIX:     MacroDataPoint{Value: 32.0, ChangePct: 10.0},
				USD_TWD: MacroDataPoint{Value: 32.5, ChangePct: 1.5},
				JPY:     MacroDataPoint{Value: 148.0, ChangePct: 0.5},
			},
			sectorData: SectorDataSnapshot{
				AIRevenueGrowth:    75.0,
				CoWoSUtilization:   95.0,
				CapexGrowth:        60.0,
				SemiconductorIndex: 4500,
			},
			wantTrendCount:   3,
			wantOverride:     true,
			wantCanWithstand: true,
		},
		{
			name: "Weak Trend - No Override",
			macroData: MacroDataSnapshot{
				VIX:     MacroDataPoint{Value: 35.0, ChangePct: 20.0},
				USD_TWD: MacroDataPoint{Value: 33.0, ChangePct: 2.0},
				JPY:     MacroDataPoint{Value: 140.0, ChangePct: 3.0},
			},
			sectorData: SectorDataSnapshot{
				AIRevenueGrowth:  20.0,
				CoWoSUtilization: 60.0,
				CapexGrowth:      15.0,
			},
			wantTrendCount:   0,
			wantOverride:     false,
			wantCanWithstand: false,
		},
		{
			name: "Moderate Trend with Orange Risk",
			macroData: MacroDataSnapshot{
				VIX:     MacroDataPoint{Value: 32.0, ChangePct: 8.0},
				USD_TWD: MacroDataPoint{Value: 32.2, ChangePct: 1.2},
				JPY:     MacroDataPoint{Value: 149.0, ChangePct: 0.3},
			},
			sectorData: SectorDataSnapshot{
				AIRevenueGrowth:    60.0,
				CoWoSUtilization:   90.0,
				CapexGrowth:        45.0,
				SemiconductorIndex: 4200,
			},
			wantTrendCount:   3,
			wantOverride:     false, // Override score 0.52 < 0.65 threshold
			wantCanWithstand: false, // Orange risk requires score >= 0.55
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assessment := engine.Assess(tt.macroData, tt.sectorData)

			if len(assessment.Trends) != tt.wantTrendCount {
				t.Errorf("Trend count = %d, want %d", len(assessment.Trends), tt.wantTrendCount)
			}

			if assessment.ShouldOverrideRisk != tt.wantOverride {
				t.Errorf("ShouldOverrideRisk = %v, want %v", assessment.ShouldOverrideRisk, tt.wantOverride)
			}

			// Test with orange risk level
			macroAssessment := &MacroRiskAssessment{Level: MacroRiskOrange}
			canWithstand := engine.CanWithstandMacroRisk(MacroRiskOrange, assessment)
			if canWithstand != tt.wantCanWithstand {
				t.Errorf("CanWithstandMacroRisk = %v, want %v", canWithstand, tt.wantCanWithstand)
			}

			action, rationale := engine.GetRecommendedAction(macroAssessment, assessment)
			t.Logf("Action: %s", action)
			t.Logf("Rationale: %s", rationale)
			t.Logf("Override Score: %.2f", assessment.OverrideScore)
			if assessment.DominantTrend != nil {
				t.Logf("Dominant Trend: %s (strength: %.2f)", assessment.DominantTrend.Name, assessment.DominantTrend.Strength)
			}
		})
	}
}

func TestStructuralTrendEngine_GetRecommendedAction(t *testing.T) {
	engine := NewStructuralTrendEngine()

	tests := []struct {
		name          string
		macroLevel    MacroRiskLevel
		overrideScore float64
		wantAction    string
	}{
		{
			name:       "Green Risk - Normal",
			macroLevel: MacroRiskGreen,
			wantAction: "normal",
		},
		{
			name:       "Yellow Risk - Cautious",
			macroLevel: MacroRiskYellow,
			wantAction: "cautious",
		},
		{
			name:          "Orange Risk with Strong Trend - Hold",
			macroLevel:    MacroRiskOrange,
			overrideScore: 0.70,
			wantAction:    "hold_with_hedge",
		},
		{
			name:          "Orange Risk without Trend - Reduce",
			macroLevel:    MacroRiskOrange,
			overrideScore: 0.0,
			wantAction:    "reduce",
		},
		{
			name:          "Red Risk with Exceptional Trend - Selective Hold",
			macroLevel:    MacroRiskRed,
			overrideScore: 0.80,
			wantAction:    "selective_hold",
		},
		{
			name:          "Red Risk without Trend - Exit",
			macroLevel:    MacroRiskRed,
			overrideScore: 0.0,
			wantAction:    "exit",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			macroAssessment := &MacroRiskAssessment{Level: tt.macroLevel}
			structuralAssessment := &StructuralTrendAssessment{
				OverrideScore: tt.overrideScore,
			}

			if tt.overrideScore > 0 {
				structuralAssessment.DominantTrend = &StructuralTrend{
					Name:       "AI Capex Surge",
					Strength:   0.8,
					Confidence: 0.85,
					HitRate:    0.81,
				}
			}

			action, _ := engine.GetRecommendedAction(macroAssessment, structuralAssessment)
			if action != tt.wantAction {
				t.Errorf("GetRecommendedAction() = %s, want %s", action, tt.wantAction)
			}
		})
	}
}

func TestStructuralTrendEngine_CanWithstandMacroRisk(t *testing.T) {
	engine := NewStructuralTrendEngine()

	tests := []struct {
		name          string
		macroLevel    MacroRiskLevel
		overrideScore float64
		want          bool
	}{
		{"Green always withstands", MacroRiskGreen, 0.0, true},
		{"Yellow always withstands", MacroRiskYellow, 0.0, true},
		{"Orange with strong trend", MacroRiskOrange, 0.60, true},
		{"Orange with weak trend", MacroRiskOrange, 0.40, false},
		{"Orange no trend", MacroRiskOrange, 0.0, false},
		{"Red with exceptional trend", MacroRiskRed, 0.80, true},
		{"Red with strong trend", MacroRiskRed, 0.70, false},
		{"Red no trend", MacroRiskRed, 0.0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assessment := &StructuralTrendAssessment{OverrideScore: tt.overrideScore}
			if tt.overrideScore > 0 {
				assessment.DominantTrend = &StructuralTrend{}
			}

			got := engine.CanWithstandMacroRisk(tt.macroLevel, assessment)
			if got != tt.want {
				t.Errorf("CanWithstandMacroRisk() = %v, want %v", got, tt.want)
			}
		})
	}
}
