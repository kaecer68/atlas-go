package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

var validSourceTypes = map[string]bool{
	"heuristic":  true,
	"empirical":  true,
	"literature": true,
	"inferred":   true,
	"calibrated": true,
}

// TestParametersStubsFilled walks the entire parameters.json tree and
// verifies every parameter block has non-empty "rationale" and "source"
// fields. New parameters must include documentation.
func TestParametersStubsFilled(t *testing.T) {
	raw := loadParametersJSON(t)
	checkAllStubs(t, raw, "")
}

// TestCentralBankBuyingTrendSourceTypeValid verifies the
// central_bank_buying_trend citation uses a valid source_type.
func TestCentralBankBuyingTrendSourceTypeValid(t *testing.T) {
	raw := loadParametersJSON(t)
	params := walkTo(raw, "precious_metals", "central_bank_buying_trend", "citation")
	if params == nil {
		t.Skip("macro.central_bank_buying_trend.citation not available")
	}
	st, _ := params["source_type"].(string)
	if !validSourceTypes[st] {
		t.Errorf("central_bank_buying_trend.citation.source_type = %q; want one of [heuristic, empirical, literature, inferred, calibrated]", st)
	}
}

// TestAllSourceTypesValid verifies all citation.source_type values
// in the parameters file are valid enum values.
func TestAllSourceTypesValid(t *testing.T) {
	raw := loadParametersJSON(t)
	walkSourceTypes(t, raw, "")
}

func loadParametersJSON(t *testing.T) map[string]any {
	t.Helper()
	path := findParametersPath(t)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("parameters.json not readable: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("invalid JSON in parameters.json: %v", err)
	}
	return raw
}

func findParametersPath(t *testing.T) string {
	t.Helper()
	for _, dir := range []string{".", "..", "../..", "../../..", "../../../.."} {
		candidate := filepath.Join(dir, "configs", "parameters.json")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	t.Skip("configs/parameters.json not found — run from repo root or internal/config/")
	return ""
}

// checkAllStubs recursively walks the JSON tree and checks every map that
// has a "rationale" key for a non-empty value (leaf parameter blocks).
func checkAllStubs(t *testing.T, m map[string]any, path string) {
	t.Helper()
	for k, v := range m {
		cur := k
		if path != "" {
			cur = path + "." + k
		}
		switch val := v.(type) {
		case map[string]any:
			// If this map has a "rationale" key, it's a parameter block.
			if _, hasRationale := val["rationale"]; hasRationale {
				rationale, _ := val["rationale"].(string)
				if rationale == "" {
					t.Errorf("%s.rationale is empty", cur)
				}
				source, _ := val["source"].(string)
				if source == "" {
					t.Errorf("%s.source is empty", cur)
				}
			}
			checkAllStubs(t, val, cur)
		}
	}
}

// walkTo navigates into a nested sequence of map keys and returns the
// final map, or nil if any key is missing.
func walkTo(m map[string]any, keys ...string) map[string]any {
	for _, k := range keys {
		child, ok := m[k].(map[string]any)
		if !ok {
			return nil
		}
		m = child
	}
	return m
}

func walkSourceTypes(t *testing.T, raw map[string]any, prefix string) {
	for k, v := range raw {
		path := k
		if prefix != "" {
			path = prefix + "." + k
		}
		switch val := v.(type) {
		case map[string]any:
			// Only check source_type when it's a string value, not when
			// "source_type" happens to be the key of a nested object.
			st, ok := val["source_type"].(string)
			if ok && !validSourceTypes[st] {
				t.Errorf("%s.source_type = %q; want one of [heuristic, empirical, literature, inferred, calibrated]", path, st)
			}
			if _, hasChildren := val["children"]; !hasChildren {
				walkSourceTypes(t, val, path)
			}
		}
	}
}
