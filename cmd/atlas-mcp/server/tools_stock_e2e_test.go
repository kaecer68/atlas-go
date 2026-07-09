package server

import (
	"context"
	"encoding/json"
	"testing"
)

// TestStockE2E_Quote_InvestorFriendlyFormat verifies that stock_get_quote
// passes through the full Fugle quote response (price + open/high/low/volume)
// so that MCP bots can answer investor questions like "what's 2330's price
// today and how much has it moved?".
func TestStockE2E_Quote_InvestorFriendlyFormat(t *testing.T) {
	s, rec, done := newTestHarness(t)
	defer done()

	// Realistic Fugle response shape — symbol, name, price, open/high/low, volume,
	// change, change_pct, yesterday_close.
	rec.SetResponseBody([]byte(`{
		"symbol": "2330",
		"name": "台積電",
		"last": 680.0,
		"open": 675.0,
		"high": 685.0,
		"low": 672.0,
		"volume": 25432100,
		"change": 5.0,
		"change_pct": 0.74,
		"yesterday_close": 675.0,
		"updated_at": "2026-07-09T13:30:00+08:00"
	}`))

	_, out, err := s.handleStockGetQuote(context.Background(), nil, stockSymbolInput{Symbol: "2330"})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if rec.path != "/api/stock/quote" {
		t.Fatalf("expected /api/stock/quote, got %s", rec.path)
	}
	if out.Result == nil {
		t.Fatal("expected result")
	}

	// Verify all investor-relevant fields survive the round-trip.
	wantFields := []string{
		"symbol", "last", "change", "change_pct",
		"open", "high", "low", "volume", "yesterday_close",
	}
	for _, f := range wantFields {
		if _, ok := (*out.Result)[f]; !ok {
			t.Errorf("investor-relevant field %q missing from stock_get_quote response", f)
		}
	}
}

// TestStockE2E_Fundamentals_ValuationMetrics verifies stock_get_fundamentals
// exposes PE/PB/PS/dividend yield/sector — the 5 numbers a retail investor
// asks about when evaluating a stock.
func TestStockE2E_Fundamentals_ValuationMetrics(t *testing.T) {
	s, rec, done := newTestHarness(t)
	defer done()

	rec.SetResponseBody([]byte(`{
		"PE": 22.5,
		"PB": 5.8,
		"PS": 8.2,
		"DividendYield": 1.8,
		"Sector": "半導體"
	}`))

	_, out, err := s.handleStockGetFundamentals(context.Background(), nil, stockSymbolInput{Symbol: "2330"})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if out.Result == nil {
		t.Fatal("expected result")
	}

	// Map JSON keys (TitleCase as the backend uses) to investor-friendly names.
	wantFields := map[string]string{
		"PE":            "本益比 (Price-to-Earnings)",
		"PB":            "股價淨值比 (Price-to-Book)",
		"PS":            "股價營收比 (Price-to-Sales)",
		"DividendYield": "現金殖利率",
		"Sector":        "產業分類",
	}
	for k, label := range wantFields {
		if _, ok := (*out.Result)[k]; !ok {
			t.Errorf("missing field %q (%s) in stock_get_fundamentals response", k, label)
		}
	}
}

// TestStockE2E_Chips_InstitutionalFlow verifies stock_get_chips exposes the
// three institutional investor categories a retail investor cares about.
func TestStockE2E_Chips_InstitutionalFlow(t *testing.T) {
	s, rec, done := newTestHarness(t)
	defer done()

	rec.SetResponseBody([]byte(`{
		"symbol": "2330",
		"name": "台積電",
		"foreign_investor_net": 1542000000,
		"domestic_fund_net": 230500000,
		"dealer_net": -87500000,
		"date": "20260708"
	}`))

	_, out, err := s.handleStockGetChips(context.Background(), nil, stockChipsInput{Symbol: "2330"})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if out.Result == nil {
		t.Fatal("expected result")
	}

	// Net-buy values tell investors whether institutions are accumulating.
	wantFields := map[string]string{
		"foreign_investor_net": "外資買賣超 (NT$)",
		"domestic_fund_net":    "投信買賣超 (NT$)",
		"dealer_net":           "自營商買賣超 (NT$)",
		"date":                 "資料日期",
	}
	for k, label := range wantFields {
		if _, ok := (*out.Result)[k]; !ok {
			t.Errorf("missing field %q (%s) in stock_get_chips response", k, label)
		}
	}
}

// TestStockE2E_Technical_DayWindow verifies stock_get_technical exposes the
// three classic indicators + the explicit days=30 query (not just the default).
func TestStockE2E_Technical_DayWindow(t *testing.T) {
	s, rec, done := newTestHarness(t)
	defer done()

	rec.SetResponseBody([]byte(`{
		"symbol": "2330",
		"date": "2026-07-08",
		"close": 680.0,
		"volume": 25432100,
		"sma20": 678.5,
		"sma50": 672.3,
		"rsi14": 58.4
	}`))

	_, out, err := s.handleStockGetTechnical(context.Background(), nil, stockTechnicalInput{Symbol: "2330", Days: 30})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if rec.query.Get("days") != "30" {
		t.Fatalf("expected days=30, got %s", rec.query.Get("days"))
	}
	if out.Result == nil {
		t.Fatal("expected result")
	}

	wantFields := map[string]string{
		"close":  "最新收盤價",
		"sma20":  "20日均線 (短期趨勢)",
		"sma50":  "50日均線 (中期趨勢)",
		"rsi14":  "14日 RSI (超買/超賣)",
		"volume": "成交量",
	}
	for k, label := range wantFields {
		if _, ok := (*out.Result)[k]; !ok {
			t.Errorf("missing field %q (%s) in stock_get_technical response", k, label)
		}
	}
}

// TestStockE2E_Quote_SymbolNormalization ensures the symbol is passed through
// as a query parameter exactly as supplied (caller is responsible for
// normalization, e.g. "2330" not "台積電").
func TestStockE2E_Quote_SymbolNormalization(t *testing.T) {
	s, rec, done := newTestHarness(t)
	defer done()
	rec.SetResponseBody([]byte(`{"symbol":"2454"}`))

	cases := []struct{ in, want string }{
		{"2330", "2330"},
		{"2454", "2454"},
		{"00878", "00878"}, // ETF
	}
	for _, tc := range cases {
		_, _, err := s.handleStockGetQuote(context.Background(), nil, stockSymbolInput{Symbol: tc.in})
		if err != nil {
			t.Fatalf("symbol=%s: %v", tc.in, err)
		}
		if rec.query.Get("symbol") != tc.want {
			t.Errorf("symbol=%s: want %s, got %s", tc.in, tc.want, rec.query.Get("symbol"))
		}
	}
}

// TestStockE2E_ResponseIsJSON ensures the result is JSON-serializable so MCP
// clients (Claude Desktop, OpenClaw, Hermes) can consume it without manual
// parsing.
func TestStockE2E_ResponseIsJSON(t *testing.T) {
	s, rec, done := newTestHarness(t)
	defer done()
	rec.SetResponseBody([]byte(`{"symbol":"2330","last":680,"change_pct":0.74}`))

	_, out, err := s.handleStockGetQuote(context.Background(), nil, stockSymbolInput{Symbol: "2330"})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	encoded, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("response not JSON-serializable: %v", err)
	}
	if len(encoded) == 0 {
		t.Fatal("empty JSON response")
	}
	// Ensure it round-trips through Unmarshal without error.
	var roundTrip map[string]any
	if err := json.Unmarshal(encoded, &roundTrip); err != nil {
		t.Fatalf("response not round-trippable: %v", err)
	}
}
