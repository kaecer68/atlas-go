package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderConfig_ClaudeDesktop_NewFile(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "claude_desktop_config.json")

	target := RenderTarget{
		Client: ClientInstall{
			Name:       "claude-desktop",
			Format:     "json",
			ServerKey:  "mcpServers",
			ConfigPath: cfgPath,
		},
		BinaryPath:  "/usr/local/bin/atlas-mcp",
		AtlasBase:   "http://127.0.0.1:18080",
		AtlasAPIKey: "test-key-123",
	}

	result, err := renderConfig(target)
	if err != nil {
		t.Fatalf("renderConfig: %v", err)
	}

	// Verify the entry is under the "mcpServers" wrapper.
	var parsed map[string]any
	if err := json.Unmarshal(result.Content, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	wrapper, ok := parsed["mcpServers"].(map[string]any)
	if !ok {
		t.Fatalf("expected mcpServers wrapper, got %T: %v", parsed["mcpServers"], parsed)
	}
	entry, ok := wrapper["atlas-mcp"].(map[string]any)
	if !ok {
		t.Fatalf("expected atlas-mcp entry in mcpServers, got %T", wrapper["atlas-mcp"])
	}
	if entry["command"] != "/usr/local/bin/atlas-mcp" {
		t.Errorf("expected command=/usr/local/bin/atlas-mcp, got %v", entry["command"])
	}
	env, _ := entry["env"].(map[string]any)
	if env["ATLAS_BASE_URL"] != "http://127.0.0.1:18080" {
		t.Errorf("env ATLAS_BASE_URL wrong: %v", env["ATLAS_BASE_URL"])
	}
	if env["ATLAS_API_KEY"] != "test-key-123" {
		t.Errorf("env ATLAS_API_KEY wrong: %v", env["ATLAS_API_KEY"])
	}
}

func TestRenderConfig_Hermes_TopLevelMap(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")

	target := RenderTarget{
		Client: ClientInstall{
			Name:       "hermes",
			Format:     "yaml", // wizard writes JSON (valid YAML subset)
			ServerKey:  "",     // Hermes: top-level map
			ConfigPath: cfgPath,
		},
		BinaryPath:  "/tmp/atlas-mcp",
		AtlasBase:   "http://127.0.0.1:18080",
		AtlasAPIKey: "",
	}

	result, err := renderConfig(target)
	if err != nil {
		t.Fatalf("renderConfig: %v", err)
	}

	// Hermes: no ServerKey, so the entry is at the top level.
	var parsed map[string]any
	if err := json.Unmarshal(result.Content, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, ok := parsed["atlas-mcp"]; !ok {
		t.Fatalf("expected top-level atlas-mcp entry, got keys: %v", mapKeys(parsed))
	}
}

func TestRenderConfig_MergeWithExisting(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "claude_desktop_config.json")

	// Pre-existing config with another MCP server.
	existing := `{
  "mcpServers": {
    "other-server": {
      "command": "/usr/local/bin/other",
      "env": {"FOO": "bar"}
    }
  }
}`
	if err := os.WriteFile(cfgPath, []byte(existing), 0o600); err != nil {
		t.Fatalf("write existing: %v", err)
	}

	target := RenderTarget{
		Client: ClientInstall{
			Name:       "claude-desktop",
			Format:     "json",
			ServerKey:  "mcpServers",
			ConfigPath: cfgPath,
		},
		BinaryPath: "/usr/local/bin/atlas-mcp",
		AtlasBase:  "http://127.0.0.1:18080",
	}

	result, err := renderConfig(target)
	if err != nil {
		t.Fatalf("renderConfig: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(result.Content, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	wrapper := parsed["mcpServers"].(map[string]any)

	// other-server must be preserved
	if _, ok := wrapper["other-server"]; !ok {
		t.Errorf("other-server was overwritten; keys=%v", mapKeys(wrapper))
	}
	// atlas-mcp must be added
	if _, ok := wrapper["atlas-mcp"]; !ok {
		t.Errorf("atlas-mcp was not added; keys=%v", mapKeys(wrapper))
	}
}

func TestRenderConfig_PureYAML_ManualMergeError(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")

	// Write pure YAML (not JSON-prefixed) to trigger the manual-merge path.
	pureYAML := `mcp_servers:
  existing:
    command: foo
`
	if err := os.WriteFile(cfgPath, []byte(pureYAML), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	target := RenderTarget{
		Client: ClientInstall{
			Name:       "hermes",
			ServerKey:  "",
			ConfigPath: cfgPath,
		},
		BinaryPath: "/tmp/atlas-mcp",
		AtlasBase:  "http://127.0.0.1:18080",
	}

	_, err := renderConfig(target)
	if err == nil {
		t.Fatalf("expected error for non-JSON YAML config, got nil")
	}
	if !strings.Contains(err.Error(), "merge manually") {
		t.Errorf("expected 'merge manually' hint, got: %v", err)
	}
}

func TestWriteConfig_CreatesParentDir(t *testing.T) {
	tmpDir := t.TempDir()
	deepPath := filepath.Join(tmpDir, "a", "b", "c", "config.json")
	data := []byte(`{"hello":"world"}`)

	if err := writeConfig(deepPath, data); err != nil {
		t.Fatalf("writeConfig: %v", err)
	}

	got, err := os.ReadFile(deepPath)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(got) != string(data) {
		t.Errorf("content mismatch: got %q, want %q", got, data)
	}

	// Verify mode is 0600 (private).
	info, err := os.Stat(deepPath)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("expected mode 0600, got %o", perm)
	}
}

func TestBuildEntry_OpenCode_TypeAndArray(t *testing.T) {
	target := RenderTarget{
		Client: ClientInstall{
			Name: "opencode",
		},
		BinaryPath:  "/tmp/atlas-mcp",
		AtlasBase:   "http://127.0.0.1:18080",
		AtlasAPIKey: "k",
	}
	entry := buildEntry(target)
	if entry["type"] != "local" {
		t.Errorf("OpenCode entry should have type=local, got %v", entry["type"])
	}
	// OpenCode expects command as []string.
	cmd, ok := entry["command"].([]string)
	if !ok {
		t.Errorf("OpenCode entry should have command=[]string, got %T", entry["command"])
	}
	if len(cmd) != 1 || cmd[0] != "/tmp/atlas-mcp" {
		t.Errorf("OpenCode command wrong: %v", cmd)
	}
}

func mapKeys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
