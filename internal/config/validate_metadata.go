// SPDX-License-Identifier: AGPL-3.0

package config

import (
	"fmt"
	"io"
	"strings"
)

// CountParameterMetadataSections returns the total number of ParameterMetadata
// sections found anywhere inside the JSON config tree.
func CountParameterMetadataSections(config map[string]any) int {
	count := 0
	var walk func(v any)
	walk = func(v any) {
		switch obj := v.(type) {
		case map[string]any:
			if isParameterMetadataSection(obj) {
				count++
			}
			for _, val := range obj {
				walk(val)
			}
		case []any:
			for _, val := range obj {
				walk(val)
			}
		}
	}
	walk(config)
	return count
}

// isParameterMetadataSection decides whether an object is a ParameterMetadata section.
func isParameterMetadataSection(obj map[string]any) bool {
	// Skip citation sub-objects — they're part of ParameterMetadata,
	// not standalone sections. Citations can have calibration_method
	// (set by calibrators) but the timestamp belongs to the parent.
	if _, hasValue := obj["value"]; !hasValue {
		// ParameterMetadata always wraps a "value" field.
		// Objects without "value" that have calibration_method
		// are citation sub-objects, not ParameterMetadata sections.
		if _, hasCM := obj["calibration_method"].(string); hasCM {
			return false
		}
	}

	calMethod, hasMethod := obj["calibration_method"].(string)
	if hasMethod && calMethod != "" {
		return true
	}
	if ts, ok := obj["last_calibrated"]; ok && ts != nil && ts != "" {
		return true
	}
	if ts, ok := obj["calibration_timestamp"]; ok && ts != nil && ts != "" {
		return true
	}
	return false
}

// WalkParameterMetadataTree recursively walks a JSON config tree and validates
// every ParameterMetadata section it finds.
func WalkParameterMetadataTree(v any, path string, strict bool) (warnings, errors []string) {
	switch obj := v.(type) {
	case map[string]any:
		// Check if this object is a ParameterMetadata
		if isParameterMetadataSection(obj) {
			w, e := CheckParameterMetadataSection(obj, path, strict)
			warnings = append(warnings, w...)
			errors = append(errors, e...)
		}
		// Recurse into children (skip "value" which is the actual param data)
		for key, val := range obj {
			childPath := path
			if childPath == "" {
				childPath = key
			} else {
				childPath = childPath + "." + key
			}
			w, e := WalkParameterMetadataTree(val, childPath, strict)
			warnings = append(warnings, w...)
			errors = append(errors, e...)
		}

	case []any:
		for i, val := range obj {
			childPath := fmt.Sprintf("%s[%d]", path, i)
			w, e := WalkParameterMetadataTree(val, childPath, strict)
			warnings = append(warnings, w...)
			errors = append(errors, e...)
		}
	}
	return warnings, errors
}

// CheckParameterMetadataSection validates a single ParameterMetadata object.
func CheckParameterMetadataSection(obj map[string]any, path string, strict bool) (warnings, errors []string) {
	cite, hasCite := obj["citation"].(map[string]any)
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

	calMethod, hasCalMethod := obj["calibration_method"].(string)
	if hasCalMethod && calMethod == "" {
		hasCalMethod = false
	}

	hasTimestamp := false
	if ts, ok := obj["last_calibrated"]; ok && ts != nil && ts != "" {
		hasTimestamp = true
	}
	if !hasTimestamp {
		if ts, ok := obj["calibration_timestamp"]; ok && ts != nil && ts != "" {
			hasTimestamp = true
		}
	}

	// Rule 1: evidence_quality high/medium -> must have timestamp
	if (eq == "high" || eq == "medium") && !hasTimestamp {
		errors = append(errors, fmt.Sprintf(
			"%s: citation.evidence_quality=%q but no calibration timestamp (need last_calibrated or calibration_timestamp)",
			path, eq,
		))
	}

	// Rule 2: calibration_method set -> must have timestamp
	if hasCalMethod && !hasTimestamp {
		errors = append(errors, fmt.Sprintf(
			"%s: calibration_method=%q but no calibration timestamp",
			path, calMethod,
		))
	}

	// Rule 3: timestamp exists but evidence_quality is low/heuristic
	if hasTimestamp && eq != "" && eq != "high" && eq != "medium" {
		msg := fmt.Sprintf(
			"%s: calibration timestamp exists but citation.evidence_quality=%q (expected 'high' or 'medium' after calibration)",
			path, eq,
		)
		if strict {
			errors = append(errors, msg)
		} else {
			warnings = append(warnings, msg)
		}
	}

	// Rule 4 (strict only): synthetic data source with real data reference
	ds, hasDS := obj["calibration_data_source"].(string)
	if strict && hasDS && ds == "synthetic" && citeRef != "" && !strings.Contains(citeRef, "synthetic") {
		warnings = append(warnings, fmt.Sprintf(
			"%s: calibration_data_source='synthetic' but citation.source_reference=%q — re-run calibrator with --replay for real data",
			path, citeRef,
		))
	}

	return warnings, errors
}

// ValidateAndReportParameterMetadata walks the raw JSON config tree, validates
// every ParameterMetadata section, and writes warnings/errors to the provided
// writers. It returns the exit code for the validate-parameters command.
func ValidateAndReportParameterMetadata(config map[string]any, path string, strict bool, stdout, stderr io.Writer) int {
	warnings, errors := WalkParameterMetadataTree(config, "", strict)
	for _, w := range warnings {
		_, _ = fmt.Fprintf(stderr, "WARN: %s\n", w)
	}
	for _, e := range errors {
		_, _ = fmt.Fprintf(stderr, "FAIL: %s\n", e)
	}
	if len(errors) > 0 {
		_, _ = fmt.Fprintf(stdout, "\n%d error(s), %d warning(s)\n", len(errors), len(warnings))
		return 1
	}
	if strict && len(warnings) > 0 {
		_, _ = fmt.Fprintf(stdout, "\n%d warning(s) (strict mode)\n", len(warnings))
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "OK: %s is valid (%d sections checked)\n", path, CountParameterMetadataSections(config))
	return 0
}
