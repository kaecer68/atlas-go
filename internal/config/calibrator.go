package config

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/kaecer68/atlas-go/internal/logging"
)

// CalibratorResult records the outcome of a single parameter calibration run.
// Mirrors the pattern established by risk.CalibrationReport and portfolio.WeightCalibrationReport.
type CalibratorResult struct {
	Timestamp      time.Time          `json:"timestamp"`
	ParamCount     int                `json:"param_count"`
	Changes        []CalibratorChange `json:"changes"`
	BaselineScore  float64            `json:"baseline_score"`
	OptimizedScore float64            `json:"optimized_score"`
	Verdict        string             `json:"verdict"`
	Summary        string             `json:"summary"`
}

type CalibratorChange struct {
	ParamName  string  `json:"param_name"`
	Before     float64 `json:"before"`
	After      float64 `json:"after"`
	DeltaPct   float64 `json:"delta_pct"`
	Confidence string  `json:"confidence"`
}

// ParameterCalibrator defines the interface for any parameter-domain calibrator.
// Implementations provide parameter names and may optionally supply tight search
// bounds via BoundedCalibrator to prevent the Bayesian optimizer from exploring
// regions outside the evaluator's sensitive range.
//
// Usage mirrors RiskGate.SelfCalibrate and FactorWeightCalibrator.CalibrateWeights:
//
//	calibrator := &MacroRiskCalibrator{}
//	result, err := CalibrateParameters(ctx, calibrator, evaluator)
type ParameterCalibrator interface {
	ParamNames() []string
}

// BoundedCalibrator is an optional extension of ParameterCalibrator that
// supplies per-parameter search bounds to the Bayesian optimizer.
// CalibrateParameters checks for this interface via type assertion and
// uses the supplied bounds instead of auto-derived [val*0.3, val*3.0].
type BoundedCalibrator interface {
	ParameterCalibrator
	ParamBounds() map[string][2]float64
}

// CalibrateConfig controls the Bayesian optimization behavior.
type CalibrateConfig struct {
	InitialPoints  int
	Iterations     int
	MinImprovement float64
}

func DefaultCalibrateConfig() CalibrateConfig {
	return CalibrateConfig{
		InitialPoints:  6,
		Iterations:     10,
		MinImprovement: 0.02,
	}
}

