package server

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerStrategyRankerTools(mcpSrv *mcp.Server, s *server) {
	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "strategy_ranker",
		Description: autoDescOr("strategy_ranker", "Return the current active strategies ranked by performance with free/registered/premium tier labels."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleStrategyRanker)
}

func (s *server) handleStrategyRanker(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, strategyBaseOutput, error) {
	var out strategyBaseOutput
	if err := s.withAudit(ctx, "strategy_ranker", nil, func() error {
		return s.cli.Get(ctx, "/api/strategy-ranker/rank", nil, &out.Result)
	}); err != nil {
		return nil, strategyBaseOutput{}, err
	}
	return nil, out, nil
}
