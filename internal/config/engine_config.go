package config

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// EngineConfig holds all engine parameters that were previously hard-coded
type EngineConfig struct {
	Version           string                  `json:"version"`
	Description       string                  `json:"description"`
	MacroRisk         MacroRiskConfig         `json:"macro_risk"`
	StructuralTrend   StructuralTrendConfig   `json:"structural_trend"`
	Drawdown          DrawdownConfig          `json:"drawdown"`
	SectorRotation    SectorRotationConfig    `json:"sector_rotation"`
	IndustryAnalysis  IndustryAnalysisConfig  `json:"industry_analysis"`
	StrategyEvolution StrategyEvolutionConfig `json:"strategy_evolution"`
	Executors         ExecutorsConfig         `json:"executors"`
	Simulation        SimulationConfig        `json:"simulation"`
}

// MacroRiskConfig holds macro risk assessment parameters
type MacroRiskConfig struct {
	CarryTradeUnwindThreshold float64 `json:"carry_trade_unwind_threshold"`
	VIXThreshold              float64 `json:"vix_threshold"`
	US10YThreshold            float64 `json:"us10y_threshold"`
	OilShockThresholdPct      float64 `json:"oil_shock_threshold_pct"`
	GoldSurgeThresholdPct     float64 `json:"gold_surge_threshold_pct"`
	DXYSurgeThresholdPct      float64 `json:"dxy_surge_threshold_pct"`
	TWDStressThresholdPct     float64 `json:"twd_stress_threshold_pct"`
	OutflowProbBase           float64 `json:"outflow_prob_base"`
	OutflowProbMax            float64 `json:"outflow_prob_max"`
}

// StructuralTrendConfig holds structural trend detection parameters
type StructuralTrendConfig struct {
	MinTrendStrength            float64 `json:"min_trend_strength"`
	MinConfidence               float64 `json:"min_confidence"`
	MinHitRate                  float64 `json:"min_hit_rate"`
	OverrideThreshold           float64 `json:"override_threshold"`
	AIRevenueGrowthThreshold    float64 `json:"ai_revenue_growth_threshold"`
	CoWoSUtilizationThreshold   float64 `json:"cowos_utilization_threshold"`
	CapexGrowthThreshold        float64 `json:"capex_growth_threshold"`
	SemiconductorIndexThreshold float64 `json:"semiconductor_index_threshold"`
}

// DrawdownConfig holds drawdown level parameters
type DrawdownConfig struct {
	Levels                            map[string]DrawdownLevel `json:"levels"`
	OrangeOverrideMinScore            float64                  `json:"orange_override_min_score"`
	RedOverrideMinScore               float64                  `json:"red_override_min_score"`
	SectorConstraintsRiskOff          map[string]float64       `json:"sector_constraints_risk_off"`
	SectorConstraintsCarryTradeUnwind map[string]float64       `json:"sector_constraints_carry_trade_unwind"`
	SectorConstraintsSectorRotation   map[string]float64       `json:"sector_constraints_sector_rotation"`
}

// DrawdownLevel represents a single drawdown level
type DrawdownLevel struct {
	Percentage  float64 `json:"percentage"`
	MaxExposure float64 `json:"max_exposure"`
}

// SectorRotationConfig holds sector rotation parameters
type SectorRotationConfig struct {
	BaseAllocations    map[string]float64 `json:"base_allocations"`
	MinAllocation      float64            `json:"min_allocation"`
	MaxAllocation      float64            `json:"max_allocation"`
	RebalanceThreshold float64            `json:"rebalance_threshold"`
}

// StrategyEvolutionConfig holds strategy evolution parameters
type StrategyEvolutionConfig struct {
	CooldownPeriodHours int                            `json:"cooldown_period_hours"`
	Configs             map[string]StrategyStateConfig `json:"configs"`
}

// StrategyStateConfig holds configuration for a strategy state
type StrategyStateConfig struct {
	MaxPositionSize    float64 `json:"max_position_size"`
	MaxSectorExposure  float64 `json:"max_sector_exposure"`
	MinCashReserve     float64 `json:"min_cash_reserve"`
	HedgeRatio         float64 `json:"hedge_ratio"`
	AllowNewPositions  bool    `json:"allow_new_positions"`
	AllowConcentration bool    `json:"allow_concentration"`
}

// ExecutorsConfig holds executor parameters
type ExecutorsConfig struct {
	VIXMomentumCrashThreshold float64 `json:"vix_momentum_crash_threshold"`
	CrowdingPenaltyAgents3    float64 `json:"crowding_penalty_agents_3"`
	CrowdingPenaltyAgents4    float64 `json:"crowding_penalty_agents_4"`
	MinTradeAmount            float64 `json:"min_trade_amount"`
	MaxStocksDefault          int     `json:"max_stocks_default"`
	MaxStocksMin              int     `json:"max_stocks_min"`
	MaxStocksMax              int     `json:"max_stocks_max"`
	ConvictionFloorDefault    int     `json:"conviction_floor_default"`
}

