package calibration

import "github.com/kaecer68/atlas-go/internal/config"

// LoadWeightsConfig reads stress index weights from the centralized parameters system.
// Deprecated: Use config.GetParametersConfig().Narrative for authoritative stress index parameters.
func LoadWeightsConfig(workDir string) *StressIndexWeightsConfig {
	return loadWeightsFromParameters()
}

func loadWeightsFromParameters() *StressIndexWeightsConfig {
	p := config.GetParametersConfig()
	if p == nil {
		return nil
	}
	n := p.Narrative
	cfg := &StressIndexWeightsConfig{
		Scaling: StressIndexScaling{
			DXY:          n.TaiwanStressDXYScale.Value,
			US10Y:        n.TaiwanStressUS10YScale.Value,
			ForeignFlow:  n.TaiwanStressForeignScale.Value,
			VIX:          n.TaiwanStressVIXScale.Value,
			JPY:          n.TaiwanStressJPYScale.Value,
			Geopolitical: n.TaiwanStressGeoScale.Value,
			Oil:          n.TaiwanStressOilScale.Value,
			Gold:         n.TaiwanStressGoldScale.Value,
		},
		Weights: StressIndexWeights{
			DXY:          n.TaiwanStressDXYWeight.Value,
			US10Y:        n.TaiwanStressUS10YWeight.Value,
			ForeignFlow:  n.TaiwanStressForeignWeight.Value,
			VIX:          n.TaiwanStressVIXWeight.Value,
			JPY:          n.TaiwanStressJPYWeight.Value,
			Geopolitical: n.TaiwanStressGeoWeight.Value,
			Oil:          n.TaiwanStressOilWeight.Value,
			Gold:         n.TaiwanStressGoldWeight.Value,
		},
		Thresholds: StressIndexThresholds{
			Crisis: n.TaiwanStressCrisisThreshold.Value,
			High:   n.TaiwanStressHighThreshold.Value,
			Alert:  n.TaiwanStressAlertThreshold.Value,
		},
	}
	if !cfg.IsValid() {
		return nil
	}
	return cfg
}
