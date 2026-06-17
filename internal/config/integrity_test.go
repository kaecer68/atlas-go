package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestCheckIntegrity_ValidFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "params.json")

	cfg := DefaultParametersConfig()
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal default config: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write valid params.json: %v", err)
	}

	errs := CheckParamsIntegrity(path)
	if len(errs) != 0 {
		t.Fatalf("expected no errors for valid file, got %d: %v", len(errs), errs)
	}
}

func TestCheckIntegrity_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "params.json")

	if err := os.WriteFile(path, []byte("{broken"), 0o644); err != nil {
		t.Fatalf("write invalid params.json: %v", err)
	}

	errs := CheckParamsIntegrity(path)
	if len(errs) == 0 {
		t.Fatal("expected errors for invalid JSON, got none")
	}
}

func TestCheckIntegrity_MissingRequiredFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "params.json")

	if err := os.WriteFile(path, []byte(`{"version":1}`), 0o644); err != nil {
		t.Fatalf("write incomplete params.json: %v", err)
	}

	errs := CheckParamsIntegrity(path)
	if len(errs) == 0 {
		t.Fatal("expected errors for missing required fields, got none")
	}
}

func TestCheckIntegrity_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "params.json")

	if err := os.WriteFile(path, []byte(""), 0o644); err != nil {
		t.Fatalf("write empty params.json: %v", err)
	}

	errs := CheckParamsIntegrity(path)
	if len(errs) == 0 {
		t.Fatal("expected errors for empty file, got none")
	}
}
