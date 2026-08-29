package risktest

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/live"
	"github.com/kaecer68/atlas-go/internal/narrative"
	"github.com/kaecer68/atlas-go/internal/portfolio"
	"github.com/kaecer68/atlas-go/internal/risk"
)

func getScenarioCfg(cfg Config, name string) ScenarioConfig {
	if c, ok := cfg.Scenarios[name]; ok {
		return c
	}
	return ScenarioConfig{}
}

// RunMarketCrash simulates a full market crash (30%+ decline) and verifies VaR
// breach detection, drawdown escalation to severe/emergency, and RiskGate halt.
func RunMarketCrash(cfg Config) Result {
	scCfg := getScenarioCfg(cfg, "market_crash")
	result := Result{
		Scenario:  "market_crash",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}

	rng := rand.New(rand.NewSource(42))
	nDays := 60

	dailyReturns := make([]float64, nDays)
	portfolioValues := make([]float64, nDays)
	baseValue := 1_000_000.0
	portfolioValues[0] = baseValue

	for i := range nDays {
		if i == 0 {
			dailyReturns[i] = (rng.NormFloat64() * 0.008) + 0.0002
			continue
		}
		if i >= 30 {
			dailyReturns[i] = (rng.NormFloat64() * 0.025) - 0.015
		} else {
			dailyReturns[i] = (rng.NormFloat64() * 0.008) + 0.0002
		}
		portfolioValues[i] = portfolioValues[i-1] * (1.0 + dailyReturns[i])
	}

	calc := risk.NewVaRCalculator()
	snapshot := calc.ComputeRiskSnapshot(dailyReturns, portfolioValues)
	result.Results.RiskSnapshot = &snapshot

	varBreached := snapshot.VaR95 < -0.05 || snapshot.MaxDrawdownPct > 0.15
	result.Results.VaRBreach = varBreached

	macroLevel := narrative.MacroRiskRed
	switch scCfg.MacroRiskLevel {
	case "orange":
		macroLevel = narrative.MacroRiskOrange
	case "yellow":
		macroLevel = narrative.MacroRiskYellow
	case "green":
		macroLevel = narrative.MacroRiskGreen
	}

	outflowProb := 85.0
	if scCfg.ForeignOutflowProb > 0 {
		outflowProb = scCfg.ForeignOutflowProb
	}

	macroAssessment := &narrative.MacroRiskAssessment{
		Level:              macroLevel,
		ForeignOutflowProb: outflowProb,
		PrimaryFlow:        "risk_off",
		Confidence:         0.95,
		Rationale:          "Full market crash scenario: 30%+ decline, VIX > 40, foreign capital flight",
		Timestamp:          time.Now(),
	}

	structuralAssessment := &narrative.StructuralTrendAssessment{
		OverrideScore:      0.0,
		ShouldOverrideRisk: false,
		Rationale:          "No dominant structural trend detected during market crash",
		Timestamp:          time.Now(),
	}

	engine := risk.NewMacroAwareDrawdownEngine()
	decision := engine.Evaluate(macroAssessment, structuralAssessment)

	result.Results.DrawdownAction = decision.Action.String()
	result.Results.DrawdownPct = decision.Percentage
	result.Results.MaxExposure = decision.MaxExposure
	result.Results.Rationale = decision.Rationale

	result.Results.Breakdown = []Step{
		{
			Source:     "macro",
			Rule:       fmt.Sprintf("MacroRiskLevel=%s, OutflowProb=%.1f%%", macroAssessment.Level.String(), macroAssessment.ForeignOutflowProb),
			Action:     decision.Action.String(),
			Confidence: 1.0,
		},
		{
			Source:     "structural",
			Rule:       "no structural override (OverrideScore=0.0 < threshold)",
			Action:     "none",
			Confidence: 0.0,
		},
		{
			Source:     "var_calculator",
			Rule:       fmt.Sprintf("VaR95=%.4f, VaR99=%.4f, CVaR95=%.4f, MaxDrawdown=%.4f", snapshot.VaR95, snapshot.VaR99, snapshot.CVaR95, snapshot.MaxDrawdownPct),
			Action:     decision.Action.String(),
			Confidence: 1.0,
		},
	}

	gate := live.NewRiskGate(live.RiskGateConfig{
		MaxDailyLossPct:      0.03,
		VaRCriticalThreshold: 0.05,
	})

	gate.UpdateVaR(0.12)
	gate.UpdateDailyLoss(0.05)

	dummyOrder := domain.Order{
		Symbol:   "2330.TW",
		Side:     domain.SideBuy,
		Quantity: 1000,
	}
	err := gate.Check(context.Background(), dummyOrder)
	halted := err != nil
	result.Results.RiskGateHalt = halted

	passed := decision.Action >= risk.DrawdownSevere && varBreached && halted
	result.Passed = passed

	return result
}

