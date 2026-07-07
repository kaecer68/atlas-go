package server

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerCapitalFlowTools(mcpSrv *mcp.Server, s *server) {
	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "capital_flow_daily",
		Description: autoDescOr("capital_flow_daily", "Return the full daily Taiwan market capital flow report: seven force scores, resonance, and quality summary."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleCapitalFlowDaily)

	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "capital_flow_summary",
		Description: autoDescOr("capital_flow_summary", "Return a condensed capital flow summary for the latest trading day."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleCapitalFlowSummary)
}

func (s *server) handleCapitalFlowDaily(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, macroBaseOutput, error) {
	var out macroBaseOutput
	if err := s.withAudit(ctx, "capital_flow_daily", nil, func() error {
		return s.cli.Get(ctx, "/api/capital-flow/daily", nil, &out.Result)
	}); err != nil {
		return nil, macroBaseOutput{}, err
	}
	return nil, out, nil
}

func (s *server) handleCapitalFlowSummary(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, macroBaseOutput, error) {
	var out macroBaseOutput
	if err := s.withAudit(ctx, "capital_flow_summary", nil, func() error {
		return s.cli.Get(ctx, "/api/capital-flow/summary", nil, &out.Result)
	}); err != nil {
		return nil, macroBaseOutput{}, err
	}
	return nil, out, nil
}
