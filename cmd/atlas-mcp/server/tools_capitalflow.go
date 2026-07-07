package server

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerCapitalFlowTools(mcpSrv *mcp.Server, s *server) {
	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "capital_flow_daily",
		Description: autoDescOr("capital_flow_daily", "Full daily Taiwan stock market capital flow report: foreign, proprietary, public bank, retail and other forces with resonance score and direction."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleCapitalFlowDaily)

	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "capital_flow_summary",
		Description: autoDescOr("capital_flow_summary", "Condensed capital flow summary suitable for a quick Taiwan market briefing."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleCapitalFlowSummary)
}

func (s *server) handleCapitalFlowDaily(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, map[string]any, error) {
	var out map[string]any
	if err := s.withAudit(ctx, "capital_flow_daily", nil, func() error {
		return s.cli.Get(ctx, "/api/capital-flow/daily", nil, &out)
	}); err != nil {
		return nil, nil, err
	}
	return nil, out, nil
}

func (s *server) handleCapitalFlowSummary(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, map[string]any, error) {
	var out map[string]any
	if err := s.withAudit(ctx, "capital_flow_summary", nil, func() error {
		return s.cli.Get(ctx, "/api/capital-flow/summary", nil, &out)
	}); err != nil {
		return nil, nil, err
	}
	return nil, out, nil
}
