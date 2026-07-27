package portfolio

import (
	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/retail"
)

type MacroDataSnapshot = marketdata.MacroDataSnapshot

// FactorBridgeInput holds standardized macro data inputs for factor calculation.
type FactorBridgeInput struct {
	ForeignFlowScore     float64 // Standardized foreign investor net flow [-1, 1]
	DomesticFlowScore    float64 // Standardized domestic fund net flow [-1, 1]
	MarginBalanceScore   float64 // Standardized margin balance ratio [-1, 1]
	RetailSentimentScore float64 // Standardized retail sentiment [-1, 1]
	StressLevel          float64 // Market stress level [0, 100]
}

// FactorBridge converts raw MacroDataSnapshot into standardized factor inputs.
type FactorBridge struct {
	foreignFlowAvg    float64
	foreignFlowStd    float64
	marginAvg         float64
	marginStd         float64
	calculator        *retail.Calculator // optional RSI-tw calculator for retail sentiment
	forceRetailZScore float64            // C6 P1: ForceRetail Z-score from capitalflow (0 = not set)
	stressIndex       *StressIndex       // config-driven Taiwan Stress Index
}

// NewFactorBridge creates a FactorBridge with default calibration values.
func NewFactorBridge() *FactorBridge {
	return &FactorBridge{
		foreignFlowAvg: 0,
		foreignFlowStd: 50e8, // 50 billion TWD standard deviation
		marginAvg:      0,
		marginStd:      50e8,
		stressIndex:    NewStressIndexFromConfig(DefaultStressIndexConfig()),
	}
}

// SetCalculator attaches an RSI-tw Calculator for real retail sentiment computation.
// When nil (default), computeRetailSentiment falls back to naive margin change % logic.
func (fb *FactorBridge) SetCalculator(c *retail.Calculator) {
	fb.calculator = c
}

// SetForceRetailZScore sets the capitalflow ForceRetail Z-score for retail
// sentiment computation (C6 P1). When non-zero, it takes priority over
// the RSI-tw Calculator and the margin-change fallback. This unifies
// the retail reverse indicator across the capitalflow and portfolio modules.
func (fb *FactorBridge) SetForceRetailZScore(z float64) {
	fb.forceRetailZScore = z
}

// Convert transforms a MacroDataSnapshot into a FactorBridgeInput.
func (fb *FactorBridge) Convert(snap MacroDataSnapshot) FactorBridgeInput {
	input := FactorBridgeInput{}

	input.ForeignFlowScore = fb.standardize(snap.ForeignInvestorNet.Value, fb.foreignFlowAvg, fb.foreignFlowStd)
	input.DomesticFlowScore = fb.standardize(snap.DomesticFundNet.Value, 0, fb.marginStd)
	input.MarginBalanceScore = fb.standardize(snap.RetailMarginBalance.Value, fb.marginAvg, fb.marginStd)
	input.RetailSentimentScore = fb.computeRetailSentiment(snap)
	input.StressLevel = fb.computeStressLevel(snap)

	return input
}

// standardize converts a raw value to a z-score bounded in [-1, 1].
func (fb *FactorBridge) standardize(value, avg, std float64) float64 {
	if std == 0 {
		return 0
	}
	z := (value - avg) / std
	if z > 1 {
		return 1
	}
	if z < -1 {
		return -1
	}
	return z
}

// computeRetailSentiment derives retail sentiment from:
//  1. capitalflow ForceRetail Z-score (C6 P1) — canonical, unified source
//  2. attached RSI-tw Calculator (legacy)
//  3. naive margin balance change percentage (fallback)
//
// Returns a value in [-1, 1] where positive = retail bullish (contrarian:
// high retail bullishness is a sell signal, handled by negative weight in
// the institutional sentiment formula).
func (fb *FactorBridge) computeRetailSentiment(snap MacroDataSnapshot) float64 {
	// C6 P1: capitalflow ForceRetail Z-score (range ~[-3, 3]) as canonical source.
	if fb.forceRetailZScore != 0 {
		// Normalize Z-score to [-1, 1]; clamp at outlier boundaries.
		normalized := fb.forceRetailZScore / 3.0
		if normalized > 1.0 {
			normalized = 1.0
		}
		if normalized < -1.0 {
			normalized = -1.0
		}
		return normalized
	}

	if fb.calculator != nil {
		input := retail.RSITwInput{
			VIXLevel:           snap.VIX.Value,
			MarginBalance:      snap.RetailMarginBalance.Value,
			ForeignInvestorNet: snap.ForeignInvestorNet.Value,
			DomesticFundNet:    snap.DomesticFundNet.Value,
			MarginPercentile:   0, // unavailable from MacroDataSnapshot; calculator falls back gracefully
		}
		snap_ := fb.calculator.ComputeFinal(input)
		return snap_.Score
	}

	// Fallback: naive retail sentiment based on margin balance change percentage
	change := snap.RetailMarginBalance.ChangePct
	if change > 10 {
		return 1.0 // Extremely bullish retail
	}
	if change < -10 {
		return -1.0 // Extremely bearish retail
	}
	return change / 10.0 // Linear scale [-1, 1]
}

// computeStressLevel calculates market stress from VIX, DXY, and rate indicators.
// Delegates to the config-driven StressIndex for indicator contributions.
func (fb *FactorBridge) computeStressLevel(snap MacroDataSnapshot) float64 {
	return fb.stressIndex.ComputeStressLevel(snap)
}
