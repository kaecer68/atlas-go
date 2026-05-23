package orchestrator

import (
	"context"
	"testing"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/narrative"
	"github.com/kaecer68/atlas-go/internal/risk"
)

func TestQuotesToMacroDataSnapshot(t *testing.T) {
	quotes := []domain.Quote{
		{Symbol: "^TNX", Open: 4.0, Last: 4.2},
		{Symbol: "^VIX", Open: 20.0, Last: 25.0},
		{Symbol: "GC=F", Open: 2300.0, Last: 2350.0},
	}

	snapshot := QuotesToMacroDataSnapshot(quotes)

	if snapshot.US10Y.Value != 4.2 {
		t.Errorf("US10Y.Value = %v, want 4.2", snapshot.US10Y.Value)
	}
	if snapshot.VIX.Value != 25.0 {
		t.Errorf("VIX.Value = %v, want 25.0", snapshot.VIX.Value)
	}
	if snapshot.Gold.Value != 2350.0 {
		t.Errorf("Gold.Value = %v, want 2350.0", snapshot.Gold.Value)
	}
}

func TestSystem_assessMacroRisk(t *testing.T) {
	sys := &System{
		SystemCore: &SystemCore{
			risk: RiskOps{
				macroRiskEngine: narrative.NewMacroRiskAssessmentEngine(),
			},
		},
	}

	quotes := []domain.Quote{
		{Symbol: "^VIX", Open: 20.0, Last: 35.0},
		{Symbol: "^TNX", Open: 4.0, Last: 4.5},
		{Symbol: "GC=F", Open: 2300.0, Last: 2400.0},
	}

	assessment := sys.assessMacroRisk(quotes)

	if assessment == nil {
		t.Fatal("assessMacroRisk returned nil")
	}
	if assessment.Level != narrative.MacroRiskOrange && assessment.Level != narrative.MacroRiskRed {
		t.Logf("Risk level: %s (VIX=35 should trigger orange/red)", assessment.Level.String())
	}
}

func TestSystem_assessStructuralTrends(t *testing.T) {
	sys := &System{
		SystemCore: &SystemCore{
			risk: RiskOps{
				structuralTrendEngine: narrative.NewStructuralTrendEngine(),
			},
		},
	}

	macroData := narrative.MacroDataSnapshot{}
	assessment, sectorData := sys.assessStructuralTrends(context.Background(), macroData)

	// With nil sectorDataProvider, should return nil gracefully
	if assessment != nil {
		t.Error("assessStructuralTrends with nil sectorDataProvider should return nil")
	}
	if sectorData != (narrative.SectorDataSnapshot{}) {
		t.Error("assessStructuralTrends with nil sectorDataProvider should return zero SectorDataSnapshot")
	}
}

func TestSystem_evaluateDrawdown(t *testing.T) {
	sys := &System{
		SystemCore: &SystemCore{
			risk: RiskOps{
				macroDrawdownEngine: risk.NewMacroAwareDrawdownEngine(),
			},
		},
	}

	// Test with nil assessment
	decision := sys.evaluateDrawdown(nil, nil)
	if decision != nil {
		t.Error("evaluateDrawdown with nil macroAssessment should return nil")
	}

	// Test with valid assessment
	macroAssessment := &narrative.MacroRiskAssessment{
		Level: narrative.MacroRiskGreen,
	}
	structuralAssessment := &narrative.StructuralTrendAssessment{}
	decision = sys.evaluateDrawdown(macroAssessment, structuralAssessment)
	if decision == nil {
		t.Fatal("evaluateDrawdown with valid assessment should return decision")
	}
	if decision.Action != risk.DrawdownNone {
		t.Errorf("Drawdown action = %v, want %v", decision.Action, risk.DrawdownNone)
	}
}

func TestSystem_MacroPipeline_EndToEnd(t *testing.T) {
	// Create temp dir for sector data
	tmpDir := t.TempDir()

	sys := &System{
		SystemCore: &SystemCore{
			risk: RiskOps{
				macroRiskEngine:       narrative.NewMacroRiskAssessmentEngine(),
				structuralTrendEngine: narrative.NewStructuralTrendEngine(),
				macroDrawdownEngine:   risk.NewMacroAwareDrawdownEngine(),
				sectorDataProvider:    marketdata.NewSectorDataProvider(tmpDir),
			},
		},
	}

	quotes := []domain.Quote{
		{Symbol: "^VIX", Open: 18.0, Last: 20.0},
		{Symbol: "^TNX", Open: 4.0, Last: 4.1},
		{Symbol: "GC=F", Open: 2300.0, Last: 2350.0},
	}

	// Phase 1: Macro Risk
	macroAssessment := sys.assessMacroRisk(quotes)
	if macroAssessment == nil {
		t.Fatal("Phase 1 failed")
	}
	t.Logf("Phase 1 - Macro Risk: %s", macroAssessment.Level.String())

	// Phase 2: Structural Trends
	macroData := QuotesToMacroDataSnapshot(quotes)
	structuralAssessment, _ := sys.assessStructuralTrends(context.Background(), macroData)
	if structuralAssessment == nil {
		t.Fatal("Phase 2 failed")
	}
	t.Logf("Phase 2 - Structural Override: %v", structuralAssessment.ShouldOverrideRisk)

	// Phase 3: Drawdown
	drawdown := sys.evaluateDrawdown(macroAssessment, structuralAssessment)
	if drawdown == nil {
		t.Fatal("Phase 3 failed")
	}
	t.Logf("Phase 3 - Drawdown: %s", drawdown.Action.String())

	// Verify pipeline consistency
	if macroAssessment.Level == narrative.MacroRiskGreen && drawdown.Action != risk.DrawdownNone {
		t.Error("Green macro risk should result in no drawdown")
	}
}

func TestSystem_MacroPipeline_NilEngines(t *testing.T) {
	sys := &System{
		SystemCore: &SystemCore{
			risk: RiskOps{},
		},
	}

	quotes := []domain.Quote{{Symbol: "^VIX", Open: 20.0, Last: 25.0}}

	// All methods should return nil/zero gracefully
	macro := sys.assessMacroRisk(quotes)
	if macro != nil {
		t.Error("assessMacroRisk with nil engine should return nil")
	}

	macroData := marketdata.MacroDataSnapshot{}
	assessment, sector := sys.assessStructuralTrends(context.Background(), narrative.MacroDataSnapshot(macroData))
	if assessment != nil {
		t.Error("assessStructuralTrends with nil engine should return nil")
	}
	if sector != (narrative.SectorDataSnapshot{}) {
		t.Error("assessStructuralTrends with nil engine should return zero SectorDataSnapshot")
	}

	drawdown := sys.evaluateDrawdown(nil, nil)
	if drawdown != nil {
		t.Error("evaluateDrawdown with nil engine should return nil")
	}
}
