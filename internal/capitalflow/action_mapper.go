package capitalflow

import (
	"context"
	"time"
)

// CapitalFlowAction 將 E07 CapitalFlowAssessment 對投資組合的影響轉成 typed enum。
// spec §6.2 + SA-INV-09：ineligible assessment 不得 mutation；沒有 walk-forward 驗證的
// mapper 不得在 production 啟用。
type CapitalFlowAction string

const (
	// CapitalFlowActionUnavailable 是 anti-corruption 預設；表示 mapper 無法或拒絕產生 action。
	CapitalFlowActionUnavailable CapitalFlowAction = "unavailable"
	// CapitalFlowActionNeutral 表示 eligible 但沒有強訊號，tilt=空 map。
	CapitalFlowActionNeutral CapitalFlowAction = "neutral"
	// CapitalFlowActionRiskOn / CapitalFlowActionRiskOff 為未來 walk-forward 通過的 mapper 預留。
	CapitalFlowActionRiskOn  CapitalFlowAction = "risk_on"
	CapitalFlowActionRiskOff CapitalFlowAction = "risk_off"
)

// CapitalFlowActionMapper 介面（spec §6.2）：walk-forward 通過、model-versioned 才可啟用。
// MapperVersion()="" 表示 disabled，呼叫 Map 必回 unavailable。
type CapitalFlowActionMapper interface {
	MapperVersion() string
	Map(ctx context.Context, asOf time.Time, a CapitalFlowAssessment) (CapitalFlowAction, map[string]float64, error)
}

// NoOpCapitalFlowActionMapper 是預設 disabled mapper；永遠回 unavailable。
// 確保 production 啟用前不會有意外的 capital-flow action mutation。
type NoOpCapitalFlowActionMapper struct{}

// MapperVersion returns empty string indicating mapper is disabled.
func (NoOpCapitalFlowActionMapper) MapperVersion() string { return "" }

// Map returns unavailable action and empty tilt.
func (NoOpCapitalFlowActionMapper) Map(ctx context.Context, asOf time.Time, a CapitalFlowAssessment) (CapitalFlowAction, map[string]float64, error) {
	return CapitalFlowActionUnavailable, nil, nil
}

// DefaultCapitalFlowActionMapper 必須帶 semver version；空 version 視為 disabled。
// walk-forward 通過後才可掛載。
type DefaultCapitalFlowActionMapper struct {
	Version string
}

// MapperVersion returns the configured version.
func (d DefaultCapitalFlowActionMapper) MapperVersion() string { return d.Version }

// Map 對 ineligible assessment 一律回 unavailable（SA-INV-09 守門）。
// eligible + 有 PrimaryFlow → 回 neutral（保守、不猜測）以避免 spec §6.2 的 double counting 風險。
// tilt 永遠為空（SA-INV-08 守 single source of truth）。
func (d DefaultCapitalFlowActionMapper) Map(ctx context.Context, asOf time.Time, a CapitalFlowAssessment) (CapitalFlowAction, map[string]float64, error) {
	if d.Version == "" {
		return CapitalFlowActionUnavailable, nil, nil
	}
	if a.CalibrationStatus != CalibrationEligible {
		return CapitalFlowActionUnavailable, nil, nil
	}
	if a.PrimaryFlow == "" {
		return CapitalFlowActionNeutral, nil, nil
	}
	// 嚴格守門：未做 walk-forward 不得自動把 bullish/bearish 轉成 risk_on/risk_off。
	// 觀察期一律回 neutral 並附 empty tilt。
	return CapitalFlowActionNeutral, nil, nil
}
