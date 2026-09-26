package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ParametersSource is the SSOT document a ParametersConfig was parsed from.
type ParametersSource struct {
	// Raw is the JSON document the config was parsed from. It is nil when the
	// SSOT file does not exist (the config is then the built-in default).
	Raw []byte
	// Config is the parsed, defaults-merged, validated configuration.
	Config *ParametersConfig
	// Dir is true when the SSOT is a directory of per-category files.
	Dir bool
}

// LoadParametersConfig loads parameters from the given path.
// If path is a directory, loads from configs/parameters/<category>.json files.
// If path is a file, loads the single JSON file (backward compatible).
// If neither exists, returns the default configuration.
//
// This is the raw SSOT read: it never applies the calibrated overlay. Callers
// that need the configuration the process runs on must use
// LoadEffectiveParametersConfig (FU-20260926-07).
func LoadParametersConfig(path string) (*ParametersConfig, error) {
	_, cfg, _, err := loadParametersSource(path)
	return cfg, err
}

// LoadParametersSource returns the raw SSOT document together with the config
// parsed from it. The raw bytes are what dotted-path overlay entries patch.
func LoadParametersSource(path string) (*ParametersSource, error) {
	raw, cfg, dir, err := loadParametersSource(path)
	if err != nil {
		return nil, err
	}
	return &ParametersSource{Raw: raw, Config: cfg, Dir: dir}, nil
}

// loadParametersSource reads and parses the SSOT at path, returning the raw
// document as well. A missing file is not an error: it yields the built-in
// default configuration and a nil document.
func loadParametersSource(path string) (raw []byte, cfg *ParametersConfig, dir bool, err error) {
	info, statErr := os.Stat(path)
	if statErr != nil {
		if os.IsNotExist(statErr) {
			return nil, DefaultParametersConfig(), false, nil
		}
		return nil, nil, false, fmt.Errorf("read parameters config: %w", statErr)
	}

	if info.IsDir() {
		parametersConfigDir = path
		merged, mergeErr := marshalParametersDir(path)
		if mergeErr != nil {
			return nil, nil, true, mergeErr
		}
		cfg, parseErr := parseParametersBytes(merged)
		if parseErr != nil {
			return nil, nil, true, parseErr
		}
		return merged, cfg, true, nil
	}

	parametersConfigDir = "" // not in directory mode
	data, readErr := os.ReadFile(path)
	if readErr != nil {
		if os.IsNotExist(readErr) {
			return nil, DefaultParametersConfig(), false, nil
		}
		return nil, nil, false, fmt.Errorf("read parameters config: %w", readErr)
	}
	cfg, parseErr := parseParametersBytes(data)
	if parseErr != nil {
		return nil, nil, false, parseErr
	}
	return data, cfg, false, nil
}

// parseParametersBytes parses, merges defaults into, and validates a parameters
// document.
func parseParametersBytes(data []byte) (*ParametersConfig, error) {
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

// loadParametersFile loads from a single JSON file (legacy mode).
func loadParametersFile(path string) (*ParametersConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultParametersConfig(), nil
		}
		return nil, fmt.Errorf("read parameters config: %w", err)
	}
	return parseParametersBytes(data)
}

// loadParametersDir loads from a directory of per-category JSON files.
// Each file is named <category>.json (e.g. darwinian.json, factor.json).
// _meta.json carries version + updated_at.
func loadParametersDir(dir string) (*ParametersConfig, error) {
	merged, err := marshalParametersDir(dir)
	if err != nil {
		return nil, err
	}
	return parseParametersBytes(merged)
}

// marshalParametersDir merges the per-category files of a parameters directory
// into one JSON document.
func marshalParametersDir(dir string) ([]byte, error) {
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
	return merged, nil
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
	mergeStockpickerDefaults(cfg)
}

// GetParametersConfig returns the singleton parameters configuration.
//
// The singleton is the configuration the process runs on, so it carries the
// calibrated overlay (FU-20260926-07): LoadParametersConfig reads the SSOT file,
// ApplyCalibratedOverlayLayer layers the runtime adaptations on top. When no
// overlay path is registered the result is byte-for-byte the SSOT read.
func GetParametersConfig() *ParametersConfig {
	if parametersConfig == nil {
		cfg, err := LoadEffectiveParametersConfig(parametersPath)
		if err != nil {
			return DefaultParametersConfig()
		}
		parametersConfig = cfg
	}
	return parametersConfig
}