// RunSectorRotation tests drawdown escalation and portfolio weight adjustments
// during simulated semiconductor recession and sector rotation flow.
func RunSectorRotation(cfg Config) Result {
	scCfg := getScenarioCfg(cfg, "sector_rotation")
	result := Result{
		Scenario:  "sector_rotation",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}

	macroAssessment := &narrative.MacroRiskAssessment{
		Level:              narrative.MacroRiskOrange,
		ForeignOutflowProb: 55.0,
		PrimaryFlow:        "sector_rotation",
		FavoredSectors:     []string{"defensive", "utilities", "consumer_staples"},
		AvoidedSectors:     []string{"semiconductor", "ai_supply_chain"},
		Confidence:         0.85,
		Rationale:          "Semiconductor cycle turning down, capital rotating to defensive sectors",
		Timestamp:          time.Now(),
	}

	structuralAssessment := &narrative.StructuralTrendAssessment{
		OverrideScore:      0.4,
		ShouldOverrideRisk: false,
		Rationale:          "AI trend weakening; not strong enough to override sector rotation risk",
		Timestamp:          time.Now(),
	}

	recessionPct := 0.40
	if scCfg.RecessionIndustryPct > 0 {
		recessionPct = scCfg.RecessionIndustryPct
	}
	totalIndustries := 10
	recessionCount := int(float64(totalIndustries) * recessionPct)
	expansionCount := 3

	industryAssessment := &risk.IndustryRiskAssessment{
		TotalIndustryCount:     totalIndustries,
		RecessionIndustryCount: recessionCount,
		ExpansionIndustryCount: expansionCount,
		WeightedCycleScore:     -0.35,
		TopRiskIndustries: []risk.IndustryRiskItem{
			{IndustryID: "semiconductor", PhaseScore: -0.80, Weight: 0.35},
			{IndustryID: "ai_supply_chain", PhaseScore: -0.65, Weight: 0.25},
			{IndustryID: "shipping", PhaseScore: -0.50, Weight: 0.10},
		},
		Timestamp: time.Now(),
	}

	engine := risk.NewMacroAwareDrawdownEngine()
	decision, breakdown := engine.EvaluateWithIndustry(macroAssessment, structuralAssessment, industryAssessment)

	result.Results.DrawdownAction = decision.Action.String()
	result.Results.DrawdownPct = decision.Percentage
	result.Results.MaxExposure = decision.MaxExposure
	result.Results.Rationale = decision.Rationale

	for _, s := range breakdown.Steps {
		result.Results.Breakdown = append(result.Results.Breakdown, Step{
			Source:     s.Source,
			Rule:       s.Rule,
			Action:     s.Action,
			Confidence: s.Confidence,
		})
	}

	cyclicalSymbols := []string{"2330.TW", "2454.TW", "3008.TW", "2308.TW"}
	defensiveSymbols := []string{"2412.TW", "4904.TW", "2881.TW", "2891.TW"}
	if len(scCfg.CyclicalSymbols) > 0 {
		cyclicalSymbols = scCfg.CyclicalSymbols
	}
	if len(scCfg.DefensiveSymbols) > 0 {
		defensiveSymbols = scCfg.DefensiveSymbols
	}

	adjuster := portfolio.NewPortfolioRiskAdjuster(engine)
	adjuster.SetCyclicalSymbols(cyclicalSymbols)
	adjuster.SetDefensiveSymbols(defensiveSymbols)

	weights := map[string]float64{
		"2330.TW": 0.25,
		"2454.TW": 0.15,
		"3008.TW": 0.10,
		"2308.TW": 0.10,
		"2412.TW": 0.15,
		"4904.TW": 0.10,
		"2881.TW": 0.10,
		"2891.TW": 0.05,
	}
	if len(scCfg.PortfolioWeights) > 0 {
		weights = scCfg.PortfolioWeights
	}

	adjustedWeights, reasons := adjuster.AdjustWeights(weights, decision.Action)

	for _, reason := range reasons {
		result.Results.WeightAdjustments = append(result.Results.WeightAdjustments, WeightAdjustment{
			Symbol: "",
			Before: 0,
			After:  0,
			Reason: reason,
		})
	}

	for sym, after := range adjustedWeights {
		before := weights[sym]
		if before != after {
			result.Results.WeightAdjustments = append(result.Results.WeightAdjustments, WeightAdjustment{
				Symbol: sym,
				Before: before,
				After:  after,
				Reason: fmt.Sprintf("adjusted from %.4f to %.4f by %s action", before, after, decision.Action.String()),
			})
		}
	}

	passed := decision.Action >= risk.DrawdownModerate

	for _, sym := range cyclicalSymbols {
		if after, ok := adjustedWeights[sym]; ok {
			if before := weights[sym]; after > before && decision.Action >= risk.DrawdownModerate {
				passed = false
			}
		}
	}

	for _, sym := range defensiveSymbols {
		if after, ok := adjustedWeights[sym]; ok {
			if before := weights[sym]; after < before && decision.Action <= risk.DrawdownModerate {
				passed = false
			}
		}
	}

	result.Passed = passed

	return result
}

