// Package main implements cmd/atlas-mcp-setup, the interactive wizard
// that detects installed MCP clients (Hermes / OpenClaw / Claude Desktop /
// Cursor / OpenCode) on the user's machine and writes correct config
// snippets pointing to the local atlas-mcp binary.
//
// The wizard is stdlib-only (no new dependencies); it uses bufio.Scanner
// for interactive prompts and reuses internal/portprobe + the exported
// server.NewHTTPClient for health probes.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

// exitFunc is overridden in tests to prevent parseFlags from terminating
// the test runner. Defaults to os.Exit which terminates the process.
var exitFunc = os.Exit

// SetupConfig is the parsed flag + environment state for the wizard.
type SetupConfig struct {
	// CLI flags
	ClientName  string // --client <hermes|openclaw|claude-desktop|cursor|opencode>
	DryRun      bool   // --dry-run: print what would be written, don't write
	NoPrompt    bool   // --no-prompt: fail instead of asking (use with --client)
	OutputPath  string // --output <path>: override default config path
	Force       bool   // --force: proceed even if probes fail
	AtlasBase   string // --atlas-base-url <url>: override ATLAS_BASE_URL
	AtlasAPIKey string // --atlas-api-key <key>: override ATLAS_API_KEY
	BinaryPath  string // --binary <path>: override auto-detected bin/atlas-mcp

	// Resolved from flags or env
	HomeDir  string // user home dir (for client config paths)
	REPOROOT string // atlas-go repo root (for binary detection)
}

func main() {
	cfg := parseFlags()

	// Resolve home dir + repo root
	if err := resolvePaths(&cfg); err != nil {
		fmt.Fprintf(os.Stderr, "atlas-mcp-setup: resolve paths: %v\n", err)
		os.Exit(1)
	}

	// Run the wizard
	if err := Run(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "atlas-mcp-setup: %v\n", err)
		os.Exit(1)
	}
}

func parseFlags() SetupConfig {
	fs := flag.NewFlagSet("atlas-mcp-setup", flag.ExitOnError)
	var cfg SetupConfig

	fs.StringVar(&cfg.ClientName, "client", "",
		"target client: hermes | openclaw | claude-desktop | cursor | opencode")
	fs.BoolVar(&cfg.DryRun, "dry-run", false,
		"print what would be written; do not modify any files")
	fs.BoolVar(&cfg.NoPrompt, "no-prompt", false,
		"non-interactive mode (requires --client)")
	fs.StringVar(&cfg.OutputPath, "output", "",
		"override default config path for the selected client")
	fs.BoolVar(&cfg.Force, "force", false,
		"proceed even if backend probes fail")
	fs.StringVar(&cfg.AtlasBase, "atlas-base-url", "",
		"override ATLAS_BASE_URL (default: http://127.0.0.1:18080)")
	fs.StringVar(&cfg.AtlasAPIKey, "atlas-api-key", "",
		"override ATLAS_API_KEY")
	fs.StringVar(&cfg.BinaryPath, "binary", "",
		"override auto-detected atlas-mcp binary path")

	if err := fs.Parse(os.Args[1:]); err != nil {
		// ExitOnError already exits
		fmt.Fprintf(os.Stderr, "atlas-mcp-setup: parse flags: %v\n", err)
		os.Exit(1)
	}

	// --no-prompt requires --client (defined behavior per Oracle audit)
	if cfg.NoPrompt && cfg.ClientName == "" {
		fmt.Fprintf(os.Stderr,
			"atlas-mcp-setup: --no-prompt requires --client <hermes|openclaw|claude-desktop|cursor|opencode>\n")
		exitFunc(1)
	}

	// Default ATLAS_BASE_URL from env or constants (mirror cmd/atlas-mcp/main.go)
	if cfg.AtlasBase == "" {
		cfg.AtlasBase = os.Getenv("ATLAS_BASE_URL")
	}
	if cfg.AtlasBase == "" {
		// default is hardcoded to avoid importing internal/constants from this X-tier tool
		cfg.AtlasBase = "http://127.0.0.1:18080"
	}
	if cfg.AtlasAPIKey == "" {
		cfg.AtlasAPIKey = os.Getenv("ATLAS_API_KEY")
	}

	return cfg
}

func resolvePaths(cfg *SetupConfig) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("user home dir: %w", err)
	}
	cfg.HomeDir = home

	// REPOROOT: walk up from cwd looking for VERSION file (atlas-go marker).
	// If not found, default to cwd (some users run from elsewhere).
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("cwd: %w", err)
	}
	dir := cwd
	for range 5 {
		if _, err := os.Stat(filepath.Join(dir, "VERSION")); err == nil {
			cfg.REPOROOT = dir
			break
		}
		parent, err := filepath.EvalSymlinks(filepath.Join(dir, ".."))
		if err != nil || parent == dir {
			break
		}
		dir = parent
	}
	if cfg.REPOROOT == "" {
		cfg.REPOROOT = cwd
	}
	return nil
}
