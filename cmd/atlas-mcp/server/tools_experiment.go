package server

import (
	"context"
	"errors"
	"net/url"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerExperimentTools(mcpSrv *mcp.Server, s *server) {
	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "experiment_diff",
		Description: autoDescOr("experiment_diff", "Diff between a candidate experiment and the baseline (config + metrics comparison)."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleExperimentDiff)

	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "experiment_history",
		Description: autoDescOr("experiment_history", "Historical list of experiments (judge results, promotions, reverts)."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleExperimentHistory)

	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "experiment_promote",
		Description: autoDescOr("experiment_promote", "Promote a candidate experiment to baseline. Side-effect: rewrites baseline policy. Requires ATLAS_API_KEY."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(true)},
	}, s.handleExperimentPromote)

	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "experiment_revert",
		Description: autoDescOr("experiment_revert", "Revert a candidate experiment (cancel promotion / restore prior baseline). Side-effect: rewrites baseline policy. Requires ATLAS_API_KEY."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(true)},
	}, s.handleExperimentRevert)
}

type experimentIDInput struct {
	ExperimentID string `json:"experiment_id" jsonschema:"the experiment id"`
}

type experimentBaseOutput struct {
	Result *map[string]any `json:"result"`
}

func (s *server) handleExperimentDiff(ctx context.Context, _ *mcp.CallToolRequest, in experimentIDInput) (*mcp.CallToolResult, experimentBaseOutput, error) {
	if in.ExperimentID == "" {
		return nil, experimentBaseOutput{}, errors.New("experiment_diff: experiment_id is required")
	}
	var out experimentBaseOutput
	query := url.Values{}
	query.Set("experiment_id", in.ExperimentID)
	if err := s.withAudit(ctx, "experiment_diff", []string{"experiment_id"}, func() error {
		return s.cli.Get(ctx, "/api/experiment/diff", query, &out.Result)
	}); err != nil {
		return nil, experimentBaseOutput{}, err
	}
	return nil, out, nil
}

func (s *server) handleExperimentHistory(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, experimentBaseOutput, error) {
	var out experimentBaseOutput
	if err := s.withAudit(ctx, "experiment_history", nil, func() error {
		return s.cli.Get(ctx, "/api/experiment/history", nil, &out.Result)
	}); err != nil {
		return nil, experimentBaseOutput{}, err
	}
	return nil, out, nil
}

func (s *server) handleExperimentPromote(ctx context.Context, _ *mcp.CallToolRequest, in experimentIDInput) (*mcp.CallToolResult, experimentBaseOutput, error) {
	if in.ExperimentID == "" {
		return nil, experimentBaseOutput{}, errors.New("experiment_promote: experiment_id is required")
	}
	var out experimentBaseOutput
	body := map[string]string{"experiment_id": in.ExperimentID}
	if err := s.withAudit(ctx, "experiment_promote", []string{"experiment_id"}, func() error {
		return s.cli.PostJSON(ctx, "/api/experiment/promote", body, &out.Result)
	}); err != nil {
		return nil, experimentBaseOutput{}, err
	}
	return nil, out, nil
}

func (s *server) handleExperimentRevert(ctx context.Context, _ *mcp.CallToolRequest, in experimentIDInput) (*mcp.CallToolResult, experimentBaseOutput, error) {
	if in.ExperimentID == "" {
		return nil, experimentBaseOutput{}, errors.New("experiment_revert: experiment_id is required")
	}
	var out experimentBaseOutput
	body := map[string]string{"experiment_id": in.ExperimentID}
	if err := s.withAudit(ctx, "experiment_revert", []string{"experiment_id"}, func() error {
		return s.cli.PostJSON(ctx, "/api/experiment/revert", body, &out.Result)
	}); err != nil {
		return nil, experimentBaseOutput{}, err
	}
	return nil, out, nil
}
