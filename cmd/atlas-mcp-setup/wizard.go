package main

import (
	"fmt"
	"os"
	"sort"
)

// Run is the main wizard entry. It detects clients, prompts the user to
// select one, runs probes, generates + writes the config, and prints
// post-setup instructions.
func Run(cfg SetupConfig) error {
	printBanner(cfg)

	// Step 1: Detect installed MCP clients.
	fmt.Fprintf(os.Stderr, "\n[1/4] Detecting MCP clients in %s...\n", cfg.HomeDir)
	clients := detectClients(cfg.HomeDir)
	installed := filterInstalled(clients)
	if len(installed) == 0 {
		fmt.Fprintf(os.Stderr,
			"\n  ⚠️  No MCP client detected.\n"+
				"  Install one of: hermes, Claude Desktop, Cursor, OpenCode, OpenClaw\n"+
				"  Then re-run: make setup-mcp\n")
		return nil // not an error — exit 0 per Oracle audit
	}
	fmt.Fprintf(os.Stderr, "  ✓ Found %d client(s):\n", len(installed))
	for _, c := range installed {
		fmt.Fprintf(os.Stderr, "    • %s → %s (%s)\n", c.Name, c.ConfigPath, c.Format)
	}

	// Step 2: Select target client.
	var selected ClientInstall
	if cfg.NoPrompt && cfg.ClientName != "" {
		// Non-interactive: use --client flag.
		idx := findClientByName(installed, cfg.ClientName)
		if idx < 0 {
			return fmt.Errorf("--client=%s not found in detected clients; available: %s",
				cfg.ClientName, joinNames(installed))
		}
		selected = installed[idx]
		fmt.Fprintf(os.Stderr, "\n[2/4] Selected (from --client flag): %s\n", selected.Name)
	} else {
		// Interactive: ask the user.
		fmt.Fprintf(os.Stderr, "\n[2/4] Select target client:\n")
		names := make([]string, len(installed))
		for i, c := range installed {
			names[i] = string(c.Name)
		}
		choice := AskChoice("Which MCP client to configure?", names)
		if choice < 0 {
			return fmt.Errorf("no choice made; aborting")
		}
		selected = installed[choice-1]
	}

	// Step 3: Run probes.
	fmt.Fprintf(os.Stderr, "\n[3/4] Running health probes...\n")
	probes := probeAll(cfg, selected)
	fmt.Fprint(os.Stderr, probes.String())
	if !probes.AtlasGoBackend.OK && !cfg.Force {
		return fmt.Errorf("atlas-go backend not reachable on :18080; " +
			"start it with 'go run ./cmd/atlas' or pass --force to proceed anyway")
	}
	if !probes.WritableTarget.OK && !cfg.Force {
		return fmt.Errorf("target config not writeable: %s — check dir permissions, or pass --force to override",
			selected.ConfigPath)
	}

	// Step 4: Generate + write config.
	fmt.Fprintf(os.Stderr, "\n[4/4] Generating config for %s...\n", selected.Name)
	target := RenderTarget{
		Client:      selected,
		BinaryPath:  effectiveBinaryPath(cfg),
		AtlasBase:   cfg.AtlasBase,
		AtlasAPIKey: cfg.AtlasAPIKey,
	}
	result, err := renderConfig(target)
	if err != nil {
		return fmt.Errorf("render config: %w", err)
	}
	fmt.Fprintf(os.Stderr, "  %s\n", result.Diff)

	if cfg.DryRun {
		fmt.Fprintf(os.Stderr, "\n  --dry-run: would write %d bytes to %s\n",
			len(result.Content), result.Path)
		fmt.Fprintf(os.Stderr, "  Content preview:\n")
		if _, err := os.Stdout.Write(result.Content); err != nil {
			return fmt.Errorf("write dry-run preview to stdout: %w", err)
		}
		return nil
	}

	if cfg.NoPrompt && fileExists(result.Path) {
		// --no-prompt skips ConfirmAction; surface overwrite explicitly so
		// CI/script users see it. Merge logic preserves other MCP entries.
		fmt.Fprintf(os.Stderr, "  ⚠️  --no-prompt: overwriting %s (merge-preserves other entries)\n", result.Path)
	} else if !cfg.NoPrompt && fileExists(result.Path) {
		if !ConfirmAction(fmt.Sprintf("Overwrite existing config at %s?", result.Path)) {
			fmt.Fprintf(os.Stderr, "  Aborted by user.\n")
			return nil
		}
	}

	if err := writeConfig(result.Path, result.Content); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	fmt.Fprintf(os.Stderr, "  ✓ Wrote %d bytes to %s\n", len(result.Content), result.Path)

	// Post-setup instructions.
	fmt.Fprintf(os.Stderr, "\n✅ Setup complete.\n")
	printPostSetupHints(selected)
	return nil
}