// CalibrateParameters runs Bayesian optimization on the given calibrator's
// parameter set using the provided evaluator, then persists results to the
// ParametersConfig singleton and marks each affected parameter as calibrated.
//
// Returns CalibratorResult with before/after values, confidence levels, and a verdict.
func CalibrateParameters(ctx context.Context, calibrator ParameterCalibrator, evaluator func(cfg *ParametersConfig) (float64, error), cfg CalibrateConfig) (*CalibratorResult, error) {
	paramNames := calibrator.ParamNames()
	if len(paramNames) == 0 {
		return nil, fmt.Errorf("calibrate: no parameter names provided")
	}

	// Check context before starting computation-intensive optimization.
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	params := GetParametersConfig()
	ie := NewInferenceEngine(params)

	baseline, err := evaluator(params)
	if err != nil {
		baseline = 0
	}

	optCfg := DefaultOptimizerConfig()
	optCfg.InitialPoints = cfg.InitialPoints
	optCfg.Iterations = cfg.Iterations

	var result OptimizeResult
	if bc, ok := calibrator.(BoundedCalibrator); ok {
		boundsMap := bc.ParamBounds()
		bounds := make([][2]float64, len(paramNames))
		for i, name := range paramNames {
			if b, hasBounds := boundsMap[name]; hasBounds {
				bounds[i] = b
			} else {
				val, _ := ie.GetParameter(name)
				if val == 0 {
					bounds[i] = [2]float64{0.01, 1.0}
				} else {
					bounds[i] = [2]float64{val * 0.3, val * 3.0}
				}
			}
		}
		wrapped := func(x []float64) (float64, error) {
			testCfg := ie.cloneParams()
			for i, name := range paramNames {
				if err := ie.setParameterOnConfig(testCfg, name, x[i]); err != nil {
					return 0, err
				}
			}
			return evaluator(testCfg)
		}
		opt := NewBayesianOptimizer(bounds, wrapped, optCfg)
		optResult, optErr := opt.Optimize()
		if optErr != nil {
			return nil, fmt.Errorf("calibrate: optimize: %w", optErr)
		}
		optResult.ParamValues = make(map[string]float64)
		for i, name := range paramNames {
			if i < len(optResult.BestX) {
				optResult.ParamValues[name] = optResult.BestX[i]
			}
		}
		result = optResult
	} else {
		result, err = ie.OptimizeBayesian(paramNames, evaluator, optCfg)
		if err != nil {
			return nil, fmt.Errorf("calibrate: optimize: %w", err)
		}
	}

	// Check context again after potentially long optimization.
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	optScore := result.BestScore
	improvement := computeImprovementPct(baseline, optScore)

	report := &CalibratorResult{
		Timestamp:      time.Now(),
		ParamCount:     len(paramNames),
		BaselineScore:  baseline,
		OptimizedScore: optScore,
	}

	var changes []CalibratorChange
	appliedCount := 0
	failedCount := 0

	for _, name := range paramNames {
		current, ok := ie.GetParameter(name)
		if !ok {
			continue
		}
		best, hasKey := result.ParamValues[name]
		if !hasKey {
			best = current
		}

		deltaPct := (best - current) / math.Abs(current+1e-10) * 100
		if math.Abs(deltaPct) < cfg.MinImprovement*100 {
			continue
		}

		conf := calibrationConfidence(deltaPct, result.Observations)
		// Apply first, report second (issue #1944 Batch 2, E-item): the change
		// used to be appended and counted even when SetParameter failed, so the
		// report claimed `applied N/M parameter changes` and Verdict=calibrated
		// for parameters that were never written.
		if err := ie.SetParameter(name, best); err != nil {
			logging.Error("calibrator", "set_parameter_failed",
				logging.FStr("param", name), logging.Err(err))
			failedCount++
			continue
		}
		changes = append(changes, CalibratorChange{
			ParamName: name, Before: current, After: best,
			DeltaPct: deltaPct, Confidence: conf,
		})
		appliedCount++
	}

	if appliedCount > 0 {
		now := time.Now()
		markCalibrated(params, paramNames, "bayesian_optimization", &now)
		persistCalibratorChanges(changes, now)
		report.Verdict = "calibrated"
		report.Summary = fmt.Sprintf("applied %d/%d parameter changes (baseline=%.4f → optimized=%.4f, %+.1f%%, %d set_parameter failures)",
			appliedCount, len(paramNames), baseline, optScore, improvement, failedCount)
	} else if failedCount > 0 {
		// Nothing was written: do not report a benign "stable"/"unchanged" verdict.
		report.Verdict = "failed"
		report.Summary = fmt.Sprintf("0/%d parameter changes applied (%d set_parameter failures)",
			len(paramNames), failedCount)
	} else if improvement > 0 {
		report.Verdict = "stable"
		report.Summary = fmt.Sprintf("no significant changes (improvement=%+.1f%% below threshold)", improvement)
	} else {
		report.Verdict = "unchanged"
		report.Summary = fmt.Sprintf("current values optimal (baseline=%.4f)", baseline)
	}

	report.Changes = changes
	return report, nil
}

// computeImprovementPct returns the signed improvement percentage from baseline
// to optimized score. Handles negative/zero baselines gracefully:
//   - baseline > 0: (opt - baseline) / baseline * 100
//   - baseline < 0: (opt - baseline) / (-baseline) * 100 (relative to |baseline|)
//   - baseline ≈ 0: returns (opt - baseline) * 100 (absolute change as pseudo-%)
//
// calibratorOverlaySource identifies this writer in the overlay document.
const calibratorOverlaySource = "calibrate_parameters"

