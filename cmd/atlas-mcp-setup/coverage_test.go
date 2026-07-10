package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestParseFlags_Defaults(t *testing.T) {
	// Save and restore os.Args and env.
	origArgs := os.Args
	origEnvBase := os.Getenv("ATLAS_BASE_URL")
	origEnvKey := os.Getenv("ATLAS_API_KEY")
	defer func() {
		os.Args = origArgs
		os.Setenv("ATLAS_BASE_URL", origEnvBase)
		os.Setenv("ATLAS_API_KEY", origEnvKey)
	}()

	os.Args = []string{"atlas-mcp-setup"}
	os.Unsetenv("ATLAS_BASE_URL")
	os.Unsetenv("ATLAS_API_KEY")

	cfg := parseFlags()
	if cfg.ClientName != "" {
		t.Errorf("expected empty client, got %q", cfg.ClientName)
	}
	if cfg.DryRun || cfg.NoPrompt || cfg.Force {
		t.Errorf("expected all bool flags false, got dry=%v noprompt=%v force=%v", cfg.DryRun, cfg.NoPrompt, cfg.Force)
	}
	if cfg.AtlasBase != "http://127.0.0.1:18080" {
		t.Errorf("expected default ATLAS_BASE_URL, got %q", cfg.AtlasBase)
	}
	if cfg.AtlasAPIKey != "" {
		t.Errorf("expected empty ATLAS_API_KEY, got %q", cfg.AtlasAPIKey)
	}
}

func TestParseFlags_EnvOverride(t *testing.T) {
	origArgs := os.Args
	origEnvBase := os.Getenv("ATLAS_BASE_URL")
	origEnvKey := os.Getenv("ATLAS_API_KEY")
	defer func() {
		os.Args = origArgs
		os.Setenv("ATLAS_BASE_URL", origEnvBase)
		os.Setenv("ATLAS_API_KEY", origEnvKey)
	}()

	os.Args = []string{"atlas-mcp-setup"}
	os.Setenv("ATLAS_BASE_URL", "http://custom:9999")
	os.Setenv("ATLAS_API_KEY", "env-key")

	cfg := parseFlags()
	if cfg.AtlasBase != "http://custom:9999" {
		t.Errorf("expected env override, got %q", cfg.AtlasBase)
	}
	if cfg.AtlasAPIKey != "env-key" {
		t.Errorf("expected env key, got %q", cfg.AtlasAPIKey)
	}
}

func TestParseFlags_FlagOverrides(t *testing.T) {
	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	os.Args = []string{
		"atlas-mcp-setup",
		"--client", "opencode",
		"--dry-run",
		"--no-prompt",
		"--output", "/tmp/cfg.json",
		"--force",
		"--atlas-base-url", "http://flag:1234",
		"--atlas-api-key", "flag-key",
		"--binary", "/custom/atlas-mcp",
	}

	cfg := parseFlags()
	if cfg.ClientName != "opencode" {
		t.Errorf("client: %q", cfg.ClientName)
	}
	if !cfg.DryRun || !cfg.NoPrompt || !cfg.Force {
		t.Errorf("bool flags: dry=%v noprompt=%v force=%v", cfg.DryRun, cfg.NoPrompt, cfg.Force)
	}
	if cfg.OutputPath != "/tmp/cfg.json" {
		t.Errorf("output: %q", cfg.OutputPath)
	}
	if cfg.AtlasBase != "http://flag:1234" {
		t.Errorf("atlas base: %q", cfg.AtlasBase)
	}
	if cfg.AtlasAPIKey != "flag-key" {
		t.Errorf("api key: %q", cfg.AtlasAPIKey)
	}
	if cfg.BinaryPath != "/custom/atlas-mcp" {
		t.Errorf("binary: %q", cfg.BinaryPath)
	}
}

func TestResolvePaths_HomeDirAndVersion(t *testing.T) {
	tmpHome := t.TempDir()
	tmpRepo := t.TempDir()
	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)
	os.Setenv("HOME", tmpHome)

	// Create VERSION marker in tmpRepo.
	if err := os.WriteFile(filepath.Join(tmpRepo, "VERSION"), []byte("0.0.0.32\n"), 0o644); err != nil {
		t.Fatalf("write VERSION: %v", err)
	}

	origCwd, _ := os.Getwd()
	defer os.Chdir(origCwd)
	os.Chdir(tmpRepo)

	cfg := SetupConfig{}
	if err := resolvePaths(&cfg); err != nil {
		t.Fatalf("resolvePaths: %v", err)
	}
	// macOS resolves /tmp → /private/tmp via symlinks; compare via EvalSymlinks.
	if evalSymlink(t, cfg.HomeDir) != evalSymlink(t, tmpHome) {
		t.Errorf("HomeDir: %q, want %q", cfg.HomeDir, tmpHome)
	}
	if evalSymlink(t, cfg.REPOROOT) != evalSymlink(t, tmpRepo) {
		t.Errorf("REPOROOT: %q, want %q", cfg.REPOROOT, tmpRepo)
	}
}

