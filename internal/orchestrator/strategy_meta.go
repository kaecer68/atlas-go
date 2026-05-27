package orchestrator

import (
	"time"
)

// StrategyMeta describes an executor's factor usage and calibratable parameters.
// Each executor exposes this metadata so a calibration engine can understand
// which factors drive its decisions and what parameter ranges are searchable.
type StrategyMeta struct {
	ID          string      `json:"id"`
	Skill       string      `json:"skill"`
	Description string      `json:"description"`
	Factors     []string    `json:"factors"`    // factor types used (e.g. "momentum", "value")
	Parameters  []ParamMeta `json:"parameters"` // calibratable parameters with bounds
}

// ParamMeta describes a single calibratable parameter within an executor's strategy.
// Name identifies the parameter in config (e.g. "momentum_high_threshold").
// Value is the current effective value (from config or hardcoded default).
// Min/Max define the search range for calibration.
// Step is the recommended granularity for grid search / Bayesian optimization.
type ParamMeta struct {
	Name  string  `json:"name"`
	Value float64 `json:"value"`
	Min   float64 `json:"min"`
	Max   float64 `json:"max"`
	Step  float64 `json:"step"`
	Desc  string  `json:"desc"`
}

// StrategyProvider is implemented by executors that support autonomous calibration.
// Calibration engines type-assert AgentExecutor to StrategyProvider to discover
// calibratable parameters without hardcoding per-executor knowledge.
type StrategyProvider interface {
	StrategyMeta() StrategyMeta
}

// Compile-time check: all executors implement StrategyProvider.
var (
	_ StrategyProvider = SemiconductorExecutor{}
	_ StrategyProvider = AISupplyChainExecutor{}
	_ StrategyProvider = LEOSatelliteExecutor{}
	_ StrategyProvider = ETFRotationExecutor{}
	_ StrategyProvider = FinancialsExecutor{}
	_ StrategyProvider = ShippingExecutor{}
	_ StrategyProvider = ValueYieldExecutor{}
	_ StrategyProvider = EarningsQualityExecutor{}
	_ StrategyProvider = TechnicalBreakoutExecutor{}
	_ StrategyProvider = GrowthMomentumExecutor{}
)

// CalibrationRecord records an executor's decision for a symbol in a session.
// Persisted alongside ledger outcomes, these records enable the calibration
// engine to correlate factor scores with forward returns and optimize thresholds.
type CalibrationRecord struct {
	SessionID  string             `json:"session_id"`
	ExecutorID string             `json:"executor_id"`
	Skill      string             `json:"skill"`
	Symbol     string             `json:"symbol"`
	Conviction int                `json:"conviction"`
	Factors    map[string]float64 `json:"factors"`
	RecordedAt time.Time          `json:"recorded_at"`
}

// momentumParams returns the ParamMeta entries for momentum factor thresholds and deltas.
func momentumParams(fc factorConfig) []ParamMeta {
	return []ParamMeta{
		{Name: "momentum_high_threshold", Value: fc.momHigh, Min: 0.2, Max: 0.7, Step: 0.05, Desc: "Threshold for strong momentum signal"},
		{Name: "momentum_high_delta", Value: float64(fc.momHighD), Min: 3, Max: 15, Step: 1, Desc: "Conviction delta for strong momentum"},
		{Name: "momentum_mod_threshold", Value: fc.momMod, Min: 0.05, Max: 0.35, Step: 0.05, Desc: "Threshold for moderate momentum signal"},
		{Name: "momentum_mod_delta", Value: float64(fc.momModD), Min: 2, Max: 10, Step: 1, Desc: "Conviction delta for moderate momentum"},
		{Name: "momentum_weak_threshold", Value: fc.momWeak, Min: -0.4, Max: 0.05, Step: 0.05, Desc: "Threshold below which momentum is penalized"},
		{Name: "momentum_weak_delta", Value: float64(fc.momWeakD), Min: -10, Max: -2, Step: 1, Desc: "Conviction penalty for weak momentum"},
	}
}

// valueParams returns the ParamMeta entries for value factor thresholds and deltas.
func valueParams(fc factorConfig) []ParamMeta {
	return []ParamMeta{
		{Name: "value_high_threshold", Value: fc.valHigh, Min: 0.15, Max: 0.6, Step: 0.05, Desc: "Threshold for strong value signal"},
		{Name: "value_high_delta", Value: float64(fc.valHighD), Min: 3, Max: 15, Step: 1, Desc: "Conviction delta for strong value"},
		{Name: "value_mod_threshold", Value: fc.valMod, Min: 0.05, Max: 0.25, Step: 0.05, Desc: "Threshold for moderate value signal"},
		{Name: "value_mod_delta", Value: float64(fc.valModD), Min: 2, Max: 10, Step: 1, Desc: "Conviction delta for moderate value"},
		{Name: "value_weak_threshold", Value: fc.valWeak, Min: -0.4, Max: 0.0, Step: 0.05, Desc: "Threshold below which value is penalized"},
		{Name: "value_weak_delta", Value: float64(fc.valWeakD), Min: -10, Max: -2, Step: 1, Desc: "Conviction penalty for weak value"},
	}
}

// qualityParams returns the ParamMeta entries for quality factor threshold and delta.
func qualityParams(fc factorConfig) []ParamMeta {
	return []ParamMeta{
		{Name: "quality_threshold", Value: fc.qualThresh, Min: 0.1, Max: 0.5, Step: 0.05, Desc: "Threshold for quality boost"},
		{Name: "quality_delta", Value: float64(fc.qualDelta), Min: 2, Max: 10, Step: 1, Desc: "Conviction delta for quality signal"},
	}
}

// liquidityParams returns the ParamMeta entries for liquidity factor thresholds and deltas.
func liquidityParams(fc factorConfig) []ParamMeta {
	return []ParamMeta{
		{Name: "liquidity_high_threshold", Value: fc.liqHigh, Min: 0.2, Max: 0.8, Step: 0.05, Desc: "Threshold for high liquidity signal"},
		{Name: "liquidity_high_delta", Value: float64(fc.liqHighD), Min: 2, Max: 10, Step: 1, Desc: "Conviction delta for high liquidity"},
		{Name: "liquidity_good_threshold", Value: fc.liqGood, Min: 0.1, Max: 0.5, Step: 0.05, Desc: "Threshold for good liquidity signal"},
		{Name: "liquidity_good_delta", Value: float64(fc.liqGoodD), Min: 1, Max: 8, Step: 1, Desc: "Conviction delta for good liquidity"},
		{Name: "liquidity_low_threshold", Value: fc.liqLow, Min: -0.7, Max: 0.0, Step: 0.05, Desc: "Threshold below which liquidity is penalized"},
		{Name: "liquidity_low_delta", Value: float64(fc.liqLowD), Min: -10, Max: -2, Step: 1, Desc: "Conviction penalty for low liquidity"},
	}
}
