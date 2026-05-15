package industry

import (
	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// DynamicEnvModulator adjusts seasonal pattern factors based on real-world
// macro indicators (oil prices, USD strength, etc.), moving beyond pure
// calendar-driven seasonality into environment-aware adjustments.
type DynamicEnvModulator struct {
	current  marketdata.MacroDataSnapshot
	baseline marketdata.MacroDataSnapshot
}

// NewDynamicEnvModulator creates a modulator with the given baseline
// (typically a long-term average snapshot) and current snapshot.
func NewDynamicEnvModulator(baseline, current marketdata.MacroDataSnapshot) *DynamicEnvModulator {
	return &DynamicEnvModulator{baseline: baseline, current: current}
}

// UpdateCurrent sets the latest macro snapshot.
func (dem *DynamicEnvModulator) UpdateCurrent(snap marketdata.MacroDataSnapshot) {
	dem.current = snap
}

// UpdateBaseline sets the baseline (historical average) snapshot.
func (dem *DynamicEnvModulator) UpdateBaseline(snap marketdata.MacroDataSnapshot) {
	dem.baseline = snap
}

// OilDeviation returns the percentage deviation of current oil price from baseline.
// Positive = oil is expensive relative to history.
func (dem *DynamicEnvModulator) OilDeviation() float64 {
	if dem.baseline.Oil.Value <= 0 {
		return 0
	}
	return (dem.current.Oil.Value - dem.baseline.Oil.Value) / dem.baseline.Oil.Value
}

// DXYDeviation returns the percentage deviation of DXY from baseline.
// Positive = USD is strong relative to history.
func (dem *DynamicEnvModulator) DXYDeviation() float64 {
	if dem.baseline.DXY.Value <= 0 {
		return 0
	}
	return (dem.current.DXY.Value - dem.baseline.DXY.Value) / dem.baseline.DXY.Value
}

// BDIDeviation returns the percentage deviation of BDI from baseline.
// Positive = shipping demand is strong relative to history.
func (dem *DynamicEnvModulator) BDIDeviation() float64 {
	if dem.baseline.BDI.Value <= 0 {
		return 0
	}
	return (dem.current.BDI.Value - dem.baseline.BDI.Value) / dem.baseline.BDI.Value
}

// SeasonalModulation computes a multiplicative adjustment for seasonal patterns
// based on current macro conditions, applied per-industry.
//
// Oil deviations affect energy and industrial sectors.
// DXY deviations affect export-heavy sectors (tech, semiconductor).
// BDI deviations affect shipping demand and downstream logistics costs.
// Returns a multiplier: 1.0 = no change, >1.0 = amplify seasonal effect.
func (dem *DynamicEnvModulator) SeasonalModulation(industryID string) float64 {
	oilDev := dem.OilDeviation()
	dxyDev := dem.DXYDeviation()
	bdiDev := dem.BDIDeviation()

	switch industryID {
	case "energy":
		if oilDev > 0.10 {
			return 1.0 + oilDev*0.5
		}
		if oilDev < -0.10 {
			return 1.0 + oilDev*0.5
		}
		return 1.0

	case "shipping":
		mod := 1.0
		if oilDev > 0.15 {
			mod *= 0.95
		}
		if oilDev < -0.10 {
			mod *= 1.05
		}
		// BDI above baseline = strong shipping demand = amplify shipping sector
		if bdiDev > 0.10 {
			mod *= 1.0 + bdiDev*0.3
		}
		if bdiDev < -0.10 {
			mod *= 1.0 + bdiDev*0.2
		}
		return mod

	case "industrial", "petrochemicals", "steel":
		mod := 1.0
		if oilDev > 0.15 {
			mod *= 0.94
		}
		if oilDev < -0.10 {
			mod *= 1.04
		}
		// BDI above baseline = higher input transport costs = negative for manufacturing
		if bdiDev > 0.15 {
			mod *= 0.96
		}
		return mod

	case "semiconductor", "ai_supply_chain", "electronics":
		mod := 1.0
		if dxyDev > 0.05 {
			mod *= 0.95
		}
		if dxyDev < -0.03 {
			mod *= 1.04
		}
		// BDI above baseline = higher logistics costs for export-oriented sectors
		if bdiDev > 0.20 {
			mod *= 0.97
		}
		return mod

	default:
		return 1.0
	}
}

// IsOilElevated returns true when oil is significantly above baseline.
func (dem *DynamicEnvModulator) IsOilElevated(threshold float64) bool {
	return dem.OilDeviation() > threshold
}

// IsDollarStrong returns true when DXY is significantly above baseline.
func (dem *DynamicEnvModulator) IsDollarStrong(threshold float64) bool {
	return dem.DXYDeviation() > threshold
}
