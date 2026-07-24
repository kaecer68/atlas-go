package calibration

// MarketRegime classifies market conditions based on VIX levels.
// Each regime corresponds to a range of market volatility that
// determines which factors should be emphasized in stress index computation.
type MarketRegime string

const (
	// RegimeBull represents low volatility (VIX < 15). In bull markets,
	// VIX and Geopolitical weights are boosted to improve black-swan early warning.
	RegimeBull MarketRegime = "bull"
	// RegimeNormal represents typical volatility (VIX 15–25). Standard
	// accuracy-based weights with no regime-specific adjustments.
	RegimeNormal MarketRegime = "normal"
	// RegimeBear represents elevated volatility (VIX 25–35). ForeignFlow
	// and US10Y weights are boosted for trend-following emphasis.
	RegimeBear MarketRegime = "bear"
	// RegimeCrisis represents extreme volatility (VIX >= 35). All weights
	// are flattened to equal (0.125 each) for comprehensive pressure monitoring.
	RegimeCrisis MarketRegime = "crisis"
)

// RegimeCalibratedConfig holds separate weight/scaling/threshold configurations
// for each market regime. When loaded into a TaiwanStressCalculator, it enables
// dynamic regime-aware stress index computation.
type RegimeCalibratedConfig struct {
	Bull   StressIndexWeightsConfig `json:"bull"`
	Normal StressIndexWeightsConfig `json:"normal"`
	Bear   StressIndexWeightsConfig `json:"bear"`
	Crisis StressIndexWeightsConfig `json:"crisis"`
}

// RegimeAwareCalibrator calibrates weights/scales/thresholds per market regime.
// It partitions historical records by VIX-based regime classification and generates
// regime-specific configurations with appropriate factor emphasis.
type RegimeAwareCalibrator struct {
	TargetMedianContribution float64 // default 20.0, passed through to ScaleCalibrator
}

// NewRegimeAwareCalibrator creates a calibrator with default target of 20.0.
func NewRegimeAwareCalibrator() *RegimeAwareCalibrator {
	return &RegimeAwareCalibrator{
		TargetMedianContribution: 20.0,
	}
}

// ClassifyRegime determines the market regime from VIX value.
//   - VIX < 15: Bull
//   - VIX [15, 25): Normal
//   - VIX [25, 35): Bear
//   - VIX >= 35: Crisis
func ClassifyRegime(vix float64) MarketRegime {
	if vix < 15 {
		return RegimeBull
	}
	if vix < 25 {
		return RegimeNormal
	}
	if vix < 35 {
		return RegimeBear
	}
	return RegimeCrisis
}

// DefaultRegimeConfig returns a RegimeCalibratedConfig where all four regimes
// use the compile-time default weights, scaling, and thresholds.
func DefaultRegimeConfig() *RegimeCalibratedConfig {
	defaultCfg := StressIndexWeightsConfig{
		Scaling: StressIndexScaling{
			DXY: StressScaleDXY, US10Y: StressScaleUS10Y,
			ForeignFlow: StressScaleForeignFlow, VIX: StressScaleVIX,
			JPY: StressScaleJPY, Geopolitical: StressScaleGeopolitical,
			Oil: StressScaleOil, Gold: StressScaleGold,
		},
		Weights: StressIndexWeights{
			DXY: StressWeightDXY, US10Y: StressWeightUS10Y,
			ForeignFlow: StressWeightForeignFlow, VIX: StressWeightVIX,
			JPY: StressWeightJPY, Geopolitical: StressWeightGeopolitical,
			Oil: StressWeightOil, Gold: StressWeightGold,
		},
		Thresholds: StressIndexThresholds{
			Crisis: StressThresholdCrisis,
			High:   StressThresholdHigh,
			Alert:  StressThresholdAlert,
		},
	}
	return &RegimeCalibratedConfig{
		Bull:   defaultCfg,
		Normal: defaultCfg,
		Bear:   defaultCfg,
		Crisis: defaultCfg,
	}
}

