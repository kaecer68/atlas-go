package risk

import (
	"testing"

	"github.com/kaecer68/atlas-go/internal/narrative"
)

func TestMacroAwareDrawdownEngine_Evaluate(t *testing.T) {
	engine := NewMacroAwareDrawdownEngine()

	tests := []struct {
		name            string
		macroLevel      narrative.MacroRiskLevel
		outflowProb     float64
		structuralScore float64
		shouldOverride  bool
		wantAction      DrawdownAction
		wantPercentage  float64
		wantMaxExposure float64
	}{
		{
			name:            "Green Risk - No Drawdown",
			macroLevel:      narrative.MacroRiskGreen,
			outflowProb:     10.0,
			structuralScore: 0.0,
			shouldOverride:  false,
			wantAction:      DrawdownNone,
			wantPercentage:  0.0,
			wantMaxExposure: 1.0,
		},
		{
			name:            "Yellow Risk - Light Drawdown",
			macroLevel:      narrative.MacroRiskYellow,
			outflowProb:     35.0,
			structuralScore: 0.0,
			shouldOverride:  false,
			wantAction:      DrawdownLight,
			wantPercentage:  0.15,
			wantMaxExposure: 0.85,
		},
		{
			name:            "Orange Risk without Trend - Moderate Drawdown",
			macroLevel:      narrative.MacroRiskOrange,
			outflowProb:     55.0,
			structuralScore: 0.0,
			shouldOverride:  false,
			wantAction:      DrawdownModerate,
			wantPercentage:  0.35,
			wantMaxExposure: 0.65,
		},
		{
			name:            "Orange Risk with Strong Trend - Light Drawdown",
			macroLevel:      narrative.MacroRiskOrange,
			outflowProb:     55.0,
			structuralScore: 0.70,
			shouldOverride:  true,
			wantAction:      DrawdownLight,
			wantPercentage:  0.15,
			wantMaxExposure: 0.85,
		},
		{
			name:            "Red Risk without Trend - Severe Drawdown",
			macroLevel:      narrative.MacroRiskRed,
			outflowProb:     80.0,
			structuralScore: 0.0,
			shouldOverride:  false,
			wantAction:      DrawdownSevere,
			wantPercentage:  0.60,
			wantMaxExposure: 0.40,
		},
		{
			name:            "Red Risk with Exceptional Trend - Moderate Drawdown",
			macroLevel:      narrative.MacroRiskRed,
			outflowProb:     80.0,
			structuralScore: 0.80,
			shouldOverride:  true,
			wantAction:      DrawdownModerate,
			wantPercentage:  0.35,
			wantMaxExposure: 0.65,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			macroAssessment := &narrative.MacroRiskAssessment{
				Level:              tt.macroLevel,
				ForeignOutflowProb: tt.outflowProb,
			}

			structuralAssessment := &narrative.StructuralTrendAssessment{
				OverrideScore:      tt.structuralScore,
				ShouldOverrideRisk: tt.shouldOverride,
			}

			if tt.structuralScore > 0 {
				structuralAssessment.DominantTrend = &narrative.StructuralTrend{
					Name:       "AI Capex Surge",
					Strength:   0.8,
					Confidence: 0.85,
					HitRate:    0.81,
				}
			}

			decision := engine.Evaluate(macroAssessment, structuralAssessment)

			if decision.Action != tt.wantAction {
				t.Errorf("Action = %v, want %v", decision.Action, tt.wantAction)
			}

			if decision.Percentage != tt.wantPercentage {
				t.Errorf("Percentage = %.2f, want %.2f", decision.Percentage, tt.wantPercentage)
			}

			if decision.MaxExposure != tt.wantMaxExposure {
				t.Errorf("MaxExposure = %.2f, want %.2f", decision.MaxExposure, tt.wantMaxExposure)
			}

			if decision.StructuralOverride != tt.shouldOverride {
				t.Errorf("StructuralOverride = %v, want %v", decision.StructuralOverride, tt.shouldOverride)
			}

			t.Logf("Decision: %s (%.0f%% reduction)", decision.Action.String(), decision.Percentage*100)
			t.Logf("Rationale: %s", decision.Rationale)
		})
	}
}

func TestMacroAwareDrawdownEngine_GetSectorConstraints(t *testing.T) {
	engine := NewMacroAwareDrawdownEngine()

	tests := []struct {
		name            string
		primaryFlow     string
		wantConstraints map[string]float64
	}{
		{
			name:        "Risk Off Flow",
			primaryFlow: "risk_off",
			wantConstraints: map[string]float64{
				"ai_supply_chain": 0.3,
				"gold":            1.5,
			},
		},
		{
			name:        "Carry Trade Unwind Flow",
			primaryFlow: "carry_trade_unwind",
			wantConstraints: map[string]float64{
				"all_equities": 0.1,
				"cash":         2.0,
			},
		},
		{
			name:        "Sector Rotation Flow",
			primaryFlow: "sector_rotation",
			wantConstraints: map[string]float64{
				"energy":              1.8,
				"high_valuation_tech": 0.3,
			},
		},
		{
			name:            "Mixed Flow - No Constraints",
			primaryFlow:     "mixed",
			wantConstraints: map[string]float64{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			macroAssessment := &narrative.MacroRiskAssessment{
				PrimaryFlow: tt.primaryFlow,
			}

			constraints := engine.GetSectorConstraints(macroAssessment)

			for sector, wantMultiplier := range tt.wantConstraints {
				got, exists := constraints[sector]
				if !exists {
					t.Errorf("Expected constraint for %s not found", sector)
					continue
				}
				if got != wantMultiplier {
					t.Errorf("Constraint for %s = %.2f, want %.2f", sector, got, wantMultiplier)
				}
			}
		})
	}
}

