package server

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerNarrativeTools(mcpSrv *mcp.Server, s *server) {
	mcp.AddTool(mcpSrv, &mcp.Tool{
		Name:        "narrative_get_events",
		Description: "Latest narrative events (regime shifts, capital flows, macro shocks).",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleNarrativeGetEvents)

	mcp.AddTool(mcpSrv, &mcp.Tool{
		Name:        "narrative_get_chains",
		Description: "Current narrative chains (cause-effect graphs) for the latest detected event.",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleNarrativeGetChains)

	mcp.AddTool(mcpSrv, &mcp.Tool{
		Name:        "narrative_get_models",
		Description: "Active narrative models (regime detector, flow forecaster, etc.).",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleNarrativeGetModels)

	mcp.AddTool(mcpSrv, &mcp.Tool{
		Name:        "narrative_get_templates",
		Description: "Cause-effect templates available to the narrative model registry.",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleNarrativeGetTemplates)

	mcp.AddTool(mcpSrv, &mcp.Tool{
		Name:        "narrative_get_seasonal",
		Description: "Latest seasonal narrative packet (regime-by-month statistics).",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleNarrativeGetSeasonal)

	mcp.AddTool(mcpSrv, &mcp.Tool{
		Name:        "narrative_get_bundle",
		Description: "Compiled 'briefing bundle' (events + chains + templates) suitable for an LLM agent's morning summary.",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleNarrativeGetBundle)

	mcp.AddTool(mcpSrv, &mcp.Tool{
		Name:        "narrative_stress_index_thresholds",
		Description: "Configurable thresholds for the stress index (used by the narrative engine to flag regime shifts).",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleNarrativeStressIndexThresholds)
}

type narrativeBaseOutput struct {
	Result *map[string]any `json:"result"`
}

func (s *server) handleNarrativeGetEvents(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, narrativeBaseOutput, error) {
	var out narrativeBaseOutput
	if err := s.withAudit(ctx, "narrative_get_events", nil, func() error {
		return s.cli.Get(ctx, "/api/narrative/events", nil, &out.Result)
	}); err != nil {
		return nil, narrativeBaseOutput{}, err
	}
	return nil, out, nil
}

func (s *server) handleNarrativeGetChains(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, narrativeBaseOutput, error) {
	var out narrativeBaseOutput
	if err := s.withAudit(ctx, "narrative_get_chains", nil, func() error {
		return s.cli.Get(ctx, "/api/narrative/chains", nil, &out.Result)
	}); err != nil {
		return nil, narrativeBaseOutput{}, err
	}
	return nil, out, nil
}

func (s *server) handleNarrativeGetModels(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, narrativeBaseOutput, error) {
	var out narrativeBaseOutput
	if err := s.withAudit(ctx, "narrative_get_models", nil, func() error {
		return s.cli.Get(ctx, "/api/narrative/models", nil, &out.Result)
	}); err != nil {
		return nil, narrativeBaseOutput{}, err
	}
	return nil, out, nil
}

func (s *server) handleNarrativeGetTemplates(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, narrativeBaseOutput, error) {
	var out narrativeBaseOutput
	if err := s.withAudit(ctx, "narrative_get_templates", nil, func() error {
		return s.cli.Get(ctx, "/api/narrative/templates", nil, &out.Result)
	}); err != nil {
		return nil, narrativeBaseOutput{}, err
	}
	return nil, out, nil
}

func (s *server) handleNarrativeGetSeasonal(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, narrativeBaseOutput, error) {
	var out narrativeBaseOutput
	if err := s.withAudit(ctx, "narrative_get_seasonal", nil, func() error {
		return s.cli.Get(ctx, "/api/narrative/seasonal", nil, &out.Result)
	}); err != nil {
		return nil, narrativeBaseOutput{}, err
	}
	return nil, out, nil
}

func (s *server) handleNarrativeGetBundle(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, narrativeBaseOutput, error) {
	var out narrativeBaseOutput
	if err := s.withAudit(ctx, "narrative_get_bundle", nil, func() error {
		return s.cli.Get(ctx, "/api/narrative/bundle", nil, &out.Result)
	}); err != nil {
		return nil, narrativeBaseOutput{}, err
	}
	return nil, out, nil
}

func (s *server) handleNarrativeStressIndexThresholds(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, narrativeBaseOutput, error) {
	var out narrativeBaseOutput
	if err := s.withAudit(ctx, "narrative_stress_index_thresholds", nil, func() error {
		return s.cli.Get(ctx, "/api/narrative/stress-index/thresholds", nil, &out.Result)
	}); err != nil {
		return nil, narrativeBaseOutput{}, err
	}
	return nil, out, nil
}
