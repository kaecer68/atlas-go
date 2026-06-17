package calibration

import (
	"fmt"
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
)

func UpdateParameterMetadata(cfg *config.ParametersConfig, p CalibratedParameter) error {
	pathParts := strings.Split(p.Path, ".")
	if len(pathParts) != 2 {
		return fmt.Errorf("invalid path format: %s", p.Path)
	}
	section, key := pathParts[0], pathParts[1]
	now := time.Now()

	switch section {
	case "garch":
		switch key {
		case "omega":
			cfg.GARCH.Omega.Source = config.SourceCalibrated
			cfg.GARCH.Omega.CalibrationMethod = p.Method
			cfg.GARCH.Omega.LastCalibrated = &now
		case "alpha":
			cfg.GARCH.Alpha.Source = config.SourceCalibrated
			cfg.GARCH.Alpha.CalibrationMethod = p.Method
			cfg.GARCH.Alpha.LastCalibrated = &now
		case "beta":
			cfg.GARCH.Beta.Source = config.SourceCalibrated
			cfg.GARCH.Beta.CalibrationMethod = p.Method
			cfg.GARCH.Beta.LastCalibrated = &now
		}
	case "sizing":
		switch key {
		case "target_volatility":
			cfg.Sizing.TargetVolatility.Source = config.SourceCalibrated
			cfg.Sizing.TargetVolatility.CalibrationMethod = p.Method
			cfg.Sizing.TargetVolatility.LastCalibrated = &now
		case "max_drawdown_limit":
			cfg.Sizing.MaxDrawdownLimit.Source = config.SourceCalibrated
			cfg.Sizing.MaxDrawdownLimit.CalibrationMethod = p.Method
			cfg.Sizing.MaxDrawdownLimit.LastCalibrated = &now
		}
	case "darwinian":
		switch key {
		case "hit_rate_high_threshold":
			cfg.Darwinian.HitRateHighThreshold.Source = config.SourceCalibrated
			cfg.Darwinian.HitRateHighThreshold.CalibrationMethod = p.Method
			cfg.Darwinian.HitRateHighThreshold.LastCalibrated = &now
		case "hit_rate_low_threshold":
			cfg.Darwinian.HitRateLowThreshold.Source = config.SourceCalibrated
			cfg.Darwinian.HitRateLowThreshold.CalibrationMethod = p.Method
			cfg.Darwinian.HitRateLowThreshold.LastCalibrated = &now
		}
	case "factor":
		switch key {
		case "momentum_stddev_divisor":
			cfg.Factor.MomentumStdDevDivisor.Source = config.SourceCalibrated
			cfg.Factor.MomentumStdDevDivisor.CalibrationMethod = p.Method
			cfg.Factor.MomentumStdDevDivisor.LastCalibrated = &now
		case "momentum_lookback_days":
			cfg.Factor.MomentumLookbackDays.Source = config.SourceCalibrated
			cfg.Factor.MomentumLookbackDays.CalibrationMethod = p.Method
			cfg.Factor.MomentumLookbackDays.LastCalibrated = &now
		}
	}
	return nil
}

func SaveResults(cfg *config.ParametersConfig, results []CalibrationResult, paramsPath string) error {
	cfg.UpdatedAt = time.Now()
	for _, res := range results {
		for _, p := range res.Parameters {
			if err := UpdateParameterMetadata(cfg, p); err != nil {
				return fmt.Errorf("update metadata for %s: %w", p.Path, err)
			}
		}
	}
	if err := cfg.LockedSaveWithRollback(paramsPath); err != nil {
		return fmt.Errorf("save parameters: %w", err)
	}
	return nil
}