// CalibrateWeightsByRegime computes regime-aware weights from historical records.
// For each regime:
//  1. Filter records by VIX regime classification
//  2. Compute factor accuracy via WeightCalibrationEngine
//  3. Generate weights from accuracy (higher accuracy = higher weight, floor=0.05)
//  4. Bull: VIX+Geopolitical weights boosted 1.5x, then re-normalize
//  5. Bear: ForeignFlow+US10Y weights boosted 1.3x, then re-normalize
//  6. Crisis: all weights flattened to equal (0.125 each)
//  7. Normal: standard accuracy-based weights (no boost)
//
// Returns RegimeCalibratedConfig with all four regimes populated.
// If a regime has insufficient records (< 3), that regime uses DefaultRegimeConfig.
func (c *RegimeAwareCalibrator) CalibrateWeightsByRegime(records []CalibrationRecord) *RegimeCalibratedConfig {
	defaults := DefaultRegimeConfig()
	if len(records) == 0 {
		return defaults
	}

	engine := &WeightCalibrationEngine{}
	scaleCal := NewScaleCalibrator().WithTarget(c.TargetMedianContribution)

	bullRecords := filterByRegime(records, RegimeBull)
	normalRecords := filterByRegime(records, RegimeNormal)
	bearRecords := filterByRegime(records, RegimeBear)
	crisisRecords := filterByRegime(records, RegimeCrisis)

	cfg := &RegimeCalibratedConfig{}

	// Bull regime: boost VIX + Geopolitical for early warning
	if len(bullRecords) >= 3 {
		acc := engine.ComputeFactorAccuracy(bullRecords)
		weights := engine.CalibrateWeights(acc)
		boosted := boostWeights(weights, map[string]float64{"vix": 1.5, "geopolitical": 1.5})
		scaling := scaleCal.CalibrateScales(bullRecords)
		cfg.Bull = StressIndexWeightsConfig{
			Scaling:    scaling,
			Weights:    boosted,
			Thresholds: defaults.Bull.Thresholds,
		}
	} else {
		cfg.Bull = defaults.Bull
	}

	// Normal regime: standard accuracy-based weights
	if len(normalRecords) >= 3 {
		acc := engine.ComputeFactorAccuracy(normalRecords)
		weights := engine.CalibrateWeights(acc)
		scaling := scaleCal.CalibrateScales(normalRecords)
		cfg.Normal = StressIndexWeightsConfig{
			Scaling:    scaling,
			Weights:    weights,
			Thresholds: defaults.Normal.Thresholds,
		}
	} else {
		cfg.Normal = defaults.Normal
	}

	// Bear regime: boost ForeignFlow + US10Y for trend following
	if len(bearRecords) >= 3 {
		acc := engine.ComputeFactorAccuracy(bearRecords)
		weights := engine.CalibrateWeights(acc)
		boosted := boostWeights(weights, map[string]float64{"foreign_flow": 1.3, "us10y": 1.3})
		scaling := scaleCal.CalibrateScales(bearRecords)
		cfg.Bear = StressIndexWeightsConfig{
			Scaling:    scaling,
			Weights:    boosted,
			Thresholds: defaults.Bear.Thresholds,
		}
	} else {
		cfg.Bear = defaults.Bear
	}

	// Crisis regime: flatten all weights to equal
	if len(crisisRecords) >= 3 {
		scaling := scaleCal.CalibrateScales(crisisRecords)
		cfg.Crisis = StressIndexWeightsConfig{
			Scaling: scaling,
			Weights: equalWeights(),
			Thresholds: StressIndexThresholds{
				Crisis: defaults.Crisis.Thresholds.Crisis,
				High:   defaults.Crisis.Thresholds.High,
				Alert:  defaults.Crisis.Thresholds.Alert,
			},
		}
	} else {
		cfg.Crisis = defaults.Crisis
	}

	return cfg
}

// SelectConfig returns the appropriate StressIndexWeightsConfig for a given regime.
// Falls back to Normal if the regime is unrecognized or the config is nil.
func (rc *RegimeCalibratedConfig) SelectConfig(regime MarketRegime) StressIndexWeightsConfig {
	if rc == nil {
		return DefaultRegimeConfig().Normal
	}
	switch regime {
	case RegimeBull:
		return rc.Bull
	case RegimeNormal:
		return rc.Normal
	case RegimeBear:
		return rc.Bear
	case RegimeCrisis:
		return rc.Crisis
	default:
		return rc.Normal
	}
}

// boostWeights boosts specified factors by a multiplier and re-normalizes.
// Used for Bull (VIX+Geo×1.5) and Bear (Flow+US10Y×1.3).
// Approach:
//  1. Copy input weights
//  2. Multiply specified factors by boost factor
//  3. Compute new sum
//  4. Divide each weight by new sum to normalize to 1.0
func boostWeights(weights StressIndexWeights, boosts map[string]float64) StressIndexWeights {
	// Apply boosts to specified factors
	w := weights
	if m, ok := boosts["dxy"]; ok {
		w.DXY *= m
	}
	if m, ok := boosts["us10y"]; ok {
		w.US10Y *= m
	}
	if m, ok := boosts["foreign_flow"]; ok {
		w.ForeignFlow *= m
	}
	if m, ok := boosts["vix"]; ok {
		w.VIX *= m
	}
	if m, ok := boosts["jpy"]; ok {
		w.JPY *= m
	}
	if m, ok := boosts["geopolitical"]; ok {
		w.Geopolitical *= m
	}
	if m, ok := boosts["oil"]; ok {
		w.Oil *= m
	}
	if m, ok := boosts["gold"]; ok {
		w.Gold *= m
	}

	// Re-normalize to sum=1.0
	return normalizeWeights(w)
}

