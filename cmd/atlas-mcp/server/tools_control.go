package server

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerControlTools(mcpSrv *mcp.Server, s *server) {
	mcp.AddTool(mcpSrv, &mcp.Tool{
		Name:        "control_get_audit_log",
		Description: "Control override audit log (which agents were paused/banned, by which operator). Read-only by design.",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleControlGetAuditLog)

	mcp.AddTool(mcpSrv, &mcp.Tool{
		Name:        "control_get_active_overrides",
		Description: "Currently active control overrides (paused agents, sector bans, weight overrides).",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleControlGetActiveOverrides)

	mcp.AddTool(mcpSrv, &mcp.Tool{
		Name:        "control_approve_recommendation",
		Description: "Status of an approve-recommendation override (read-only state inspection; actual approval is admin-only).",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleControlApproveRecommendation)

	mcp.AddTool(mcpSrv, &mcp.Tool{
		Name:        "control_reject_recommendation",
		Description: "Status of a reject-recommendation override (read-only state inspection).",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleControlRejectRecommendation)
}

type controlBaseOutput struct {
	Result *map[string]any `json:"result"`
}

func (s *server) handleControlGetAuditLog(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, controlBaseOutput, error) {
	var out controlBaseOutput
	if err := s.withAudit(ctx, "control_get_audit_log", nil, func() error {
		return s.cli.Get(ctx, "/api/control/audit-log", nil, &out.Result)
	}); err != nil {
		return nil, controlBaseOutput{}, err
	}
	return nil, out, nil
}

func (s *server) handleControlGetActiveOverrides(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, controlBaseOutput, error) {
	var out controlBaseOutput
	if err := s.withAudit(ctx, "control_get_active_overrides", nil, func() error {
		return s.cli.Get(ctx, "/api/control/active-overrides", nil, &out.Result)
	}); err != nil {
		return nil, controlBaseOutput{}, err
	}
	return nil, out, nil
}

func (s *server) handleControlApproveRecommendation(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, controlBaseOutput, error) {
	var out controlBaseOutput
	if err := s.withAudit(ctx, "control_approve_recommendation", nil, func() error {
		return s.cli.Get(ctx, "/api/control/approve-recommendation", nil, &out.Result)
	}); err != nil {
		return nil, controlBaseOutput{}, err
	}
	return nil, out, nil
}

func (s *server) handleControlRejectRecommendation(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, controlBaseOutput, error) {
	var out controlBaseOutput
	if err := s.withAudit(ctx, "control_reject_recommendation", nil, func() error {
		return s.cli.Get(ctx, "/api/control/reject-recommendation", nil, &out.Result)
	}); err != nil {
		return nil, controlBaseOutput{}, err
	}
	return nil, out, nil
}
