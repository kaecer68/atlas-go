package portfolio

import (
	"testing"

	"github.com/kaecer68/atlas-go/internal/narrative"
	"github.com/kaecer68/atlas-go/internal/risk"
)

func TestSectorRotator_GeneratePlan(t *testing.T) {
	rotator := NewSectorRotator()

	tests := []struct {
		name           string
		macroLevel     narrative.MacroRiskLevel
		primaryFlow    string
		favoredSectors []string
		avoidedSectors []string
		wantMinSectors int
	}{
		{
			name:           "Green Risk - Normal Allocation",
			macroLevel:     narrative.MacroRiskGreen,
			primaryFlow:    "mixed",
			wantMinSectors: 5,
		},
		{
			name:           "Risk Off - Defensive Tilt",
			macroLevel:     narrative.MacroRiskOrange,
			primaryFlow:    "risk_off",
			favoredSectors: []string{"gold", "utilities"},
			avoidedSectors: []string{"ai_supply_chain"},
			wantMinSectors: 5,
		},
		{
			name:           "Sector Rotation - Energy Focus",
			macroLevel:     narrative.MacroRiskOrange,
			primaryFlow:    "sector_rotation",
			favoredSectors: []string{"energy", "oil_services"},
			avoidedSectors: []string{"high_valuation_tech"},
			wantMinSectors: 5,
		},
		{
			name:           "Carry Trade Unwind - Cash Heavy",
			macroLevel:     narrative.MacroRiskRed,
			primaryFlow:    "carry_trade_unwind",
			favoredSectors: []string{"cash", "short_term_bonds"},
			avoidedSectors: []string{"all_equities"},
			wantMinSectors: 4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			macroAssessment := &narrative.MacroRiskAssessment{
				Level:          tt.macroLevel,
				PrimaryFlow:    tt.primaryFlow,
				FavoredSectors: tt.favoredSectors,
				AvoidedSectors: tt.avoidedSectors,
			}

			currentAllocations := map[string]float64{
				"semiconductor":   0.25,
				"ai_supply_chain": 0.20,
				"financials":      0.15,
				"shipping":        0.10,
				"energy":          0.10,
				"defensive":       0.10,
				"cash":            0.10,
			}

			plan := rotator.GeneratePlan(macroAssessment, currentAllocations, nil)

			if len(plan.Allocations) < tt.wantMinSectors {
				t.Errorf("Allocation count = %d, want at least %d", len(plan.Allocations), tt.wantMinSectors)
			}

			// Verify allocations sum to ~1.0
			total := 0.0
			for _, alloc := range plan.Allocations {
				total += alloc.TargetPct
			}
			tolerance := 0.01
			if total < 1.0-tolerance || total > 1.0+tolerance {
				t.Errorf("Total allocation = %.2f, want ~1.0", total)
			}

			t.Logf("Plan: %s", plan.Rationale)
			for _, alloc := range plan.Allocations[:min(5, len(plan.Allocations))] {
				t.Logf("  %s: %.1f%% (delta: %+.1f%%)", alloc.Sector, alloc.TargetPct*100, alloc.Delta*100)
			}
		})
	}
}

func TestSectorRotator_CanExecuteRotation(t *testing.T) {
	rotator := NewSectorRotator()

	tests := []struct {
		name       string
		action     risk.DrawdownAction
		wantCan    bool
		wantReason string
	}{
		{"None - Allowed", risk.DrawdownNone, true, "Normal conditions"},
		{"Light - Allowed", risk.DrawdownLight, true, "Normal conditions"},
		{"Moderate - Allowed", risk.DrawdownModerate, true, "Moderate drawdown"},
		{"Severe - Blocked", risk.DrawdownSevere, false, "Severe drawdown"},
		{"Emergency - Blocked", risk.DrawdownEmergency, false, "Emergency drawdown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decision := &risk.MacroAwareDrawdownDecision{Action: tt.action}
			can, reason := rotator.CanExecuteRotation(decision)

			if can != tt.wantCan {
				t.Errorf("CanExecuteRotation() = %v, want %v", can, tt.wantCan)
			}

			if len(reason) == 0 {
				t.Error("Expected non-empty reason")
			}
		})
	}
}

func TestSectorRotator_GetRebalancingTrades(t *testing.T) {
	rotator := NewSectorRotator()

	plan := &SectorRotationPlan{
		Allocations: []SectorAllocation{
			{Sector: "semiconductor", TargetPct: 0.30, CurrentPct: 0.20, Delta: 0.10},
			{Sector: "cash", TargetPct: 0.15, CurrentPct: 0.05, Delta: 0.10},
			{Sector: "ai_supply_chain", TargetPct: 0.10, CurrentPct: 0.25, Delta: -0.15},
			{Sector: "energy", TargetPct: 0.05, CurrentPct: 0.05, Delta: 0.0},
		},
	}

	trades := rotator.GetRebalancingTrades(plan, 1000000)

	if len(trades) != 3 {
		t.Errorf("Trade count = %d, want 3", len(trades))
	}

	// Verify trades are sorted by absolute delta
	for i := 1; i < len(trades); i++ {
		if absFloat64(trades[i-1].DeltaPct) < absFloat64(trades[i].DeltaPct) {
			t.Error("Trades not sorted by absolute delta")
		}
	}

	for _, trade := range trades {
		t.Logf("Trade: %s, Delta: %+.1f%%, Value: $%.0f", trade.Sector, trade.DeltaPct*100, trade.DeltaValue)
	}
}

// TestSectorRotator_GeneratePlan_ConfigSource verifies that GeneratePlan()
// sets ConfigSource on the returned plan.
func TestSectorRotator_GeneratePlan_ConfigSource(t *testing.T) {
	rotator := NewSectorRotator()
	assessment := &narrative.MacroRiskAssessment{
		Level:       narrative.MacroRiskGreen,
		PrimaryFlow: "risk_on",
	}
	plan := rotator.GeneratePlan(assessment, nil, nil)
	if plan.ConfigSource == "" {
		t.Error("expected ConfigSource to be non-empty on generated plan")
	}
	t.Logf("ConfigSource: %s", plan.ConfigSource)
}
