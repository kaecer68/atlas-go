package server

import (
	"context"
	"net/url"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type marketExplainInput struct {
	Format string `json:"format,omitempty" jsonschema:"output format: emoji (default) or plain (no emoji)"`
}

func registerMarketExplainTools(mcpSrv *mcp.Server, s *server) {
	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "explain_market_move",
		Description: autoDescOr("explain_market_move", "提供繁體中文的「為什麼漲跌」市場解說 (HTTP: GET /api/market/explain)。回傳今日台股漲跌原因，包含大盤表現、資金面、國際環境與風險提示。接受 optional format=plain 以移除 emoji。"),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleExplainMarketMove)
}

func (s *server) handleExplainMarketMove(ctx context.Context, _ *mcp.CallToolRequest, in marketExplainInput) (*mcp.CallToolResult, map[string]any, error) {
	var out map[string]any
	var q url.Values
	if in.Format != "" {
		q = url.Values{"format": {in.Format}}
	}
	if err := s.withAudit(ctx, "explain_market_move", nil, func() error {
		if err := s.cli.Get(ctx, "/api/market/explain", q, &out); err != nil {
			return err
		}
		var chains map[string]any
		if err := s.cli.Get(ctx, "/api/narrative/chains", nil, &chains); err == nil {
			out["causal_chains"] = chains
		}
		return nil
	}); err != nil {
		return nil, nil, err
	}
	return nil, out, nil
}
