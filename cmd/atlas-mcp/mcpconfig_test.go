package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLoadMCPConfig_MissingFile_ReturnsNil(t *testing.T) {
	got, err := loadMCPConfig(filepath.Join(t.TempDir(), "nonexistent.json"))
	if err != nil {
		t.Fatalf("missing file must not error: %v", err)
	}
	if got != nil {
		t.Fatalf("missing file must return nil, got %+v", got)
	}
}

func TestLoadMCPConfig_NoMCPSection_ReturnsNil(t *testing.T) {
	path := filepath.Join(t.TempDir(), "params.json")
	if err := os.WriteFile(path, []byte(`{"version":"1.2","darwinian":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := loadMCPConfig(path)
	if err != nil {
		t.Fatalf("missing mcp section must not error: %v", err)
	}
	if got != nil {
		t.Fatalf("missing mcp section must return nil, got %+v", got)
	}
}

func TestLoadMCPConfig_NoRootsSection_ReturnsNil(t *testing.T) {
	path := filepath.Join(t.TempDir(), "params.json")
	if err := os.WriteFile(path, []byte(`{"mcp":{"sampling":{}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := loadMCPConfig(path)
	if err != nil {
		t.Fatalf("missing mcp.roots must not error: %v", err)
	}
	if got != nil {
		t.Fatalf("missing mcp.roots must return nil, got %+v", got)
	}
}

func TestLoadMCPConfig_FullRoots(t *testing.T) {
	path := filepath.Join(t.TempDir(), "params.json")
	body := `{
		"version": "1.2",
		"mcp": {
			"roots": {
				"allowed_roots": ["file:///tmp/atlas-mcp-allow", "file:///var/log/atlas-mcp"],
				"read_size_cap_bytes": 2097152,
				"alert_on_change": true
			}
		}
	}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := loadMCPConfig(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	want := &mcpRootsConfig{
		AllowedRoots:  []string{"file:///tmp/atlas-mcp-allow", "file:///var/log/atlas-mcp"},
		ReadSizeCap:   2097152,
		AlertOnChange: true,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestLoadMCPConfig_PartialRoots_DefaultsFillIn(t *testing.T) {
	path := filepath.Join(t.TempDir(), "params.json")
	body := `{"mcp":{"roots":{"read_size_cap_bytes":1048576}}}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := loadMCPConfig(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got == nil {
		t.Fatal("got nil, want partial config")
	}
	if got.ReadSizeCap != 1048576 {
		t.Errorf("ReadSizeCap = %d, want 1048576", got.ReadSizeCap)
	}
	if len(got.AllowedRoots) != 0 {
		t.Errorf("AllowedRoots = %v, want empty", got.AllowedRoots)
	}
	if got.AlertOnChange {
		t.Errorf("AlertOnChange = true, want false (zero value default)")
	}
}

func TestLoadMCPConfig_InvalidJSON_ReturnsError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "params.json")
	if err := os.WriteFile(path, []byte(`{"mcp":{"roots":{`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadMCPConfig(path); err == nil {
		t.Fatal("invalid JSON must return error")
	}
}

func TestLoadMCPConfig_RootsFileURIs_Preserved(t *testing.T) {
	path := filepath.Join(t.TempDir(), "params.json")
	body := `{"mcp":{"roots":{"allowed_roots":["file:///home/user/workspace"]}}}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := loadMCPConfig(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(got.AllowedRoots) != 1 || got.AllowedRoots[0] != "file:///home/user/workspace" {
		t.Errorf("AllowedRoots not preserved: %v", got.AllowedRoots)
	}
}

func TestMergeMCPConfig_NilBase_EnvWins(t *testing.T) {
	env := envMCPRootsConfig{
		AllowedRoots:  []string{"file:///env"},
		ReadSizeCap:   524288,
		AlertOnChange: true,
	}
	got := mergeMCPConfig(nil, env)
	want := mcpRootsConfig{
		AllowedRoots:  []string{"file:///env"},
		ReadSizeCap:   524288,
		AlertOnChange: true,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("nil base: got %+v, want %+v", got, want)
	}
}

func TestMergeMCPConfig_EnvOverridesJSON(t *testing.T) {
	base := &mcpRootsConfig{
		AllowedRoots:  []string{"file:///from-json"},
		ReadSizeCap:   1024,
		AlertOnChange: false,
	}
	env := envMCPRootsConfig{
		AllowedRoots:  []string{"file:///from-env"},
		ReadSizeCap:   4096,
		AlertOnChange: true,
	}
	got := mergeMCPConfig(base, env)
	if !reflect.DeepEqual(got.AllowedRoots, []string{"file:///from-env"}) {
		t.Errorf("AllowedRoots not overridden by env: %v", got.AllowedRoots)
	}
	if got.ReadSizeCap != 4096 {
		t.Errorf("ReadSizeCap not overridden by env: %d", got.ReadSizeCap)
	}
	if !got.AlertOnChange {
		t.Errorf("AlertOnChange not overridden by env")
	}
}

func TestMergeMCPConfig_EmptyEnv_DoesNotOverrideJSON(t *testing.T) {
	base := &mcpRootsConfig{
		AllowedRoots:  []string{"file:///from-json"},
		ReadSizeCap:   1024,
		AlertOnChange: true,
	}
	env := envMCPRootsConfig{} // zero values
	got := mergeMCPConfig(base, env)
	if !reflect.DeepEqual(got, *base) {
		t.Fatalf("empty env must not change JSON base: got %+v, want %+v", got, *base)
	}
}

func TestMergeMCPConfig_PartialEnv_FillsMissingOnly(t *testing.T) {
	base := &mcpRootsConfig{
		AllowedRoots: []string{"file:///from-json"},
		ReadSizeCap:  1024,
	}
	env := envMCPRootsConfig{
		AlertOnChange: true, // only this is set in env
	}
	got := mergeMCPConfig(base, env)
	want := mcpRootsConfig{
		AllowedRoots:  []string{"file:///from-json"}, // preserved
		ReadSizeCap:   1024,                          // preserved
		AlertOnChange: true,                          // added
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("partial env: got %+v, want %+v", got, want)
	}
}
