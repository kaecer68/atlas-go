package llm

import (
	"context"
	"encoding/json"
)

// TestTools returns the 3 L2.3 PoC test tools: factor-weight,
// regime, liquidity. They are real Tool instances with deterministic
// handlers that return canned mock data. Intended for use by
// SectorAgentLLM.RunToolCall in PR5b's E2E tests; production code
// paths do not import this file.
//
// The tools share the same args schema ({"symbol": "<ticker>"}) so
// a single factory pattern can produce them.
func TestTools() []Tool {
	return []Tool{
		factorWeightTool(),
		regimeTool(),
		liquidityTool(),
	}
}

// toolArgs is the common input schema for all 3 test tools.
var toolArgs = json.RawMessage(`{"type":"object","properties":{"symbol":{"type":"string"}},"required":["symbol"]}`)

func factorWeightTool() Tool {
	return Tool{
		Name:        "get_factor_weight",
		Description: "Returns the current factor weights for a given symbol (mock data for L2.3 PoC).",
		InputSchema: toolArgs,
		Handler: func(_ context.Context, args json.RawMessage) (json.RawMessage, error) {
			var a struct {
				Symbol string `json:"symbol"`
			}
			if err := json.Unmarshal(args, &a); err != nil {
				return nil, err
			}
			return json.RawMessage(`{
				"momentum": 0.35,
				"value": 0.25,
				"quality": 0.20,
				"size": 0.10,
				"volatility": 0.10
			}`), nil
		},
	}
}

func regimeTool() Tool {
	return Tool{
		Name:        "get_regime",
		Description: "Returns the current market regime classification for a given symbol (mock data for L2.3 PoC).",
		InputSchema: toolArgs,
		Handler: func(_ context.Context, args json.RawMessage) (json.RawMessage, error) {
			var a struct {
				Symbol string `json:"symbol"`
			}
			if err := json.Unmarshal(args, &a); err != nil {
				return nil, err
			}
			return json.RawMessage(`{
				"regime": "risk_on",
				"confidence": 0.78,
				"drivers": ["fed_pause", "earnings_recovery"]
			}`), nil
		},
	}
}

func liquidityTool() Tool {
	return Tool{
		Name:        "get_liquidity",
		Description: "Returns the current liquidity metrics for a given symbol (mock data for L2.3 PoC).",
		InputSchema: toolArgs,
		Handler: func(_ context.Context, args json.RawMessage) (json.RawMessage, error) {
			var a struct {
				Symbol string `json:"symbol"`
			}
			if err := json.Unmarshal(args, &a); err != nil {
				return nil, err
			}
			return json.RawMessage(`{
				"avg_daily_volume": 12500000,
				"bid_ask_spread_bps": 8,
				"market_impact_bps": 12
			}`), nil
		},
	}
}
