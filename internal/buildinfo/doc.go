// Package buildinfo exposes runtime metadata about the running binary:
// version, commit hash, and build timestamp. Used by the system health
// endpoint, MCP tools, and dashboards to report the live binary provenance.
//
// Tier: utility (CLI/runtime metadata helper, not a core business module).
//
// Maturity: utility
package buildinfo