func evalSymlink(t *testing.T, p string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(p)
	if err != nil {
		return p
	}
	return resolved
}

func TestBuildEntry_AllClients(t *testing.T) {
	clients := []struct {
		name    string
		typeKey string
		hasList bool
	}{
		{"hermes", "", false},
		{"openclaw", "stdio", false},
		{"claude-desktop", "", false},
		{"cursor", "", false},
		{"opencode", "local", true},
	}
	for _, c := range clients {
		t.Run(c.name, func(t *testing.T) {
			target := RenderTarget{
				Client:      ClientInstall{Name: ClientKind(c.name)},
				BinaryPath:  "/tmp/atlas-mcp",
				AtlasBase:   "http://127.0.0.1:18080",
				AtlasAPIKey: "k",
			}
			entry := buildEntry(target)
			if c.typeKey != "" {
				if entry["type"] != c.typeKey {
					t.Errorf("type: %v, want %q", entry["type"], c.typeKey)
				}
			}
			if c.hasList {
				if _, ok := entry["command"].([]string); !ok {
					t.Errorf("command should be []string, got %T", entry["command"])
				}
			} else {
				if entry["command"] != "/tmp/atlas-mcp" {
					t.Errorf("command: %v", entry["command"])
				}
			}
		})
	}
}

func TestProbeAll_BinaryCheck(t *testing.T) {
	tmpRepo := t.TempDir()
	binPath := filepath.Join(tmpRepo, "bin")
	if err := os.MkdirAll(binPath, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Create an executable binary.
	binary := filepath.Join(binPath, "atlas-mcp")
	if err := os.WriteFile(binary, []byte("#!/bin/sh\necho ok\n"), 0o755); err != nil {
		t.Fatalf("write binary: %v", err)
	}

	cfg := SetupConfig{
		REPOROOT:   tmpRepo,
		BinaryPath: binary,
	}
	result := probeAll(cfg, ClientInstall{})
	if !result.AtlasMCPBinary.OK {
		t.Errorf("executable binary should pass probe: %s", result.AtlasMCPBinary.Detail)
	}
}

func TestProbeAll_BinaryMissing(t *testing.T) {
	cfg := SetupConfig{
		REPOROOT: "/nonexistent",
	}
	result := probeAll(cfg, ClientInstall{})
	if result.AtlasMCPBinary.OK {
		t.Errorf("missing binary should fail probe: %s", result.AtlasMCPBinary.Detail)
	}
}

func TestProbeAll_NonExecutableBinary(t *testing.T) {
	tmpRepo := t.TempDir()
	binary := filepath.Join(tmpRepo, "atlas-mcp")
	if err := os.WriteFile(binary, []byte("not executable"), 0o644); err != nil {
		t.Fatalf("write binary: %v", err)
	}
	cfg := SetupConfig{
		REPOROOT:   tmpRepo,
		BinaryPath: binary,
	}
	result := probeAll(cfg, ClientInstall{})
	if result.AtlasMCPBinary.OK {
		t.Errorf("non-executable binary should fail probe: %s", result.AtlasMCPBinary.Detail)
	}
}

func TestProbeResultString(t *testing.T) {
	r := ProbeResult{
		AtlasGoBackend: ProbeCheck{OK: true, Detail: "backend ok"},
		AtlasMCPBinary: ProbeCheck{OK: false, Detail: "binary missing"},
		AtlasMCPAdmin:  ProbeCheck{OK: true, Detail: "admin ok"},
		WritableTarget: ProbeCheck{OK: true, Detail: "writable"},
	}
	s := r.String()
	if s == "" {
		t.Fatal("String() returned empty")
	}
	// Should contain both ✓ and ✗ marks.
	if !contains(s, "✓") {
		t.Errorf("expected ✓ in output, got: %s", s)
	}
	if !contains(s, "✗") {
		t.Errorf("expected ✗ in output, got: %s", s)
	}
}

func contains(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestServerKeyOrTop(t *testing.T) {
	if got := serverKeyOrTop(""); got != "(top-level)" {
		t.Errorf("empty key: %q", got)
	}
	if got := serverKeyOrTop("mcpServers"); got != "mcpServers" {
		t.Errorf("named key: %q", got)
	}
}

func TestEntryName(t *testing.T) {
	if got := entryName(); got != "atlas-mcp" {
		t.Errorf("entryName: %q", got)
	}
}

func TestEffectiveBinaryPath(t *testing.T) {
	// BinaryPath flag takes precedence.
	cfg := SetupConfig{
		BinaryPath: "/explicit/atlas-mcp",
		REPOROOT:   "/somewhere",
	}
	if got := effectiveBinaryPath(cfg); got != "/explicit/atlas-mcp" {
		t.Errorf("explicit: %q", got)
	}
	// Default: REPOROOT/bin/atlas-mcp.
	cfg2 := SetupConfig{REPOROOT: "/repo"}
	if got := effectiveBinaryPath(cfg2); got != "/repo/bin/atlas-mcp" {
		t.Errorf("default: %q", got)
	}
}

func TestJoinNames(t *testing.T) {
	clients := []ClientInstall{
		{Name: "hermes"},
		{Name: "opencode"},
	}
	got := joinNames(clients)
	if got == "" {
		t.Errorf("joinNames returned empty")
	}
	if !contains(got, "hermes") || !contains(got, "opencode") {
		t.Errorf("joinNames missing names: %s", got)
	}
}

func TestPrintPostSetupHints_AllClients(t *testing.T) {
	// Capture stderr to verify each client gets a "Next steps" hint.
	clients := []ClientKind{"hermes", "openclaw", "claude-desktop", "cursor", "opencode"}
	for _, c := range clients {
		t.Run(string(c), func(t *testing.T) {
			// Redirect stderr.
			origStderr := os.Stderr
			r, w, _ := os.Pipe()
			os.Stderr = w
			defer func() { os.Stderr = origStderr }()

			printPostSetupHints(ClientInstall{Name: c})

			w.Close()
			buf := make([]byte, 4096)
			n, _ := r.Read(buf)
			out := string(buf[:n])
			if !contains(out, "Next steps") {
				t.Errorf("expected 'Next steps' in output for %s, got: %s", c, out)
			}
		})
	}
}

func TestPrintBanner(t *testing.T) {
	origStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w
	defer func() { os.Stderr = origStderr }()

	cfg := SetupConfig{
		BinaryPath:  "/x",
		AtlasBase:   "http://x",
		AtlasAPIKey: "secret",
	}
	printBanner(cfg)

	w.Close()
	buf := make([]byte, 4096)
	n, _ := r.Read(buf)
	out := string(buf[:n])
	if !contains(out, "atlas-mcp-setup") {
		t.Errorf("banner missing title: %s", out)
	}
	if !contains(out, "ATLAS_API_KEY") {
		t.Errorf("banner missing API key section: %s", out)
	}
}

func TestPrintBanner_NoAPIKey(t *testing.T) {
	origStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w
	defer func() { os.Stderr = origStderr }()

	printBanner(SetupConfig{
		BinaryPath: "/x",
		AtlasBase:  "http://x",
	})
	w.Close()
	buf := make([]byte, 4096)
	n, _ := r.Read(buf)
	out := string(buf[:n])
	if !contains(out, "not set") {
		t.Errorf("expected 'not set' for missing key: %s", out)
	}
}

func TestRun_NoClientsInstalled(t *testing.T) {
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)
	os.Setenv("HOME", tmpHome)

	// Suppress stderr for the duration of this test.
	origStderr := os.Stderr
	os.Stderr, _ = os.Open(os.DevNull)
	defer func() { os.Stderr = origStderr }()

	cfg := SetupConfig{
		HomeDir:   tmpHome,
		REPOROOT:  "/nonexistent",
		AtlasBase: "http://127.0.0.1:18080",
		Force:     true,
	}
	// No clients installed → Run returns nil with a warning (per Oracle audit).
	if err := Run(cfg); err != nil {
		t.Errorf("Run with no clients: %v", err)
	}
}

func TestRun_NoPromptMissingClient(t *testing.T) {
	// "--no-prompt without --client" guard lives in parseFlags, not Run.
	// Verify parseFlags rejects this combination by checking it would exit.
	origArgs := os.Args
	defer func() { os.Args = origArgs }()
	os.Args = []string{"atlas-mcp-setup", "--no-prompt"}

	origExit := exitFunc
	defer func() { exitFunc = origExit }()
	called := false
	exitFunc = func(code int) { called = true }

	parseFlags()
	if !called {
		t.Errorf("expected parseFlags to call exitFunc for --no-prompt without --client")
	}
}

func TestFileExistsHelper(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "exists.txt")
	if fileExists(tmpFile) {
		t.Errorf("file should not exist yet")
	}
	if err := os.WriteFile(tmpFile, []byte("x"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if !fileExists(tmpFile) {
		t.Errorf("file should exist after write")
	}
}

func TestRenderConfig_NoKey_TopLevel(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.json")

	target := RenderTarget{
		Client: ClientInstall{
			Name:       "test-no-key",
			Format:     "json",
			ServerKey:  "", // no wrapper
			ConfigPath: cfgPath,
		},
		BinaryPath: "/x",
		AtlasBase:  "http://x",
	}
	result, err := renderConfig(target)
	if err != nil {
		t.Fatalf("renderConfig: %v", err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(result.Content, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, ok := parsed["atlas-mcp"]; !ok {
		t.Errorf("expected top-level atlas-mcp; keys=%v", keysOf(parsed))
	}
}

func keysOf(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
