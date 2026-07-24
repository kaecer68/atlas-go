package server

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerStrategyRankerTools(mcpSrv *mcp.Server, s *server) {
	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "strategy_ranker",
		Description: autoDescOr("strategy_ranker", "Return the current active market techniques ranked by performance with free/registered/premium tier labels. These are signal detectors (e.g. foreign-3day-inflow, margin-balance-extreme), NOT portfolio allocation strategies — see get_recommendations for portfolio strategies.  HTTP: GET /api/strategy-ranker/rank. Alternative: strategy_get_summary, strategy_list_active."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleStrategyRanker)
}

// strategyRankerOutput decodes the JSON array returned by
// GET /api/strategy-ranker/rank (internal/strategy_ranker/handler.go
// HandleRank). Items stay as map[string]any to keep MCP schema decoupled
// from strategy_ranker.RankedReport.
type strategyRankerOutput struct {
	Strategies []map[string]any `json:"strategies"`
}

func (s *server) handleStrategyRanker(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, strategyRankerOutput, error) {
	var out strategyRankerOutput
	if err := s.withAudit(ctx, "strategy_ranker", nil, func() error {
		return s.cli.Get(ctx, "/api/strategy-ranker/rank", nil, &out.Strategies)
	}); err != nil {
		return nil, strategyRankerOutput{}, err
	}
	return nil, out, nil
}
