package config

import (
	"encoding/json"
	"fmt"
	"os"
)

// requiredParamsKeys lists the top-level keys that must be present in params.json.
// These correspond to the non-omitempty fields on ParametersConfig.
var requiredParamsKeys = []string{
	"version",
	"updated_at",
	"darwinian",
	"factor",
	"optimizer",
	"sizing",
	"health",
	"garch",
	"experiment",
	"baseline",
	"orchestrator",
	"risk",
	"drawdown",
	"realtime",
	"janus",
	"narrative",
	"marketdata",
	"industry",
	"strategy",
	"precious_metals",
	"alert",
	"sector_allocation",
	"reporting",
}

// CheckParamsIntegrity validates that the file at path is a non-empty, valid JSON
// object containing all required top-level keys for ParametersConfig.
// It returns a slice of errors; an empty (non-nil) slice means the file passed.
func CheckParamsIntegrity(path string) []error {
	data, err := os.ReadFile(path)
	if err != nil {
		return []error{fmt.Errorf("read params.json: %w", err)}
	}

	if len(data) == 0 {
		return []error{fmt.Errorf("params.json is empty")}
	}

	var obj map[string]json.RawMessage
	if err := json.Unmarshal(data, &obj); err != nil {
		return []error{fmt.Errorf("invalid JSON in params.json: %w", err)}
	}

	var errs []error
	for _, key := range requiredParamsKeys {
		if _, ok := obj[key]; !ok {
			errs = append(errs, fmt.Errorf("params.json missing required top-level key: %s", key))
		}
	}

	if errs == nil {
		return []error{}
	}
	return errs
}
