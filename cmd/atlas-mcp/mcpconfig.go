package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
)

// mcpRootsConfig mirrors the mcp.roots section of configs/parameters.json.
// Field names follow snake_case JSON to match the parameter-system convention.
type mcpRootsConfig struct {
	AllowedRoots  []string `json:"allowed_roots"`
	ReadSizeCap   int64    `json:"read_size_cap_bytes"`
	AlertOnChange bool     `json:"alert_on_change"`
}

// mcpConfigFile is the envelope that wraps the optional `mcp` section.
// Future sub-sections (sampling, elicitation, etc.) can be added here
// without breaking existing consumers.
type mcpConfigFile struct {
	MCP struct {
		Roots *mcpRootsConfig `json:"roots"`
	} `json:"mcp"`
}

// loadMCPConfig reads the `mcp` section from a parameters.json file.
// Returns (nil, nil) when the section is absent so callers fall back to
// environment variables. Returns a non-nil error only on malformed JSON
// or unreadable file.
func loadMCPConfig(path string) (*mcpRootsConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read parameters config: %w", err)
	}

	var cfg mcpConfigFile
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse parameters config: %w", err)
	}
	return cfg.MCP.Roots, nil
}

// envMCPRootsConfig reads MCP roots settings from environment variables.
// Each field is optional; empty/zero means "not set in env".
type envMCPRootsConfig struct {
	AllowedRoots  []string
	ReadSizeCap   int64
	AlertOnChange bool
}

// mergeMCPConfig combines a parameters.json base with env-var overrides.
// Env values override JSON values only when the env value is non-zero
// (non-empty for slices, non-zero for numbers, true for bools).
func mergeMCPConfig(base *mcpRootsConfig, env envMCPRootsConfig) mcpRootsConfig {
	out := mcpRootsConfig{}
	if base != nil {
		out = *base
	}
	if len(env.AllowedRoots) > 0 {
		out.AllowedRoots = env.AllowedRoots
	}
	if env.ReadSizeCap > 0 {
		out.ReadSizeCap = env.ReadSizeCap
	}
	if env.AlertOnChange {
		out.AlertOnChange = env.AlertOnChange
	}
	return out
}
