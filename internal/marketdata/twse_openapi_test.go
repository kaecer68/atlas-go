package marketdata

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
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

// TestTWSEClient_GetDailyQuote_DateFiltered pins the per-date contract of
// GetDailyQuote. TWSE STOCK_DAY ignores the day component of the `date`
// parameter and returns the whole month; the client must return the row
// matching the requested date (ROC calendar), not Data[0] (the month's first
// trading day). Regression for the replay gap-backfill path: backfilling a
// missing date with Data[0] would write the month's first trading day's
// OHLCV under the wrong date.
func TestTWSEClient_GetDailyQuote_DateFiltered(t *testing.T) {
	const monthResp = `{
		"stat": "OK",
		"date": "20260820",
		"title": "115年08月 2330 台積電           各日成交資訊",
		"fields": ["日期","成交股數","成交金額","開盤價","最高價","最低價","收盤價","漲跌價差","成交筆數","註記"],
		"data": [
			["115/08/03","35,209,944","83,673,350,698","2,390.00","2,395.00","2,365.00","2,370.00","-55.00","174,489",""],
			["115/08/04","41,021,199","95,455,200,000","2,330.00","2,360.00","2,310.00","2,340.00","-30.00","180,000",""],
			["115/08/20","30,000,000","70,000,000,000","2,100.00","2,120.00","2,080.00","2,110.00","+10.00","150,000",""]
		]
	}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(monthResp))
	}))
	defer server.Close()

	c := &TWSEClient{
		httpClient:  server.Client(),
		baseURL:     server.URL,
		rateLimiter: rate.NewLimiter(rate.Inf, 0),
	}

	t.Run("returns requested day row not month-first row", func(t *testing.T) {
		q, err := c.GetDailyQuote(context.Background(), "20260820", "2330")
		if err != nil {
			t.Fatalf("GetDailyQuote: %v", err)
		}
		// Row 115/08/20: open 2100, high 2120, low 2080, close 2110, volume 30000000.
		if q.Last != 2110 {
			t.Errorf("Last = %v, want 2110 (requested day), not 2370 (month first row)", q.Last)
		}
		if q.Open != 2100 || q.High != 2120 || q.Low != 2080 {
			t.Errorf("OHLC = %v/%v/%v, want 2100/2120/2080", q.Open, q.High, q.Low)
		}
		if q.Volume != 30000000 {
			t.Errorf("Volume = %d, want 30000000", q.Volume)
		}
		if q.Symbol != "2330" {
			t.Errorf("Symbol = %q, want 2330", q.Symbol)
		}
	})

	t.Run("non-trading day returns no-data error", func(t *testing.T) {
		if _, err := c.GetDailyQuote(context.Background(), "20260822", "2330"); err == nil {
			t.Fatal("GetDailyQuote for Saturday 2026-08-22 = nil, want no-data error")
		}
	})
}

// ---------------------------------------------------------------------------
// P0-4: GetQuotes Stat check + non-empty validation. A non-OK stat, an
// empty payload, or all-rows-unparseable must surface an error instead of
// an empty success slice (which hid schema changes and never tripped the
// gateway breaker).
// ---------------------------------------------------------------------------

func TestTWSEClient_GetQuotes_NonOKStat_ReturnsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = w.Write([]byte(`{"stat":"假日","date":"20260823","title":"","fields":[],"data":[]}`))
	}))
	defer server.Close()

	c := &TWSEClient{
		httpClient:  server.Client(),
		baseURL:     server.URL,
		rateLimiter: rate.NewLimiter(rate.Inf, 0),
	}

	_, err := c.GetQuotes(context.Background())
	if err == nil {
		t.Fatal("GetQuotes with stat!=OK = nil error, want error")
	}
	if !strings.Contains(err.Error(), `stat="假日"`) {
		t.Errorf("error %q should surface the upstream stat value", err.Error())
	}
}

func TestTWSEClient_GetQuotes_EmptyData_ReturnsErrTWSEEmptyData(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = w.Write([]byte(`{"stat":"OK","date":"20260823","title":"","fields":[],"data":[]}`))
	}))
	defer server.Close()

	c := &TWSEClient{
		httpClient:  server.Client(),
		baseURL:     server.URL,
		rateLimiter: rate.NewLimiter(rate.Inf, 0),
	}

	_, err := c.GetQuotes(context.Background())
	if err == nil {
		t.Fatal("GetQuotes with empty data = nil error, want ErrTWSEEmptyData")
	}
	if !errors.Is(err, ErrTWSEEmptyData) {
		t.Errorf("err = %v, want wrapped ErrTWSEEmptyData", err)
	}
}

func TestTWSEClient_GetQuotes_AllRowsUnparseable_ReturnsError(t *testing.T) {
	// Every row has a non-numeric ClosingPrice → upstream schema changed.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = w.Write([]byte(`{
			"stat": "OK",
			"date": "20260513",
			"title": "上市個股日成交",
			"fields": ["Code","Name","TradeVolume","TradeValue","OpeningPrice","HighestPrice","LowestPrice","ClosingPrice","Change","Transaction"],
			"data": [
				["2330","台積電","100","100","190","191","189","--","+0.50","1"],
				["2308","台達電","100","100","380","381","377","--","-0.50","1"]
			]
		}`))
	}))
	defer server.Close()

	c := &TWSEClient{
		httpClient:  server.Client(),
		baseURL:     server.URL,
		rateLimiter: rate.NewLimiter(rate.Inf, 0),
	}

	_, err := c.GetQuotes(context.Background())
	if err == nil {
		t.Fatal("GetQuotes with all rows unparseable = nil error, want error")
	}
	if !strings.Contains(err.Error(), "failed to parse") {
		t.Errorf("error %q should mention the parse failure count", err.Error())
	}
}

func TestTWSEClient_GetQuotes_PartialFailureStillReturnsValid(t *testing.T) {
	// One good row + one bad row → keep partial tolerance (no error).
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = w.Write([]byte(`{
			"stat": "OK",
			"date": "20260513",
			"title": "上市個股日成交",
			"fields": ["Code","Name","TradeVolume","TradeValue","OpeningPrice","HighestPrice","LowestPrice","ClosingPrice","Change","Transaction"],
			"data": [
				["2330","台積電","81160741","15450000000","190","191.23","189.07","190.64","+0.50","35000"],
				["2308","台達電","100","100","380","381","377","--","-0.50","1"]
			]
		}`))
	}))
	defer server.Close()

	c := &TWSEClient{
		httpClient:  server.Client(),
		baseURL:     server.URL,
		rateLimiter: rate.NewLimiter(rate.Inf, 0),
	}

	quotes, err := c.GetQuotes(context.Background())
	if err != nil {
		t.Fatalf("GetQuotes with partial failure = error %v, want the valid row", err)
	}
	if len(quotes) != 1 || quotes[0].Symbol != "2330" {
		t.Errorf("quotes = %+v, want only the valid 2330 row", quotes)
	}
}

func TestTWSEClient_GetQuotes_EmptyCSV_ReturnsErrTWSEEmptyData(t *testing.T) {
	// CSV fallback path: header only, no rows → ErrTWSEEmptyData.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		_, _ = w.Write([]byte("日期,證券代號,證券名稱,成交股數,成交金額,開盤價,最高價,最低價,收盤價,漲跌價差,成交筆數\n"))
	}))
	defer server.Close()

	c := &TWSEClient{
		httpClient:  server.Client(),
		baseURL:     server.URL,
		rateLimiter: rate.NewLimiter(rate.Inf, 0),
	}

	_, err := c.GetQuotes(context.Background())
	if err == nil {
		t.Fatal("GetQuotes with empty CSV = nil error, want ErrTWSEEmptyData")
	}
	if !errors.Is(err, ErrTWSEEmptyData) {
		t.Errorf("err = %v, want wrapped ErrTWSEEmptyData", err)
	}
}
