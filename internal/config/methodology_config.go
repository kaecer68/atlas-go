// Package config — methodology rules YAML loader.
//
// Loads configs/methodology_rules.yaml into a typed MethodologyRules struct
// following the same pattern as internal/llm/config.go (yaml.v3 + typed structs).
//
// Maturity: E (evolving)
package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// ─── YAML schema types ────────────────────────────────────────────────

// MethodologyRules is the root document of configs/methodology_rules.yaml.
type MethodologyRules struct {
	DetectionOrder []string         `yaml:"detection_order"`
	Regimes        []RegimeRule     `yaml:"regimes"`
	Strategies     []StrategyRule   `yaml:"strategies"`
	Transitions    []TransitionRule `yaml:"transitions"`
}

// RegimeRule defines a single market period's detection parameters and
// strategy mapping.
type RegimeRule struct {
	ID               string           `yaml:"id"`
	Name             string           `yaml:"name"`
	NameEn           string           `yaml:"name_en"`
	Description      string           `yaml:"description"`
	TriggerLogic     string           `yaml:"trigger_logic"`     // "all" or "any"
	TriggerThreshold int              `yaml:"trigger_threshold"` // for "any" logic
	Indicators       []IndicatorRule  `yaml:"indicators"`
	Strategies       StrategyMapping  `yaml:"strategies"`
	Macroflow        MacroflowMapping `yaml:"macroflow"`
	CashReservePct   float64          `yaml:"cash_reserve_pct"`
}

// IndicatorRule defines a single detection indicator for a regime.
type IndicatorRule struct {
	ID          string         `yaml:"id"`
	Field       string         `yaml:"field"`
	Description string         `yaml:"description"`
	Type        string         `yaml:"type"`
	Params      map[string]any `yaml:"params"`
}

// StrategyMapping maps primary and secondary strategy IDs for a regime.
type StrategyMapping struct {
	Primary   []string `yaml:"primary"`
	Secondary []string `yaml:"secondary"`
}

// MacroflowMapping defines the macroflow risk level and stress trigger.
type MacroflowMapping struct {
	RiskLevel        string  `yaml:"risk_level"`
	StressTriggerVIX float64 `yaml:"stress_trigger_vix"`
}

// StrategyRule defines a single strategy with its applicable regimes.
type StrategyRule struct {
	ID                string             `yaml:"id"`
	Name              string             `yaml:"name"`
	NameEn            string             `yaml:"name_en"`
	Description       string             `yaml:"description"`
	ApplicableRegimes []string           `yaml:"applicable_regimes"`
	FactorWeights     map[string]float64 `yaml:"factor_weights"`
	MaxExposurePct    float64            `yaml:"max_exposure_pct"`
	Category          string             `yaml:"category"`
}

// TransitionRule defines allowed period transitions.
type TransitionRule struct {
	From  string   `yaml:"from"`
	To    []string `yaml:"to"`
	Notes string   `yaml:"notes,omitempty"`
}

// ─── Loader ───────────────────────────────────────────────────────────

// LoadMethodologyRules reads and validates a methodology_rules.yaml file.
func LoadMethodologyRules(path string) (*MethodologyRules, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("config: read methodology rules %s: %w", path, err)
	}

	var rules MethodologyRules
	if err := yaml.Unmarshal(data, &rules); err != nil {
		return nil, fmt.Errorf("config: parse methodology rules %s: %w", path, err)
	}

	if len(rules.Regimes) == 0 {
		return nil, fmt.Errorf("config: methodology rules %s: regimes is empty", path)
	}
	if len(rules.Strategies) == 0 {
		return nil, fmt.Errorf("config: methodology rules %s: strategies is empty", path)
	}

	return &rules, nil
}

// TryLoadMethodologyRules loads methodology rules with a fallback.
// On any error, returns a minimal default config so the system always starts.
func TryLoadMethodologyRules(path string) *MethodologyRules {
	rules, err := LoadMethodologyRules(path)
	if err != nil {
		return defaultMethodologyRules()
	}
	return rules
}