// equalWeights returns weights where all 8 factors have equal weight (0.125).
func equalWeights() StressIndexWeights {
	return StressIndexWeights{
		DXY: 0.125, US10Y: 0.125, ForeignFlow: 0.125, VIX: 0.125,
		JPY: 0.125, Geopolitical: 0.125, Oil: 0.125, Gold: 0.125,
	}
}

// RegimeCorrelation holds the 4-tier regime-dependent correlation coefficients
// between US and Taiwan equity markets, as empirically observed in 2024-2025 data.
// These replace any hardcoded constant-correlation assumptions in cross-market
// risk models with empirically grounded regime-switching values.
type RegimeCorrelation struct {
	// Calm market (VIX < 15): ρ ≈ 0.35-0.40 — normal trading, no systemic shock
	Calm float64 `json:"calm"`
	// AI-driven positive cycle (VIX 15-25, SOX/NVDA trending up): ρ ≈ 0.55-0.70
	// TSMC heavily benefits from AI capex, creating concentrated US-TW linkage
	AIBoom float64 `json:"ai_boom"`
	// Systemic stress (VIX 25-35, risk-off): ρ ≈ 0.80-0.93
	// Foreign capital accelerates withdrawal, correlations surge globally
	SystemicStress float64 `json:"systemic_stress"`
	// Tariff/geopolitical shock (VIX ≥ 35, Taiwan-specific policy risk): ρ ≈ 0.85-0.95
	// Direct trade-policy impact on Taiwan creates near-perfect correlation
	TariffGeopolitical float64 `json:"tariff_geopolitical"`
}

// DefaultRegimeCorrelation returns empirically calibrated regime correlation
// coefficients based on the 2024-2025 US-TW equity market study.
// Values are midpoints of the observed ranges:
//
//	Calm:             0.375  (observed range 0.35–0.40)
//	AIBoom:           0.625  (observed range 0.55–0.70)
//	SystemicStress:   0.865  (observed range 0.80–0.93)
//	TariffGeopolitical: 0.90  (observed range 0.85–0.95)
func DefaultRegimeCorrelation() RegimeCorrelation {
	return RegimeCorrelation{
		Calm:               0.375,
		AIBoom:             0.625,
		SystemicStress:     0.865,
		TariffGeopolitical: 0.90,
	}
}

// GetRegimeCorrelation returns the appropriate correlation coefficient for the
// current VIX level. The mapping uses the same VIX thresholds as ClassifyRegime:
//
//	VIX < 15          → Calm (0.375)
//	VIX [15, 25)      → AIBoom (0.625)
//	VIX [25, 35)      → SystemicStress (0.865)
//	VIX ≥ 35           → TariffGeopolitical (0.90)
//
// An optional aiBoomOverride flag can force AIBoom regime when VIX levels are
// normal but AI/tech sentiment is driving concentrated US-TW linkage (e.g.,
// NVDA earnings season, TSMC capex announcements).
func GetRegimeCorrelation(vix float64, aiBoomOverride bool) float64 {
	rc := DefaultRegimeCorrelation()
	regime := ClassifyRegime(vix)
	switch regime {
	case RegimeBull:
		if aiBoomOverride {
			return rc.AIBoom
		}
		return rc.Calm
	case RegimeNormal:
		if aiBoomOverride {
			return rc.AIBoom
		}
		return rc.AIBoom // Normal VIX + AI cycle → AIBoom regime
	case RegimeBear:
		return rc.SystemicStress
	case RegimeCrisis:
		return rc.TariffGeopolitical
	default:
		return rc.AIBoom
	}
}

// filterByRegime filters calibration records to only those whose VIX value
// falls within the specified regime's range.
func filterByRegime(records []CalibrationRecord, regime MarketRegime) []CalibrationRecord {
	filtered := make([]CalibrationRecord, 0)
	for _, r := range records {
		if ClassifyRegime(r.Snapshot.VIX.Value) == regime {
			filtered = append(filtered, r)
		}
	}
	return filtered
}
