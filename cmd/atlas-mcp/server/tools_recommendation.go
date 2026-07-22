package server

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerRecommendationTools(mcpSrv *mcp.Server, s *server) {
	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "get_recommendations",
		Description: autoDescOr("get_recommendations", "Tier-appropriate portfolio allocation recommendations (growth, momentum, defensive, all_weather, value). Returns market light overview for free tier. These are portfolio-level strategy IDs — for market signal detectors (e.g. foreign-3day-inflow, margin-balance-extreme), use strategy_list_active."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleGetRecommendations)
}

func (s *server) handleGetRecommendations(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, map[string]any, error) {
	var out map[string]any
	if err := s.withAudit(ctx, "get_recommendations", nil, func() error {
		return s.cli.Get(ctx, "/api/recommendations", nil, &out)
	}); err != nil {
		return nil, nil, err
	}
	return nil, out, nil
}
