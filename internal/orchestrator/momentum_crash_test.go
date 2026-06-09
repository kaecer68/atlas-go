package orchestrator

import (
	"math"
	"testing"

	"github.com/kaecer68/atlas-go/internal/domain"
)

func TestApplyMomentumCrashProtectionWhenVIXHigh(t *testing.T) {
	recs := []domain.Recommendation{
		{
			Symbol: "2330.TW",
			FactorScores: domain.FactorScores{
				Momentum: 80,
				Value:    70,
				Quality:  60,
				Agent:    50,
				Total:    66.5, // 80*0.3 + 70*0.25 + 60*0.25 + 50*0.2
				Breakdown: &domain.FactorScoreBreakdown{
					Momentum: domain.FactorScoreItem{Score: 80},
					Value:    domain.FactorScoreItem{Score: 70},
					Quality:  domain.FactorScoreItem{Score: 60},
					Agent:    domain.FactorScoreItem{Score: 50},
					Total:    domain.FactorScoreItem{Score: 66.5},
				},
			},
		},
	}

	quotes := map[string]domain.Quote{
		"VIX": {Symbol: "VIX", Last: 35},
	}

	result := applyMomentumCrashProtection(recs, quotes)

	if result[0].FactorScores.Momentum != 0 {
		t.Errorf("momentum should be 0, got %v", result[0].FactorScores.Momentum)
	}
	if result[0].FactorScores.Breakdown.Momentum.Score != 0 {
		t.Errorf("momentum breakdown should be 0, got %v", result[0].FactorScores.Breakdown.Momentum.Score)
	}
	// With normalized weights (value=0.25/0.70, quality=0.25/0.70, agent=0.20/0.70)
	// Use epsilon comparison for floating point to avoid precision issues
	expectedTotal := 70*(0.25/0.70) + 60*(0.25/0.70) + 50*(0.20/0.70)
	if math.Abs(result[0].FactorScores.Total-expectedTotal) > 1e-9 {
		t.Errorf("total should be %.6f (without momentum), got %.6f", expectedTotal, result[0].FactorScores.Total)
	}
}

func TestApplyMomentumCrashProtectionWhenVIXLow(t *testing.T) {
	recs := []domain.Recommendation{
		{
			Symbol: "2330.TW",
			FactorScores: domain.FactorScores{
				Momentum: 80,
				Value:    70,
				Quality:  60,
				Agent:    50,
				Total:    66.5,
			},
		},
	}

	quotes := map[string]domain.Quote{
		"VIX": {Symbol: "VIX", Last: 20},
	}

	result := applyMomentumCrashProtection(recs, quotes)

	if result[0].FactorScores.Momentum != 80 {
		t.Errorf("momentum should remain 80 when VIX low, got %v", result[0].FactorScores.Momentum)
	}
}

func TestApplyMomentumCrashProtectionWithNoVIXQuote(t *testing.T) {
	recs := []domain.Recommendation{
		{
			Symbol: "2330.TW",
			FactorScores: domain.FactorScores{
				Momentum: 80,
				Value:    70,
				Quality:  60,
				Agent:    50,
				Total:    66.5,
			},
		},
	}

	quotes := map[string]domain.Quote{
		"2330.TW": {Symbol: "2330.TW", Last: 800},
	}

	result := applyMomentumCrashProtection(recs, quotes)

	if result[0].FactorScores.Momentum != 80 {
		t.Errorf("momentum should remain 80 when no VIX quote, got %v", result[0].FactorScores.Momentum)
	}
}

func TestApplyMomentumCrashProtectionWithVIXThresholdExactly30(t *testing.T) {
	recs := []domain.Recommendation{
		{
			Symbol: "2330.TW",
			FactorScores: domain.FactorScores{
				Momentum: 80,
				Value:    70,
				Quality:  60,
				Agent:    50,
				Total:    66.5,
			},
		},
	}

	quotes := map[string]domain.Quote{
		"VIX": {Symbol: "VIX", Last: 30},
	}

	result := applyMomentumCrashProtection(recs, quotes)

	// VIX <= 30 should NOT trigger protection
	if result[0].FactorScores.Momentum != 80 {
		t.Errorf("momentum should remain 80 when VIX == 30, got %v", result[0].FactorScores.Momentum)
	}
}

func TestApplyMomentumCrashProtectionMultipleRecs(t *testing.T) {
	recs := []domain.Recommendation{
		{Symbol: "A", FactorScores: domain.FactorScores{Momentum: 80, Value: 70, Quality: 60, Agent: 50, Total: 66.5}},
		{Symbol: "B", FactorScores: domain.FactorScores{Momentum: 90, Value: 60, Quality: 70, Agent: 40, Total: 68.0}},
	}

	quotes := map[string]domain.Quote{
		"^VIX": {Symbol: "^VIX", Last: 35},
	}

	result := applyMomentumCrashProtection(recs, quotes)

	for i, r := range result {
		if r.FactorScores.Momentum != 0 {
			t.Errorf("rec %d momentum should be 0, got %v", i, r.FactorScores.Momentum)
		}
	}
}

func TestExecuteRegistryResearchWithMomentumCrashProtection(t *testing.T) {
	registry := domain.AgentRegistry{
		Agents: []domain.AgentSpec{
			{ID: "test", Layer: domain.LayerSector, Skill: "test", Enabled: true},
		},
	}
	quotes := []domain.Quote{
		{Symbol: "2330.TW", Last: 800, Volume: 1000000, IsTradable: true},
		{Symbol: "VIX", Last: 35, IsTradable: true},
	}

	policy := domain.ExecutionPolicy{
		RequireCROPass:          false,
		MomentumCrashProtection: true,
	}

	_, raw, _, _ := ExecuteRegistryResearchDetailedWithPolicyAndGuards(registry, quotes, map[string]string{}, policy)

	for _, r := range raw {
		if r.FactorScores.Momentum != 0 {
			t.Errorf("expected momentum to be 0 when VIX=35, got %v", r.FactorScores.Momentum)
		}
	}
}
