package server

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerExperimentTools(mcpSrv *mcp.Server, s *server) {
	mcp.AddTool(mcpSrv, &mcp.Tool{
		Name:        "experiment_diff",
		Description: "Diff between a candidate experiment and the baseline (config + metrics comparison).",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleExperimentDiff)

	mcp.AddTool(mcpSrv, &mcp.Tool{
		Name:        "experiment_history",
		Description: "Historical list of experiments (judge results, promotions, reverts).",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleExperimentHistory)
}

type experimentIDInput struct {
	ExperimentID string `json:"experiment_id" jsonschema:"the experiment id"`
}

type experimentBaseOutput struct {
	Result *map[string]any `json:"result"`
}

func (s *server) handleExperimentDiff(ctx context.Context, _ *mcp.CallToolRequest, in experimentIDInput) (*mcp.CallToolResult, experimentBaseOutput, error) {
	var out experimentBaseOutput
	if err := s.withAudit("experiment_diff", []string{"experiment_id"}, func() error {
		return s.cli.Get(ctx, "/api/experiment/diff", nil, &out.Result)
	}); err != nil {
		return nil, experimentBaseOutput{}, err
	}
	return nil, out, nil
}

func (s *server) handleExperimentHistory(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, experimentBaseOutput, error) {
	var out experimentBaseOutput
	if err := s.withAudit("experiment_history", nil, func() error {
		return s.cli.Get(ctx, "/api/experiment/history", nil, &out.Result)
	}); err != nil {
		return nil, experimentBaseOutput{}, err
	}
	return nil, out, nil
}
