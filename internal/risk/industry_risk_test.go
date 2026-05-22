package risk

import (
	"testing"

	"github.com/kaecer68/atlas-go/internal/industry"
)

func TestCycleTrackerRiskProvider_Assess(t *testing.T) {
	tracker := industry.NewCycleTracker()
	provider := NewCycleTrackerRiskProvider(tracker, nil)

	assessment, err := provider.Assess()
	if err != nil {
		t.Fatalf("Assess() error: %v", err)
	}

	if assessment.TotalIndustryCount == 0 {
		t.Error("expected non-zero total industry count")
	}

	if len(assessment.TopRiskIndustries) == 0 {
		t.Error("expected non-empty top risk industries")
	}

	if len(assessment.TopRiskIndustries) > 5 {
		t.Errorf("expected at most 5 top risk industries, got %d", len(assessment.TopRiskIndustries))
	}

	// Verify sorting: first item should have lowest phase score
	for i := 1; i < len(assessment.TopRiskIndustries); i++ {
		if assessment.TopRiskIndustries[i-1].PhaseScore > assessment.TopRiskIndustries[i].PhaseScore {
			t.Errorf("top risk industries not sorted: item %d score %.2f > item %d score %.2f",
				i-1, assessment.TopRiskIndustries[i-1].PhaseScore,
				i, assessment.TopRiskIndustries[i].PhaseScore)
		}
	}

	// WeightedCycleScore should be non-zero with default tracker data
	if assessment.WeightedCycleScore == 0 {
		t.Error("WeightedCycleScore is zero, expected non-zero with default data")
	}

	t.Logf("Total industries: %d", assessment.TotalIndustryCount)
	t.Logf("Recession: %d, Expansion: %d", assessment.RecessionIndustryCount, assessment.ExpansionIndustryCount)
	t.Logf("WeightedCycleScore: %.3f", assessment.WeightedCycleScore)
	for _, item := range assessment.TopRiskIndustries {
		t.Logf("  Risk: %s phase=%s score=%.2f conf=%.2f weight=%.2f",
			item.IndustryID, item.BusinessCycle, item.PhaseScore, item.Confidence, item.Weight)
	}
}

func TestCycleTrackerRiskProvider_Assess_WithWeights(t *testing.T) {
	tracker := industry.NewCycleTracker()
	weights := map[string]float64{
		"semiconductor":   2.0,
		"ai_supply_chain": 2.5,
		"shipping":        0.5,
		"financials":      1.0,
		"consumer":        1.0,
		"electronics":     1.5,
		"robotics":        0.8,
		"energy":          0.7,
		"industrial":      0.6,
	}
	provider := NewCycleTrackerRiskProvider(tracker, weights)

	assessment, err := provider.Assess()
	if err != nil {
		t.Fatalf("Assess() error: %v", err)
	}

	if assessment.TotalIndustryCount == 0 {
		t.Error("expected non-zero total industry count")
	}

	// Verify weighted items have their weights
	for _, item := range assessment.TopRiskIndustries {
		if expectedWeight, ok := weights[item.IndustryID]; ok {
			if item.Weight != expectedWeight {
				t.Errorf("item %s weight = %.2f, want %.2f", item.IndustryID, item.Weight, expectedWeight)
			}
		}
	}

	t.Logf("WeightedCycleScore (with weights): %.3f", assessment.WeightedCycleScore)
}

func TestIndustryRiskAssessment_WeightedScore(t *testing.T) {
	tracker := industry.NewCycleTracker()
	provider := NewCycleTrackerRiskProvider(tracker, nil)

	assessment, err := provider.Assess()
	if err != nil {
		t.Fatalf("Assess() error: %v", err)
	}

	// With default metrics, shipping should be in recession (negative growth)
	// and semiconductor/ai_supply_chain should be in expansion
	if assessment.RecessionIndustryCount == 0 && assessment.ExpansionIndustryCount == 0 {
		t.Error("expected non-zero recession or expansion counts with default data")
	}

	// WeightedCycleScore should reflect the mix of phases
	// With many expansion industries and some recession, score should be somewhere in the middle
	t.Logf("WeightedCycleScore: %.3f (recession=%d, expansion=%d)",
		assessment.WeightedCycleScore, assessment.RecessionIndustryCount, assessment.ExpansionIndustryCount)
}

func TestNewCycleTrackerRiskProvider_NilTracker(t *testing.T) {
	provider := NewCycleTrackerRiskProvider(nil, nil)

	_, err := provider.Assess()
	if err == nil {
		t.Error("expected error for nil tracker")
	}
}

func TestNewCycleTrackerRiskProvider_NilWeights(t *testing.T) {
	tracker := industry.NewCycleTracker()
	provider := NewCycleTrackerRiskProvider(tracker, nil)

	assessment, err := provider.Assess()
	if err != nil {
		t.Fatalf("Assess() error with nil weights: %v", err)
	}

	// With nil weights, all industries should have weight 1.0
	for _, item := range assessment.TopRiskIndustries {
		if item.Weight != 1.0 {
			t.Errorf("item %s weight = %.2f, want 1.0 (nil weights = equal weight)", item.IndustryID, item.Weight)
		}
	}
}
