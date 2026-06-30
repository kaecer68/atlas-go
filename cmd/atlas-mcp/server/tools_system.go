package server

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerSystemTools(mcpSrv *mcp.Server, s *server) {
	mcp.AddTool(mcpSrv, &mcp.Tool{
		Name:        "system_get_metrics",
		Description: autoDescOr("system_get_metrics", "Live system metrics (request rate, error rate, circuit-breaker state)."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleSystemGetMetrics)

	mcp.AddTool(mcpSrv, &mcp.Tool{
		Name:        "system_get_metrics_trend",
		Description: autoDescOr("system_get_metrics_trend", "System metrics trend over a recent window (per-minute aggregates)."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleSystemGetMetricsTrend)

	mcp.AddTool(mcpSrv, &mcp.Tool{
		Name:        "system_get_thresholds",
		Description: autoDescOr("system_get_thresholds", "Configured SLO thresholds (latency, error rate, saturation)."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleSystemGetThresholds)

	mcp.AddTool(mcpSrv, &mcp.Tool{
		Name:        "system_get_data_pipeline",
		Description: autoDescOr("system_get_data_pipeline", "Data pipeline state (which channels are flowing, latency, lag)."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleSystemGetDataPipeline)

	mcp.AddTool(mcpSrv, &mcp.Tool{
		Name:        "system_get_circuit_breaker",
		Description: autoDescOr("system_get_circuit_breaker", "Circuit-breaker state per external call site."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleSystemGetCircuitBreaker)

	mcp.AddTool(mcpSrv, &mcp.Tool{
		Name:        "system_get_maturity",
		Description: autoDescOr("system_get_maturity", "Maturity ratings per module (S/E/X/U per docs/specs convention)."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleSystemGetMaturity)
}

type systemBaseOutput struct {
	Result *map[string]any `json:"result"`
}

func (s *server) handleSystemGetMetrics(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, systemBaseOutput, error) {
	var out systemBaseOutput
	if err := s.withAudit(ctx, "system_get_metrics", nil, func() error {
		return s.cli.Get(ctx, "/api/dashboard/metrics", nil, &out.Result)
	}); err != nil {
		return nil, systemBaseOutput{}, err
	}
	return nil, out, nil
}

func (s *server) handleSystemGetMetricsTrend(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, systemBaseOutput, error) {
	var out systemBaseOutput
	if err := s.withAudit(ctx, "system_get_metrics_trend", nil, func() error {
		return s.cli.Get(ctx, "/api/dashboard/metrics/trend", nil, &out.Result)
	}); err != nil {
		return nil, systemBaseOutput{}, err
	}
	return nil, out, nil
}

func (s *server) handleSystemGetThresholds(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, systemBaseOutput, error) {
	var out systemBaseOutput
	if err := s.withAudit(ctx, "system_get_thresholds", nil, func() error {
		return s.cli.Get(ctx, "/api/dashboard/metrics/thresholds", nil, &out.Result)
	}); err != nil {
		return nil, systemBaseOutput{}, err
	}
	return nil, out, nil
}

func (s *server) handleSystemGetDataPipeline(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, systemBaseOutput, error) {
	var out systemBaseOutput
	if err := s.withAudit(ctx, "system_get_data_pipeline", nil, func() error {
		return s.cli.Get(ctx, "/api/dashboard/data-pipeline", nil, &out.Result)
	}); err != nil {
		return nil, systemBaseOutput{}, err
	}
	return nil, out, nil
}

func (s *server) handleSystemGetCircuitBreaker(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, systemBaseOutput, error) {
	var out systemBaseOutput
	if err := s.withAudit(ctx, "system_get_circuit_breaker", nil, func() error {
		return s.cli.Get(ctx, "/api/dashboard/circuit-breaker", nil, &out.Result)
	}); err != nil {
		return nil, systemBaseOutput{}, err
	}
	return nil, out, nil
}

func (s *server) handleSystemGetMaturity(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, systemBaseOutput, error) {
	var out systemBaseOutput
	if err := s.withAudit(ctx, "system_get_maturity", nil, func() error {
		return s.cli.Get(ctx, "/api/dashboard/maturity", nil, &out.Result)
	}); err != nil {
		return nil, systemBaseOutput{}, err
	}
	return nil, out, nil
}
