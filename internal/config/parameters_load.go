package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// LoadParametersConfig loads parameters from the given path.
// If path is a directory, loads from configs/parameters/<category>.json files.
// If path is a file, loads the single JSON file (backward compatible).
// If neither exists, returns the default configuration.
func LoadParametersConfig(path string) (*ParametersConfig, error) {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultParametersConfig(), nil
		}
		return nil, fmt.Errorf("read parameters config: %w", err)
	}

	if info.IsDir() {
		return loadParametersDir(path)
	}
	return loadParametersFile(path)
}

// loadParametersFile loads from a single JSON file (legacy mode).
func loadParametersFile(path string) (*ParametersConfig, error) {
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

	mergeAllDefaults(&cfg)

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validate parameters config: %w", err)
	}

	return &cfg, nil
}

// loadParametersDir loads from a directory of per-category JSON files.
// Each file is named <category>.json (e.g. darwinian.json, factor.json).
// _meta.json carries version + updated_at.
func loadParametersDir(dir string) (*ParametersConfig, error) {
	data := make(map[string]json.RawMessage)

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read parameters dir %s: %w", dir, err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		cat := strings.TrimSuffix(entry.Name(), ".json")

		fileData, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", entry.Name(), err)
		}
		data[cat] = fileData
	}

	// Build merged JSON: each category becomes a top-level key.
	var merged []byte
	merged = append(merged, '{')
	first := true
	for cat, raw := range data {
		if !first {
			merged = append(merged, ',')
		}
		first = false
		merged = append(merged, fmt.Sprintf(`"%s":`, cat)...)
		merged = append(merged, raw...)
	}
	merged = append(merged, '}')

	var cfg ParametersConfig
	if err := json.Unmarshal(merged, &cfg); err != nil {
		return nil, fmt.Errorf("parse merged parameters: %w", err)
	}

	mergeAllDefaults(&cfg)

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validate parameters config: %w", err)
	}

	return &cfg, nil
}

// mergeAllDefaults applies all category-level default merges.
func mergeAllDefaults(cfg *ParametersConfig) {
	mergeNarrativeDefaults(cfg)
	mergeDrawdownDefaults(cfg)
	mergeAlertDefaults(cfg)
	mergeRiskGateDefaults(cfg)
	mergeEngineDefaults(cfg)
	mergeSectorExecutorDefaults(cfg)
	mergeIndustryDefaults(cfg)
	mergeRSITwDefaults(cfg)
	mergeFallbackPriceTargetsDefaults(cfg)
	mergeReportingDefaults(cfg)
	mergeDarwinianDefaults(cfg)
	mergeFactorDefaults(cfg)
	mergeOptimizerDefaults(cfg)
	mergeSizingDefaults(cfg)
	mergeExperimentDefaults(cfg)
	mergeBaselineDefaults(cfg)
	mergeRiskDefaults(cfg)
	mergeFactorWeightDefaults(cfg)
	mergeHealthDefaults(cfg)
	mergeGARCHDefaults(cfg)
	mergeOrchestratorDefaults(cfg)
	mergeStrategyDefaults(cfg)
	mergeJanusDefaults(cfg)
	mergeMarketdataDefaults(cfg)
	mergeRealtimeDefaults(cfg)
	mergeNarrativeConvictionDefaults(cfg)
	mergePreciousMetalsDefaults(cfg)
	mergeForwardReturnDefaults(cfg)
	mergeTaxDefaults(cfg)
	mergeSectorAllocationDefaults(cfg)
	mergeSmartUniverseDefaults(cfg)
	mergeCapitalflowDefaults(cfg)
}

// GetParametersConfig returns the singleton parameters configuration.
func GetParametersConfig() *ParametersConfig {
	if parametersConfig == nil {
		cfg, err := LoadParametersConfig(parametersPath)
		if err != nil {
			return DefaultParametersConfig()
		}
		parametersConfig = cfg
	}
	return parametersConfig
}
