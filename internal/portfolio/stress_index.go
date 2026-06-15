package portfolio

import "github.com/kaecer68/atlas-go/internal/marketdata"

// StressIndicator computes a contribution to the Taiwan Stress Index (0-100)
// from a single macro indicator (e.g., VIX, DXY, US10Y).
type StressIndicator interface {
	// Name returns a human-readable identifier for this indicator.
	Name() string

	// Contribution returns the stress contribution [0, maxContribution]
	// for the given macro snapshot.
	Contribution(snap marketdata.MacroDataSnapshot) float64
}

// StressIndexConfig holds the configuration for the Taiwan Stress Index.
type StressIndexConfig struct {
	Indicators []StressIndicator
}

// StressIndex computes the Taiwan Stress Index from registered indicators.
type StressIndex struct {
	indicators []StressIndicator
}

// NewStressIndexFromConfig creates a StressIndex from the given configuration.
func NewStressIndexFromConfig(cfg StressIndexConfig) *StressIndex {
	return &StressIndex{indicators: cfg.Indicators}
}

// ComputeStressLevel calculates the market stress level [0, 100] by summing
// contributions from all registered indicators and capping at 100.
func (si *StressIndex) ComputeStressLevel(snap marketdata.MacroDataSnapshot) float64 {
	stress := 0.0
	for _, ind := range si.indicators {
		stress += ind.Contribution(snap)
	}
	if stress > 100 {
		stress = 100
	}
	return stress
}

// DefaultStressIndexConfig returns the built-in configuration that exactly
// replicates the legacy hardcoded computeStressLevel behavior.
func DefaultStressIndexConfig() StressIndexConfig {
	return StressIndexConfig{
		Indicators: []StressIndicator{
			&vixStressIndicator{},
			&dxyStressIndicator{},
			&us10yStressIndicator{},
		},
	}
}

// --- Built-in StressIndicator implementations ---

// vixStressIndicator computes VIX contribution (0-40 points).
// Legacy: VIX > 30 → 40, VIX > 20 → (VIX-20)*4, else 0.
type vixStressIndicator struct{}

func (v *vixStressIndicator) Name() string { return "VIX" }

func (v *vixStressIndicator) Contribution(snap marketdata.MacroDataSnapshot) float64 {
	vix := snap.VIX.Value
	if vix > 30 {
		return 40
	}
	if vix > 20 {
		return (vix - 20) * 4
	}
	return 0
}

// dxyStressIndicator computes DXY contribution (0-30 points).
// Legacy: DXY > 105 → 30, DXY > 100 → (DXY-100)*6, else 0.
type dxyStressIndicator struct{}

func (d *dxyStressIndicator) Name() string { return "DXY" }

func (d *dxyStressIndicator) Contribution(snap marketdata.MacroDataSnapshot) float64 {
	dxy := snap.DXY.Value
	if dxy > 105 {
		return 30
	}
	if dxy > 100 {
		return (dxy - 100) * 6
	}
	return 0
}

// us10yStressIndicator computes US10Y contribution (0-30 points).
// Legacy: US10Y > 4.5 → 30, US10Y > 3.5 → (US10Y-3.5)*30, else 0.
type us10yStressIndicator struct{}

func (u *us10yStressIndicator) Name() string { return "US10Y" }

func (u *us10yStressIndicator) Contribution(snap marketdata.MacroDataSnapshot) float64 {
	us10y := snap.US10Y.Value
	if us10y > 4.5 {
		return 30
	}
	if us10y > 3.5 {
		return (us10y - 3.5) * 30
	}
	return 0
}
