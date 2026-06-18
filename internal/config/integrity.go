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
		raw, ok := obj[key]
		if !ok {
			errs = append(errs, fmt.Errorf("params.json missing required top-level key: %s", key))
			continue
		}

		var nested map[string]json.RawMessage
		if err := json.Unmarshal(raw, &nested); err != nil {
			// version and updated_at are scalar metadata fields; any JSON type is allowed.
			if key == "version" || key == "updated_at" {
				continue
			}

			var val interface{}
			if json.Unmarshal(raw, &val) == nil {
				switch val.(type) {
				case string:
					errs = append(errs, fmt.Errorf("params.json key '%s' has type string, expected object", key))
				case float64:
					errs = append(errs, fmt.Errorf("params.json key '%s' has type number, expected object", key))
				case bool:
					errs = append(errs, fmt.Errorf("params.json key '%s' has type bool, expected object", key))
				case []interface{}:
					errs = append(errs, fmt.Errorf("params.json key '%s' has type array, expected object", key))
				case nil:
					errs = append(errs, fmt.Errorf("params.json key '%s' has type null, expected object", key))
				default:
					errs = append(errs, fmt.Errorf("params.json key '%s' has type unknown, expected object", key))
				}
			} else {
				errs = append(errs, fmt.Errorf("params.json key '%s' has type unknown, expected object", key))
			}
			continue
		}

		if nested == nil {
			errs = append(errs, fmt.Errorf("params.json key '%s' is null", key))
			continue
		}

		if len(nested) == 0 {
			errs = append(errs, fmt.Errorf("params.json key '%s' is empty object", key))
		}
	}

	if errs == nil {
		return []error{}
	}
	return errs
}
