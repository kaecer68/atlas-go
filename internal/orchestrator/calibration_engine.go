package orchestrator

import (
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/logging"
)

// CalibrationEngine optimizes factor conviction thresholds and deltas for each
// executor using historical recommendation data. It reads StrategyMeta from
// executors to discover calibratable parameters, then applies coordinate-descent
// optimization to find parameter values that maximize the correlation between
// factor-driven conviction adjustments and forward returns.
type CalibrationEngine struct {
	maturityTracker *domain.MaturityTracker
}

// WithMaturityTracker attaches a maturity tracker for burn-in gating.
func (e *CalibrationEngine) WithMaturityTracker(mt *domain.MaturityTracker) *CalibrationEngine {
	e.maturityTracker = mt
	return e
}

// CalibRecommendation is a historical recommendation with factor scores and
// realized forward return, used as input to the calibration optimizer.
type CalibRecommendation struct {
	Symbol       string
	ForwardRet   float64            // realized forward return (e.g. next-session return)
	FactorScores map[string]float64 // factor scores at decision time (momentum, value, etc.)
}

// CalibrationDataProvider abstracts the source of historical calibration records.
// Implementations may load from JSONL ledger files, replay data, or a database.
type CalibrationDataProvider interface {
	// Recommendations returns all historical recommendations for the given executor skill name.
	Recommendations(executorSkill string) ([]CalibRecommendation, error)
}

// ConvictionCalibrationReport summarizes the results of a calibration run for one executor.
type ConvictionCalibrationReport struct {
	ExecutorID       string             `json:"executor_id"`
	Skill            string             `json:"skill"`
	ParametersBefore map[string]float64 `json:"parameters_before"`
	ParametersAfter  map[string]float64 `json:"parameters_after"`
	BaselineScore    float64            `json:"baseline_score"`
	OptimizedScore   float64            `json:"optimized_score"`
	ImprovementPct   float64            `json:"improvement_pct"`
	SamplesEvaluated int                `json:"samples_evaluated"`
	Rounds           int                `json:"rounds"`
	Verdict          string             `json:"verdict"`
	Timestamp        time.Time          `json:"timestamp"`
}

// CalibrateAll runs calibration for every executor that implements
// StrategyProvider and has sufficient historical data (>= minimumSamples).
// Returns reports for executors that were successfully calibrated.
func (e *CalibrationEngine) CalibrateAll(providers []StrategyProvider, dataProvider CalibrationDataProvider, minimumSamples int) ([]ConvictionCalibrationReport, error) {
	// Burn-in gate: skip calibration until statistical engines are reliable.
	if e.maturityTracker != nil && e.maturityTracker.Current() == domain.MaturityBurnIn {
		logging.Info("calibration_engine", "burn_in_skip",
			"days_until_calibrating", e.maturityTracker.DaysUntil(domain.MaturityCalibrating))
		return nil, nil
	}

	if minimumSamples <= 0 {
		minimumSamples = 10
	}
	// Collect StrategyMeta from each provider via the StrategyProvider interface.
	metas := make([]StrategyMeta, 0, len(providers))
	for _, p := range providers {
		metas = append(metas, p.StrategyMeta())
	}
	reports := make([]ConvictionCalibrationReport, 0, len(metas))
	for _, m := range metas {
		if len(m.Parameters) == 0 {
			continue
		}
		recs, err := dataProvider.Recommendations(m.Skill)
		if err != nil {
			return reports, fmt.Errorf("load %s: %w", m.Skill, err)
		}
		if len(recs) < minimumSamples {
			continue
		}
		report, err := e.Calibrate(m, recs, 3)
		if err != nil {
			return reports, fmt.Errorf("calibrate %s: %w", m.ID, err)
		}
		reports = append(reports, *report)
	}
	return reports, nil
}

