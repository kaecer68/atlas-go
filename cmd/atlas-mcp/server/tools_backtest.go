package server

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type BacktestStatusOutput struct {
	Result *map[string]any `json:"result"`
}

type BacktestSignalsOutput struct {
	Result *map[string]any `json:"result"`
}

func (s *server) handleBacktestStatus(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, BacktestStatusOutput, error) {
	var out BacktestStatusOutput
	if err := s.withAudit(ctx, "backtest_status", nil, func() error {
		return s.cli.Get(ctx, "/api/backtest/status", nil, &out.Result)
	}); err != nil {
		return nil, BacktestStatusOutput{}, err
	}
	return nil, out, nil
}

func (s *server) handleBacktestSignals(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, BacktestSignalsOutput, error) {
	var out BacktestSignalsOutput
	if err := s.withAudit(ctx, "backtest_signals", nil, func() error {
		return s.cli.Get(ctx, "/api/backtest/signals", nil, &out.Result)
	}); err != nil {
		return nil, BacktestSignalsOutput{}, err
	}
	return nil, out, nil
}

func registerBacktestTools(mcpSrv *mcp.Server, s *server) {
	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "backtest_status",
		Description: autoDescOr("backtest_status", "Latest backtest run summary (last_auto_date, last_auto_portfolio_val) HTTP: GET /api/backtest/status. Alternative: backtest_signals, report_get_performance."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleBacktestStatus)

	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "backtest_signals",
		Description: autoDescOr("backtest_signals", "Active backtest signals (active_signals, var_95/99, sharpe_short/long, drawdown_pct) HTTP: GET /api/backtest/signals. Alternative: backtest_status."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleBacktestSignals)
}
