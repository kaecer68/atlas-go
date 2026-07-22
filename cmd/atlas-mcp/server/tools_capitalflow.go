package server

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerCapitalFlowTools(mcpSrv *mcp.Server, s *server) {
	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "capital_flow_daily",
		Description: autoDescOr("capital_flow_daily", "Full daily Taiwan stock market capital flow report: 七維錢潮雷達（3+2+2 分層）— three official actor forces (foreign / institutional / dealer, T86 first-party), two behavioral proxies (government / retail) and two leading / cross-market signals (foreign futures OI / TSM ADR). Each dimension carries role, source, unit and availability flags; the resonance model only votes on the official_actor tier."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleCapitalFlowDaily)

	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "capital_flow_summary",
		Description: autoDescOr("capital_flow_summary", "Condensed 七維錢潮雷達（3+2+2 分層） summary suitable for a quick Taiwan market briefing; actor consensus is derived from the three official actor dimensions only."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleCapitalFlowSummary)
}

func (s *server) handleCapitalFlowDaily(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, map[string]any, error) {
	var out map[string]any
	var fetchErr error
	if err := s.withAudit(ctx, "capital_flow_daily", nil, func() error {
		fetchErr = s.cli.Get(ctx, "/api/capital-flow/daily", nil, &out)
		return fetchErr
	}); err != nil {
		return nil, nil, err
	}
	injectDataQuality(out, "capital_flow_daily", fetchErr)
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
