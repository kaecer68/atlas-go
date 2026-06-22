package config

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// TestParametersConfig_RoundTrip_Default verifies the S4 contract: DefaultParametersConfig()
// round-trips through JSON marshal/unmarshal + LoadParametersConfig with no data loss,
// modulo the updated_at field (which Save/LoadParametersConfig always refreshes).
//
// S4 contract: DefaultParametersConfig() → JSON → LoadParametersConfig() produces a
// config deep-equal to the input modulo updated_at.
func TestParametersConfig_RoundTrip_Default(t *testing.T) {
	// Step 1: Get the original default config.
	original := DefaultParametersConfig()

	// Step 2: Marshal to JSON (simulates what Save() does internally).
	// Use json.Marshal (compact) so we control the canonical form.
	jsonData, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal default config to JSON: %v", err)
	}

	// Step 3: Write to a temp file (simulates Save() writing to disk).
	tmpDir := t.TempDir()
	paramsPath := filepath.Join(tmpDir, "roundtrip.json")
	if err := os.WriteFile(paramsPath, jsonData, 0o644); err != nil {
		t.Fatalf("write JSON to temp file: %v", err)
	}

	// Step 4: Load through LoadParametersConfig (the actual consumer).
	loaded, err := LoadParametersConfig(paramsPath)
	if err != nil {
		t.Fatalf("LoadParametersConfig failed: %v", err)
	}

	// Step 5: Compare field-by-field, excluding UpdatedAt.
	// UpdatedAt is explicitly excluded: Save() always sets UpdatedAt = time.Now()
	// before writing, so the round-tripped config will have a different timestamp.
	// The S4 contract explicitly allows this.
	if !configDeepEqualExcluding(original, loaded, "UpdatedAt") {
		t.Errorf("round-tripped config does not match original (modulo UpdatedAt)\n"+
			"original.UpdatedAt=%v\nloaded.UpdatedAt=%v",
			original.UpdatedAt, loaded.UpdatedAt)
	}
}

// TestParametersConfig_RoundTrip_EmptyJSON documents the behavior when an empty JSON
// object {} is round-tripped through LoadParametersConfig. An empty ParametersConfig{}
// will have all zero-valued fields; LoadParametersConfig merges defaults before
// validation, so the result will be the full default config (not an error).
func TestParametersConfig_RoundTrip_EmptyJSON(t *testing.T) {
	// Start with an empty struct.
	emptyCfg := &ParametersConfig{}

	// Marshal the empty config.
	jsonData, err := json.Marshal(emptyCfg)
	if err != nil {
		t.Fatalf("marshal empty config: %v", err)
	}

	// Write to temp file.
	tmpDir := t.TempDir()
	paramsPath := filepath.Join(tmpDir, "empty.json")
	if err := os.WriteFile(paramsPath, jsonData, 0o644); err != nil {
		t.Fatalf("write empty JSON: %v", err)
	}

	// Load through LoadParametersConfig.
	loaded, err := LoadParametersConfig(paramsPath)
	if err != nil {
		t.Fatalf("LoadParametersConfig should not fail on empty JSON (merges defaults): %v", err)
	}

	// After merging defaults, loaded should be equivalent to DefaultParametersConfig().
	defaultCfg := DefaultParametersConfig()
	if !configDeepEqualExcluding(loaded, defaultCfg, "UpdatedAt") {
		t.Errorf("empty JSON should merge to full defaults after LoadParametersConfig\n" +
			"loaded differs from DefaultParametersConfig() (modulo UpdatedAt)")
	}
}

// TestParametersConfig_RoundTrip_CanonicalJSON verifies that DefaultParametersConfig()
// serializes to a stable, human-readable JSON form. Go's json.Marshal produces
// consistent output for primitive types; the only source of non-determinism is
// map key iteration order. This test documents the canonical form and confirms
// that re-marshaling the same config produces identical output.
func TestParametersConfig_RoundTrip_CanonicalJSON(t *testing.T) {
	cfg := DefaultParametersConfig()

	// First marshal.
	buf1 := &bytes.Buffer{}
	enc := json.NewEncoder(buf1)
	enc.SetIndent("", "  ")
	if err := enc.Encode(cfg); err != nil {
		t.Fatalf("first marshal: %v", err)
	}

	// Unmarshal then re-marshal.
	var cfg2 ParametersConfig
	if err := json.Unmarshal(buf1.Bytes(), &cfg2); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	buf2 := &bytes.Buffer{}
	enc2 := json.NewEncoder(buf2)
	enc2.SetIndent("", "  ")
	if err := enc2.Encode(&cfg2); err != nil {
		t.Fatalf("second marshal: %v", err)
	}

	// After unmarshal + re-marshal the config (without writing to a file),
	// the JSON output should be identical. This confirms that all custom
	// MarshalJSON/UnmarshalJSON implementations are self-consistent.
	if !bytes.Equal(buf1.Bytes(), buf2.Bytes()) {
		t.Errorf("JSON output changed after unmarshal+remarshal\n"+
			"first:  %d bytes\nsecond: %d bytes", len(buf1.Bytes()), len(buf2.Bytes()))
	}
}

// configDeepEqualExcluding compares two ParametersConfig values for deep equality,
// skipping the field named by excludeField (typically "UpdatedAt").
// Returns true if all fields match except excludeField.
func configDeepEqualExcluding(a, b *ParametersConfig, excludeField string) bool {
	va := reflect.ValueOf(a).Elem()
	vb := reflect.ValueOf(b).Elem()
	typ := va.Type()

	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		if field.Name == excludeField {
			continue
		}
		if !reflect.DeepEqual(va.Field(i).Interface(), vb.Field(i).Interface()) {
			return false
		}
	}
	return true
}

// BenchmarkParametersConfig_RoundTrip measures the cost of a full round-trip:
// DefaultParametersConfig → JSON marshal → JSON unmarshal → LoadParametersConfig.
// This is the S4 safety net baseline.
func BenchmarkParametersConfig_RoundTrip(b *testing.B) {
	cfg := DefaultParametersConfig()
	tmpDir := b.TempDir()
	paramsPath := filepath.Join(tmpDir, "benchmark.json")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		jsonData, err := json.Marshal(cfg)
		if err != nil {
			b.Fatalf("marshal: %v", err)
		}
		if err := os.WriteFile(paramsPath, jsonData, 0o644); err != nil {
			b.Fatalf("write: %v", err)
		}
		if _, err := LoadParametersConfig(paramsPath); err != nil {
			b.Fatalf("load: %v", err)
		}
	}
}
