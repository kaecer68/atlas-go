package server

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerNarrativeTools(mcpSrv *mcp.Server, s *server) {
	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "narrative_get_events",
		Description: autoDescOr("narrative_get_events", "Latest narrative events (regime shifts, capital flows, macro shocks) HTTP: GET /api/narrative/events. Alternative: event_calendar, narrative_get_chains."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleNarrativeGetEvents)

	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "narrative_get_chains",
		Description: autoDescOr("narrative_get_chains", "Current narrative chains (cause-effect graphs) for the latest detected event.  HTTP: GET /api/narrative/chains. Alternative: narrative_get_events, narrative_get_models."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleNarrativeGetChains)

	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "narrative_get_models",
		Description: autoDescOr("narrative_get_models", "Active narrative models (regime detector, flow forecaster, etc.) HTTP: GET /api/narrative/models. Alternative: narrative_get_templates, narrative_get_seasonal."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleNarrativeGetModels)

	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "narrative_get_templates",
		Description: autoDescOr("narrative_get_templates", "Cause-effect templates available to the narrative model registry.  HTTP: GET /api/narrative/templates. Alternative: narrative_get_models, detector_registry_list."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleNarrativeGetTemplates)

	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "narrative_get_seasonal",
		Description: autoDescOr("narrative_get_seasonal", "Latest seasonal narrative packet (regime-by-month statistics) HTTP: GET /api/narrative/seasonal. Alternative: narrative_get_models, narrative_get_templates."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleNarrativeGetSeasonal)

	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "narrative_get_bundle",
		Description: autoDescOr("narrative_get_bundle", "Compiled 'briefing bundle' (events + chains + templates) suitable for an LLM agent's morning summary.  HTTP: GET /api/narrative/bundle. Alternative: mcp_quickstart, narrative_get_events."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleNarrativeGetBundle)

	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "narrative_stress_index_thresholds",
		Description: autoDescOr("narrative_stress_index_thresholds", "Configurable thresholds for the stress index (used by the narrative engine to flag regime shifts) HTTP: GET /api/narrative/stress-index/thresholds. Alternative: macro_get_stress_index_current, narrative_get_bundle."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleNarrativeStressIndexThresholds)
}

type narrativeBaseOutput struct {
	Result *map[string]any `json:"result"`
}

func (s *server) handleNarrativeGetEvents(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, narrativeBaseOutput, error) {
	var out narrativeBaseOutput
	if err := s.withAudit(ctx, "narrative_get_events", nil, func() error {
		if err := s.cli.Get(ctx, "/api/narrative/events", nil, &out.Result); err != nil {
			return err
		}
		if out.Result != nil && *out.Result != nil {
			if period, nameZH := s.fetchCurrentPeriod(ctx); period != "" {
				(*out.Result)["current_period"] = period
				(*out.Result)["current_period_name_zh"] = nameZH
				(*out.Result)["period_weight_note"] = "detector confidence multiplied by PeriodWeight(period); see ATLAS_METHODOLOGY.md appendix B"
			}
		}
		return nil
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
