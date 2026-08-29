package server

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerSynergyTools(mcpSrv *mcp.Server, s *server) {
	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "synergy_get_darwinian_status",
		Description: autoDescOr("synergy_get_darwinian_status", "Current Darwinian weight state (which strategies are being promoted/demoted, current weight delta)."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: new(false)},
	}, s.handleSynergyGetDarwinianStatus)

	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "synergy_get_darwinian_trend",
		Description: autoDescOr("synergy_get_darwinian_trend", "Historical Darwinian weight trend per strategy (last N days)."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: new(false)},
	}, s.handleSynergyGetDarwinianTrend)

	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "synergy_get_l2_4_schedule",
		Description: autoDescOr("synergy_get_l2_4_schedule", "L2.4 observation window schedule (current state, next boundary)."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: new(false)},
	}, s.handleSynergyGetL24Schedule)
}

type synergyBaseOutput struct {
	Result *map[string]any `json:"result"`
}

func (s *server) handleSynergyGetDarwinianStatus(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, synergyBaseOutput, error) {
	var out synergyBaseOutput
	if err := s.withAudit(ctx, "synergy_get_darwinian_status", nil, func() error {
		return s.cli.Get(ctx, "/api/synergy/darwinian/status", nil, &out.Result)
	}); err != nil {
		return nil, synergyBaseOutput{}, err
	}
	return nil, out, nil
}

func (s *server) handleSynergyGetDarwinianTrend(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, synergyBaseOutput, error) {
	var out synergyBaseOutput
	if err := s.withAudit(ctx, "synergy_get_darwinian_trend", nil, func() error {
		return s.cli.Get(ctx, "/api/synergy/darwinian/trend", nil, &out.Result)
	}); err != nil {
		return nil, synergyBaseOutput{}, err
	}
	return nil, out, nil
}

func (s *server) handleSynergyGetL24Schedule(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, synergyBaseOutput, error) {
	var out synergyBaseOutput
	if err := s.withAudit(ctx, "synergy_get_l2_4_schedule", nil, func() error {
		return s.cli.Get(ctx, "/api/synergy/l2-4-schedule", nil, &out.Result)
	}); err != nil {
		return nil, synergyBaseOutput{}, err
	}
	return nil, out, nil
}
