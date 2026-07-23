package server

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerCrossmarketTools(mcpSrv *mcp.Server, s *server) {
	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "crossmarket_get_status",
		Description: autoDescOr("crossmarket_get_status", "Cross-market data feed status (US indices source, freshness, error count)."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleCrossmarketGetStatus)

	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "crossmarket_get_correlation",
		Description: autoDescOr("crossmarket_get_correlation", "Latest cross-market correlation matrix (Taiwan sector vs US indices)."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleCrossmarketGetCorrelation)

	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "crossmarket_get_us_indices",
		Description: autoDescOr("crossmarket_get_us_indices", "Latest US index and tech stock snapshots — live-fetched from Yahoo Finance (real-time freshness). Includes S&P 500, NASDAQ, Dow Jones, SOX, NVDA, AAPL, MSFT, TSM ADR. Prefer this over macro_get_snapshot_latest when you need the most current data."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleCrossmarketGetUsIndices)
}

type crossmarketBaseOutput struct {
	Result *map[string]any `json:"result"`
}

func (s *server) handleCrossmarketGetStatus(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, crossmarketBaseOutput, error) {
	var out crossmarketBaseOutput
	if err := s.withAudit(ctx, "crossmarket_get_status", nil, func() error {
		return s.cli.Get(ctx, "/api/cross-market/status", nil, &out.Result)
	}); err != nil {
		return nil, crossmarketBaseOutput{}, err
	}
	return nil, out, nil
}

func (s *server) handleCrossmarketGetCorrelation(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, crossmarketBaseOutput, error) {
	var out crossmarketBaseOutput
	var fetchErr error
	if err := s.withAudit(ctx, "crossmarket_get_correlation", nil, func() error {
		fetchErr = s.cli.Get(ctx, "/api/cross-market/correlation", nil, &out.Result)
		return fetchErr
	}); err != nil {
		return nil, crossmarketBaseOutput{}, err
	}
	if out.Result != nil {
		injectDataQuality(*out.Result, "crossmarket_get_correlation", fetchErr)
	}
	return nil, out, nil
}

func (s *server) handleCrossmarketGetUsIndices(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, crossmarketBaseOutput, error) {
	var out crossmarketBaseOutput
	if err := s.withAudit(ctx, "crossmarket_get_us_indices", nil, func() error {
		return s.cli.Get(ctx, "/api/dashboard/us-indices", nil, &out.Result)
	}); err != nil {
		return nil, crossmarketBaseOutput{}, err
	}
	return nil, out, nil
}
