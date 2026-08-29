package server

import (
	"context"
	"errors"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerControlTools(mcpSrv *mcp.Server, s *server) {
	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "control_get_audit_log",
		Description: autoDescOr("control_get_audit_log", "Control override audit log (which agents were paused/banned, by which operator). Read-only by design."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: new(false)},
	}, s.handleControlGetAuditLog)

	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "control_get_active_overrides",
		Description: autoDescOr("control_get_active_overrides", "Currently active control overrides (paused agents, sector bans, weight overrides)."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: new(false)},
	}, s.handleControlGetActiveOverrides)

	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "control_approve_recommendation",
		Description: autoDescOr("control_approve_recommendation", "Status of an approve-recommendation override (read-only state inspection; actual approval is admin-only)."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: new(false)},
	}, s.handleControlApproveRecommendation)

	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "control_reject_recommendation",
		Description: autoDescOr("control_reject_recommendation", "Status of a reject-recommendation override (read-only state inspection)."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: new(false)},
	}, s.handleControlRejectRecommendation)

	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "control_pause_agent",
		Description: autoDescOr("control_pause_agent", "Pause a specific agent (suspend its recommendations). Side-effect: persists in control store. Requires ATLAS_API_KEY."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: new(true)},
	}, s.handleControlPauseAgent)

	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "control_resume_agent",
		Description: autoDescOr("control_resume_agent", "Resume a previously-paused agent. Side-effect: removes pause override. Requires ATLAS_API_KEY."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: new(true)},
	}, s.handleControlResumeAgent)

	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "control_sector_ban",
		Description: autoDescOr("control_sector_ban", "Ban a sector from new positions. Side-effect: applies sector override. Requires ATLAS_API_KEY."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: new(true)},
	}, s.handleControlSectorBan)
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

type controlAgentInterventionInput struct {
	AgentID  string `json:"agent_id" jsonschema:"agent identifier (required)"`
	Reason   string `json:"reason,omitempty" jsonschema:"human-readable reason for audit log"`
	Operator string `json:"operator,omitempty" jsonschema:"operator identifier (defaults to MCP server's authenticated identity)"`
}

type controlSectorBanInput struct {
	Sector   string `json:"sector" jsonschema:"sector identifier (required)"`
	Banned   bool   `json:"banned" jsonschema:"true to apply ban, false to lift existing ban"`
	Reason   string `json:"reason,omitempty" jsonschema:"human-readable reason for audit log"`
	Operator string `json:"operator,omitempty" jsonschema:"operator identifier"`
}

func (s *server) handleControlPauseAgent(ctx context.Context, _ *mcp.CallToolRequest, in controlAgentInterventionInput) (*mcp.CallToolResult, controlBaseOutput, error) {
	if in.AgentID == "" {
		return nil, controlBaseOutput{}, errors.New("control_pause_agent: agent_id is required")
	}
	var out controlBaseOutput
	body := map[string]string{"agent_id": in.AgentID}
	if in.Reason != "" {
		body["reason"] = in.Reason
	}
	if in.Operator != "" {
		body["operator"] = in.Operator
	}
	if err := s.withAudit(ctx, "control_pause_agent", []string{"agent_id"}, func() error {
		return s.cli.PostJSON(ctx, "/api/control/pause-agent", body, &out.Result)
	}); err != nil {
		return nil, controlBaseOutput{}, err
	}
	return nil, out, nil
}

func (s *server) handleControlResumeAgent(ctx context.Context, _ *mcp.CallToolRequest, in controlAgentInterventionInput) (*mcp.CallToolResult, controlBaseOutput, error) {
	if in.AgentID == "" {
		return nil, controlBaseOutput{}, errors.New("control_resume_agent: agent_id is required")
	}
	var out controlBaseOutput
	body := map[string]string{"agent_id": in.AgentID}
	if in.Reason != "" {
		body["reason"] = in.Reason
	}
	if in.Operator != "" {
		body["operator"] = in.Operator
	}
	if err := s.withAudit(ctx, "control_resume_agent", []string{"agent_id"}, func() error {
		return s.cli.PostJSON(ctx, "/api/control/resume-agent", body, &out.Result)
	}); err != nil {
		return nil, controlBaseOutput{}, err
	}
	return nil, out, nil
}

func (s *server) handleControlSectorBan(ctx context.Context, _ *mcp.CallToolRequest, in controlSectorBanInput) (*mcp.CallToolResult, controlBaseOutput, error) {
	if in.Sector == "" {
		return nil, controlBaseOutput{}, errors.New("control_sector_ban: sector is required")
	}
	var out controlBaseOutput
	body := map[string]any{"sector": in.Sector, "banned": in.Banned}
	if in.Reason != "" {
		body["reason"] = in.Reason
	}
	if in.Operator != "" {
		body["operator"] = in.Operator
	}
	if err := s.withAudit(ctx, "control_sector_ban", []string{"sector", "banned"}, func() error {
		return s.cli.PostJSON(ctx, "/api/control/sector-ban", body, &out.Result)
	}); err != nil {
		return nil, controlBaseOutput{}, err
	}
	return nil, out, nil
}