// SimulationConfig holds simulation parameters
type SimulationConfig struct {
	NeutralRegimeSizingFactor  float64 `json:"neutral_regime_sizing_factor"`
	StartingCashDefault        float64 `json:"starting_cash_default"`
	MaxPositionWeightDefault   float64 `json:"max_position_weight_default"`
	MaxOpenPositionsDefault    int     `json:"max_open_positions_default"`
	MinTradableVolumeDefault   int     `json:"min_tradable_volume_default"`
	TransactionCostBPSDefault  int     `json:"transaction_cost_bps_default"`
	SlippageBPSDefault         int     `json:"slippage_bps_default"`
	ReserveCashFractionDefault float64 `json:"reserve_cash_fraction_default"`
}

// IndustryAnalysisConfig holds industry analysis parameters
type IndustryAnalysisConfig struct {
	Classification  IndustryClassificationConfig `json:"classification"`
	Seasonality     SeasonalityConfig            `json:"seasonality"`
	CycleTracking   CycleTrackingConfig          `json:"cycle_tracking"`
	LinkageAnalysis LinkageAnalysisConfig        `json:"linkage_analysis"`
	RiskMonitoring  RiskMonitoringConfig         `json:"risk_monitoring"`
}

// IndustryClassificationConfig holds industry classification parameters
type IndustryClassificationConfig struct {
	Version            string   `json:"version"`
	Levels             int      `json:"levels"`
	TopLevelIndustries []string `json:"top_level_industries"`
}

// SeasonalityConfig holds seasonality analysis parameters
type SeasonalityConfig struct {
	Enabled  bool     `json:"enabled"`
	Patterns []string `json:"patterns"`
}

// CycleTrackingConfig holds cycle tracking parameters
type CycleTrackingConfig struct {
	Enabled             bool    `json:"enabled"`
	UpdateFrequencyDays int     `json:"update_frequency_days"`
	ConfidenceThreshold float64 `json:"confidence_threshold"`
}

// LinkageAnalysisConfig holds supply chain linkage parameters
type LinkageAnalysisConfig struct {
	Enabled               bool `json:"enabled"`
	CorrelationWindowDays int  `json:"correlation_window_days"`
	MaxPropagationDepth   int  `json:"max_propagation_depth"`
}

// RiskMonitoringConfig holds risk monitoring parameters
type RiskMonitoringConfig struct {
	Enabled                        bool    `json:"enabled"`
	CustomerConcentrationThreshold float64 `json:"customer_concentration_threshold"`
	USExposureWarningThreshold     float64 `json:"us_exposure_warning_threshold"`
	NewsLatencyHoursWarning        float64 `json:"news_latency_hours_warning"`
	AsymmetricBadNewsThreshold     float64 `json:"asymmetric_bad_news_threshold"`
	VolumeSpikeMultiplier          float64 `json:"volume_spike_multiplier"`
}

//go:embed engine.json
var defaultEngineJSON []byte

var (
	// engineConfig is the singleton instance
	engineConfig *EngineConfig
	// configPath is the path to the engine config file
	configPath = envOr("ATLAS_ENGINE_CONFIG", "engine.json")
)

// LoadEngineConfig loads the engine configuration from file
func LoadEngineConfig() (*EngineConfig, error) {
	if engineConfig != nil {
		return engineConfig, nil
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read engine config: %w", err)
	}

	var cfg EngineConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse engine config: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid engine config: %w", err)
	}

	engineConfig = &cfg
	return engineConfig, nil
}

// GetEngineConfig returns the loaded engine configuration
func GetEngineConfig() *EngineConfig {
	if engineConfig == nil {
		cfg, err := LoadEngineConfig()
		if err != nil {
			// Return default config if loading fails
			return defaultEngineConfig()
		}
		return cfg
	}
	return engineConfig
}

// ReloadEngineConfig reloads the engine configuration from file
func ReloadEngineConfig() (*EngineConfig, error) {
	engineConfig = nil
	return LoadEngineConfig()
}

// Validate checks if the configuration is valid
func (c *EngineConfig) Validate() error {
	if c.Version == "" {
		return fmt.Errorf("version is required")
	}

	// Validate macro risk thresholds
	if c.MacroRisk.CarryTradeUnwindThreshold <= 0 {
		return fmt.Errorf("carry_trade_unwind_threshold must be positive")
	}
	if c.MacroRisk.VIXThreshold <= 0 {
		return fmt.Errorf("vix_threshold must be positive")
	}

	// Validate drawdown levels
	for name, level := range c.Drawdown.Levels {
		if level.Percentage < 0 || level.Percentage > 1 {
			return fmt.Errorf("drawdown level %s: percentage must be between 0 and 1", name)
		}
		if level.MaxExposure < 0 || level.MaxExposure > 1 {
			return fmt.Errorf("drawdown level %s: max_exposure must be between 0 and 1", name)
		}
	}

	// Validate sector rotation
	total := 0.0
	for _, alloc := range c.SectorRotation.BaseAllocations {
		total += alloc
	}
	if total < 0.99 || total > 1.01 {
		return fmt.Errorf("base_allocations must sum to 1.0, got %.2f", total)
	}

	return nil
}

func defaultEngineConfig() *EngineConfig {
	var cfg EngineConfig
	if err := json.Unmarshal(defaultEngineJSON, &cfg); err != nil {
		panic("embedded engine.json is invalid: " + err.Error())
	}
	return &cfg
}

// GetCooldownDuration returns the cooldown period as time.Duration
func (c *StrategyEvolutionConfig) GetCooldownDuration() time.Duration {
	return time.Duration(c.CooldownPeriodHours) * time.Hour
}
