package server

import (
	"context"
	"fmt"
	"net/url"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerMacroTools(mcpSrv *mcp.Server, s *server) {
	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "macro_get_snapshot_latest",
		Description: autoDescOr("macro_get_snapshot_latest", "Return the latest macro data snapshot (HTTP: GET /api/macro/snapshot/latest) — from on-disk cache (5-min refresh cycle, may lag real-time data). Use crossmarket_get_us_indices for real-time US stock/index data."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleMacroGetSnapshotLatest)

	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "macro_get_snapshot_history",
		Description: autoDescOr("macro_get_snapshot_history", "Macro snapshot history over the last N days (HTTP: GET /api/macro/snapshot/timeline); default 30, max 365."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleMacroGetSnapshotHistory)

	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "macro_get_stress_index_current",
		Description: autoDescOr("macro_get_stress_index_current", "Current Taiwan stress index (TRJ narrative). Use to assess market risk appetite."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleMacroGetStressIndexCurrent)

	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "macro_get_stress_index_history",
		Description: autoDescOr("macro_get_stress_index_history", "Stress index history over the last N days."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleMacroGetStressIndexHistory)

	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "macro_get_capital_flow_latest",
		Description: autoDescOr("macro_get_capital_flow_latest", "Latest foreign / institutional / retail capital flow snapshot."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleMacroGetCapitalFlowLatest)

	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "macro_get_ingest_status",
		Description: autoDescOr("macro_get_ingest_status", "Channel ingestion status (last fetch times, error counts). Use to diagnose data freshness."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleMacroGetIngestStatus)
}

type macroBaseOutput struct {
	Result *map[string]any `json:"result"`
}

type macroSnapshotHistoryInput struct {
	Days int `json:"days,omitempty" jsonschema:"how many days back; default 30"`
}

type macroStressIndexHistoryInput struct {
	Days int `json:"days,omitempty" jsonschema:"how many days back; default 30"`
}

func (s *server) handleMacroGetSnapshotLatest(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, macroBaseOutput, error) {
	var out macroBaseOutput
	var fetchErr error
	if err := s.withAudit(ctx, "macro_get_snapshot_latest", nil, func() error {
		fetchErr = s.cli.Get(ctx, "/api/macro/snapshot/latest", nil, &out.Result)
		return fetchErr
	}); err != nil {
		return nil, macroBaseOutput{}, err
	}
	if out.Result != nil {
		if dq := dataQualityFromIngestStatus(*out.Result, "macro_get_snapshot_latest"); dq != nil {
			(*out.Result)["data_quality"] = dq.ToMap()
		}
	}
	return nil, out, nil
}

func (s *server) handleMacroGetSnapshotHistory(ctx context.Context, _ *mcp.CallToolRequest, in macroSnapshotHistoryInput) (*mcp.CallToolResult, macroBaseOutput, error) {
	days := in.Days
	if days <= 0 {
		days = 30
	}
	if days > 365 {
		days = 365
	}
	q := url.Values{"days": {fmt.Sprintf("%d", days)}}
	var out macroBaseOutput
	if err := s.withAudit(ctx, "macro_get_snapshot_history", []string{"days"}, func() error {
		return s.cli.Get(ctx, "/api/macro/snapshot/timeline", q, &out.Result)
	}); err != nil {
		return nil, macroBaseOutput{}, err
	}
	return nil, out, nil
}

func (s *server) handleMacroGetStressIndexCurrent(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, macroBaseOutput, error) {
	var out macroBaseOutput
	if err := s.withAudit(ctx, "macro_get_stress_index_current", nil, func() error {
		return s.cli.Get(ctx, "/api/narrative/stress-index/current", nil, &out.Result)
	}); err != nil {
		return nil, macroBaseOutput{}, err
	}
	return nil, out, nil
}

func (s *server) handleMacroGetStressIndexHistory(ctx context.Context, _ *mcp.CallToolRequest, in macroStressIndexHistoryInput) (*mcp.CallToolResult, macroBaseOutput, error) {
	days := in.Days
	if days <= 0 {
		days = 30
	}
	if days > 365 {
		days = 365
	}
	q := url.Values{"days": {fmt.Sprintf("%d", days)}}
	var out macroBaseOutput
	if err := s.withAudit(ctx, "macro_get_stress_index_history", []string{"days"}, func() error {
		return s.cli.Get(ctx, "/api/narrative/stress-index/history", q, &out.Result)
	}); err != nil {
		return nil, macroBaseOutput{}, err
	}
	return nil, out, nil
}

func (s *server) handleMacroGetCapitalFlowLatest(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, macroBaseOutput, error) {
	var out macroBaseOutput
	if err := s.withAudit(ctx, "macro_get_capital_flow_latest", nil, func() error {
		return s.cli.Get(ctx, "/api/macro/capital-flow/latest", nil, &out.Result)
	}); err != nil {
		return nil, macroBaseOutput{}, err
	}
	return nil, out, nil
}

func (s *server) handleMacroGetIngestStatus(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, macroBaseOutput, error) {
	var out macroBaseOutput
	if err := s.withAudit(ctx, "macro_get_ingest_status", nil, func() error {
		return s.cli.Get(ctx, "/api/dashboard/macro-data-health", nil, &out.Result)
	}); err != nil {
		return nil, macroBaseOutput{}, err
	}
	return nil, out, nil
}