func printBanner(cfg SetupConfig) {
	fmt.Fprintf(os.Stderr, "╔════════════════════════════════════════╗\n")
	fmt.Fprintf(os.Stderr, "║   atlas-mcp-setup (v0.0.0.32)         ║\n")
	fmt.Fprintf(os.Stderr, "║   Interactive MCP client configurator   ║\n")
	fmt.Fprintf(os.Stderr, "╚════════════════════════════════════════╝\n")
	fmt.Fprintf(os.Stderr, "  atlas-mcp binary: %s\n", effectiveBinaryPath(cfg))
	fmt.Fprintf(os.Stderr, "  atlas-go backend: %s\n", cfg.AtlasBase)
	if cfg.AtlasAPIKey != "" {
		fmt.Fprintf(os.Stderr, "  ATLAS_API_KEY:    (set, %d chars)\n", len(cfg.AtlasAPIKey))
	} else {
		fmt.Fprintf(os.Stderr, "  ATLAS_API_KEY:    (not set — only public tools will be accessible)\n")
	}
}

func effectiveBinaryPath(cfg SetupConfig) string {
	if cfg.BinaryPath != "" {
		return cfg.BinaryPath
	}
	return cfg.REPOROOT + "/bin/atlas-mcp"
}

func filterInstalled(clients []ClientInstall) []ClientInstall {
	out := make([]ClientInstall, 0, len(clients))
	for _, c := range clients {
		if c.Exists {
			out = append(out, c)
		}
	}
	sort.Slice(out, func(i, j int) bool { return string(out[i].Name) < string(out[j].Name) })
	return out
}

func findClientByName(clients []ClientInstall, name string) int {
	for i, c := range clients {
		if string(c.Name) == name {
			return i
		}
	}
	return -1
}

func joinNames(clients []ClientInstall) string {
	names := make([]string, len(clients))
	for i, c := range clients {
		names[i] = string(c.Name)
	}
	return fmt.Sprintf("[%s]", fmt.Sprint(names))
}

func printPostSetupHints(c ClientInstall) {
	fmt.Fprintf(os.Stderr, "\nNext steps:\n")
	switch string(c.Name) {
	case "hermes":
		fmt.Fprintf(os.Stderr, "  Restart Hermes (or run: hermes mcp reload)\n")
		fmt.Fprintf(os.Stderr, "  Verify: hermes mcp test atlas-mcp\n")
	case "openclaw":
		fmt.Fprintf(os.Stderr, "  Run: openclaw mcp reload\n")
		fmt.Fprintf(os.Stderr, "  Verify: openclaw mcp test atlas-mcp\n")
	case "claude-desktop":
		fmt.Fprintf(os.Stderr, "  Quit and re-launch Claude Desktop\n")
		fmt.Fprintf(os.Stderr, "  (Claude Desktop does not hot-reload MCP config)\n")
	case "cursor":
		fmt.Fprintf(os.Stderr, "  In Cursor, run: MCP: Reload (command palette)\n")
		fmt.Fprintf(os.Stderr, "  Or restart Cursor\n")
	case "opencode":
		fmt.Fprintf(os.Stderr, "  Run: opencode mcp auth atlas-mcp  (if needed)\n")
		fmt.Fprintf(os.Stderr, "  Verify: opencode mcp list\n")
	}
	fmt.Fprintf(os.Stderr, "\nExpected: 111 tools registered (107 business + 4 audit; up to 113 with sampling/elicitation enabled).\n")
}
