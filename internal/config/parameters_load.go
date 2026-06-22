package config

import (
	"encoding/json"
	"fmt"
	"os"
)

// LoadParametersConfig loads parameters from the given JSON file.
// If the file does not exist or is invalid, it returns the default configuration.
func LoadParametersConfig(path string) (*ParametersConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultParametersConfig(), nil
		}
		return nil, fmt.Errorf("read parameters config: %w", err)
	}

	var cfg ParametersConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse parameters config: %w", err)
	}

	// Merge defaults before validation so newly-added fields (missing from
	// the saved JSON) receive valid values instead of zero.
	mergeNarrativeDefaults(&cfg)
	mergeDrawdownDefaults(&cfg)
	mergeAlertDefaults(&cfg)
	mergeRiskGateDefaults(&cfg)
	mergeEngineDefaults(&cfg)
	mergeSectorExecutorDefaults(&cfg)
	mergeIndustryDefaults(&cfg)
	mergeRSITwDefaults(&cfg)
	mergeFallbackPriceTargetsDefaults(&cfg)
	mergeReportingDefaults(&cfg)

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validate parameters config: %w", err)
	}

	return &cfg, nil
}

// GetParametersConfig returns the singleton parameters configuration.
func GetParametersConfig() *ParametersConfig {
	if parametersConfig == nil {
		cfg, err := LoadParametersConfig(parametersPath)
		if err != nil {
			return DefaultParametersConfig()
		}
		// Defaults are already merged inside LoadParametersConfig.
		parametersConfig = cfg
	}
	return parametersConfig
}