// Calibrate runs coordinate-descent optimization for a single executor.
// rounds controls how many passes through all parameters are performed.
func (e *CalibrationEngine) Calibrate(meta StrategyMeta, data []CalibRecommendation, rounds int) (*ConvictionCalibrationReport, error) {
	if rounds <= 0 {
		rounds = 3
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("calibrate: no data for executor %s", meta.ID)
	}

	params := meta.Parameters
	current := extractValueMap(params)
	baseline := scoreParameters(params, meta.Factors, data)

	best := cloneMap(current)
	bestScore := baseline
	samples := 0

	for r := 0; r < rounds; r++ {
		improved := false
		// Sort by name for deterministic ordering
		paramNames := make([]string, len(params))
		for i, p := range params {
			paramNames[i] = p.Name
		}
		sort.Strings(paramNames)
		for _, name := range paramNames {
			p := findByKey(params, name)
			if p == nil {
				continue
			}
			candidates := enumerateCandidates(*p)
			bestLocal := best[name]
			bestLocalScore := bestScore
			for _, candidate := range candidates {
				test := cloneMap(current) // evaluate against current snapshot
				test[name] = candidate
				testParams := applyValueMap(params, test)
				score := scoreParameters(testParams, meta.Factors, data)
				samples++
				if score > bestLocalScore {
					bestLocalScore = score
					bestLocal = candidate
					improved = true
				}
			}
			current[name] = bestLocal
			if bestLocalScore > bestScore {
				bestScore = bestLocalScore
				for k := range best {
					best[k] = current[k]
				}
			}
		}
		if !improved {
			break
		}
	}

	verdict := "stable"
	improvement := (bestScore - baseline) / math.Abs(baseline+1e-10) * 100
	switch {
	case improvement > 5.0:
		verdict = "applied"
	case improvement > 0:
		verdict = "marginal"
	case improvement < -5.0:
		verdict = "degraded"
	}

	return &ConvictionCalibrationReport{
		ExecutorID:       meta.ID,
		Skill:            meta.Skill,
		ParametersBefore: extractValueMap(params),
		ParametersAfter:  best,
		BaselineScore:    baseline,
		OptimizedScore:   bestScore,
		ImprovementPct:   improvement,
		SamplesEvaluated: samples,
		Rounds:           rounds,
		Verdict:          verdict,
		Timestamp:        time.Now().UTC(),
	}, nil
}

// scoreParameters evaluates a parameter configuration by computing the average
// forward return weighted by conviction direction for each recommendation.
//
// For each recommendation:
//  1. Look up the factor scores from CalibRecommendation.FactorScores
//  2. Apply the same threshold logic as executors (add*Adjustment helpers)
//  3. Determine direction: positive if factors suggest boost, negative if penalty
//  4. Multiply direction × forwardRet → returns above zero = correct signal
//
// The final score is the mean of these weighted returns.
func scoreParameters(params []ParamMeta, factors []string, data []CalibRecommendation) float64 {
	if len(data) == 0 {
		return 0
	}
	pm := toValueMap(params)
	total := 0.0
	for _, rec := range data {
		fs := rec.FactorScores
		if fs == nil {
			continue
		}
		delta := 0.0
		for _, f := range factors {
			score, ok := fs[f]
			if !ok {
				continue
			}
			switch f {
			case "momentum":
				if score > pm.get("momentum_high_threshold") {
					delta += pm.get("momentum_high_delta")
				} else if score > pm.get("momentum_mod_threshold") {
					delta += pm.get("momentum_mod_delta")
				} else if score < pm.get("momentum_weak_threshold") {
					delta += pm.get("momentum_weak_delta")
				}
			case "value":
				if score > pm.get("value_high_threshold") {
					delta += pm.get("value_high_delta")
				} else if score > pm.get("value_mod_threshold") {
					delta += pm.get("value_mod_delta")
				} else if score < pm.get("value_weak_threshold") {
					delta += pm.get("value_weak_delta")
				}
			case "quality":
				if score > pm.get("quality_threshold") {
					delta += pm.get("quality_delta")
				}
			case "liquidity":
				if score > pm.get("liquidity_high_threshold") {
					delta += pm.get("liquidity_high_delta")
				} else if score > pm.get("liquidity_good_threshold") {
					delta += pm.get("liquidity_good_delta")
				} else if score < pm.get("liquidity_low_threshold") {
					delta += pm.get("liquidity_low_delta")
				}
			}
		}
		// Score: if delta is positive (more conviction) and forward return is positive,
		// this is a good signal. If delta is negative and forward return is negative,
		// also good (the penalty was correct).
		total += math.Copysign(1.0, delta) * rec.ForwardRet
	}
	return total / float64(len(data))
}

type valueMap map[string]float64

func (m valueMap) get(key string) float64 {
	if v, ok := m[key]; ok {
		return v
	}
	return 0
}

func extractValueMap(params []ParamMeta) map[string]float64 {
	m := make(map[string]float64, len(params))
	for _, p := range params {
		m[p.Name] = p.Value
	}
	return m
}

func applyValueMap(params []ParamMeta, values map[string]float64) []ParamMeta {
	result := make([]ParamMeta, len(params))
	copy(result, params)
	for i := range result {
		if v, ok := values[result[i].Name]; ok {
			result[i].Value = v
		}
	}
	return result
}

