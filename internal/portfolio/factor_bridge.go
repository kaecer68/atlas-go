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
	foreignFlowAvg float64
	foreignFlowStd float64
	marginAvg      float64
	marginStd      float64
	calculator     *retail.Calculator // optional RSI-tw calculator for retail sentiment
}

// NewFactorBridge creates a FactorBridge with default calibration values.
func NewFactorBridge() *FactorBridge {
	return &FactorBridge{
		foreignFlowAvg: 0,
		foreignFlowStd: 50e8, // 50 billion TWD standard deviation
		marginAvg:      0,
		marginStd:      50e8,
	}
}

// SetCalculator attaches an RSI-tw Calculator for real retail sentiment computation.
// When nil (default), computeRetailSentiment falls back to naive margin change % logic.
func (fb *FactorBridge) SetCalculator(c *retail.Calculator) {
	fb.calculator = c
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

// computeRetailSentiment derives retail sentiment from margin balance changes,
// or from the attached RSI-tw Calculator when available.
func (fb *FactorBridge) computeRetailSentiment(snap MacroDataSnapshot) float64 {
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
func (fb *FactorBridge) computeStressLevel(snap MacroDataSnapshot) float64 {
	stress := 0.0

	// VIX contribution (0-40 points)
	vix := snap.VIX.Value
	if vix > 30 {
		stress += 40
	} else if vix > 20 {
		stress += (vix - 20) * 4
	}

	// DXY contribution (0-30 points) - USD strengthening = stress
	dxy := snap.DXY.Value
	if dxy > 105 {
		stress += 30
	} else if dxy > 100 {
		stress += (dxy - 100) * 6
	}

	// Rate spread contribution (0-30 points)
	us10y := snap.US10Y.Value
	if us10y > 4.5 {
		stress += 30
	} else if us10y > 3.5 {
		stress += (us10y - 3.5) * 30
	}

	// Normalize to [0, 100]
	if stress > 100 {
		stress = 100
	}
	return stress
}
