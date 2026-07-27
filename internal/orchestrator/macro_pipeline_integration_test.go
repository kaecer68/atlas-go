package orchestrator

import (
	"testing"

	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/narrative"
	"github.com/kaecer68/atlas-go/internal/portfolio"
	"github.com/kaecer68/atlas-go/internal/risk"
)

// TestMacroAwareDrawdownPipeline_Integration tests the complete 4-phase pipeline
// demonstrating how macro risk assessment, structural trend analysis,
// drawdown decisions, and sector rotation work together.
func TestMacroAwareDrawdownPipeline_Integration(t *testing.T) {
	tests := []struct {
		name             string
		macroData        marketdata.MacroDataSnapshot
		sectorData       narrative.SectorDataSnapshot
		portfolioValue   float64
		currentAlloc     map[string]float64
		wantRiskLevel    narrative.MacroRiskLevel
		wantCanWithstand bool
		wantDrawdown     risk.DrawdownAction
		wantStrategy     StrategyState
	}{
		{
			name: "2024 Aug 5 Carry Trade Unwind - Full Pipeline",
			macroData: marketdata.MacroDataSnapshot{
				VIX:     marketdata.MacroDataPoint{Value: 65.0, ChangePct: 150.0},
				USD_TWD: marketdata.MacroDataPoint{Value: 32.5, ChangePct: 1.8},
				JPY:     marketdata.MacroDataPoint{Value: 138.0, ChangePct: 5.0},
				US10Y:   marketdata.MacroDataPoint{Value: 3.8, ChangePct: -0.2},
				Gold:    marketdata.MacroDataPoint{Value: 2400, ChangePct: 1.5},
				Oil:     marketdata.MacroDataPoint{Value: 72.0, ChangePct: -2.0},
			},
			sectorData: narrative.SectorDataSnapshot{
				AIRevenueGrowth:    75.0,
				CoWoSUtilization:   95.0,
				CapexGrowth:        60.0,
				SemiconductorIndex: 4500,
			},
			portfolioValue: 1000000,
			currentAlloc: map[string]float64{
				"semiconductor":   0.25,
				"ai_supply_chain": 0.20,
				"financials":      0.15,
				"shipping":        0.10,
				"energy":          0.10,
				"defensive":       0.10,
				"cash":            0.10,
			},
			wantRiskLevel:    narrative.MacroRiskRed,
			wantCanWithstand: false, // VIX 65 is extreme - structural trend not strong enough
			wantDrawdown:     risk.DrawdownSevere,
			wantStrategy:     StrategySuspended,
		},
		{
			name: "2026 Iran War - Energy Crisis Pipeline",
			macroData: marketdata.MacroDataSnapshot{
				VIX:     marketdata.MacroDataPoint{Value: 32.0, ChangePct: 25.0},
				USD_TWD: marketdata.MacroDataPoint{Value: 33.0, ChangePct: 0.8},
				JPY:     marketdata.MacroDataPoint{Value: 148.0, ChangePct: 0.5},
				US10Y:   marketdata.MacroDataPoint{Value: 4.2, ChangePct: 0.1},
				Gold:    marketdata.MacroDataPoint{Value: 2800, ChangePct: 1.0},
				Oil:     marketdata.MacroDataPoint{Value: 95.0, ChangePct: 15.0},
			},
			sectorData: narrative.SectorDataSnapshot{
				AIRevenueGrowth:    60.0,
				CoWoSUtilization:   90.0,
				CapexGrowth:        45.0,
				SemiconductorIndex: 4200,
			},
			portfolioValue: 1000000,
			currentAlloc: map[string]float64{
				"semiconductor":   0.25,
				"ai_supply_chain": 0.20,
				"financials":      0.15,
				"shipping":        0.10,
				"energy":          0.10,
				"defensive":       0.10,
				"cash":            0.10,
			},
			wantRiskLevel:    narrative.MacroRiskOrange,
			wantCanWithstand: false,
			wantDrawdown:     risk.DrawdownModerate,
			wantStrategy:     StrategyDefensive,
		},
		{
			name: "Normal Market - No Action Needed",
			macroData: marketdata.MacroDataSnapshot{
				VIX:     marketdata.MacroDataPoint{Value: 18.0, ChangePct: 5.0},
				USD_TWD: marketdata.MacroDataPoint{Value: 32.0, ChangePct: 0.2},
				JPY:     marketdata.MacroDataPoint{Value: 150.0, ChangePct: -0.1},
				US10Y:   marketdata.MacroDataPoint{Value: 4.0, ChangePct: 0.0},
				Gold:    marketdata.MacroDataPoint{Value: 2350, ChangePct: 0.5},
				Oil:     marketdata.MacroDataPoint{Value: 75.0, ChangePct: 1.0},
			},
			sectorData: narrative.SectorDataSnapshot{
				AIRevenueGrowth:    55.0,
				CoWoSUtilization:   88.0,
				CapexGrowth:        40.0,
				SemiconductorIndex: 4000,
			},
			portfolioValue: 1000000,
			currentAlloc: map[string]float64{
				"semiconductor":   0.25,
				"ai_supply_chain": 0.20,
				"financials":      0.15,
				"shipping":        0.10,
				"energy":          0.10,
				"defensive":       0.10,
				"cash":            0.10,
			},
			wantRiskLevel:    narrative.MacroRiskGreen,
			wantCanWithstand: true,
			wantDrawdown:     risk.DrawdownNone,
			wantStrategy:     StrategyNormal,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Phase 1: Macro Risk Assessment
			macroEngine := narrative.NewMacroRiskAssessmentEngine()
			macroData := narrative.MacroDataSnapshot{
				US10Y:   narrative.MacroDataPoint{Value: tt.macroData.US10Y.Value, ChangePct: tt.macroData.US10Y.ChangePct},
				DXY:     narrative.MacroDataPoint{Value: tt.macroData.DXY.Value, ChangePct: tt.macroData.DXY.ChangePct},
				VIX:     narrative.MacroDataPoint{Value: tt.macroData.VIX.Value, ChangePct: tt.macroData.VIX.ChangePct},
				USD_TWD: narrative.MacroDataPoint{Value: tt.macroData.USD_TWD.Value, ChangePct: tt.macroData.USD_TWD.ChangePct},
				Oil:     narrative.MacroDataPoint{Value: tt.macroData.Oil.Value, ChangePct: tt.macroData.Oil.ChangePct},
				Gold:    narrative.MacroDataPoint{Value: tt.macroData.Gold.Value, ChangePct: tt.macroData.Gold.ChangePct},
				JPY:     narrative.MacroDataPoint{Value: tt.macroData.JPY.Value, ChangePct: tt.macroData.JPY.ChangePct},
			}
			macroAssessment := macroEngine.Assess(macroData)

			if macroAssessment.Level != tt.wantRiskLevel {
				t.Errorf("Phase 1 - Risk Level = %v, want %v", macroAssessment.Level, tt.wantRiskLevel)
			}
			t.Logf("Phase 1 - Macro Risk: %s (outflow prob: %.1f%%)", macroAssessment.Level.String(), macroAssessment.ForeignOutflowProb)
			t.Logf("Phase 1 - Primary Flow: %s", macroAssessment.PrimaryFlow)

			// Phase 2: Structural Trend Assessment
			structuralEngine := narrative.NewStructuralTrendEngine()
			structuralAssessment := structuralEngine.Assess(macroData, tt.sectorData)

			canWithstand := structuralEngine.CanWithstandMacroRisk(macroAssessment.Level, structuralAssessment)
			if canWithstand != tt.wantCanWithstand {
				t.Errorf("Phase 2 - CanWithstand = %v, want %v", canWithstand, tt.wantCanWithstand)
			}
			t.Logf("Phase 2 - Structural Override: %v (score: %.2f)", structuralAssessment.ShouldOverrideRisk, structuralAssessment.OverrideScore)

			// Phase 3: Macro-Aware Drawdown Decision
			drawdownEngine := risk.NewMacroAwareDrawdownEngine()
			drawdownDecision := drawdownEngine.Evaluate(macroAssessment, structuralAssessment)

			if drawdownDecision.Action != tt.wantDrawdown {
				t.Errorf("Phase 3 - Drawdown = %v, want %v", drawdownDecision.Action, tt.wantDrawdown)
			}
			t.Logf("Phase 3 - Drawdown: %s (%.0f%% reduction)", drawdownDecision.Action.String(), drawdownDecision.Percentage*100)

			// Phase 4a: Strategy Evolution
			strategyEvolver := NewStrategyEvolver()
			evolution := strategyEvolver.Evaluate(macroAssessment, structuralAssessment, drawdownDecision)

			if strategyEvolver.GetCurrentState() != tt.wantStrategy {
				t.Errorf("Phase 4a - Strategy = %v, want %v", strategyEvolver.GetCurrentState(), tt.wantStrategy)
			}
			if evolution != nil {
				t.Logf("Phase 4a - Strategy Evolution: %s -> %s", evolution.FromState.String(), evolution.ToState.String())
			}

			// Phase 4b: Sector Rotation
			sectorRotator := portfolio.NewSectorRotator()
			canRotate, rotateReason := sectorRotator.CanExecuteRotation(drawdownDecision)
			t.Logf("Phase 4b - Can Rotate: %v (%s)", canRotate, rotateReason)

			if canRotate {
				rotationPlan := sectorRotator.GeneratePlan(macroAssessment, tt.currentAlloc, nil)
				trades := sectorRotator.GetRebalancingTrades(rotationPlan, tt.portfolioValue)

				t.Logf("Phase 4b - Rotation Plan: %s", rotationPlan.Rationale)
				for _, trade := range trades[:min(3, len(trades))] {
					t.Logf("Phase 4b - Trade: %s %+.1f%% ($%.0f)", trade.Sector, trade.DeltaPct*100, trade.DeltaValue)
				}
			}

			// Verify final state consistency
			config := strategyEvolver.GetStrategyConfig()
			if drawdownDecision.Action >= risk.DrawdownSevere && config.AllowNewPositions {
				t.Error("Inconsistency: Severe drawdown but new positions allowed")
			}
		})
	}
}
