package industry

import (
	"sort"
	"sync"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// DynamicEnvModulator adjusts seasonal pattern factors based on real-world
// macro indicators (oil prices, USD strength, etc.), moving beyond pure
// calendar-driven seasonality into environment-aware adjustments.
type DynamicEnvModulator struct {
	mu         sync.RWMutex
	current    marketdata.MacroDataSnapshot
	baseline   marketdata.MacroDataSnapshot
	history    []marketdata.MacroDataSnapshot // rolling history
	windowDays int                            // default 90
}

// NewDynamicEnvModulator creates a modulator with the given baseline
// (typically a long-term average snapshot) and current snapshot.
func NewDynamicEnvModulator(baseline, current marketdata.MacroDataSnapshot) *DynamicEnvModulator {
	return &DynamicEnvModulator{
		baseline:   baseline,
		current:    current,
		windowDays: 90,
	}
}

// UpdateCurrent sets the latest macro snapshot.
func (dem *DynamicEnvModulator) UpdateCurrent(snap marketdata.MacroDataSnapshot) {
	dem.mu.Lock()
	dem.current = snap
	dem.mu.Unlock()
}

// UpdateBaseline sets the baseline (historical average) snapshot.
func (dem *DynamicEnvModulator) UpdateBaseline(snap marketdata.MacroDataSnapshot) {
	dem.mu.Lock()
	dem.baseline = snap
	dem.mu.Unlock()
}

// UpdateRollingBaseline recalculates baselines using a rolling window median.
// Call this periodically (e.g., daily) to adapt to market regime shifts.
func (dem *DynamicEnvModulator) UpdateRollingBaseline() {
	dem.mu.Lock()
	defer dem.mu.Unlock()

	if len(dem.history) == 0 {
		return
	}

	window := dem.windowDays
	if window <= 0 {
		window = 90
	}
	start := 0
	if len(dem.history) > window {
		start = len(dem.history) - window
	}
	windowData := dem.history[start:]

	oilValues := make([]float64, len(windowData))
	dxyValues := make([]float64, len(windowData))
	bdiValues := make([]float64, len(windowData))
	for i, snap := range windowData {
		oilValues[i] = snap.Oil.Value
		dxyValues[i] = snap.DXY.Value
		bdiValues[i] = snap.BDI.Value
	}

	dem.baseline.Oil.Value = median(oilValues)
	dem.baseline.DXY.Value = median(dxyValues)
	dem.baseline.BDI.Value = median(bdiValues)
}

// RecordSnapshot appends a macro data snapshot to the rolling history.
func (dem *DynamicEnvModulator) RecordSnapshot(snap marketdata.MacroDataSnapshot) {
	dem.mu.Lock()
	defer dem.mu.Unlock()
	dem.history = append(dem.history, snap)
	// Cap history at 2x window to prevent unbounded growth
	if dem.windowDays <= 0 {
		dem.windowDays = 90
	}
	maxHistory := dem.windowDays * 2
	if len(dem.history) > maxHistory {
		dem.history = dem.history[len(dem.history)-maxHistory:]
	}
}

// OilDeviation returns the percentage deviation of current oil price from baseline.
// Positive = oil is expensive relative to history.
func (dem *DynamicEnvModulator) OilDeviation() float64 {
	dem.mu.RLock()
	baselineOil := dem.baseline.Oil.Value
	currentOil := dem.current.Oil.Value
	dem.mu.RUnlock()
	if baselineOil <= 0 {
		return 0
	}
	return (currentOil - baselineOil) / baselineOil
}

// DXYDeviation returns the percentage deviation of DXY from baseline.
// Positive = USD is strong relative to history.
func (dem *DynamicEnvModulator) DXYDeviation() float64 {
	dem.mu.RLock()
	baselineDXY := dem.baseline.DXY.Value
	currentDXY := dem.current.DXY.Value
	dem.mu.RUnlock()
	if baselineDXY <= 0 {
		return 0
	}
	return (currentDXY - baselineDXY) / baselineDXY
}

// BDIDeviation returns the percentage deviation of BDI from baseline.
// Positive = shipping demand is strong relative to history.
func (dem *DynamicEnvModulator) BDIDeviation() float64 {
	dem.mu.RLock()
	baselineBDI := dem.baseline.BDI.Value
	currentBDI := dem.current.BDI.Value
	dem.mu.RUnlock()
	if baselineBDI <= 0 {
		return 0
	}
	return (currentBDI - baselineBDI) / baselineBDI
}

// SeasonalModulation computes a multiplicative adjustment for seasonal patterns
// based on current macro conditions, applied per-industry.
//
// Oil deviations affect energy and industrial sectors.
// DXY deviations affect export-heavy sectors (tech, semiconductor).
// BDI deviations affect shipping demand and downstream logistics costs.
// Returns a multiplier: 1.0 = no change, >1.0 = amplify seasonal effect.
func (dem *DynamicEnvModulator) SeasonalModulation(industryID string) float64 {
	cfg := config.GetParametersConfig().Industry.DynamicEnv.Value
	oilDev := dem.OilDeviation()
	dxyDev := dem.DXYDeviation()
	bdiDev := dem.BDIDeviation()

	switch industryID {
	case "energy":
		if oilDev > cfg.OilHighThreshold {
			return 1.0 + oilDev*cfg.OilEnergyMult
		}
		if oilDev < -cfg.OilHighThreshold {
			return 1.0 + oilDev*cfg.OilEnergyMult
		}
		return 1.0

	case "shipping":
		mod := 1.0
		if oilDev > cfg.OilHighThreshold {
			mod *= 1.0 - cfg.OilShippingPenalty
		}
		if oilDev < -cfg.OilLowThreshold {
			mod *= 1.0 + cfg.OilShippingBenefit
		}
		if bdiDev > cfg.BDIHighThreshold {
			mod *= 1.0 + bdiDev*cfg.BDIShippingBoost
		}
		if bdiDev < -cfg.BDIHighThreshold {
			mod *= 1.0 + bdiDev*cfg.BDIShippingBoost*0.5
		}
		return mod

	case "industrial", "petrochemicals", "steel":
		mod := 1.0
		if oilDev > cfg.OilHighThreshold {
			mod *= 1.0 - cfg.OilIndustrialPenalty
		}
		if oilDev < -cfg.OilLowThreshold {
			mod *= 1.0 + cfg.OilIndustrialBenefit
		}
		if bdiDev > cfg.BDIHighThreshold {
			mod *= 1.0 - cfg.BDICostPenalty
		}
		return mod

	case "semiconductor", "ai_supply_chain", "electronics":
		mod := 1.0
		if dxyDev > cfg.DXYHighThreshold {
			mod *= 1.0 - cfg.DXYExportPenalty
		}
		if dxyDev < -cfg.DXYLowThreshold {
			mod *= 1.0 + dxyDev*cfg.DXYExportBenefit
		}
		if bdiDev > cfg.BDIHighThreshold*2 {
			mod *= 1.0 - cfg.BDICostPenalty*0.5
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

// median returns the median value from a slice of float64.
func median(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := make([]float64, len(values))
	copy(sorted, values)
	sort.Float64s(sorted)
	n := len(sorted)
	if n%2 == 1 {
		return sorted[n/2]
	}
	return (sorted[n/2-1] + sorted[n/2]) / 2
}