// persistCalibratorChanges writes the accepted changes to the
// calibrated-parameters overlay (FU-20260926-07) instead of rewriting
// configs/parameters.json.
//
// The SSOT file is deliberately left untouched: configs/ is not bind-mounted, so
// a write there landed in the container's writable layer (invisible to git, lost
// on the next container recreate). The adaptation now lands in the overlay under
// the bind-mounted data/ tree and is layered back on top of the SSOT at load
// time; each entry records the SSOT value it was computed from, so a later
// reviewed charter edit invalidates a stale adaptation.
//
// Provenance note: the in-memory ParametersConfig still carries the
// `last_calibrated` / `calibration_method` metadata fields, but they are no
// longer persisted to the SSOT file. The durable provenance is the overlay
// entry (calibrated_at / method / rationale) plus the structured logs emitted
// here.
func persistCalibratorChanges(changes []CalibratorChange, at time.Time) {
	if len(changes) == 0 {
		return
	}
	path := GetCalibratedOverlayPath()
	if path == "" {
		logging.Warn("calibrator", "calibration_overlay_disabled",
			"detail", "no calibrated-parameters overlay path registered; calibrated values apply in memory only and will not survive a restart")
		return
	}
	entries := make(map[string]CalibrationOverlayEntry, len(changes))
	for _, c := range changes {
		rationale := fmt.Sprintf("baseline delta %+.1f%% (confidence %s) from bayesian optimization over the configured evaluator",
			c.DeltaPct, c.Confidence)
		entries[c.ParamName] = CalibratedEntryForParameter(
			c.ParamName, c.After, c.Before, "bayesian_optimization", rationale, at)
	}
	ov, err := UpdateCalibrationOverlay(path, calibratorOverlaySource, entries)
	if err != nil {
		logging.Error("calibrator", "calibration_overlay_write_failed",
			logging.FStr("path", path), logging.Err(err))
		return
	}
	for _, c := range changes {
		logging.Info("calibrator", "calibration_persisted",
			logging.FStr("param", c.ParamName),
			logging.FFloat64("before", c.Before),
			logging.FFloat64("after", c.After),
			logging.FStr("path", path))
	}
	logging.Info("calibrator", "calibration_overlay_written",
		logging.FStr("path", path), logging.FInt("entries", len(ov.Entries)))
}

func computeImprovementPct(baseline, opt float64) float64 {
	const epsilon = 1e-10
	switch {
	case baseline > epsilon:
		return (opt - baseline) / baseline * 100
	case baseline < -epsilon:
		return (opt - baseline) / (-baseline) * 100
	default:
		return (opt - baseline) * 100
	}
}

func calibrationConfidence(deltaPct float64, observations int) string {
	absDelta := math.Abs(deltaPct)
	switch {
	case observations >= 20 && absDelta > 5:
		return "high"
	case observations >= 10 && absDelta > 3:
		return "medium"
	default:
		return "low"
	}
}

