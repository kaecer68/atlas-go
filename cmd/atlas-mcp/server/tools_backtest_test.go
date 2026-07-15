package server

import (
	"context"
	"testing"
)

func TestHandleBacktestStatus_HitCorrectBackendPath(t *testing.T) {
	s, rec, cleanup := newTestHarness(t)
	defer cleanup()

	rec.SetResponseBody([]byte(`{"result":{"last_auto_date":"2026-05-18","last_auto_portfolio_val":2787086.542433}}`))

	_, _, err := s.handleBacktestStatus(context.Background(), nil, struct{}{})
	if err != nil {
		t.Fatalf("handleBacktestStatus: %v", err)
	}
	if rec.path != "/api/backtest/status" {
		t.Fatalf("path=%s", rec.path)
	}
}

func TestHandleBacktestSignals_HitCorrectBackendPath(t *testing.T) {
	s, rec, cleanup := newTestHarness(t)
	defer cleanup()

	rec.SetResponseBody([]byte(`{"result":{"active_signals":["CIRCUIT_BREAKER"],"drawdown_pct":1,"sharpe_long":0.287,"sharpe_short":0.542,"var_95":-0.022,"var_99":-0.036}}`))

	_, _, err := s.handleBacktestSignals(context.Background(), nil, struct{}{})
	if err != nil {
		t.Fatalf("handleBacktestSignals: %v", err)
	}
	if rec.path != "/api/backtest/signals" {
		t.Fatalf("path=%s", rec.path)
	}
}
