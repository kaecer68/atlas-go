package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// ParameterSnapshot records the full parameter state at a specific point in time.
// Used for experiment tracking, audit trails, and rollback capability.
type ParameterSnapshot struct {
	ID        string            `json:"id"`
	Timestamp time.Time         `json:"timestamp"`
	Reason    string            `json:"reason,omitempty"`
	User      string            `json:"user,omitempty"`
	Params    *ParametersConfig `json:"params"`
	Changes   []ParameterChange `json:"changes,omitempty"`
}

// ParameterChange records a single parameter modification.
type ParameterChange struct {
	Parameter string    `json:"parameter"`
	OldValue  any       `json:"old_value"`
	NewValue  any       `json:"new_value"`
	Reason    string    `json:"reason,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

// SnapshotStore manages parameter snapshots and audit history.
type SnapshotStore struct {
	dir string
}

// NewSnapshotStore creates a snapshot store in the given directory.
func NewSnapshotStore(dir string) *SnapshotStore {
	return &SnapshotStore{dir: dir}
}

// SaveSnapshot persists a parameter snapshot to disk.
func (s *SnapshotStore) SaveSnapshot(snap *ParameterSnapshot) error {
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return fmt.Errorf("create snapshot dir: %w", err)
	}

	filename := filepath.Join(s.dir, fmt.Sprintf("snapshot-%s.json", snap.ID))
	data, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal snapshot: %w", err)
	}

	if err := os.WriteFile(filename, data, 0o644); err != nil {
		return fmt.Errorf("write snapshot: %w", err)
	}

	return nil
}

// LoadSnapshot retrieves a snapshot by ID.
func (s *SnapshotStore) LoadSnapshot(id string) (*ParameterSnapshot, error) {
	filename := filepath.Join(s.dir, fmt.Sprintf("snapshot-%s.json", id))
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("read snapshot: %w", err)
	}

	var snap ParameterSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return nil, fmt.Errorf("parse snapshot: %w", err)
	}

	return &snap, nil
}

// ListSnapshots returns all snapshot IDs sorted by timestamp (newest first).
func (s *SnapshotStore) ListSnapshots() ([]ParameterSnapshot, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read snapshot dir: %w", err)
	}

	var snaps []ParameterSnapshot
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		data, err := os.ReadFile(filepath.Join(s.dir, entry.Name()))
		if err != nil {
			continue
		}

		var snap ParameterSnapshot
		if err := json.Unmarshal(data, &snap); err != nil {
			continue
		}

		snaps = append(snaps, snap)
	}

	// Sort by timestamp descending
	for i := 0; i < len(snaps)-1; i++ {
		for j := i + 1; j < len(snaps); j++ {
			if snaps[j].Timestamp.After(snaps[i].Timestamp) {
				snaps[i], snaps[j] = snaps[j], snaps[i]
			}
		}
	}

	return snaps, nil
}

// DiffSnapshots computes the differences between two parameter snapshots.
func DiffSnapshots(old, new *ParameterSnapshot) []ParameterChange {
	if old == nil || new == nil || old.Params == nil || new.Params == nil {
		return nil
	}

	var changes []ParameterChange
	now := time.Now()

	// Compare Darwinian parameters
	compareFloat(&changes, "darwinian.weight_min", old.Params.Darwinian.WeightMin.Value, new.Params.Darwinian.WeightMin.Value, now)
	compareFloat(&changes, "darwinian.weight_max", old.Params.Darwinian.WeightMax.Value, new.Params.Darwinian.WeightMax.Value, now)
	compareFloat(&changes, "darwinian.top_quartile_multiplier", old.Params.Darwinian.TopQuartileMultiplier.Value, new.Params.Darwinian.TopQuartileMultiplier.Value, now)
	compareFloat(&changes, "darwinian.bottom_quartile_multiplier", old.Params.Darwinian.BottomQuartileMultiplier.Value, new.Params.Darwinian.BottomQuartileMultiplier.Value, now)
	compareFloat(&changes, "darwinian.zero_signal_penalty_multiplier", old.Params.Darwinian.ZeroSignalPenaltyMultiplier.Value, new.Params.Darwinian.ZeroSignalPenaltyMultiplier.Value, now)
	compareFloat(&changes, "darwinian.loss_penalty_multiplier", old.Params.Darwinian.LossPenaltyMultiplier.Value, new.Params.Darwinian.LossPenaltyMultiplier.Value, now)
	compareFloat(&changes, "darwinian.weight_change_alert_threshold", old.Params.Darwinian.WeightChangeAlertThreshold.Value, new.Params.Darwinian.WeightChangeAlertThreshold.Value, now)
	compareFloat(&changes, "darwinian.zero_signal_penalty_after_days", float64(old.Params.Darwinian.ZeroSignalPenaltyAfterDays.Value), float64(new.Params.Darwinian.ZeroSignalPenaltyAfterDays.Value), now)

	// Compare Factor parameters
	compareFloat(&changes, "factor.momentum_stddev_divisor", old.Params.Factor.MomentumStdDevDivisor.Value, new.Params.Factor.MomentumStdDevDivisor.Value, now)
	compareFloat(&changes, "factor.momentum_lookback_days", float64(old.Params.Factor.MomentumLookbackDays.Value), float64(new.Params.Factor.MomentumLookbackDays.Value), now)

	// Compare Sizing parameters
	compareFloat(&changes, "sizing.kelly_fraction", old.Params.Sizing.KellyFraction.Value, new.Params.Sizing.KellyFraction.Value, now)
	compareFloat(&changes, "sizing.target_volatility", old.Params.Sizing.TargetVolatility.Value, new.Params.Sizing.TargetVolatility.Value, now)
	compareFloat(&changes, "sizing.max_drawdown_limit", old.Params.Sizing.MaxDrawdownLimit.Value, new.Params.Sizing.MaxDrawdownLimit.Value, now)

	// Compare Health parameters
	compareFloat(&changes, "health.sharpe_weight", old.Params.Health.SharpeWeight.Value, new.Params.Health.SharpeWeight.Value, now)
	compareFloat(&changes, "health.hitrate_weight", old.Params.Health.HitRateWeight.Value, new.Params.Health.HitRateWeight.Value, now)
	compareFloat(&changes, "health.streak_weight", old.Params.Health.StreakWeight.Value, new.Params.Health.StreakWeight.Value, now)

	// Compare GARCH parameters
	compareFloat(&changes, "garch.alpha", old.Params.GARCH.Alpha.Value, new.Params.GARCH.Alpha.Value, now)
	compareFloat(&changes, "garch.beta", old.Params.GARCH.Beta.Value, new.Params.GARCH.Beta.Value, now)
	compareFloat(&changes, "garch.omega", old.Params.GARCH.Omega.Value, new.Params.GARCH.Omega.Value, now)

	// Compare Experiment parameters
	compareFloat(&changes, "experiment.improvement_threshold", old.Params.Experiment.ImprovementThreshold.Value, new.Params.Experiment.ImprovementThreshold.Value, now)
	compareFloat(&changes, "experiment.welch_ttest_threshold", old.Params.Experiment.WelchTTestThreshold.Value, new.Params.Experiment.WelchTTestThreshold.Value, now)

	// Compare Baseline parameters
	compareFloat(&changes, "baseline.max_position_weight", old.Params.Baseline.MaxPositionWeight.Value, new.Params.Baseline.MaxPositionWeight.Value, now)
	compareFloat(&changes, "baseline.reserve_cash_fraction", old.Params.Baseline.ReserveCashFraction.Value, new.Params.Baseline.ReserveCashFraction.Value, now)

	// Compare Orchestrator parameters
	compareFloat(&changes, "orchestrator.conviction_floor", float64(old.Params.Orchestrator.ConvictionFloorDefault.Value), float64(new.Params.Orchestrator.ConvictionFloorDefault.Value), now)
	compareFloat(&changes, "orchestrator.cro_zscore_threshold", old.Params.Orchestrator.CROZScoreThreshold.Value, new.Params.Orchestrator.CROZScoreThreshold.Value, now)

	// Compare Risk parameters
	compareFloat(&changes, "risk.max_drawdown_pct", old.Params.Risk.MaxDrawdownPct.Value, new.Params.Risk.MaxDrawdownPct.Value, now)
	compareFloat(&changes, "risk.max_position_size", old.Params.Risk.MaxPositionSize.Value, new.Params.Risk.MaxPositionSize.Value, now)
	compareFloat(&changes, "risk.max_daily_loss_pct", old.Params.Risk.MaxDailyLossPct.Value, new.Params.Risk.MaxDailyLossPct.Value, now)

	// Compare Realtime parameters
	compareFloat(&changes, "realtime.volatility_threshold", old.Params.Realtime.VolatilityThreshold.Value, new.Params.Realtime.VolatilityThreshold.Value, now)
	compareFloat(&changes, "realtime.volume_spike_threshold", old.Params.Realtime.VolumeSpikeThreshold.Value, new.Params.Realtime.VolumeSpikeThreshold.Value, now)
	compareFloat(&changes, "realtime.min_confidence", old.Params.Realtime.MinConfidence.Value, new.Params.Realtime.MinConfidence.Value, now)

	compareFloat(&changes, "narrative.min_trend_strength", old.Params.Narrative.MinTrendStrength.Value, new.Params.Narrative.MinTrendStrength.Value, now)
	compareFloat(&changes, "narrative.min_confidence", old.Params.Narrative.MinConfidence.Value, new.Params.Narrative.MinConfidence.Value, now)
	compareFloat(&changes, "narrative.min_hit_rate", old.Params.Narrative.MinHitRate.Value, new.Params.Narrative.MinHitRate.Value, now)
	compareFloat(&changes, "narrative.override_threshold", old.Params.Narrative.OverrideThreshold.Value, new.Params.Narrative.OverrideThreshold.Value, now)
	compareFloat(&changes, "narrative.ai_revenue_growth_threshold", old.Params.Narrative.AIRevenueGrowthThreshold.Value, new.Params.Narrative.AIRevenueGrowthThreshold.Value, now)
	compareFloat(&changes, "narrative.cowos_utilization_threshold", old.Params.Narrative.CoWoSUtilizationThreshold.Value, new.Params.Narrative.CoWoSUtilizationThreshold.Value, now)
	compareFloat(&changes, "narrative.capex_growth_threshold", old.Params.Narrative.CapexGrowthThreshold.Value, new.Params.Narrative.CapexGrowthThreshold.Value, now)
	compareFloat(&changes, "narrative.us10y_change_bps_threshold", old.Params.Narrative.US10YChangeBpsThreshold.Value, new.Params.Narrative.US10YChangeBpsThreshold.Value, now)
	compareFloat(&changes, "narrative.dxy_change_pct_threshold", old.Params.Narrative.DXYChangePctThreshold.Value, new.Params.Narrative.DXYChangePctThreshold.Value, now)
	compareFloat(&changes, "narrative.geopolitical_gpr_threshold", old.Params.Narrative.GeopoliticalGPRThreshold.Value, new.Params.Narrative.GeopoliticalGPRThreshold.Value, now)
	compareFloat(&changes, "narrative.oil_change_pct_threshold", old.Params.Narrative.OilChangePctThreshold.Value, new.Params.Narrative.OilChangePctThreshold.Value, now)
	compareFloat(&changes, "narrative.jpy_change_pct_threshold", old.Params.Narrative.JPYChangePctThreshold.Value, new.Params.Narrative.JPYChangePctThreshold.Value, now)
	compareFloat(&changes, "narrative.vix_level_threshold", old.Params.Narrative.VIXLevelThreshold.Value, new.Params.Narrative.VIXLevelThreshold.Value, now)
	compareFloat(&changes, "narrative.model_lookback_days", float64(old.Params.Narrative.ModelLookbackDays.Value), float64(new.Params.Narrative.ModelLookbackDays.Value), now)
	compareFloat(&changes, "narrative.model_hold_window_days", float64(old.Params.Narrative.ModelHoldWindowDays.Value), float64(new.Params.Narrative.ModelHoldWindowDays.Value), now)

	compareFloat(&changes, "janus.short_window_days", float64(old.Params.Janus.ShortWindowDays.Value), float64(new.Params.Janus.ShortWindowDays.Value), now)
	compareFloat(&changes, "janus.medium_window_days", float64(old.Params.Janus.MediumWindowDays.Value), float64(new.Params.Janus.MediumWindowDays.Value), now)
	compareFloat(&changes, "janus.long_window_days", float64(old.Params.Janus.LongWindowDays.Value), float64(new.Params.Janus.LongWindowDays.Value), now)
	compareFloat(&changes, "janus.min_weight", old.Params.Janus.MinWeight.Value, new.Params.Janus.MinWeight.Value, now)
	compareFloat(&changes, "janus.max_weight", old.Params.Janus.MaxWeight.Value, new.Params.Janus.MaxWeight.Value, now)
	compareFloat(&changes, "janus.novel_threshold", old.Params.Janus.NovelThreshold.Value, new.Params.Janus.NovelThreshold.Value, now)
	compareFloat(&changes, "janus.historical_threshold", old.Params.Janus.HistoricalThreshold.Value, new.Params.Janus.HistoricalThreshold.Value, now)
	compareFloat(&changes, "janus.epsilon_weight", old.Params.Janus.EpsilonWeight.Value, new.Params.Janus.EpsilonWeight.Value, now)
	compareFloat(&changes, "janus.short_window_blend", old.Params.Janus.ShortWindowBlend.Value, new.Params.Janus.ShortWindowBlend.Value, now)
	compareFloat(&changes, "janus.medium_window_blend", old.Params.Janus.MediumWindowBlend.Value, new.Params.Janus.MediumWindowBlend.Value, now)
	compareFloat(&changes, "janus.long_window_blend", old.Params.Janus.LongWindowBlend.Value, new.Params.Janus.LongWindowBlend.Value, now)

	compareFloat(&changes, "marketdata.twse_api_rate_limit", old.Params.Marketdata.TWSEAPIRateLimit.Value, new.Params.Marketdata.TWSEAPIRateLimit.Value, now)
	compareFloat(&changes, "marketdata.twse_api_rate_burst", float64(old.Params.Marketdata.TWSEAPIRateBurst.Value), float64(new.Params.Marketdata.TWSEAPIRateBurst.Value), now)
	compareFloat(&changes, "marketdata.twse_api_timeout_sec", float64(old.Params.Marketdata.TWSEAPITimeoutSec.Value), float64(new.Params.Marketdata.TWSEAPITimeoutSec.Value), now)
	compareFloat(&changes, "marketdata.fubon_intraday_limit", float64(old.Params.Marketdata.FubonIntradayLimit.Value), float64(new.Params.Marketdata.FubonIntradayLimit.Value), now)
	compareFloat(&changes, "marketdata.fubon_historical_limit", float64(old.Params.Marketdata.FubonHistoricalLimit.Value), float64(new.Params.Marketdata.FubonHistoricalLimit.Value), now)
	compareFloat(&changes, "marketdata.fubon_api_timeout_sec", float64(old.Params.Marketdata.FubonAPITimeoutSec.Value), float64(new.Params.Marketdata.FubonAPITimeoutSec.Value), now)
	compareFloat(&changes, "marketdata.tej_calls_per_second", float64(old.Params.Marketdata.TEJCallsPerSecond.Value), float64(new.Params.Marketdata.TEJCallsPerSecond.Value), now)
	compareFloat(&changes, "marketdata.tej_api_timeout_sec", float64(old.Params.Marketdata.TEJAPITimeoutSec.Value), float64(new.Params.Marketdata.TEJAPITimeoutSec.Value), now)
	compareFloat(&changes, "marketdata.fugle_rate_limit", float64(old.Params.Marketdata.FugleRateLimit.Value), float64(new.Params.Marketdata.FugleRateLimit.Value), now)
	compareFloat(&changes, "marketdata.fugle_api_timeout_sec", float64(old.Params.Marketdata.FugleAPITimeoutSec.Value), float64(new.Params.Marketdata.FugleAPITimeoutSec.Value), now)
	compareFloat(&changes, "marketdata.max_retry_attempts", float64(old.Params.Marketdata.MaxRetryAttempts.Value), float64(new.Params.Marketdata.MaxRetryAttempts.Value), now)
	compareFloat(&changes, "marketdata.retry_backoff_ms", float64(old.Params.Marketdata.RetryBackoffMs.Value), float64(new.Params.Marketdata.RetryBackoffMs.Value), now)

	compareBool(&changes, "industry.concentration_risk_enabled", old.Params.Industry.ConcentrationRiskEnabled.Value, new.Params.Industry.ConcentrationRiskEnabled.Value, now)
	compareBool(&changes, "industry.news_latency_risk_enabled", old.Params.Industry.NewsLatencyRiskEnabled.Value, new.Params.Industry.NewsLatencyRiskEnabled.Value, now)
	compareBool(&changes, "industry.asymmetric_risk_enabled", old.Params.Industry.AsymmetricRiskEnabled.Value, new.Params.Industry.AsymmetricRiskEnabled.Value, now)
	compareFloat(&changes, "industry.customer_concentration_limit", old.Params.Industry.CustomerConcentrationLimit.Value, new.Params.Industry.CustomerConcentrationLimit.Value, now)
	compareFloat(&changes, "industry.geographic_exposure_limit", old.Params.Industry.GeographicExposureLimit.Value, new.Params.Industry.GeographicExposureLimit.Value, now)
	// DEPRECATED: industry.sector_weights diff removed; use sector_allocation.base_weights
	compareMapCycleThreshold(&changes, "industry.cycle_thresholds", old.Params.Industry.CycleThresholds.Value, new.Params.Industry.CycleThresholds.Value, now)
	compareInventoryCycleThreshold(&changes, "industry.inventory_cycle_thresholds", old.Params.Industry.InventoryCycleThresholds.Value, new.Params.Industry.InventoryCycleThresholds.Value, now)
	compareCapexCycleThreshold(&changes, "industry.capex_cycle_thresholds", old.Params.Industry.CapexCycleThresholds.Value, new.Params.Industry.CapexCycleThresholds.Value, now)
	compareConfidenceSignal(&changes, "industry.confidence_signal", old.Params.Industry.ConfidenceSignal.Value, new.Params.Industry.ConfidenceSignal.Value, now)
	compareConfidenceMix(&changes, "industry.confidence_mix", old.Params.Industry.ConfidenceMix.Value, new.Params.Industry.ConfidenceMix.Value, now)
	compareSeasonalPatterns(&changes, "industry.seasonal_patterns", old.Params.Industry.SeasonalPatterns.Value, new.Params.Industry.SeasonalPatterns.Value, now)
	compareLinkageParams(&changes, "industry.linkage_params", old.Params.Industry.LinkageParams.Value, new.Params.Industry.LinkageParams.Value, now)

	compareFloat(&changes, "strategy.min_switch_interval_days", float64(old.Params.Strategy.MinSwitchIntervalDays.Value), float64(new.Params.Strategy.MinSwitchIntervalDays.Value), now)
	compareFloat(&changes, "strategy.switch_threshold", old.Params.Strategy.SwitchThreshold.Value, new.Params.Strategy.SwitchThreshold.Value, now)
	compareFloat(&changes, "strategy.score_lookback_days", float64(old.Params.Strategy.ScoreLookbackDays.Value), float64(new.Params.Strategy.ScoreLookbackDays.Value), now)

	return changes
}

func compareFloat(changes *[]ParameterChange, param string, oldVal, newVal float64, t time.Time) {
	if oldVal != newVal {
		*changes = append(*changes, ParameterChange{
			Parameter: param,
			OldValue:  oldVal,
			NewValue:  newVal,
			Timestamp: t,
		})
	}
}

func compareBool(changes *[]ParameterChange, param string, oldVal, newVal bool, t time.Time) {
	if oldVal != newVal {
		*changes = append(*changes, ParameterChange{
			Parameter: param,
			OldValue:  oldVal,
			NewValue:  newVal,
			Timestamp: t,
		})
	}
}

func compareConfidenceSignal(changes *[]ParameterChange, param string, oldVal, newVal ConfidenceSignalConfig, t time.Time) {
	fields := []struct {
		name string
		o    float64
		n    float64
	}{
		{"signal_base", oldVal.SignalBase, newVal.SignalBase},
		{"revenue_norm_denom", oldVal.RevenueNormDenom, newVal.RevenueNormDenom},
		{"revenue_weight", oldVal.RevenueWeight, newVal.RevenueWeight},
		{"profit_norm_denom", oldVal.ProfitNormDenom, newVal.ProfitNormDenom},
		{"profit_weight", oldVal.ProfitWeight, newVal.ProfitWeight},
		{"inventory_norm_denom", oldVal.InventoryNormDenom, newVal.InventoryNormDenom},
		{"inventory_weight", oldVal.InventoryWeight, newVal.InventoryWeight},
		{"utilization_weight", oldVal.UtilizationWeight, newVal.UtilizationWeight},
		{"signal_boundary_mix", oldVal.SignalBoundaryMix, newVal.SignalBoundaryMix},
		{"boundary_denom_factor", oldVal.BoundaryDenomFactor, newVal.BoundaryDenomFactor},
		{"confidence_floor", oldVal.ConfidenceFloor, newVal.ConfidenceFloor},
		{"confidence_ceiling", oldVal.ConfidenceCeiling, newVal.ConfidenceCeiling},
	}
	for _, f := range fields {
		if f.o != f.n {
			*changes = append(*changes, ParameterChange{
				Parameter: param + "." + f.name,
				OldValue:  f.o,
				NewValue:  f.n,
				Timestamp: t,
			})
		}
	}
}

func compareConfidenceMix(changes *[]ParameterChange, param string, oldVal, newVal ConfidenceMixConfig, t time.Time) {
	fields := []struct {
		name string
		o    float64
		n    float64
	}{
		{"weight_boundary", oldVal.WeightBoundary, newVal.WeightBoundary},
		{"weight_freshness", oldVal.WeightFreshness, newVal.WeightFreshness},
		{"weight_seasonal", oldVal.WeightSeasonal, newVal.WeightSeasonal},
		{"weight_linkage", oldVal.WeightLinkage, newVal.WeightLinkage},
		{"weight_narrative", oldVal.WeightNarrative, newVal.WeightNarrative},
		{"favorable_confidence_min", oldVal.FavorableConfidenceMin, newVal.FavorableConfidenceMin},
	}
	for _, f := range fields {
		if f.o != f.n {
			*changes = append(*changes, ParameterChange{
				Parameter: param + "." + f.name,
				OldValue:  f.o,
				NewValue:  f.n,
				Timestamp: t,
			})
		}
	}
}

func compareSeasonalPatterns(changes *[]ParameterChange, param string, oldVal, newVal []SeasonalPatternConfig, t time.Time) {
	if len(oldVal) != len(newVal) {
		*changes = append(*changes, ParameterChange{
			Parameter: param + " (count)",
			OldValue:  len(oldVal),
			NewValue:  len(newVal),
			Timestamp: t,
		})
		return
	}
	for i := range oldVal {
		o, n := oldVal[i], newVal[i]
		if o.ID != n.ID {
			*changes = append(*changes, ParameterChange{
				Parameter: param + "[" + fmt.Sprintf("%d", i) + "].id",
				OldValue:  o.ID,
				NewValue:  n.ID,
				Timestamp: t,
			})
		}
		compareFloat(changes, param+"["+fmt.Sprintf("%d", i)+"].adjustment_factor", o.AdjustmentFactor, n.AdjustmentFactor, t)
		compareFloat(changes, param+"["+fmt.Sprintf("%d", i)+"].historical_accuracy", o.HistoricalAccuracy, n.HistoricalAccuracy, t)
		compareFloat(changes, param+"["+fmt.Sprintf("%d", i)+"].avg_market_return", o.AvgMarketReturn, n.AvgMarketReturn, t)
	}
}

func compareLinkageParams(changes *[]ParameterChange, param string, oldVal, newVal LinkageConfig, t time.Time) {
	compareFloat(changes, param+".downstream_decay_factor", oldVal.DownstreamDecayFactor, newVal.DownstreamDecayFactor, t)
	compareFloat(changes, param+".upstream_decay_factor", oldVal.UpstreamDecayFactor, newVal.UpstreamDecayFactor, t)
	compareFloat(changes, param+".seasonal_decay_factor", oldVal.SeasonalDecayFactor, newVal.SeasonalDecayFactor, t)
	compareFloat(changes, param+".default_correlation", oldVal.DefaultCorrelation, newVal.DefaultCorrelation, t)
	compareFloat(changes, param+".systemic_importance_divisor", oldVal.SystemicImportanceDivisor, newVal.SystemicImportanceDivisor, t)
	compareFloat(changes, param+".min_correlation_threshold", oldVal.MinCorrelationThreshold, newVal.MinCorrelationThreshold, t)
}

func compareMapCycleThreshold(changes *[]ParameterChange, param string, oldVal, newVal map[string]CycleThresholdConfig, t time.Time) {
	if len(oldVal) != len(newVal) {
		*changes = append(*changes, ParameterChange{
			Parameter: param + " (count)",
			OldValue:  len(oldVal),
			NewValue:  len(newVal),
			Timestamp: t,
		})
	}
	for k, v := range oldVal {
		newV, ok := newVal[k]
		if !ok {
			*changes = append(*changes, ParameterChange{
				Parameter: param + "." + k,
				OldValue:  "present",
				NewValue:  "removed",
				Timestamp: t,
			})
			continue
		}
		fields := []struct {
			name string
			o    float64
			n    float64
		}{
			{"expansion_revenue_pct", v.ExpansionRevenuePct, newV.ExpansionRevenuePct},
			{"expansion_profit_pct", v.ExpansionProfitPct, newV.ExpansionProfitPct},
			{"recovery_revenue_pct", v.RecoveryRevenuePct, newV.RecoveryRevenuePct},
			{"recovery_profit_pct", v.RecoveryProfitPct, newV.RecoveryProfitPct},
			{"mature_revenue_pct", v.MatureRevenuePct, newV.MatureRevenuePct},
			{"mature_profit_pct", v.MatureProfitPct, newV.MatureProfitPct},
		}
		for _, f := range fields {
			if f.o != f.n {
				*changes = append(*changes, ParameterChange{
					Parameter: param + "." + k + "." + f.name,
					OldValue:  f.o,
					NewValue:  f.n,
					Timestamp: t,
				})
			}
		}
	}
	for k := range newVal {
		if _, ok := oldVal[k]; !ok {
			*changes = append(*changes, ParameterChange{
				Parameter: param + "." + k,
				OldValue:  "absent",
				NewValue:  "added",
				Timestamp: t,
			})
		}
	}
}

func compareInventoryCycleThreshold(changes *[]ParameterChange, param string, oldVal, newVal InventoryCycleThresholdConfig, t time.Time) {
	fields := []struct {
		name string
		o    float64
		n    float64
	}{
		{"active_restocking_inventory_min", oldVal.ActiveRestockingInventoryMin, newVal.ActiveRestockingInventoryMin},
		{"active_restocking_capacity_min", oldVal.ActiveRestockingCapacityMin, newVal.ActiveRestockingCapacityMin},
		{"passive_restocking_inventory_min", oldVal.PassiveRestockingInventoryMin, newVal.PassiveRestockingInventoryMin},
		{"passive_restocking_capacity_min", oldVal.PassiveRestockingCapacityMin, newVal.PassiveRestockingCapacityMin},
		{"active_destocking_inventory_max", oldVal.ActiveDestockingInventoryMax, newVal.ActiveDestockingInventoryMax},
		{"active_destocking_capacity_max", oldVal.ActiveDestockingCapacityMax, newVal.ActiveDestockingCapacityMax},
	}
	for _, f := range fields {
		if f.o != f.n {
			*changes = append(*changes, ParameterChange{
				Parameter: param + "." + f.name,
				OldValue:  f.o,
				NewValue:  f.n,
				Timestamp: t,
			})
		}
	}
}

func compareCapexCycleThreshold(changes *[]ParameterChange, param string, oldVal, newVal CapexCycleThresholdConfig, t time.Time) {
	fields := []struct {
		name string
		o    float64
		n    float64
	}{
		{"expansion_capacity_min", oldVal.ExpansionCapacityMin, newVal.ExpansionCapacityMin},
		{"expansion_revenue_min", oldVal.ExpansionRevenueMin, newVal.ExpansionRevenueMin},
		{"maintenance_capacity_min", oldVal.MaintenanceCapacityMin, newVal.MaintenanceCapacityMin},
		{"maintenance_revenue_min", oldVal.MaintenanceRevenueMin, newVal.MaintenanceRevenueMin},
	}
	for _, f := range fields {
		if f.o != f.n {
			*changes = append(*changes, ParameterChange{
				Parameter: param + "." + f.name,
				OldValue:  f.o,
				NewValue:  f.n,
				Timestamp: t,
			})
		}
	}
}

// SnapshotForExperiment creates a snapshot tied to an experiment run.
func SnapshotForExperiment(params *ParametersConfig, experimentID string) *ParameterSnapshot {
	return &ParameterSnapshot{
		ID:        fmt.Sprintf("exp-%s", experimentID),
		Timestamp: time.Now(),
		Reason:    fmt.Sprintf("Experiment execution: %s", experimentID),
		Params:    params,
	}
}

// RollbackToSnapshot restores parameters from a historical snapshot.
func (s *SnapshotStore) RollbackToSnapshot(id string, reason string, user string) (*ParameterSnapshot, error) {
	target, err := s.LoadSnapshot(id)
	if err != nil {
		return nil, fmt.Errorf("load target snapshot %s: %w", id, err)
	}

	currentSnap := &ParameterSnapshot{
		ID:        fmt.Sprintf("pre-rollback-%d", time.Now().Unix()),
		Timestamp: time.Now(),
		Reason:    fmt.Sprintf("Pre-rollback state before restoring to %s", id),
		User:      user,
		Params:    GetParametersConfig(),
	}
	if err := s.SaveSnapshot(currentSnap); err != nil {
		return nil, fmt.Errorf("save pre-rollback snapshot: %w", err)
	}

	changes := DiffSnapshots(currentSnap, target)

	rollbackSnap := &ParameterSnapshot{
		ID:        fmt.Sprintf("rollback-%d", time.Now().Unix()),
		Timestamp: time.Now(),
		Reason:    fmt.Sprintf("Rollback to snapshot %s: %s", id, reason),
		User:      user,
		Params:    target.Params,
		Changes:   changes,
	}
	if err := s.SaveSnapshot(rollbackSnap); err != nil {
		return nil, fmt.Errorf("save rollback snapshot: %w", err)
	}

	parametersConfig = target.Params

	return rollbackSnap, nil
}

// GetAuditLog returns snapshots with parameter modifications.
func (s *SnapshotStore) GetAuditLog() ([]ParameterSnapshot, error) {
	snaps, err := s.ListSnapshots()
	if err != nil {
		return nil, fmt.Errorf("list snapshots: %w", err)
	}

	var auditLog []ParameterSnapshot
	for _, snap := range snaps {
		if len(snap.Changes) > 0 || (snap.Reason != "" && snap.User != "") {
			auditLog = append(auditLog, snap)
		}
	}

	return auditLog, nil
}