// defaultMethodologyRules returns a minimal hardcoded fallback when the
// YAML file is missing or invalid. This ensures the system never crashes
// on config load failure.
func defaultMethodologyRules() *MethodologyRules {
	return &MethodologyRules{
		DetectionOrder: []string{"black_swan", "turnaround_down", "downturn", "turnaround_up", "bull", "plateau", "consolidation"},
		Regimes: []RegimeRule{
			{
				ID: "bull", Name: "上升／多頭",
				Strategies: StrategyMapping{
					Primary:   []string{"momentum"},
					Secondary: []string{"growth", "event_arbitrage"},
				},
				Macroflow:      MacroflowMapping{RiskLevel: "yellow"},
				CashReservePct: 7,
			},
			{
				ID: "downturn", Name: "低迷",
				Strategies: StrategyMapping{
					Primary:   []string{"all_weather"},
					Secondary: []string{"value"},
				},
				Macroflow:      MacroflowMapping{RiskLevel: "orange"},
				CashReservePct: 45,
			},
			{
				ID: "black_swan", Name: "黑天鵝",
				Strategies: StrategyMapping{
					Primary:   []string{"all_weather"},
					Secondary: []string{"cash_only"},
				},
				Macroflow:      MacroflowMapping{RiskLevel: "red"},
				CashReservePct: 90,
			},
		},
		Strategies: []StrategyRule{
			{ID: "all_weather", Name: "全天候防禦", Category: "defensive"},
			{ID: "value", Name: "價值投資", Category: "defensive"},
			{ID: "growth", Name: "成長動能", Category: "aggressive"},
			{ID: "momentum", Name: "動能追蹤", Category: "aggressive"},
			{ID: "event_arbitrage", Name: "事件套利", Category: "tactical"},
			{ID: "cash_only", Name: "現金為主", Category: "defensive"},
		},
	}
}

// ─── Query helpers ────────────────────────────────────────────────────

// GetAllowedStrategies returns the combined primary + secondary strategy
// IDs for a given period ID. Returns nil if the period is unknown.
func (r *MethodologyRules) GetAllowedStrategies(periodID string) []string {
	for _, regime := range r.Regimes {
		if regime.ID == periodID {
			result := make([]string, 0, len(regime.Strategies.Primary)+len(regime.Strategies.Secondary))
			result = append(result, regime.Strategies.Primary...)
			result = append(result, regime.Strategies.Secondary...)
			return result
		}
	}
	return nil
}

// GetCashReserve returns the recommended cash reserve percentage for a
// given period ID. Returns 20 as a safe default for unknown periods.
func (r *MethodologyRules) GetCashReserve(periodID string) float64 {
	for _, regime := range r.Regimes {
		if regime.ID == periodID {
			return regime.CashReservePct
		}
	}
	return 20
}

// GetMacroflowRiskLevel returns the macroflow risk level string for a
// given period ID. Returns "yellow" as default for unknown periods.
func (r *MethodologyRules) GetMacroflowRiskLevel(periodID string) string {
	for _, regime := range r.Regimes {
		if regime.ID == periodID {
			return regime.Macroflow.RiskLevel
		}
	}
	return "yellow"
}

// GetStrategyCategory returns the category (defensive/aggressive/tactical)
// for a given strategy ID. Returns "" for unknown strategies.
func (r *MethodologyRules) GetStrategyCategory(strategyID string) string {
	for _, s := range r.Strategies {
		if s.ID == strategyID {
			return s.Category
		}
	}
	return ""
}

// IsStrategyAllowed checks whether a strategy ID is in the allowed list
// for a given period.
func (r *MethodologyRules) IsStrategyAllowed(periodID, strategyID string) bool {
	allowed := r.GetAllowedStrategies(periodID)
	for _, id := range allowed {
		if id == strategyID {
			return true
		}
	}
	return false
}