// markCalibrated sets LastCalibrated and CalibrationMethod on a set of parameters.
// It maps parameter names to their ParameterMetadata fields via a lookup table.
func markCalibrated(params *ParametersConfig, names []string, method string, ts *time.Time) {
	for _, name := range names {
		switch name {
		// Engine — MacroRisk
		case "engine_macro_risk_carry_trade_unwind_threshold":
			params.Engine.MacroRisk.CarryTradeUnwindThreshold.LastCalibrated = ts
			params.Engine.MacroRisk.CarryTradeUnwindThreshold.CalibrationMethod = method
		case "engine_macro_risk_vix_threshold":
			params.Engine.MacroRisk.VIXThreshold.LastCalibrated = ts
			params.Engine.MacroRisk.VIXThreshold.CalibrationMethod = method
		case "engine_macro_risk_us10y_threshold":
			params.Engine.MacroRisk.US10YThreshold.LastCalibrated = ts
			params.Engine.MacroRisk.US10YThreshold.CalibrationMethod = method
		case "engine_macro_risk_oil_shock_threshold_pct":
			params.Engine.MacroRisk.OilShockThresholdPct.LastCalibrated = ts
			params.Engine.MacroRisk.OilShockThresholdPct.CalibrationMethod = method
		case "engine_macro_risk_gold_surge_threshold_pct":
			params.Engine.MacroRisk.GoldSurgeThresholdPct.LastCalibrated = ts
			params.Engine.MacroRisk.GoldSurgeThresholdPct.CalibrationMethod = method
		case "engine_macro_risk_dxy_surge_threshold_pct":
			params.Engine.MacroRisk.DXYSurgeThresholdPct.LastCalibrated = ts
			params.Engine.MacroRisk.DXYSurgeThresholdPct.CalibrationMethod = method
		case "engine_macro_risk_twd_stress_threshold_pct":
			params.Engine.MacroRisk.TWDStressThresholdPct.LastCalibrated = ts
			params.Engine.MacroRisk.TWDStressThresholdPct.CalibrationMethod = method
		case "engine_macro_risk_outflow_prob_base":
			params.Engine.MacroRisk.OutflowProbBase.LastCalibrated = ts
			params.Engine.MacroRisk.OutflowProbBase.CalibrationMethod = method
		case "engine_macro_risk_outflow_prob_max":
			params.Engine.MacroRisk.OutflowProbMax.LastCalibrated = ts
			params.Engine.MacroRisk.OutflowProbMax.CalibrationMethod = method
		// Engine — StructuralTrend
		case "engine_structural_trend_min_trend_strength":
			params.Engine.StructuralTrend.MinTrendStrength.LastCalibrated = ts
			params.Engine.StructuralTrend.MinTrendStrength.CalibrationMethod = method
		case "engine_structural_trend_min_confidence":
			params.Engine.StructuralTrend.MinConfidence.LastCalibrated = ts
			params.Engine.StructuralTrend.MinConfidence.CalibrationMethod = method
		case "engine_structural_trend_min_hit_rate":
			params.Engine.StructuralTrend.MinHitRate.LastCalibrated = ts
			params.Engine.StructuralTrend.MinHitRate.CalibrationMethod = method
		case "engine_structural_trend_override_threshold":
			params.Engine.StructuralTrend.OverrideThreshold.LastCalibrated = ts
			params.Engine.StructuralTrend.OverrideThreshold.CalibrationMethod = method
		case "engine_structural_trend_ai_revenue_growth_threshold":
			params.Engine.StructuralTrend.AIRevenueGrowthThreshold.LastCalibrated = ts
			params.Engine.StructuralTrend.AIRevenueGrowthThreshold.CalibrationMethod = method
		case "engine_structural_trend_cowos_utilization_threshold":
			params.Engine.StructuralTrend.CoWoSUtilizationThreshold.LastCalibrated = ts
			params.Engine.StructuralTrend.CoWoSUtilizationThreshold.CalibrationMethod = method
		case "engine_structural_trend_capex_growth_threshold":
			params.Engine.StructuralTrend.CapexGrowthThreshold.LastCalibrated = ts
			params.Engine.StructuralTrend.CapexGrowthThreshold.CalibrationMethod = method
		case "engine_structural_trend_semiconductor_index_threshold":
			params.Engine.StructuralTrend.SemiconductorIndexThreshold.LastCalibrated = ts
			params.Engine.StructuralTrend.SemiconductorIndexThreshold.CalibrationMethod = method
		// Engine — Drawdown
		case "engine_drawdown_orange_override_min_score":
			params.Engine.Drawdown.OrangeOverrideMinScore.LastCalibrated = ts
			params.Engine.Drawdown.OrangeOverrideMinScore.CalibrationMethod = method
		case "engine_drawdown_red_override_min_score":
			params.Engine.Drawdown.RedOverrideMinScore.LastCalibrated = ts
			params.Engine.Drawdown.RedOverrideMinScore.CalibrationMethod = method
		// Engine — Executors
		case "engine_executors_vix_momentum_crash_threshold":
			params.Engine.Executors.VIXMomentumCrashThreshold.LastCalibrated = ts
			params.Engine.Executors.VIXMomentumCrashThreshold.CalibrationMethod = method
		case "engine_executors_crowding_penalty_agents_3":
			params.Engine.Executors.CrowdingPenaltyAgents3.LastCalibrated = ts
			params.Engine.Executors.CrowdingPenaltyAgents3.CalibrationMethod = method
		case "engine_executors_crowding_penalty_agents_4":
			params.Engine.Executors.CrowdingPenaltyAgents4.LastCalibrated = ts
			params.Engine.Executors.CrowdingPenaltyAgents4.CalibrationMethod = method
		case "engine_executors_min_trade_amount":
			params.Engine.Executors.MinTradeAmount.LastCalibrated = ts
			params.Engine.Executors.MinTradeAmount.CalibrationMethod = method
		case "engine_executors_conviction_floor_default":
			params.Engine.Executors.ConvictionFloorDefault.LastCalibrated = ts
			params.Engine.Executors.ConvictionFloorDefault.CalibrationMethod = method
		// Engine — Simulation
		case "engine_simulation_neutral_regime_sizing_factor":
			params.Engine.Simulation.NeutralRegimeSizingFactor.LastCalibrated = ts
			params.Engine.Simulation.NeutralRegimeSizingFactor.CalibrationMethod = method
		// Narrative — Event Detection Thresholds
		case "narrative_ai_revenue_growth_threshold":
			params.Narrative.AIRevenueGrowthThreshold.LastCalibrated = ts
			params.Narrative.AIRevenueGrowthThreshold.CalibrationMethod = method
		case "narrative_cowos_utilization_threshold":
			params.Narrative.CoWoSUtilizationThreshold.LastCalibrated = ts
			params.Narrative.CoWoSUtilizationThreshold.CalibrationMethod = method
		case "narrative_capex_growth_threshold":
			params.Narrative.CapexGrowthThreshold.LastCalibrated = ts
			params.Narrative.CapexGrowthThreshold.CalibrationMethod = method
		case "narrative_us10y_change_bps_threshold":
			params.Narrative.US10YChangeBpsThreshold.LastCalibrated = ts
			params.Narrative.US10YChangeBpsThreshold.CalibrationMethod = method
		case "narrative_dxy_change_pct_threshold":
			params.Narrative.DXYChangePctThreshold.LastCalibrated = ts
			params.Narrative.DXYChangePctThreshold.CalibrationMethod = method
		case "narrative_geopolitical_gpr_threshold":
			params.Narrative.GeopoliticalGPRThreshold.LastCalibrated = ts
			params.Narrative.GeopoliticalGPRThreshold.CalibrationMethod = method
		case "narrative_oil_change_pct_threshold":
			params.Narrative.OilChangePctThreshold.LastCalibrated = ts
			params.Narrative.OilChangePctThreshold.CalibrationMethod = method
		case "narrative_jpy_change_pct_threshold":
			params.Narrative.JPYChangePctThreshold.LastCalibrated = ts
			params.Narrative.JPYChangePctThreshold.CalibrationMethod = method
		case "narrative_vix_level_threshold":
			params.Narrative.VIXLevelThreshold.LastCalibrated = ts
			params.Narrative.VIXLevelThreshold.CalibrationMethod = method
		// FactorWeight — Strategy Deltas
		case "factor_weight_conservative_value":
			params.FactorWeight.ConservativeValue.LastCalibrated = ts
			params.FactorWeight.ConservativeValue.CalibrationMethod = method
		case "factor_weight_conservative_quality":
			params.FactorWeight.ConservativeQuality.LastCalibrated = ts
			params.FactorWeight.ConservativeQuality.CalibrationMethod = method
		case "factor_weight_conservative_momentum":
			params.FactorWeight.ConservativeMomentum.LastCalibrated = ts
			params.FactorWeight.ConservativeMomentum.CalibrationMethod = method
		case "factor_weight_aggressive_momentum":
			params.FactorWeight.AggressiveMomentum.LastCalibrated = ts
			params.FactorWeight.AggressiveMomentum.CalibrationMethod = method
		case "factor_weight_aggressive_inst_sent":
			params.FactorWeight.AggressiveInstSent.LastCalibrated = ts
			params.FactorWeight.AggressiveInstSent.CalibrationMethod = method
		case "factor_weight_aggressive_value":
			params.FactorWeight.AggressiveValue.LastCalibrated = ts
			params.FactorWeight.AggressiveValue.CalibrationMethod = method
		case "factor_weight_aggressive_quality":
			params.FactorWeight.AggressiveQuality.LastCalibrated = ts
			params.FactorWeight.AggressiveQuality.CalibrationMethod = method
		case "factor_weight_risk_on_momentum":
			params.FactorWeight.RiskOnMomentum.LastCalibrated = ts
			params.FactorWeight.RiskOnMomentum.CalibrationMethod = method
		case "factor_weight_risk_on_quality":
			params.FactorWeight.RiskOnQuality.LastCalibrated = ts
			params.FactorWeight.RiskOnQuality.CalibrationMethod = method
		case "factor_weight_risk_off_momentum":
			params.FactorWeight.RiskOffMomentum.LastCalibrated = ts
			params.FactorWeight.RiskOffMomentum.CalibrationMethod = method
		case "factor_weight_risk_off_quality":
			params.FactorWeight.RiskOffQuality.LastCalibrated = ts
			params.FactorWeight.RiskOffQuality.CalibrationMethod = method
		case "factor_weight_risk_off_liquidity":
			params.FactorWeight.RiskOffLiquidity.LastCalibrated = ts
			params.FactorWeight.RiskOffLiquidity.CalibrationMethod = method
		}
	}
}

// LinkageAmplifierCalibrator calibrates the RecessionShockAmplifier parameter
// that controls shock magnitude amplification during recession regimes.
type LinkageAmplifierCalibrator struct{}

func (c *LinkageAmplifierCalibrator) ParamNames() []string {
	return []string{"industry_linkage_recession_shock_amplifier"}
}

func (c *LinkageAmplifierCalibrator) ParamBounds() map[string][2]float64 {
	return map[string][2]float64{
		"industry_linkage_recession_shock_amplifier": {0.8, 2.5},
	}
}
