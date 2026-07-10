package main

import (
	"fmt"
	"os"
	"path/filepath"
)

// ClientKind enumerates the 5 MCP clients this wizard can configure.
type ClientKind string

const (
	ClientHermes        ClientKind = "hermes"
	ClientOpenClaw      ClientKind = "openclaw"
	ClientClaudeDesktop ClientKind = "claude-desktop"
	ClientCursor        ClientKind = "cursor"
	ClientOpenCode      ClientKind = "opencode"
)

// ClientInstall describes a detected MCP client installation.
// Format is the config file format (json/yaml/json5).
// ServerKey is the top-level wrapper key in the config (mcpServers / mcp / mcp_servers / mcp.servers).
type ClientInstall struct {
	Name       ClientKind
	Format     string // "json" | "yaml" | "json5"
	ServerKey  string // "mcpServers" | "mcp" | "mcp_servers" | "mcp.servers"
	ConfigPath string // absolute path to config file
	Exists     bool
	Readable   bool
	Writeable  bool
}

// detectClients scans the user's machine for the 5 supported MCP clients
// and returns a slice of ClientInstall (one per found client, in stable order).
func detectClients(homeDir string) []ClientInstall {
	candidates := []ClientInstall{
		detectHermes(homeDir),
		detectOpenClaw(homeDir),
		detectClaudeDesktop(homeDir),
		detectCursor(homeDir),
		detectOpenCode(homeDir),
	}
	out := make([]ClientInstall, 0, len(candidates))
	for _, c := range candidates {
		if c.ConfigPath != "" {
			out = append(out, c)
		}
	}
	return out
}

func detectHermes(home string) ClientInstall {
	path := filepath.Join(home, ".hermes", "config.yaml")
	return inspectClient(ClientHermes, "yaml", "mcp_servers", path)
}

func detectOpenClaw(home string) ClientInstall {
	// OpenClaw uses ~/.openclaw/ JSON5
	path := filepath.Join(home, ".openclaw", "mcp.json")
	return inspectClient(ClientOpenClaw, "json5", "mcp.servers", path)
}

func detectClaudeDesktop(home string) ClientInstall {
	// macOS: ~/Library/Application Support/Claude/claude_desktop_config.json
	// Linux: ~/.config/Claude/claude_desktop_config.json
	// Windows: %APPDATA%/Claude/claude_desktop_config.json (out of scope for now)
	var path string
	if isMacOS() {
		path = filepath.Join(home, "Library", "Application Support", "Claude", "claude_desktop_config.json")
	} else {
		path = filepath.Join(home, ".config", "Claude", "claude_desktop_config.json")
	}
	return inspectClient(ClientClaudeDesktop, "json", "mcpServers", path)
}

func detectCursor(home string) ClientInstall {
	path := filepath.Join(home, ".cursor", "mcp.json")
	return inspectClient(ClientCursor, "json", "mcpServers", path)
}

func detectOpenCode(home string) ClientInstall {
	// OpenCode uses ~/.config/opencode/opencode.json (note: "mcp" not "mcpServers")
	path := filepath.Join(home, ".config", "opencode", "opencode.json")
	return inspectClient(ClientOpenCode, "json", "mcp", path)
}

// inspectClient checks whether the config file exists, is readable, and is
// writeable. Returns a ClientInstall with ConfigPath set; callers filter on
// non-empty ConfigPath to know "detected".
func inspectClient(name ClientKind, format, serverKey, path string) ClientInstall {
	c := ClientInstall{
		Name:       name,
		Format:     format,
		ServerKey:  serverKey,
		ConfigPath: path,
	}
	if info, err := os.Stat(path); err == nil && !info.IsDir() {
		c.Exists = true
	}
	if c.Exists {
		if f, err := os.Open(path); err == nil {
			c.Readable = true
			_ = f.Close()
		}
		// Test writeable by opening for append (won't modify, but checks perms)
		if f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0); err == nil {
			c.Writeable = true
			_ = f.Close()
		}
	}
	return c
}

func isMacOS() bool {
	return os.Getenv("GOOS") == "darwin" || filepath.Separator == '/' && fileExists("/System/Library/CoreServices/SystemVersion.plist")
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// String returns a human-readable description for the wizard UI.
func (c ClientInstall) String() string {
	status := "✓"
	if !c.Exists {
		status = "○ (not yet created)"
	} else if !c.Writeable {
		status = "⚠ (not writeable)"
	}
	return fmt.Sprintf("  %s %-16s %s", status, c.Name, c.ConfigPath)
}
