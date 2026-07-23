package server

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// tools_industry_ext.go — atlas-mcp coverage audit PR 2 (2026-07-12)
//
// New read-only MCP tools that surface industry / risk / cross-market
// extension REST API endpoints to MCP clients. Backend endpoints exist
// already (see internal/monitoring/api/industry/handlers.go,
// internal/monitoring/api/macro/handlers.go,
// internal/monitoring/api/live/handlers.go,
// internal/monitoring/dashboard_api.go); this PR closes the MCP
// coverage gap identified in the 2026-07-12 audit.
//
// All handlers are read-only and return *map[string]any passthrough
// so the same wire format is preserved end-to-end.

type CalendarEventsOutput struct {
	Result *map[string]any `json:"result"`
}

type SectorAllocationPlanOutput struct {
	Result *map[string]any `json:"result"`
}

type ChannelHealthOutput struct {
	Result *map[string]any `json:"result"`
}

type TaiwanStressIndexOutput struct {
	Result *map[string]any `json:"result"`
}

type RiskExposureOutput struct {
	Result *map[string]any `json:"result"`
}

func (s *server) handleCalendarEvents(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, CalendarEventsOutput, error) {
	var out CalendarEventsOutput
	if err := s.withAudit(ctx, "calendar_events", nil, func() error {
		return s.cli.Get(ctx, "/api/dashboard/calendar-events", nil, &out.Result)
	}); err != nil {
		return nil, CalendarEventsOutput{}, err
	}
	return nil, out, nil
}

func (s *server) handleSectorAllocationPlan(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, SectorAllocationPlanOutput, error) {
	var out SectorAllocationPlanOutput
	if err := s.withAudit(ctx, "sector_allocation_plan", nil, func() error {
		return s.cli.Get(ctx, "/api/dashboard/sector-allocation-plan", nil, &out.Result)
	}); err != nil {
		return nil, SectorAllocationPlanOutput{}, err
	}
	return nil, out, nil
}

func (s *server) handleChannelHealth(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, ChannelHealthOutput, error) {
	var out ChannelHealthOutput
	if err := s.withAudit(ctx, "channel_health", nil, func() error {
		return s.cli.Get(ctx, "/api/dashboard/channel-health", nil, &out.Result)
	}); err != nil {
		return nil, ChannelHealthOutput{}, err
	}
	return nil, out, nil
}

func (s *server) handleTaiwanStressIndex(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, TaiwanStressIndexOutput, error) {
	var out TaiwanStressIndexOutput
	if err := s.withAudit(ctx, "taiwan_stress_index", nil, func() error {
		return s.cli.Get(ctx, "/api/taiwan/stress-index", nil, &out.Result)
	}); err != nil {
		return nil, TaiwanStressIndexOutput{}, err
	}
	return nil, out, nil
}

func (s *server) handleRiskExposure(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, RiskExposureOutput, error) {
	var out RiskExposureOutput
	if err := s.withAudit(ctx, "risk_exposure", nil, func() error {
		return s.cli.Get(ctx, "/api/dashboard/risk-exposure", nil, &out.Result)
	}); err != nil {
		return nil, RiskExposureOutput{}, err
	}
	return nil, out, nil
}

func registerIndustryExtTools(mcpSrv *mcp.Server, s *server) {
	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "calendar_events",
		Description: autoDescOr("calendar_events", "Industry / market calendar events (ETF rebalances, MSCI, revenue, shareholder meetings, window dressing, holidays). 14-day forward window.  HTTP: GET /api/dashboard/calendar-events. Alternative: event_calendar, event_flow_prediction."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleCalendarEvents)

	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "sector_allocation_plan",
		Description: autoDescOr("sector_allocation_plan", "Latest persisted simulation sector-allocation snapshot, including target/current/delta, provenance, fallback status, mutation receipt, and next-session consumption evidence.  HTTP: GET /api/dashboard/sector-allocation-plan. Alternative: industry_sector_list, industry_sector_lookup."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleSectorAllocationPlan)

	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "channel_health",
		Description: autoDescOr("channel_health", "Channel-level health summary (channel_id, status, updated_at) for the data ingestion pipeline.  HTTP: GET /api/dashboard/channel-health. Alternative: system_get_data_pipeline, macro_get_ingest_status."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleChannelHealth)

	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "taiwan_stress_index",
		Description: autoDescOr("taiwan_stress_index", "Taiwan market stress index (TRJ narrative) — score, regime, components by source. Use for risk appetite assessment.  HTTP: GET /api/taiwan/stress-index. Alternative: macro_get_stress_index_current, narrative_stress_index_thresholds."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleTaiwanStressIndex)

	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "risk_exposure",
		Description: autoDescOr("risk_exposure", "Current portfolio risk exposure: var_95/99, cvar_95, max_drawdown_pct, cash_ratio, sector/factor/concentration breakdown.  HTTP: GET /api/dashboard/risk-exposure. Alternative: risk_get_metrics, risk_get_drawdown."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleRiskExposure)
}
