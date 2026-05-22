package risk

import (
	"testing"

	"github.com/kaecer68/atlas-go/internal/narrative"
)

func TestPortfolioRiskAssessment_TotalScore(t *testing.T) {
	assessment := &PortfolioRiskAssessment{
		ConcentrationScore: 0.3,
		SectorExposure: map[string]float64{
			"semiconductor": 0.4,
			"financials":    0.2,
		},
		FactorExposure: map[string]float64{
			"momentum": 0.5,
			"value":    0.3,
		},
		TotalRiskScore: 0.45,
	}

	if assessment.ConcentrationScore != 0.3 {
		t.Errorf("ConcentrationScore = %.2f, want 0.30", assessment.ConcentrationScore)
	}
	if len(assessment.SectorExposure) != 2 {
		t.Errorf("SectorExposure len = %d, want 2", len(assessment.SectorExposure))
	}
	if len(assessment.FactorExposure) != 2 {
		t.Errorf("FactorExposure len = %d, want 2", len(assessment.FactorExposure))
	}
	if assessment.TotalRiskScore != 0.45 {
		t.Errorf("TotalRiskScore = %.2f, want 0.45", assessment.TotalRiskScore)
	}
}

// stubPortfolioRiskProvider implements PortfolioRiskProvider for testing.
type stubPortfolioRiskProvider struct {
	assessment *PortfolioRiskAssessment
}

func (s *stubPortfolioRiskProvider) Assess() *PortfolioRiskAssessment {
	return s.assessment
}

func TestPortfolioRiskProvider_Nil(t *testing.T) {
	engine := NewMacroAwareDrawdownEngine()

	macroAssessment := &narrative.MacroRiskAssessment{
		Level:              narrative.MacroRiskOrange,
		ForeignOutflowProb: 55.0,
	}
	structuralAssessment := &narrative.StructuralTrendAssessment{
		OverrideScore:      0.0,
		ShouldOverrideRisk: false,
	}

	decision, breakdown := engine.EvaluateWithPortfolio(
		macroAssessment, structuralAssessment, nil, nil,
	)

	if decision.Action != DrawdownModerate {
		t.Errorf("Action = %v, want DrawdownModerate", decision.Action)
	}
	// Steps: macro, structural (no override), no industry, no portfolio
	if len(breakdown.Steps) != 2 {
		t.Errorf("Steps count = %d, want 2 (macro + structural)", len(breakdown.Steps))
	}
}

func TestEvaluateWithPortfolio_LowRisk(t *testing.T) {
	engine := NewMacroAwareDrawdownEngine()

	macroAssessment := &narrative.MacroRiskAssessment{
		Level:              narrative.MacroRiskYellow,
		ForeignOutflowProb: 35.0,
	}
	structuralAssessment := &narrative.StructuralTrendAssessment{}

	portfolioProvider := &stubPortfolioRiskProvider{
		assessment: &PortfolioRiskAssessment{
			ConcentrationScore: 0.1,
			SectorExposure: map[string]float64{
				"semiconductor": 0.3,
				"financials":    0.3,
			},
			FactorExposure: map[string]float64{
				"momentum": 0.3,
				"value":    0.3,
			},
			TotalRiskScore: 0.2,
		},
	}

	decision, breakdown := engine.EvaluateWithPortfolio(
		macroAssessment, structuralAssessment, nil, portfolioProvider,
	)

	if decision.Action != DrawdownLight {
		t.Errorf("Action = %v, want DrawdownLight (low risk should NOT escalate)", decision.Action)
	}

	// Should have 3 steps: macro, structural, portfolio_risk
	if len(breakdown.Steps) != 3 {
		t.Errorf("Steps count = %d, want 3 (macro + structural + portfolio_risk)", len(breakdown.Steps))
	}

	lastStep := breakdown.Steps[len(breakdown.Steps)-1]
	if lastStep.Source != "portfolio_risk" {
		t.Errorf("Last step source = %s, want portfolio_risk", lastStep.Source)
	}
	if lastStep.Action != "no_change" {
		t.Errorf("Last step action = %s, want no_change", lastStep.Action)
	}
}

func TestEvaluateWithPortfolio_HighRiskEscalates(t *testing.T) {
	engine := NewMacroAwareDrawdownEngine()

	macroAssessment := &narrative.MacroRiskAssessment{
		Level:              narrative.MacroRiskYellow,
		ForeignOutflowProb: 35.0,
	}
	structuralAssessment := &narrative.StructuralTrendAssessment{}

	portfolioProvider := &stubPortfolioRiskProvider{
		assessment: &PortfolioRiskAssessment{
			ConcentrationScore: 0.85,
			SectorExposure: map[string]float64{
				"semiconductor": 0.6,
			},
			FactorExposure: map[string]float64{
				"momentum": 0.7,
			},
			TotalRiskScore: 0.85,
		},
	}

	decision, breakdown := engine.EvaluateWithPortfolio(
		macroAssessment, structuralAssessment, nil, portfolioProvider,
	)

	// Yellow base = Light, high portfolio risk should escalate to Moderate
	if decision.Action != DrawdownModerate {
		t.Errorf("Action = %v, want DrawdownModerate (high risk SHOULD escalate)", decision.Action)
	}

	lastStep := breakdown.Steps[len(breakdown.Steps)-1]
	if lastStep.Source != "portfolio_risk" {
		t.Errorf("Last step source = %s, want portfolio_risk", lastStep.Source)
	}
	if lastStep.Action != "escalate" {
		t.Errorf("Last step action = %s, want escalate", lastStep.Action)
	}
}

func TestEvaluateWithPortfolio_ModerateRiskNoEscalation(t *testing.T) {
	engine := NewMacroAwareDrawdownEngine()

	macroAssessment := &narrative.MacroRiskAssessment{
		Level:              narrative.MacroRiskYellow,
		ForeignOutflowProb: 35.0,
	}
	structuralAssessment := &narrative.StructuralTrendAssessment{}

	portfolioProvider := &stubPortfolioRiskProvider{
		assessment: &PortfolioRiskAssessment{
			ConcentrationScore: 0.4,
			SectorExposure: map[string]float64{
				"semiconductor": 0.35,
				"shipping":      0.25,
			},
			FactorExposure: map[string]float64{
				"momentum": 0.4,
				"value":    0.3,
			},
			TotalRiskScore: 0.55,
		},
	}

	decision, _ := engine.EvaluateWithPortfolio(
		macroAssessment, structuralAssessment, nil, portfolioProvider,
	)

	// TotalRiskScore 0.55 is above 0.5 but below 0.8, so no escalation
	if decision.Action != DrawdownLight {
		t.Errorf("Action = %v, want DrawdownLight (moderate risk should NOT escalate)", decision.Action)
	}
}
