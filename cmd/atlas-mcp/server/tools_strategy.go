package server

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerStrategyTools(mcpSrv *mcp.Server, s *server) {
	mcp.AddTool(mcpSrv, &mcp.Tool{
		Name:        "strategy_get_layers",
		Description: autoDescOr("strategy_get_layers", "All strategy layers (L1-L5) currently configured in the system."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleStrategyGetLayers)

	mcp.AddTool(mcpSrv, &mcp.Tool{
		Name:        "strategy_get",
		Description: autoDescOr("strategy_get", "Fetch a single strategy by id (returns full config + state metadata)."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleStrategyGet)

	mcp.AddTool(mcpSrv, &mcp.Tool{
		Name:        "strategy_get_attribution",
		Description: autoDescOr("strategy_get_attribution", "Performance attribution for a strategy over the requested window."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleStrategyGetAttribution)

	mcp.AddTool(mcpSrv, &mcp.Tool{
		Name:        "strategy_get_summary",
		Description: autoDescOr("strategy_get_summary", "Compact summary of a strategy (hit rate, Sharpe, drawdown, regime behavior)."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleStrategyGetSummary)
}

type strategyBaseOutput struct {
	Result *map[string]any `json:"result"`
}

type strategyIDInput struct {
	ID string `json:"id" jsonschema:"the strategy id"`
}

func (s *server) handleStrategyGetLayers(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, strategyBaseOutput, error) {
	var out strategyBaseOutput
	if err := s.withAudit(ctx, "strategy_get_layers", nil, func() error {
		return s.cli.Get(ctx, "/api/strategies/layers", nil, &out.Result)
	}); err != nil {
		return nil, strategyBaseOutput{}, err
	}
	return nil, out, nil
}

func (s *server) handleStrategyGet(ctx context.Context, _ *mcp.CallToolRequest, in strategyIDInput) (*mcp.CallToolResult, strategyBaseOutput, error) {
	var out strategyBaseOutput
	if err := s.withAudit(ctx, "strategy_get", []string{"id"}, func() error {
		return s.cli.Get(ctx, "/api/strategies/"+in.ID, nil, &out.Result)
	}); err != nil {
		return nil, strategyBaseOutput{}, err
	}
	return nil, out, nil
}

func (s *server) handleStrategyGetAttribution(ctx context.Context, _ *mcp.CallToolRequest, in strategyIDInput) (*mcp.CallToolResult, strategyBaseOutput, error) {
	var out strategyBaseOutput
	if err := s.withAudit(ctx, "strategy_get_attribution", []string{"id"}, func() error {
		return s.cli.Get(ctx, "/api/strategies/"+in.ID+"/attribution", nil, &out.Result)
	}); err != nil {
		return nil, strategyBaseOutput{}, err
	}
	return nil, out, nil
}

func (s *server) handleStrategyGetSummary(ctx context.Context, _ *mcp.CallToolRequest, in strategyIDInput) (*mcp.CallToolResult, strategyBaseOutput, error) {
	var out strategyBaseOutput
	if err := s.withAudit(ctx, "strategy_get_summary", []string{"id"}, func() error {
		return s.cli.Get(ctx, "/api/strategies/"+in.ID+"/summary", nil, &out.Result)
	}); err != nil {
		return nil, strategyBaseOutput{}, err
	}
	return nil, out, nil
}