func TestMacroAwareDrawdownEngine_ShouldHaltTrading(t *testing.T) {
	engine := NewMacroAwareDrawdownEngine()

	tests := []struct {
		name     string
		action   DrawdownAction
		wantHalt bool
	}{
		{"None - Continue", DrawdownNone, false},
		{"Light - Continue", DrawdownLight, false},
		{"Moderate - Continue", DrawdownModerate, false},
		{"Severe - Halt", DrawdownSevere, true},
		{"Emergency - Halt", DrawdownEmergency, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decision := &MacroAwareDrawdownDecision{Action: tt.action}
			got := engine.ShouldHaltTrading(decision)
			if got != tt.wantHalt {
				t.Errorf("ShouldHaltTrading() = %v, want %v", got, tt.wantHalt)
			}
		})
	}
}

func TestMacroAwareDrawdownEngine_CalculatePortfolioAdjustment(t *testing.T) {
	engine := NewMacroAwareDrawdownEngine()

	tests := []struct {
		name            string
		currentExposure float64
		decision        *MacroAwareDrawdownDecision
		wantTarget      float64
		wantAdjustment  float64
	}{
		{
			name:            "Reduce from 100% to 65%",
			currentExposure: 1.0,
			decision:        &MacroAwareDrawdownDecision{MaxExposure: 0.65},
			wantTarget:      0.65,
			wantAdjustment:  -0.35,
		},
		{
			name:            "Increase from 40% to 85%",
			currentExposure: 0.40,
			decision:        &MacroAwareDrawdownDecision{MaxExposure: 0.85},
			wantTarget:      0.85,
			wantAdjustment:  0.45,
		},
		{
			name:            "No change at target",
			currentExposure: 0.85,
			decision:        &MacroAwareDrawdownDecision{MaxExposure: 0.85},
			wantTarget:      0.85,
			wantAdjustment:  0.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target, adjustment := engine.CalculatePortfolioAdjustment(tt.currentExposure, tt.decision)

			if target != tt.wantTarget {
				t.Errorf("Target = %.2f, want %.2f", target, tt.wantTarget)
			}

			tolerance := 0.0001
			if adjustment < tt.wantAdjustment-tolerance || adjustment > tt.wantAdjustment+tolerance {
				t.Errorf("Adjustment = %.2f, want %.2f", adjustment, tt.wantAdjustment)
			}
		})
	}
}

func TestNewMacroAwareDrawdownEngineFromParameters(t *testing.T) {
	engine := NewMacroAwareDrawdownEngineFromParameters()
	if engine == nil {
		t.Fatal("NewMacroAwareDrawdownEngineFromParameters returned nil")
	}
	if len(engine.levels) == 0 {
		t.Error("expected levels to be populated from parameters")
	}

	if engine.cfg.OrangeOverrideMinScore != 0.55 {
		t.Errorf("OrangeOverrideMinScore = %.2f, want 0.55", engine.cfg.OrangeOverrideMinScore)
	}
	if engine.cfg.RedOverrideMinScore != 0.75 {
		t.Errorf("RedOverrideMinScore = %.2f, want 0.75", engine.cfg.RedOverrideMinScore)
	}

	light, ok := engine.levels[DrawdownLight]
	if !ok {
		t.Fatal("expected light drawdown level")
	}
	if light.Percentage != 0.15 || light.MaxExposure != 0.85 {
		t.Errorf("light level = (%v, %v), want (0.15, 0.85)", light.Percentage, light.MaxExposure)
	}
}

func TestMacroAwareDrawdownEngine_GetPositionSizeAdjustment(t *testing.T) {
	engine := NewMacroAwareDrawdownEngine()
	decision := &MacroAwareDrawdownDecision{MaxExposure: 0.65}
	if got := engine.GetPositionSizeAdjustment(decision); got != 0.65 {
		t.Errorf("GetPositionSizeAdjustment = %.2f, want 0.65", got)
	}
}

func TestMacroAwareDrawdownEngine_EvaluateWithIndustry_Escalation(t *testing.T) {
	engine := NewMacroAwareDrawdownEngine()
	macro := &narrative.MacroRiskAssessment{Level: narrative.MacroRiskOrange, ForeignOutflowProb: 55.0}
	structural := &narrative.StructuralTrendAssessment{OverrideScore: 0.0, ShouldOverrideRisk: false}
	industry := &IndustryRiskAssessment{
		TotalIndustryCount:     10,
		RecessionIndustryCount: 3,
		ExpansionIndustryCount: 2,
	}

	dec, breakdown := engine.EvaluateWithIndustry(macro, structural, industry)
	if dec.Action != DrawdownSevere {
		t.Errorf("expected escalation to SEVERE, got %v", dec.Action)
	}
	if breakdown == nil {
		t.Fatal("expected breakdown")
	}
	if len(breakdown.Steps) < 3 {
		t.Errorf("expected at least 3 breakdown steps, got %d", len(breakdown.Steps))
	}
	if breakdown.FinalAction != DrawdownSevere {
		t.Errorf("expected FinalAction SEVERE, got %v", breakdown.FinalAction)
	}
}