// RunLiquidityCrisis tests VaR/CVaR/MaxDrawdown computation and portfolio risk
// escalation using synthetic high-volatility, fat-tailed returns data.
func RunLiquidityCrisis(cfg Config) Result {
	scCfg := getScenarioCfg(cfg, "liquidity_crisis")
	result := Result{
		Scenario:  "liquidity_crisis",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}

	volatilityBoost := 3.0
	if scCfg.VolatilityBoost > 0 {
		volatilityBoost = scCfg.VolatilityBoost
	}

	rng := rand.New(rand.NewSource(77))
	nDays := 90

	dailyReturns := make([]float64, nDays)
	portfolioValues := make([]float64, nDays)
	baseValue := 1_000_000.0
	portfolioValues[0] = baseValue

	for i := range nDays {
		if i == 0 {
			dailyReturns[i] = (rng.NormFloat64() * 0.01) + 0.0003
			continue
		}
		if i >= 20 && i < 35 {
			dailyReturns[i] = (rng.NormFloat64() * 0.06 * volatilityBoost) - 0.025
		} else {
			dailyReturns[i] = (rng.NormFloat64() * 0.02 * volatilityBoost) - 0.003
		}
		portfolioValues[i] = portfolioValues[i-1] * (1.0 + dailyReturns[i])
	}

	calc := risk.NewVaRCalculator()
	snapshot := calc.ComputeRiskSnapshot(dailyReturns, portfolioValues)
	result.Results.RiskSnapshot = &snapshot

	macroAssessment := &narrative.MacroRiskAssessment{
		Level:              narrative.MacroRiskOrange,
		ForeignOutflowProb: 70.0,
		PrimaryFlow:        "carry_trade_unwind",
		Confidence:         0.90,
		Rationale:          "Market liquidity drying up; bid-ask spreads widening, volume collapsing",
		Timestamp:          time.Now(),
	}

	structuralAssessment := &narrative.StructuralTrendAssessment{
		OverrideScore:      0.0,
		ShouldOverrideRisk: false,
		Rationale:          "No structural trend active during liquidity crisis",
		Timestamp:          time.Now(),
	}

	portfolioRiskScore := 0.85
	if scCfg.PortfolioRiskScore > 0 {
		portfolioRiskScore = scCfg.PortfolioRiskScore
	}

	mockProvider := &mockPortfolioRiskProvider{
		assessment: &risk.PortfolioRiskAssessment{
			ConcentrationScore: 0.75,
			SectorExposure: map[string]float64{
				"semiconductor": 0.45,
				"finance":       0.20,
			},
			FactorExposure: map[string]float64{
				"momentum": -0.60,
				"value":    0.30,
			},
			TotalRiskScore: portfolioRiskScore,
		},
	}

	engine := risk.NewMacroAwareDrawdownEngine()
	decision, breakdown := engine.EvaluateWithPortfolio(
		macroAssessment,
		structuralAssessment,
		nil,
		mockProvider,
	)

	result.Results.DrawdownAction = decision.Action.String()
	result.Results.DrawdownPct = decision.Percentage
	result.Results.MaxExposure = decision.MaxExposure
	result.Results.Rationale = decision.Rationale

	for _, s := range breakdown.Steps {
		result.Results.Breakdown = append(result.Results.Breakdown, Step{
			Source:     s.Source,
			Rule:       s.Rule,
			Action:     s.Action,
			Confidence: s.Confidence,
		})
	}

	varBreached := snapshot.VaR95 < -0.05 || snapshot.MaxDrawdownPct > 0.15
	result.Results.VaRBreach = varBreached

	gate := live.NewRiskGate(live.RiskGateConfig{
		MaxDailyLossPct:      0.03,
		VaRCriticalThreshold: 0.05,
	})
	gate.UpdateVaR(0.18)
	gate.UpdateDailyLoss(0.04)

	dummyOrder := domain.Order{
		Symbol:   "2330.TW",
		Side:     domain.SideBuy,
		Quantity: 500,
	}
	err := gate.Check(context.Background(), dummyOrder)
	result.Results.RiskGateHalt = err != nil

	passed := decision.Action >= risk.DrawdownSevere && varBreached && err != nil

	if snapshot.MaxDrawdownPct < 0.15 {
		passed = false
	}

	result.Passed = passed

	return result
}

