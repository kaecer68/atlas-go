// validate-parameters checks configs/parameters.json for data integrity violations.
//
// Usage:
//
//	go run ./cmd/validate-parameters                        # basic check
//	go run ./cmd/validate-parameters --strict                # strict mode (exit 1 on warnings too)
//	go run ./cmd/validate-parameters path/to/params.json     # custom file path
package main

import (
	"encoding/json"
	"fmt"
	"os"
)

func main() {
	path := "configs/parameters.json"
	strict := false
	for _, arg := range os.Args[1:] {
		if arg == "--strict" {
			strict = true
		} else {
			path = arg
		}
	}

	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: cannot read %s: %v\n", path, err)
		os.Exit(1)
	}

	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: invalid JSON in %s: %v\n", path, err)
		os.Exit(1)
	}

	warnings, errors := validateParameters(config, strict)

	for _, w := range warnings {
		fmt.Fprintf(os.Stderr, "WARN: %s\n", w)
	}
	for _, e := range errors {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", e)
	}

	if len(errors) > 0 || (strict && len(warnings) > 0) {
		os.Exit(1)
	}

	if strict {
		fmt.Println("OK: parameters.json passes strict validation")
	} else {
		fmt.Println("OK: parameters.json is valid")
	}
}

func validateParameters(config map[string]any, strict bool) (warnings, errors []string) {
	industryCfg, ok := config["industry"].(map[string]any)
	if !ok {
		return warnings, errors // industry section not required
	}

	sp, ok := industryCfg["seasonal_patterns"].(map[string]any)
	if !ok {
		return warnings, errors // seasonal_patterns not required
	}

	// Extract calibration evidence fields
	cite, hasCite := sp["citation"].(map[string]any)
	eq := ""
	var citeRef string
	if hasCite {
		if q, ok := cite["evidence_quality"].(string); ok {
			eq = q
		}
		if r, ok := cite["source_reference"].(string); ok {
			citeRef = r
		}
	}

	calMethod, hasCalMethod := sp["calibration_method"].(string)
	if hasCalMethod && calMethod == "" {
		hasCalMethod = false
	}

	// Check timestamp exists (either format)
	hasTimestamp := false
	if ts, ok := sp["last_calibrated"]; ok && ts != nil && ts != "" {
		hasTimestamp = true
	}
	if !hasTimestamp {
		if ts, ok := sp["calibration_timestamp"]; ok && ts != nil && ts != "" {
			hasTimestamp = true
		}
	}

	// Rule 1: evidence_quality high/medium → must have timestamp
	if (eq == "high" || eq == "medium") && !hasTimestamp {
		errors = append(errors, fmt.Sprintf(
			"industry.seasonal_patterns: citation.evidence_quality=%q but no calibration timestamp (need last_calibrated or calibration_timestamp)",
			eq))
	}

	// Rule 2: calibration_method set → must have timestamp
	if hasCalMethod && !hasTimestamp {
		errors = append(errors, fmt.Sprintf(
			"industry.seasonal_patterns: calibration_method=%q but no calibration timestamp",
			calMethod))
	}

	// Rule 3: timestamp exists but evidence_quality is low/heuristic → flagged in strict mode
	if hasTimestamp && eq != "high" && eq != "medium" {
		msg := fmt.Sprintf(
			"industry.seasonal_patterns: calibration timestamp exists but citation.evidence_quality=%q (expected 'high' or 'medium' after calibration)",
			eq)
		if strict {
			errors = append(errors, msg)
		} else {
			warnings = append(warnings, msg)
		}
	}

	// Rule 4 (strict only): synthetic data source with real data reference
	ds, hasDS := sp["calibration_data_source"].(string)
	if strict && hasDS && ds == "synthetic" && citeRef != "" {
		warnings = append(warnings, fmt.Sprintf(
			"industry.seasonal_patterns: calibration_data_source='synthetic' but citation.source_reference=%q — re-run calibrator with --replay for real data",
			citeRef))
	}

	return warnings, errors
}
