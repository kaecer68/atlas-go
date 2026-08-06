package server

import (
	"context"
	"fmt"
	"net/url"
	"strconv"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerStockTools(mcpSrv *mcp.Server, s *server) {
	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "stock_get_quote",
		Description: autoDescOr("stock_get_quote", "Return the latest intraday quote for a Taiwan stock symbol. Coverage: TWSE-listed common stocks primarily; Fugle may also return quotes for some non-listed symbols. Out-of-scope chips/fundamentals surface as `coverage_note: NOT_COVERED`. HTTP: GET /api/stock/quote. Alternative: stock_get_technical, stock_get_chips."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleStockGetQuote)

	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "stock_get_fundamentals",
		Description: autoDescOr("stock_get_fundamentals", "Return fundamental metrics (PE, PB, PS, dividend yield, sector) for a Taiwan-listed symbol. Coverage: TWSE-listed common stocks (~1070 names) backed by data/fundamentals.json; out-of-scope symbols return 200 with `coverage_note: NOT_COVERED` instead of zero data. HTTP: GET /api/stock/fundamentals. Alternative: stock_get_quote, stock_get_technical."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleStockGetFundamentals)

	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "stock_get_chips",
		Description: autoDescOr("stock_get_chips", "Return institutional investor flow (foreign, domestic fund, dealer net buy/sell) for a Taiwan-listed symbol on a given trading day. Coverage: TWSE-listed common stocks (sourced via TWSE T86 ~1231 names; ETFs excluded). Out-of-scope symbols return 200 with `coverage_note: NOT_COVERED` (no upstream 7-day fallback wait). HTTP: GET /api/stock/chips. Alternative: stock_get_quote, stock_get_fundamentals."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleStockGetChips)

	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "stock_get_technical",
		Description: autoDescOr("stock_get_technical", "Return simple technical indicators (SMA20, SMA50, RSI14) for a Taiwan-listed symbol over the last N days. Coverage: TWSE-listed common stocks whose bars exist in QuoteStore or are fetchable via Fugle. Out-of-scope symbols return 200 with `coverage_note: NOT_COVERED`. HTTP: GET /api/stock/technical. Alternative: stock_get_quote, stock_get_fundamentals."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleStockGetTechnical)
}

type stockSymbolInput struct {
	Symbol string `json:"symbol" jsonschema:"the Taiwan stock symbol, e.g. 2330"`
}

type stockTechnicalInput struct {
	Symbol string `json:"symbol" jsonschema:"the Taiwan stock symbol, e.g. 2330"`
	Days   int    `json:"days,omitempty" jsonschema:"number of trading days to include; default 90, max 365"`
}

type stockChipsInput struct {
	Symbol string `json:"symbol" jsonschema:"the Taiwan stock symbol, e.g. 2330"`
	Date   string `json:"date,omitempty" jsonschema:"trading day in YYYYMMDD; defaults to the most recent trading day with data"`
}

type stockBaseOutput struct {
	Result *map[string]any `json:"result"`
}

func (s *server) handleStockGetQuote(ctx context.Context, _ *mcp.CallToolRequest, in stockSymbolInput) (*mcp.CallToolResult, stockBaseOutput, error) {
	if in.Symbol == "" {
		return nil, stockBaseOutput{}, fmt.Errorf("stock_get_quote: symbol is required")
	}
	var out stockBaseOutput
	q := url.Values{"symbol": {in.Symbol}}
	if err := s.withAudit(ctx, "stock_get_quote", []string{"symbol"}, func() error {
		return s.cli.Get(ctx, "/api/stock/quote", q, &out.Result)
	}); err != nil {
		return nil, stockBaseOutput{}, err
	}
	return nil, out, nil
}

func (s *server) handleStockGetFundamentals(ctx context.Context, _ *mcp.CallToolRequest, in stockSymbolInput) (*mcp.CallToolResult, stockBaseOutput, error) {
	if in.Symbol == "" {
		return nil, stockBaseOutput{}, fmt.Errorf("stock_get_fundamentals: symbol is required")
	}
	var out stockBaseOutput
	q := url.Values{"symbol": {in.Symbol}}
	if err := s.withAudit(ctx, "stock_get_fundamentals", []string{"symbol"}, func() error {
		return s.cli.Get(ctx, "/api/stock/fundamentals", q, &out.Result)
	}); err != nil {
		return nil, stockBaseOutput{}, err
	}
	return nil, out, nil
}

func (s *server) handleStockGetChips(ctx context.Context, _ *mcp.CallToolRequest, in stockChipsInput) (*mcp.CallToolResult, stockBaseOutput, error) {
	if in.Symbol == "" {
		return nil, stockBaseOutput{}, fmt.Errorf("stock_get_chips: symbol is required")
	}
	var out stockBaseOutput
	q := url.Values{"symbol": {in.Symbol}}
	if in.Date != "" {
		q.Set("date", in.Date)
	}
	if err := s.withAudit(ctx, "stock_get_chips", []string{"symbol", "date"}, func() error {
		return s.cli.Get(ctx, "/api/stock/chips", q, &out.Result)
	}); err != nil {
		return nil, stockBaseOutput{}, err
	}
	return nil, out, nil
}

func (s *server) handleStockGetTechnical(ctx context.Context, _ *mcp.CallToolRequest, in stockTechnicalInput) (*mcp.CallToolResult, stockBaseOutput, error) {
	if in.Symbol == "" {
		return nil, stockBaseOutput{}, fmt.Errorf("stock_get_technical: symbol is required")
	}
	if in.Days <= 0 {
		in.Days = 90
	}
	if in.Days > 365 {
		in.Days = 365
	}
	var out stockBaseOutput
	q := url.Values{"symbol": {in.Symbol}, "days": {strconv.Itoa(in.Days)}}
	if err := s.withAudit(ctx, "stock_get_technical", []string{"symbol", "days"}, func() error {
		return s.cli.Get(ctx, "/api/stock/technical", q, &out.Result)
	}); err != nil {
		return nil, stockBaseOutput{}, err
	}
	return nil, out, nil
}