// RunScenario runs a single named scenario. The second return value is false if
// name is not a known scenario.
func RunScenario(name string, cfg Config) (Result, bool) {
	switch name {
	case "market_crash":
		return RunMarketCrash(cfg), true
	case "sector_rotation":
		return RunSectorRotation(cfg), true
	case "liquidity_crisis":
		return RunLiquidityCrisis(cfg), true
	default:
		return Result{}, false
	}
}

// RunAll runs all three stress-test scenarios.
func RunAll(cfg Config) []Result {
	return []Result{RunMarketCrash(cfg), RunSectorRotation(cfg), RunLiquidityCrisis(cfg)}
}

// SummarizeResults builds an AllResults value from a slice of scenario results.
func SummarizeResults(results []Result) AllResults {
	all := AllResults{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Scenarios: results,
	}
	all.Summary.Total = len(results)
	for _, r := range results {
		if r.Passed {
			all.Summary.Passed++
		} else {
			all.Summary.Failed++
		}
	}
	return all
}

// mockPortfolioRiskProvider implements risk.PortfolioRiskProvider for testing.
type mockPortfolioRiskProvider struct {
	assessment *risk.PortfolioRiskAssessment
}

func (m *mockPortfolioRiskProvider) Assess() *risk.PortfolioRiskAssessment {
	return m.assessment
}
