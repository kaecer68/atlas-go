package server

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerMarketExplainTools(mcpSrv *mcp.Server, s *server) {
	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "explain_market_move",
		Description: autoDescOr("explain_market_move", "提供繁體中文的「為什麼漲跌」市場解說。回傳今日台股漲跌原因，包含大盤表現、資金面、國際環境與風險提示。適合散戶快速理解市場變動的背後因素。LLM agent（如 Hermes、OpenClaw）可用此工具為用戶生成白話市場解讀。"),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleExplainMarketMove)
}

func (s *server) handleExplainMarketMove(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, map[string]any, error) {
	var out map[string]any
	if err := s.withAudit(ctx, "explain_market_move", nil, func() error {
		return s.cli.Get(ctx, "/api/market/explain", nil, &out)
	}); err != nil {
		return nil, nil, err
	}
	return nil, out, nil
}
