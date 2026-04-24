package orchestrator

import (
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/narrative"
	"github.com/kaecer68/atlas-go/internal/risk"
)

func TestStrategyEvolver_Evaluate(t *testing.T) {
	evolver := NewStrategyEvolver()

	tests := []struct {
		name            string
		macroLevel      narrative.MacroRiskLevel
		structuralScore float64
		shouldOverride  bool
		drawdownAction  risk.DrawdownAction
		wantState       StrategyState
		wantEvolution   bool
	}{
		{
			name:            "Green Risk - Normal",
			macroLevel:      narrative.MacroRiskGreen,
			structuralScore: 0.0,
			shouldOverride:  false,
			drawdownAction:  risk.DrawdownNone,
			wantState:       StrategyNormal,
			wantEvolution:   false,
		},
		{
			name:            "Yellow Risk - Cautious",
			macroLevel:      narrative.MacroRiskYellow,
			structuralScore: 0.0,
			shouldOverride:  false,
			drawdownAction:  risk.DrawdownLight,
			wantState:       StrategyCautious,
			wantEvolution:   true,
		},
		{
			name:            "Orange Risk - Defensive",
			macroLevel:      narrative.MacroRiskOrange,
			structuralScore: 0.0,
			shouldOverride:  false,
			drawdownAction:  risk.DrawdownModerate,
			wantState:       StrategyDefensive,
			wantEvolution:   true,
		},
		{
			name:            "Orange with Strong Trend - Hedged",
			macroLevel:      narrative.MacroRiskOrange,
			structuralScore: 0.70,
			shouldOverride:  true,
			drawdownAction:  risk.DrawdownLight,
			wantState:       StrategyHedged,
			wantEvolution:   true,
		},
		{
			name:            "Red Risk - Defensive",
			macroLevel:      narrative.MacroRiskRed,
			structuralScore: 0.0,
			shouldOverride:  false,
			drawdownAction:  risk.DrawdownSevere,
			wantState:       StrategySuspended,
			wantEvolution:   true,
		},
		{
			name:            "Red with Exceptional Trend - Hedged",
			macroLevel:      narrative.MacroRiskRed,
			structuralScore: 0.80,
			shouldOverride:  true,
			drawdownAction:  risk.DrawdownModerate,
			wantState:       StrategyHedged,
			wantEvolution:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			evolver.Reset()

			macroAssessment := &narrative.MacroRiskAssessment{
				Level: tt.macroLevel,
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

			drawdownDecision := &risk.MacroAwareDrawdownDecision{
				Action: tt.drawdownAction,
			}

			evolution := evolver.Evaluate(macroAssessment, structuralAssessment, drawdownDecision)

			if tt.wantEvolution {
				if evolution == nil {
					t.Fatal("Expected evolution, got nil")
				}
				if evolution.ToState != tt.wantState {
					t.Errorf("ToState = %v, want %v", evolution.ToState, tt.wantState)
				}
				t.Logf("Evolution: %s -> %s", evolution.FromState.String(), evolution.ToState.String())
				t.Logf("Reason: %s", evolution.Reason)
			} else {
				if evolution != nil {
					t.Errorf("Expected no evolution, got %v", evolution)
				}
			}

			if evolver.GetCurrentState() != tt.wantState {
				t.Errorf("CurrentState = %v, want %v", evolver.GetCurrentState(), tt.wantState)
			}
		})
	}
}

func TestStrategyEvolver_GetStrategyConfig(t *testing.T) {
	evolver := NewStrategyEvolver()

	tests := []struct {
		name            string
		state           StrategyState
		wantMaxPosSize  float64
		wantHedgeRatio  float64
		wantAllowNewPos bool
	}{
		{"Normal", StrategyNormal, 0.15, 0.0, true},
		{"Cautious", StrategyCautious, 0.12, 0.10, true},
		{"Defensive", StrategyDefensive, 0.08, 0.20, false},
		{"Hedged", StrategyHedged, 0.10, 0.30, true},
		{"Suspended", StrategySuspended, 0.0, 0.0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			evolver.currentState = tt.state
			config := evolver.GetStrategyConfig()

			if config.MaxPositionSize != tt.wantMaxPosSize {
				t.Errorf("MaxPositionSize = %.2f, want %.2f", config.MaxPositionSize, tt.wantMaxPosSize)
			}

			if config.HedgeRatio != tt.wantHedgeRatio {
				t.Errorf("HedgeRatio = %.2f, want %.2f", config.HedgeRatio, tt.wantHedgeRatio)
			}

			if config.AllowNewPositions != tt.wantAllowNewPos {
				t.Errorf("AllowNewPositions = %v, want %v", config.AllowNewPositions, tt.wantAllowNewPos)
			}
		})
	}
}

func TestStrategyEvolver_ShouldEnterPosition(t *testing.T) {
	evolver := NewStrategyEvolver()

	tests := []struct {
		name    string
		state   StrategyState
		wantCan bool
	}{
		{"Normal allows", StrategyNormal, true},
		{"Cautious allows", StrategyCautious, true},
		{"Defensive blocks", StrategyDefensive, false},
		{"Hedged allows", StrategyHedged, true},
		{"Suspended blocks", StrategySuspended, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			evolver.currentState = tt.state
			can, reason := evolver.ShouldEnterPosition("AAPL", "tech")

			if can != tt.wantCan {
				t.Errorf("ShouldEnterPosition() = %v, want %v", can, tt.wantCan)
			}

			if len(reason) == 0 {
				t.Error("Expected non-empty reason")
			}
		})
	}
}

func TestStrategyEvolver_Cooldown(t *testing.T) {
	evolver := NewStrategyEvolver()
	evolver.cooldownPeriod = 1 * time.Hour

	macroAssessment := &narrative.MacroRiskAssessment{
		Level: narrative.MacroRiskYellow,
	}
	structuralAssessment := &narrative.StructuralTrendAssessment{}
	drawdownDecision := &risk.MacroAwareDrawdownDecision{
		Action: risk.DrawdownLight,
	}

	// First evolution should work
	evolution1 := evolver.Evaluate(macroAssessment, structuralAssessment, drawdownDecision)
	if evolution1 == nil {
		t.Fatal("Expected first evolution")
	}

	// Second evolution immediately should be blocked by cooldown
	macroAssessment.Level = narrative.MacroRiskOrange
	evolution2 := evolver.Evaluate(macroAssessment, structuralAssessment, drawdownDecision)
	if evolution2 != nil {
		t.Error("Expected cooldown to block second evolution")
	}
}
