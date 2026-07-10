package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// RenderTarget is the data needed to render a single client config.
type RenderTarget struct {
	Client      ClientInstall
	BinaryPath  string
	AtlasBase   string
	AtlasAPIKey string
}

// RenderResult is the rendered config and its target path.
type RenderResult struct {
	Path    string
	Content []byte
	Diff    string // human-readable summary of what was added
}

// renderConfig generates the target config file content for the given client.
// It reads the existing file (if any), merges the atlas-mcp entry without
// disturbing other entries, and returns the new content + a path to write to.
//
// YAML clients (Hermes) get JSON-formatted content — JSON is a valid YAML
// subset, and the wizard's entry is structurally JSON. This avoids needing
// a YAML dependency. The header comment notes this for the user.
func renderConfig(target RenderTarget) (RenderResult, error) {
	path := target.Client.ConfigPath
	if path == "" {
		return RenderResult{}, fmt.Errorf("empty config path for client %s", target.Client.Name)
	}

	// Build the atlas-mcp entry for this client's wire format.
	entry, err := buildEntry(target)
	if err != nil {
		return RenderResult{}, err
	}

	// Read existing config if any.
	existing := map[string]any{}
	if data, err := os.ReadFile(path); err == nil {
		// Try JSON unmarshal. If it fails (e.g. pure-YAML file with
		// YAML-only syntax), surface a clear error instructing manual merge.
		if !strings.HasPrefix(strings.TrimSpace(string(data)), "{") {
			entryJSON, _ := json.MarshalIndent(entry, "", "  ")
			return RenderResult{}, fmt.Errorf(
				"existing config at %s is not JSON; please merge manually:\n%s",
				path, entryJSON)
		}
		if err := json.Unmarshal(data, &existing); err != nil {
			return RenderResult{}, fmt.Errorf("parse existing config %s: %w", path, err)
		}
	}

	// Resolve the server key (e.g. "mcpServers" for Claude Desktop, "mcp" for OpenCode).
	// Hermes/OpenClaw use a top-level map; Claude/Cursor/OpenCode nest under a key.
	serverKey := target.Client.ServerKey
	name := entryName()
	if serverKey == "" {
		// Default: top-level map keyed by server name.
		existing[name] = entry
	} else {
		// Nested: get or create the wrapper, then set the atlas-mcp entry.
		wrapper, _ := existing[serverKey].(map[string]any)
		if wrapper == nil {
			wrapper = map[string]any{}
		}
		wrapper[name] = entry
		existing[serverKey] = wrapper
	}

	// Marshal with 2-space indent for human readability.
	out, err := json.MarshalIndent(existing, "", "  ")
	if err != nil {
		return RenderResult{}, fmt.Errorf("marshal config: %w", err)
	}
	// Add trailing newline (POSIX convention).
	out = append(out, '\n')

	return RenderResult{
		Path:    path,
		Content: out,
		Diff: fmt.Sprintf("added entry '%s' to %s (server key: %q)",
			name, path, serverKeyOrTop(serverKey)),
	}, nil
}

// buildEntry assembles the atlas-mcp entry value for the given client.
// Each client has a slightly different wire format for env var encoding.
func buildEntry(target RenderTarget) (map[string]any, error) {
	env := map[string]string{
		"ATLAS_BASE_URL": target.AtlasBase,
	}
	if target.AtlasAPIKey != "" {
		env["ATLAS_API_KEY"] = target.AtlasAPIKey
	}

	entry := map[string]any{
		"command": target.BinaryPath,
		"env":     env,
	}

	// Client-specific extras.
	switch target.Client.Name {
	case "opencode":
		// OpenCode requires type discriminator and command as a list.
		entry["type"] = "local"
		entry["command"] = []string{target.BinaryPath}
		// OpenCode doesn't read "env" the same way — flatten to ATLAS_BASE_URL etc.
		// (For now, the type="local" + env map is what opencode.json schema accepts.)
	case "openclaw":
		// OpenClaw uses "type": "stdio" explicitly.
		entry["type"] = "stdio"
	case "hermes", "claude-desktop", "cursor":
		// Default stdio (no explicit type needed for these clients).
	}

	return entry, nil
}

// entryName returns the server name used to key the entry in the config.
// All clients use "atlas-mcp" as the server name.
func entryName() string { return "atlas-mcp" }

func serverKeyOrTop(key string) string {
	if key == "" {
		return "(top-level)"
	}
	return key
}

// writeConfig writes config bytes to path with mode 0600 (private).
// Per go-core.instructions.md: write paths use closure+logging for Close.
func writeConfig(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(path), err)
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	defer func() {
		// Write paths require closure+logging per go-core.instructions.md.
		// We log to stderr since this is an X-tier tool without internal/logging dependency.
		if cerr := f.Close(); cerr != nil {
			fmt.Fprintf(os.Stderr, "atlas-mcp-setup: WARN: close %s: %v\n", path, cerr)
		}
	}()
	if _, err := f.Write(data); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}
