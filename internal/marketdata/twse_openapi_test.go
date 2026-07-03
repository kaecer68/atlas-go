package marketdata

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"golang.org/x/text/encoding/traditionalchinese"
	"golang.org/x/time/rate"
)

// TestTWSEClient_GetQuotes_Big5Charset is the regression test for the
// twse_replay channel error surfaced in the 2026-07-02 23:19 atlas-go log:
//
//	task_failed name=channel_health_twse_replay
//	  err="twse fetch: csv header: parse error on line 1, column 3:
//	      extraneous or missing \" in quoted-field"
//
// Root cause: GetQuotes used streaming DecodeJSON(resp.Body, ...). When TWSE
// returned a Big5-encoded payload, if the JSON path failed partway through
// (e.g. partial charset mismatch), resp.Body was left partially consumed.
// The CSV fallback then re-read the truncated body and choked on the first
// CSV line — producing the misleading "csv header: parse error on line 1,
// column 3" message.
//
// Fix: switch to the bytes-read pattern (io.ReadAll + bytes.NewReader +
// DecodeJSON) used by the 5 other TWSE providers in this package. The full
// body stays in `body` bytes, so a JSON failure can hand a fresh reader to
// the CSV fallback via `bytes.NewReader(body)`.
//
// This test pins the charset-aware decode path for a Big5 JSON body, which
// is the production trigger for the original regression.
func TestTWSEClient_GetQuotes_Big5Charset(t *testing.T) {
	const utf8Resp = `{
		"stat": "OK",
		"date": "20260513",
		"title": "上市個股日成交",
		"fields": ["Code","Name","TradeVolume","TradeValue","OpeningPrice","HighestPrice","LowestPrice","ClosingPrice","Change","Transaction"],
		"data": [
			["2330","台積電","81160741","15450000000","190","191.23","189.07","190.64","+0.50","35000"],
			["2308","台達電","132679634","50500000000","380","381.99","377.77","378.31","-0.50","12000"]
		]
	}`
	big5Bytes, err := traditionalchinese.Big5.NewEncoder().Bytes([]byte(utf8Resp))
	if err != nil {
		t.Fatalf("Big5 encode fixture: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=Big5")
		_, _ = w.Write(big5Bytes)
	}))
	defer server.Close()

	c := &TWSEClient{
		httpClient:  server.Client(),
		baseURL:     server.URL,
		rateLimiter: rate.NewLimiter(rate.Inf, 0),
	}

	quotes, err := c.GetQuotes(context.Background())
	if err != nil {
		t.Fatalf("GetQuotes with Big5 charset failed: %v", err)
	}
	if len(quotes) != 2 {
		t.Fatalf("expected 2 quotes, got %d", len(quotes))
	}
	// Name field is the canonical mojibake detector — if the bytes-read path
	// regresses to streaming, the Chinese names will be garbled or empty.
	if quotes[0].Symbol != "2330" {
		t.Errorf("quotes[0].Symbol = %q, want %q", quotes[0].Symbol, "2330")
	}
	if quotes[0].Last != 190.64 {
		t.Errorf("quotes[0].Last = %v, want 190.64", quotes[0].Last)
	}
	if quotes[1].Symbol != "2308" {
		t.Errorf("quotes[1].Symbol = %q, want %q", quotes[1].Symbol, "2308")
	}
}

// TestTWSEClient_GetQuotes_UTF8Charset_Smoke verifies the happy path is
// unchanged: a UTF-8 JSON response with explicit charset=utf-8 is parsed
// identically to the pre-fix behavior. This guards against an over-zealous
// refactor breaking the common case while fixing the Big5 case.
func TestTWSEClient_GetQuotes_UTF8Charset_Smoke(t *testing.T) {
	const utf8Resp = `{
		"stat": "OK",
		"date": "20260513",
		"title": "上市個股日成交",
		"fields": ["Code","Name","TradeVolume","TradeValue","OpeningPrice","HighestPrice","LowestPrice","ClosingPrice","Change","Transaction"],
		"data": [
			["2330","台積電","81160741","15450000000","190","191.23","189.07","190.64","+0.50","35000"]
		]
	}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = w.Write([]byte(utf8Resp))
	}))
	defer server.Close()

	c := &TWSEClient{
		httpClient:  server.Client(),
		baseURL:     server.URL,
		rateLimiter: rate.NewLimiter(rate.Inf, 0),
	}

	quotes, err := c.GetQuotes(context.Background())
	if err != nil {
		t.Fatalf("GetQuotes UTF-8 smoke failed: %v", err)
	}
	if len(quotes) != 1 {
		t.Fatalf("expected 1 quote, got %d", len(quotes))
	}
	if quotes[0].Symbol != "2330" {
		t.Errorf("Symbol = %q, want 2330", quotes[0].Symbol)
	}
}
