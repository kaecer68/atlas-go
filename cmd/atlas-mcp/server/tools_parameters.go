package server

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// tools_parameters.go — atlas-mcp coverage audit PR 2 (2026-07-12)
//
// New read-only MCP tools that surface the parameters and backtest
// REST API surface to MCP clients. Backend endpoints exist already
// (see internal/monitoring/api/parameters/handlers.go and
// internal/monitoring/api/backtest/handlers.go); this PR closes the
// MCP coverage gap identified in the 2026-07-12 audit.
//
// All handlers are read-only and return *map[string]any passthrough
// so the same wire format is preserved end-to-end (per the
// reportPrismBaseOutput / riskAlertBaseOutput pattern already
// established in this package).

// ParametersGetOutput wraps /api/parameters response. Backend returns
// a flat key → type map (see internal/monitoring/api/parameters/handlers.go
// HandleGetParameters) — verified live 2026-07-12 via
// `curl -H "X-API-Key: $KEY" /api/parameters` returning
// {"alert.alert_sla_critical_sec": "int", ...}.
type ParametersGetOutput struct {
	Result *map[string]any `json:"result"`
}

type ParametersGetCategoriesOutput struct {
	Result *map[string]any `json:"result"`
}

type ParametersGetAuditLogOutput struct {
	Result *map[string]any `json:"result"`
}

type ParametersGetMetadataOutput struct {
	Result *map[string]any `json:"result"`
}

type ParametersGetSnapshotsInput struct {
	Days *int `json:"days,omitempty" jsonschema:"how many snapshots back; default 20, max 365"`
}

type ParametersGetSnapshotsOutput struct {
	Result *map[string]any `json:"result"`
}

func (s *server) handleParametersGet(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, ParametersGetOutput, error) {
	var out ParametersGetOutput
	if err := s.withAudit(ctx, "parameters_get", nil, func() error {
		return s.cli.Get(ctx, "/api/parameters", nil, &out.Result)
	}); err != nil {
		return nil, ParametersGetOutput{}, err
	}
	return nil, out, nil
}

func (s *server) handleParametersGetCategories(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, ParametersGetCategoriesOutput, error) {
	var out ParametersGetCategoriesOutput
	if err := s.withAudit(ctx, "parameters_get_categories", nil, func() error {
		return s.cli.Get(ctx, "/api/parameters/categories", nil, &out.Result)
	}); err != nil {
		return nil, ParametersGetCategoriesOutput{}, err
	}
	return nil, out, nil
}

func (s *server) handleParametersGetAuditLog(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, ParametersGetAuditLogOutput, error) {
	var out ParametersGetAuditLogOutput
	if err := s.withAudit(ctx, "parameters_get_audit_log", nil, func() error {
		return s.cli.Get(ctx, "/api/parameters/audit-log", nil, &out.Result)
	}); err != nil {
		return nil, ParametersGetAuditLogOutput{}, err
	}
	return nil, out, nil
}

func (s *server) handleParametersGetMetadata(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, ParametersGetMetadataOutput, error) {
	var out ParametersGetMetadataOutput
	if err := s.withAudit(ctx, "parameters_get_metadata", nil, func() error {
		return s.cli.Get(ctx, "/api/parameters/metadata", nil, &out.Result)
	}); err != nil {
		return nil, ParametersGetMetadataOutput{}, err
	}
	return nil, out, nil
}

func (s *server) handleParametersGetSnapshots(ctx context.Context, _ *mcp.CallToolRequest, in ParametersGetSnapshotsInput) (*mcp.CallToolResult, ParametersGetSnapshotsOutput, error) {
	days := 20
	if in.Days != nil && *in.Days > 0 {
		days = *in.Days
	}
	if days > 365 {
		days = 365
	}
	q := map[string]string{"days": fmt.Sprintf("%d", days)}
	var out ParametersGetSnapshotsOutput
	if err := s.withAudit(ctx, "parameters_get_snapshots", []string{"days"}, func() error {
		return s.cli.Get(ctx, "/api/parameters/snapshots", urlValues(q), &out.Result)
	}); err != nil {
		return nil, ParametersGetSnapshotsOutput{}, err
	}
	return nil, out, nil
}

func registerParametersTools(mcpSrv *mcp.Server, s *server) {
	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "parameters_get",
		Description: autoDescOr("parameters_get", "Current parameters (flat key→type map). For structured access, prefer parameters_get_categories (category breakdown) or parameters_get_metadata (provenance, rationale, citation)."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleParametersGet)

	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "parameters_get_categories",
		Description: autoDescOr("parameters_get_categories", "Available parameter categories with id/name/description (darwinian, factor, optimizer, sizing, health, garch, experiment, baseline, ...)."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleParametersGetCategories)

	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "parameters_get_audit_log",
		Description: autoDescOr("parameters_get_audit_log", "Parameter change audit log (who/when/why). Empty list when no changes recorded yet."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleParametersGetAuditLog)

	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "parameters_get_metadata",
		Description: autoDescOr("parameters_get_metadata", "Parameters with full provenance metadata (value, rationale, source, citation, last_calibrated)."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleParametersGetMetadata)

	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "parameters_get_snapshots",
		Description: autoDescOr("parameters_get_snapshots", "Historical parameter snapshots (default last 20 days, max 365)."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleParametersGetSnapshots)
}