func toValueMap(params []ParamMeta) valueMap {
	m := make(valueMap, len(params))
	for _, p := range params {
		m[p.Name] = p.Value
	}
	return m
}

func findByKey(params []ParamMeta, name string) *ParamMeta {
	for i := range params {
		if params[i].Name == name {
			return &params[i]
		}
	}
	return nil
}

// enumerateCandidates generates candidate values for a single parameter within
// its calibration bounds [Min, Max] at the configured Step granularity.
func enumerateCandidates(p ParamMeta) []float64 {
	if p.Step <= 0 || p.Min >= p.Max {
		return []float64{p.Value}
	}
	n := min(int((p.Max-p.Min)/p.Step)+1,
		// prevent combinatorial explosion
		30)
	candidates := make([]float64, 0, n)
	for v := p.Min; v <= p.Max+1e-10; v += p.Step {
		candidates = append(candidates, math.Round(v*1e6)/1e6)
		if len(candidates) >= n {
			break
		}
	}
	return candidates
}

func cloneMap(m map[string]float64) map[string]float64 {
	c := make(map[string]float64, len(m))
	for k, v := range m {
		c[k] = v
	}
	return c
}

// ApplyToConfig writes the optimized parameters from a calibration report
// back to the global ParametersConfig and persists to parameters.json. Returns
// an error if the config is unavailable or if the parameter name doesn't match
// any known FactorConvictionParams field. Delegates to ApplyToConfigPath.
func (e *CalibrationEngine) ApplyToConfig(report ConvictionCalibrationReport) error {
	params := config.GetParametersConfig()
	if params == nil {
		return fmt.Errorf("ApplyToConfig: ParametersConfig is nil")
	}
	path := config.GetParametersConfigPath()
	if path == "" {
		return fmt.Errorf("ApplyToConfig: no parameters config path configured")
	}
	return e.ApplyToConfigPath(report, params, path)
}

// ApplyToConfigPath applies calibration results to a specific ParametersConfig
// instance and persists to the given path. This is the testable variant;
// ApplyToConfig is the production wrapper that reads global config.
func (e *CalibrationEngine) ApplyToConfigPath(report ConvictionCalibrationReport, params *config.ParametersConfig, path string) error {
	fc := &params.SectorExecutor.FactorConviction
	for name, v := range report.ParametersAfter {
		if err := applyParamField(fc, name, v); err != nil {
			return fmt.Errorf("ApplyToConfig: %w", err)
		}
	}
	return params.TryLockedSaveWithRollback(path, 30*time.Second)
}

// applyParamField maps a ParameterMeta.Name string to the corresponding
// FactorConvictionParams field and assigns the value.
func applyParamField(fc *config.FactorConvictionParams, name string, v float64) error {
	switch name {
	// Momentum
	case "momentum_high_threshold":
		fc.MomentumHighThreshold.Value = v
	case "momentum_high_delta":
		fc.MomentumHighDelta.Value = int(v)
	case "momentum_mod_threshold":
		fc.MomentumModThreshold.Value = v
	case "momentum_mod_delta":
		fc.MomentumModDelta.Value = int(v)
	case "momentum_weak_threshold":
		fc.MomentumWeakThreshold.Value = v
	case "momentum_weak_delta":
		fc.MomentumWeakDelta.Value = int(v)
	// Value
	case "value_high_threshold":
		fc.ValueHighThreshold.Value = v
	case "value_high_delta":
		fc.ValueHighDelta.Value = int(v)
	case "value_mod_threshold":
		fc.ValueModThreshold.Value = v
	case "value_mod_delta":
		fc.ValueModDelta.Value = int(v)
	case "value_weak_threshold":
		fc.ValueWeakThreshold.Value = v
	case "value_weak_delta":
		fc.ValueWeakDelta.Value = int(v)
	// Quality
	case "quality_threshold":
		fc.QualityThreshold.Value = v
	case "quality_delta":
		fc.QualityDelta.Value = int(v)
	// Liquidity
	case "liquidity_high_threshold":
		fc.LiquidityHighThreshold.Value = v
	case "liquidity_high_delta":
		fc.LiquidityHighDelta.Value = int(v)
	case "liquidity_good_threshold":
		fc.LiquidityGoodThreshold.Value = v
	case "liquidity_good_delta":
		fc.LiquidityGoodDelta.Value = int(v)
	case "liquidity_low_threshold":
		fc.LiquidityLowThreshold.Value = v
	case "liquidity_low_delta":
		fc.LiquidityLowDelta.Value = int(v)
	default:
		return fmt.Errorf("unknown parameter: %s", name)
	}
	return nil
}
