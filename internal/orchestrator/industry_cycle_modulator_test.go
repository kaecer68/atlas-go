package orchestrator

import (
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/industry"
)

// TestIndustryCycleModulator_NoProvenance validates that the industry cycle
// modulator produces correct ConvictionSteps with provenance fields populated.
func TestIndustryCycleModulator_NoProvenance(t *testing.T) {
	tracker := industry.NewCycleTracker()
	tracker.UpdatePosition("semiconductor", industry.IndustryMetrics{
		IndustryID:          "semiconductor",
		RevenueGrowthYoY:    0.25,
		ProfitGrowthYoY:     0.30,
		InventoryTurnover:   5.5,
		CapacityUtilization: 0.85,
	})

	mod := NewIndustryCycleModulator(tracker)

	recs := []domain.Recommendation{
		{Agent: "agent1", Conviction: 50, ConvictionBreakdown: &domain.ConvictionBreakdown{Base: 50, Floor: 40, Final: 50, Steps: []domain.ConvictionStep{}}},
	}
	registry := domain.AgentRegistry{
		Agents: []domain.AgentSpec{{ID: "agent1", Skill: "semiconductor_desk"}},
	}

	steps := mod.CollectModulationSteps(recs, registry)
	if len(steps) == 0 {
		t.Fatal("expected at least one modulation step")
	}
	for _, ms := range steps {
		if ms.RecIndex >= len(recs) {
			continue
		}
		for _, step := range ms.Steps {
			recs[ms.RecIndex].Conviction += step.Delta
			if recs[ms.RecIndex].ConvictionBreakdown != nil {
				recs[ms.RecIndex].ConvictionBreakdown.Steps = append(recs[ms.RecIndex].ConvictionBreakdown.Steps, step)
				recs[ms.RecIndex].ConvictionBreakdown.Final = recs[ms.RecIndex].Conviction
			}
		}
	}

	if len(recs[0].ConvictionBreakdown.Steps) == 0 {
		t.Fatal("expected at least one conviction step after modulation")
	}

	step := recs[0].ConvictionBreakdown.Steps[len(recs[0].ConvictionBreakdown.Steps)-1]
	if step.Rule != "modulator:industry_cycle:cycle_phase" {
		t.Fatalf("expected rule 'modulator:industry_cycle:cycle_phase', got %q", step.Rule)
	}

	if step.Source == "" {
		t.Error("expected Source to be populated")
	}
	if step.ParamRef == "" {
		t.Error("expected ParamRef to be populated")
	}
	if step.ParamValue == "" {
		t.Error("expected ParamValue to be populated")
	}
}

func TestModulatePosition_LowConfidence(t *testing.T) {
	card := &industry.CycleStatusCard{
		Date:            time.Now(),
		CycleConfidence: 0.2,
	}
	mod := &IndustryCycleModulator{}
	result := mod.ModulatePosition(100.0, card)
	if result != 80.0 {
		t.Fatalf("expected position reduced to 80 for low confidence, got %.1f", result)
	}
}

func TestModulatePosition_HighConfidence(t *testing.T) {
	card := &industry.CycleStatusCard{
		Date:            time.Now(),
		CycleConfidence: 0.8,
	}
	mod := &IndustryCycleModulator{}
	result := mod.ModulatePosition(100.0, card)
	if result != 100.0 {
		t.Fatalf("expected position unchanged for high confidence, got %.1f", result)
	}
}

func TestModulatePosition_NilCard(t *testing.T) {
	mod := &IndustryCycleModulator{}
	result := mod.ModulatePosition(100.0, nil)
	if result != 100.0 {
		t.Fatalf("expected position unchanged for nil card, got %.1f", result)
	}
}

func TestModulatePosition_ZeroSize(t *testing.T) {
	card := &industry.CycleStatusCard{
		CycleConfidence: 0.5,
	}
	mod := &IndustryCycleModulator{}
	result := mod.ModulatePosition(0.0, card)
	if result != 0.0 {
		t.Fatalf("expected zero for zero size, got %.1f", result)
	}
}

func TestModulateScore_StrongBullish(t *testing.T) {
	card := &industry.CycleStatusCard{
		CompositeCoefficient: 1.15,
		SentimentLabel:       "強烈看多",
	}
	mod := &IndustryCycleModulator{}
	baseScore := 50
	result := mod.ModulateScore(baseScore, card)
	if result <= baseScore {
		t.Fatalf("expected score increased for bullish sentiment, got %d (base %d)", result, baseScore)
	}
}

func TestModulateScore_Bearish(t *testing.T) {
	card := &industry.CycleStatusCard{
		CompositeCoefficient: 0.85,
		SentimentLabel:       "強烈看空",
	}
	mod := &IndustryCycleModulator{}
	baseScore := 50
	result := mod.ModulateScore(baseScore, card)
	if result >= baseScore {
		t.Fatalf("expected score reduced for bearish sentiment, got %d (base %d)", result, baseScore)
	}
}

func TestModulateScore_NilCard(t *testing.T) {
	mod := &IndustryCycleModulator{}
	result := mod.ModulateScore(50, nil)
	if result != 50 {
		t.Fatalf("expected score unchanged for nil card, got %d", result)
	}
}

func TestSetAndGetCycleCard(t *testing.T) {
	mod := &IndustryCycleModulator{}
	if mod.GetCycleCard() != nil {
		t.Fatal("expected nil card initially")
	}
	card := &industry.CycleStatusCard{
		SentimentLabel:       "偏多",
		CompositeCoefficient: 1.08,
		CycleConfidence:      0.75,
	}
	mod.SetCycleCard(card)
	got := mod.GetCycleCard()
	if got == nil {
		t.Fatal("expected card after SetCycleCard")
	}
	if got.SentimentLabel != "偏多" {
		t.Fatalf("expected sentiment '偏多', got %q", got.SentimentLabel)
	}
	mod.SetCycleCard(nil)
	if mod.GetCycleCard() != nil {
		t.Fatal("expected nil card after clearing")
	}
}

func TestNilModulator_SetCycleCard(t *testing.T) {
	var mod *IndustryCycleModulator
	card := &industry.CycleStatusCard{}
	mod.SetCycleCard(card)
	if mod.GetCycleCard() != nil {
		t.Fatal("nil modulator should not panic")
	}
}

func TestCycleConfidenceFromCard_UsesCard(t *testing.T) {
	mod := &IndustryCycleModulator{}
	card := &industry.CycleStatusCard{
		CycleConfidence: 0.85,
	}
	mod.SetCycleCard(card)
	got := mod.CycleConfidenceFromCard("semiconductor")
	if got != 0.85 {
		t.Fatalf("expected confidence 0.85 from card, got %.2f", got)
	}
}

func TestCycleConfidenceFromCard_FallsBackToTracker(t *testing.T) {
	ct := industry.NewCycleTracker()
	ct.UpdatePosition("semiconductor", industry.IndustryMetrics{
		IndustryID:       "semiconductor",
		RevenueGrowthYoY: 0.25,
		ProfitGrowthYoY:  0.30,
	})
	mod := NewIndustryCycleModulator(ct)
	got := mod.CycleConfidenceFromCard("semiconductor")
	if got <= 0 {
		t.Fatalf("expected positive confidence from tracker fallback, got %.2f", got)
	}
}

func TestCycleConfidenceFromCard_ReturnsDefault(t *testing.T) {
	mod := &IndustryCycleModulator{}
	got := mod.CycleConfidenceFromCard("nonexistent")
	if got != 0.5 {
		t.Fatalf("expected default 0.5, got %.2f", got)
	}
}
