package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIsOptionalJSONTag(t *testing.T) {
	tests := []struct {
		tag  string
		want bool
	}{
		{`json:"name"`, false},
		{`json:"name,omitempty"`, true},
		{`json:"name,omitzero"`, true},
		{`json:"name,omitempty,omitzero"`, true},
		{`json:"name,string"`, false},
	}
	for _, tt := range tests {
		if got := isOptionalJSONTag(tt.tag); got != tt.want {
			t.Errorf("isOptionalJSONTag(%q) = %v, want %v", tt.tag, got, tt.want)
		}
	}
}

// TestOmitzeroStaysOptional verifies a field tagged ",omitzero" is still
// emitted as an optional TypeScript property, exactly like ",omitempty".
// This is the regression guard for Batch 0: gentags must recognize omitzero
// so field_types.ts does not flip those fields to required when Batch 2
// converts time.Time/struct omitempty tags to omitzero.
func TestOmitzeroStaysOptional(t *testing.T) {
	dir := t.TempDir()
	// TAG is replaced with a backtick so the fixture stays readable here.
	fixture := strings.ReplaceAll(`package fixtures

import "time"

type Sample struct {
Required  string    TAGjson:"required"TAG
OmitEmpty time.Time TAGjson:"omit_empty,omitempty"TAG
OmitZero  time.Time TAGjson:"omit_zero,omitzero"TAG
}
`, "TAG", "\u0060")
	if err := os.WriteFile(filepath.Join(dir, "types.go"), []byte(fixture), 0o600); err != nil {
		t.Fatal(err)
	}

	structs := parseStructsWithNames(dir, nil)
	fields, ok := structs["Sample"]
	if !ok {
		t.Fatalf("Sample struct not parsed; got %v", structs)
	}
	got := map[string]bool{}
	for _, f := range fields {
		got[f.JSONName] = f.Optional
	}
	if got["required"] {
		t.Error("required field must not be optional")
	}
	if !got["omit_empty"] {
		t.Error("omitempty field must be optional")
	}
	if !got["omit_zero"] {
		t.Error("omitzero field must be optional")
	}

	// Render the TS interface and confirm the omitzero property keeps "?".
	out := filepath.Join(dir, "field_types.ts")
	writeTypeScriptInterfaces(structs, out, true)
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	ts := string(data)
	for _, want := range []string{"omit_zero?:", "omit_empty?:", "required:"} {
		if !strings.Contains(ts, want) {
			t.Errorf("generated TS missing %q:\n%s", want, ts)
		}
	}
}
