package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectClients_EmptyHome(t *testing.T) {
	// Empty temp dir simulates a user with no MCP clients installed.
	tmpHome := t.TempDir()
	clients := detectClients(tmpHome)
	// All 5 clients should be reported as not-installed.
	for _, c := range clients {
		if c.Exists {
			t.Errorf("client %s should not exist in empty home, but reported Exists=true", c.Name)
		}
	}
	if len(clients) != 5 {
		t.Errorf("expected 5 clients (hermes/openclaw/claude-desktop/cursor/opencode), got %d", len(clients))
	}
}

func TestDetectClients_HermesInstalled(t *testing.T) {
	tmpHome := t.TempDir()
	hermesDir := filepath.Join(tmpHome, ".hermes")
	if err := os.MkdirAll(hermesDir, 0o755); err != nil {
		t.Fatalf("mkdir hermes: %v", err)
	}
	cfgPath := filepath.Join(hermesDir, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte("mcp_servers: {}\n"), 0o600); err != nil {
		t.Fatalf("write hermes config: %v", err)
	}

	clients := detectClients(tmpHome)
	var hermes *ClientInstall
	for i, c := range clients {
		if c.Name == "hermes" {
			hermes = &clients[i]
			break
		}
	}
	if hermes == nil {
		t.Fatalf("hermes client not detected")
	}
	if !hermes.Exists {
		t.Errorf("hermes.Exists should be true, got false")
	}
	if !hermes.Writeable {
		t.Errorf("hermes.Writeable should be true, got false")
	}
	if filepath.Base(hermes.ConfigPath) != "config.yaml" {
		t.Errorf("expected config.yaml, got %s", hermes.ConfigPath)
	}
}

func TestDetectClients_OpenCodeInstalled(t *testing.T) {
	tmpHome := t.TempDir()
	ocDir := filepath.Join(tmpHome, ".config", "opencode")
	if err := os.MkdirAll(ocDir, 0o755); err != nil {
		t.Fatalf("mkdir opencode: %v", err)
	}
	cfgPath := filepath.Join(ocDir, "opencode.json")
	if err := os.WriteFile(cfgPath, []byte("{\"mcp\":{}}"), 0o600); err != nil {
		t.Fatalf("write opencode config: %v", err)
	}

	clients := detectClients(tmpHome)
	var oc *ClientInstall
	for i, c := range clients {
		if c.Name == "opencode" {
			oc = &clients[i]
			break
		}
	}
	if oc == nil {
		t.Fatalf("opencode client not detected")
	}
	if !oc.Exists {
		t.Errorf("opencode.Exists should be true, got false")
	}
	if oc.ServerKey != "mcp" {
		t.Errorf("opencode ServerKey should be 'mcp' (no servers suffix), got %q", oc.ServerKey)
	}
}

func TestFilterInstalled(t *testing.T) {
	clients := []ClientInstall{
		{Name: "hermes", Exists: true},
		{Name: "openclaw", Exists: false},
		{Name: "cursor", Exists: true},
		{Name: "opencode", Exists: false},
	}
	got := filterInstalled(clients)
	if len(got) != 2 {
		t.Fatalf("expected 2 installed, got %d", len(got))
	}
	// Should be sorted alphabetically by name.
	if got[0].Name != "cursor" {
		t.Errorf("expected cursor first (sorted), got %s", got[0].Name)
	}
	if got[1].Name != "hermes" {
		t.Errorf("expected hermes second, got %s", got[1].Name)
	}
}

func TestFindClientByName(t *testing.T) {
	clients := []ClientInstall{
		{Name: "hermes"},
		{Name: "opencode"},
		{Name: "cursor"},
	}
	if i := findClientByName(clients, "opencode"); i != 1 {
		t.Errorf("expected opencode at index 1, got %d", i)
	}
	if i := findClientByName(clients, "missing"); i != -1 {
		t.Errorf("expected -1 for missing client, got %d", i)
	}
}
