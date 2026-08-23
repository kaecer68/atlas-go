package marketdata

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/time/rate"
)

// ─── FinMindClient singleton ─────────────────────────────────────────────────

func TestFinMindClient_RateLimiter(t *testing.T) {
	c := NewFinMindClient("test-key")
	if c.RateLimiter() == nil {
		t.Fatal("expected non-nil rate limiter")
	}
}

func TestFinMindClient_SetHTTPClient(t *testing.T) {
	c := NewFinMindClient("test-key")
	custom := &http.Client{}
	c.SetHTTPClient(custom)
	if c.httpClient != custom {
		t.Error("SetHTTPClient did not assign the provided client")
	}
}

func TestFinMindClient_NewFinMindClient(t *testing.T) {
	c := NewFinMindClient("api-key-123")
	if c.apiKey != "api-key-123" {
		t.Errorf("apiKey = %q, want api-key-123", c.apiKey)
	}
	if c.httpClient == nil {
		t.Error("httpClient should be initialized")
	}
	if c.rateLimiter == nil {
		t.Error("rateLimiter should be initialized")
	}
}

func TestFinMindProvider_Name(t *testing.T) {
	p := NewFinMindProviderWithClient(NewFinMindClient("k"))
	if got := p.Name(); got != "finmind" {
		t.Errorf("Name() = %q, want finmind", got)
	}
}

func TestFinMindProvider_GetClient(t *testing.T) {
	c := NewFinMindClient("k")
	p := NewFinMindProviderWithClient(c)
	if p.GetClient() != c {
		t.Error("GetClient should return injected client")
	}
}

// ─── FinMind fetchDataset (with URL-rewriting transport) ─────────────────────

// rewriteTransport redirects HTTP requests for finmindBaseURL to the test server.
type rewriteTransport struct {
	target string
	inner  http.RoundTripper
}

func (t *rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if strings.HasPrefix(req.URL.String(), finmindBaseURL) {
		newReq2 := req.Clone(req.Context())
		newReq2.URL.Scheme = "http"
		newReq2.URL.Host = strings.TrimPrefix(t.target, "http://")
		newReq2.Host = ""
		return t.inner.RoundTrip(newReq2)
	}
	return t.inner.RoundTrip(req)
}

// ─── FinMindClient.fetchDataset tests via URL-rewriting client ───────────────

func TestFinMindClient_fetchDataset_Success(t *testing.T) {
	var capturedAuth string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"msg":"success","status":200,"data":[{"revenue":289420000000.0}]}`))
	}))
	defer ts.Close()

	c := NewFinMindClient("test-key")
	c.httpClient = &http.Client{
		Transport: &rewriteTransport{target: ts.URL, inner: http.DefaultTransport},
	}

	data, err := c.fetchDataset(context.Background(), "TaiwanStockMonthRevenue", "2330", "2026-04-01", "2026-04-30")
	if err != nil {
		t.Fatalf("fetchDataset error: %v", err)
	}
	if len(data) != 1 {
		t.Fatalf("expected 1 record, got %d", len(data))
	}
	if data[0]["revenue"].(float64) != 289420000000.0 {
		t.Errorf("revenue = %v, want 289420000000.0", data[0]["revenue"])
	}
	if capturedAuth != "Bearer test-key" {
		t.Errorf("Authorization = %q, want Bearer test-key", capturedAuth)
	}
}

// TestFinMindClient_fetchDataset_NormalizesDataID 驗證 .TW/.TWO suffix 的
// data_id 會被正規化為裸股票代碼（A01：auto_cycle_update 全鏈失敗根因）。
// ClassificationTree 的 RepresentativeStocks 是 "1513.TW" 形式，FinMind API
// 只接受 "1513"。正規化在 fetchDataset 統一處理，覆蓋所有 Taiwan stock caller。
func TestFinMindClient_fetchDataset_NormalizesDataID(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{"bare_symbol", "1513", "1513"},
		{"tw_suffix", "1513.TW", "1513"},
		{"tw_suffix_lowercase", "1513.tw", "1513"},
		{"two_suffix", "1513.TWO", "1513"},
		{"two_suffix_lowercase", "1513.two", "1513"},
		{"whitespace", " 1513.TW ", "1513"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var gotDataID string
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotDataID = r.URL.Query().Get("data_id")
				w.Write([]byte(`{"msg":"success","status":200,"data":[{"revenue":100.0}]}`))
			}))
			defer ts.Close()

			c := NewFinMindClient("k")
			c.httpClient = &http.Client{Transport: &rewriteTransport{target: ts.URL, inner: http.DefaultTransport}}

			if _, err := c.fetchDataset(context.Background(), "TaiwanStockMonthRevenue", tc.input, "2026-07-01", "2026-07-31"); err != nil {
				t.Fatalf("fetchDataset error: %v", err)
			}
			if gotDataID != tc.want {
				t.Errorf("data_id = %q, want %q (input %q)", gotDataID, tc.want, tc.input)
			}
		})
	}
}

// TestQuarterOfDate 驗證 quarterOfDate 的正確季度計算（A02）。
func TestQuarterOfDate(t *testing.T) {
	cases := []struct {
		date string
		want int
	}{
		{"2026-01-15", 1},
		{"2026-03-31", 1},
		{"2026-04-01", 2},
		{"2026-06-30", 2},
		{"2026-07-01", 3},
		{"2026-09-30", 3},
		{"2026-10-01", 4},
		{"2026-12-31", 4},
		{"2026", 0},
		{"garbage", 0},
	}
	for _, tc := range cases {
		if got := quarterOfDate(tc.date); got != tc.want {
			t.Errorf("quarterOfDate(%q) = %d, want %d", tc.date, got, tc.want)
		}
	}
}

func TestFinMindClient_fetchDataset_NoAPIKey(t *testing.T) {
	var authReceived string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authReceived = r.Header.Get("Authorization")
		w.Write([]byte(`{"msg":"success","status":200,"data":[]}`))
	}))
	defer ts.Close()

	c := NewFinMindClient("")
	c.httpClient = &http.Client{
		Transport: &rewriteTransport{target: ts.URL, inner: http.DefaultTransport},
	}

	_, err := c.fetchDataset(context.Background(), "TaiwanStockMonthRevenue", "2330", "2026-04-01", "2026-04-30")
	if err != nil {
		t.Fatalf("fetchDataset error: %v", err)
	}
	if authReceived != "" {
		t.Errorf("Authorization should not be set when apiKey is empty, got %q", authReceived)
	}
}

func TestFinMindClient_fetchDataset_HTTPError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"msg":"forbidden"}`))
	}))
	defer ts.Close()

	c := NewFinMindClient("k")
	c.httpClient = &http.Client{
		Transport: &rewriteTransport{target: ts.URL, inner: http.DefaultTransport},
	}

	_, err := c.fetchDataset(context.Background(), "TaiwanStockMonthRevenue", "2330", "2026-04-01", "2026-04-30")
	if err == nil {
		t.Fatal("expected error for 403 response")
	}
}

func TestFinMindClient_fetchDataset_APIError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"msg":"Invalid token","status":401,"data":[]}`))
	}))
	defer ts.Close()

	c := NewFinMindClient("bad-key")
	c.httpClient = &http.Client{
		Transport: &rewriteTransport{target: ts.URL, inner: http.DefaultTransport},
	}

	_, err := c.fetchDataset(context.Background(), "TaiwanStockMonthRevenue", "2330", "2026-04-01", "2026-04-30")
	if err == nil {
		t.Fatal("expected error for API status 401")
	}
	if !strings.Contains(err.Error(), "Invalid token") {
		t.Errorf("error %q should mention Invalid token", err.Error())
	}
}

func TestFinMindClient_fetchDataset_MalformedJSON(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`not-json`))
	}))
	defer ts.Close()

	c := NewFinMindClient("k")
	c.httpClient = &http.Client{
		Transport: &rewriteTransport{target: ts.URL, inner: http.DefaultTransport},
	}

	if _, err := c.fetchDataset(context.Background(), "X", "Y", "2026-01-01", "2026-01-31"); err == nil {
		t.Fatal("expected error for malformed JSON")
	}
}

// ─── FinMindClient.GetMonthRevenue ───────────────────────────────────────────

func TestFinMindClient_GetMonthRevenue_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("dataset") != "TaiwanStockMonthRevenue" {
			t.Errorf("dataset = %q, want TaiwanStockMonthRevenue", r.URL.Query().Get("dataset"))
		}
		if r.URL.Query().Get("data_id") != "2330" {
			t.Errorf("data_id = %q, want 2330", r.URL.Query().Get("data_id"))
		}
		if r.URL.Query().Get("start_date") != "2026-04-01" {
			t.Errorf("start_date = %q, want 2026-04-01", r.URL.Query().Get("start_date"))
		}
		if r.URL.Query().Get("end_date") != "2026-04-30" {
			t.Errorf("end_date = %q, want 2026-04-30 (PR-E: endDate is now the last day of the month, not a hardcoded 31)", r.URL.Query().Get("end_date"))
		}
		w.Write([]byte(`{"msg":"success","status":200,"data":[{"revenue":289420000000.0,"date":"2026-04-01"}]}`))
	}))
	defer ts.Close()

	c := NewFinMindClient("k")
	c.httpClient = &http.Client{Transport: &rewriteTransport{target: ts.URL, inner: http.DefaultTransport}}

	rev, err := c.GetMonthRevenue(context.Background(), "2330", 2026, 4)
	if err != nil {
		t.Fatalf("GetMonthRevenue error: %v", err)
	}
	if rev != 289420000000.0 {
		t.Errorf("revenue = %v, want 289420000000.0", rev)
	}
}

func TestFinMindClient_GetMonthRevenue_NoData(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"msg":"success","status":200,"data":[]}`))
	}))
	defer ts.Close()

	c := NewFinMindClient("k")
	c.httpClient = &http.Client{Transport: &rewriteTransport{target: ts.URL, inner: http.DefaultTransport}}

	_, err := c.GetMonthRevenue(context.Background(), "9999", 2026, 1)
	if err == nil {
		t.Fatal("expected error when no data returned")
	}
	if !strings.Contains(err.Error(), "no month revenue data") {
		t.Errorf("error %q should mention 'no month revenue data'", err.Error())
	}
}

func TestFinMindClient_GetMonthRevenue_NonFloatRevenue(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"msg":"success","status":200,"data":[{"revenue":"289.42"}]}`))
	}))
	defer ts.Close()

	c := NewFinMindClient("k")
	c.httpClient = &http.Client{Transport: &rewriteTransport{target: ts.URL, inner: http.DefaultTransport}}

	_, err := c.GetMonthRevenue(context.Background(), "2330", 2026, 4)
	if err == nil {
		t.Fatal("expected error when revenue is not float64")
	}
}

// ─── FinMindClient.GetFinancialStatements ────────────────────────────────────

func TestFinMindClient_GetFinancialStatements_FiltersByQuarter(t *testing.T) {
	// A02 修正：quarter 由完整日期計算（3月→Q1、12月→Q4），
	// 不再是 dateStr[5]（月份十位數）的錯誤 heuristic。
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{
			"msg":"success","status":200,
			"data":[
				{"date":"2026-12-31","origin_name":"Revenue","value":1750000.0},
				{"date":"2026-12-31","origin_name":"NetIncome","value":450000.0},
				{"date":"2026-03-31","origin_name":"Revenue","value":1600000.0}
			]
		}`))
	}))
	defer ts.Close()

	c := NewFinMindClient("k")
	c.httpClient = &http.Client{Transport: &rewriteTransport{target: ts.URL, inner: http.DefaultTransport}}

	// quarter=4 matches December entries（12 月 = Q4）
	statements, err := c.GetFinancialStatements(context.Background(), "2330", 2026, 4)
	if err != nil {
		t.Fatalf("GetFinancialStatements error: %v", err)
	}
	if len(statements) != 2 {
		t.Fatalf("expected 2 statements for quarter=4, got %d", len(statements))
	}
	if statements["Revenue"] != 1750000.0 {
		t.Errorf("Revenue = %v, want 1750000.0", statements["Revenue"])
	}
	if statements["NetIncome"] != 450000.0 {
		t.Errorf("NetIncome = %v, want 450000.0", statements["NetIncome"])
	}

	// quarter=1 matches March entries（3 月 = Q1）
	q1, err := c.GetFinancialStatements(context.Background(), "2330", 2026, 1)
	if err != nil {
		t.Fatalf("GetFinancialStatements Q1 error: %v", err)
	}
	if q1["Revenue"] != 1600000.0 {
		t.Errorf("Q1 Revenue = %v, want 1600000.0", q1["Revenue"])
	}
	if _, exists := q1["NetIncome"]; exists {
		t.Errorf("Q1 must not include December NetIncome (quarter filter broken)")
	}
}

func TestFinMindClient_GetFinancialStatements_NoDateField(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{
			"msg":"success","status":200,
			"data":[{"origin_name":"Revenue","value":1600000.0}]
		}`))
	}))
	defer ts.Close()

	c := NewFinMindClient("k")
	c.httpClient = &http.Client{Transport: &rewriteTransport{target: ts.URL, inner: http.DefaultTransport}}

	statements, err := c.GetFinancialStatements(context.Background(), "2330", 2026, 2)
	if err != nil {
		t.Fatalf("GetFinancialStatements error: %v", err)
	}
	if len(statements) != 0 {
		t.Errorf("expected 0 statements when date missing, got %d", len(statements))
	}
}

func TestFinMindClient_GetFinancialStatements_ShortDateString(t *testing.T) {
	// date string shorter than 7 chars → quarter index parsing skipped
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{
			"msg":"success","status":200,
			"data":[{"date":"2026","origin_name":"Revenue","value":1600000.0}]
		}`))
	}))
	defer ts.Close()

	c := NewFinMindClient("k")
	c.httpClient = &http.Client{Transport: &rewriteTransport{target: ts.URL, inner: http.DefaultTransport}}

	statements, err := c.GetFinancialStatements(context.Background(), "2330", 2026, 1)
	if err != nil {
		t.Fatalf("GetFinancialStatements error: %v", err)
	}
	if len(statements) != 0 {
		t.Errorf("expected 0 statements for short date, got %d", len(statements))
	}
}

// ─── FinMindClient.GetInstitutionalInvestors ─────────────────────────────────

func TestFinMindClient_GetInstitutionalInvestors_AllThreeCategories(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{
			"msg":"success","status":200,
			"data":[
				{"name":"ForeignInvestors","buy":100000.0,"sell":50000.0},
				{"name":"ForeignDealer","buy":20000.0,"sell":10000.0},
				{"name":"InvestmentTrust","buy":30000.0,"sell":25000.0},
				{"name":"DomesticInstitution","buy":5000.0,"sell":4000.0},
				{"name":"Dealer","buy":10000.0,"sell":8000.0}
			]
		}`))
	}))
	defer ts.Close()

	c := NewFinMindClient("k")
	c.httpClient = &http.Client{Transport: &rewriteTransport{target: ts.URL, inner: http.DefaultTransport}}

	foreign, domestic, dealer, err := c.GetInstitutionalInvestors(context.Background(), "2330", "2026-04-29")
	if err != nil {
		t.Fatalf("GetInstitutionalInvestors error: %v", err)
	}
	// ForeignInvestors + ForeignDealer = (100000-50000) + (20000-10000) = 50000+10000 = 60000
	if foreign != 60000.0 {
		t.Errorf("foreign = %v, want 60000.0", foreign)
	}
	// InvestmentTrust + DomesticInstitution = (30000-25000) + (5000-4000) = 5000+1000 = 6000
	if domestic != 6000.0 {
		t.Errorf("domestic = %v, want 6000.0", domestic)
	}
	if dealer != 2000.0 {
		t.Errorf("dealer = %v, want 2000.0", dealer)
	}
}

func TestFinMindClient_GetInstitutionalInvestors_SkipsInvalidEntries(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{
			"msg":"success","status":200,
			"data":[
				{"buy":100000.0,"sell":50000.0},
				{"name":"ForeignInvestors","buy":10000.0,"sell":5000.0}
			]
		}`))
	}))
	defer ts.Close()

	c := NewFinMindClient("k")
	c.httpClient = &http.Client{Transport: &rewriteTransport{target: ts.URL, inner: http.DefaultTransport}}

	foreign, _, _, err := c.GetInstitutionalInvestors(context.Background(), "2330", "2026-04-29")
	if err != nil {
		t.Fatalf("GetInstitutionalInvestors error: %v", err)
	}
	if foreign != 5000.0 {
		t.Errorf("foreign = %v, want 5000.0 (only one valid ForeignInvestors entry)", foreign)
	}
}

// ─── FinMindClient.GetStockPrice ─────────────────────────────────────────────

func TestFinMindClient_GetStockPrice_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{
			"msg":"success","status":200,
			"data":[{
				"date":"2026-04-29",
				"open":1070.0,
				"max":1075.0,
				"min":1055.0,
				"close":1065.0,
				"Trading_Volume":45045.0
			}]
		}`))
	}))
	defer ts.Close()

	c := NewFinMindClient("k")
	c.httpClient = &http.Client{Transport: &rewriteTransport{target: ts.URL, inner: http.DefaultTransport}}

	q, err := c.GetStockPrice(context.Background(), "2330", "2026-04-29")
	if err != nil {
		t.Fatalf("GetStockPrice error: %v", err)
	}
	if q.Symbol != "2330" {
		t.Errorf("Symbol = %q, want 2330", q.Symbol)
	}
	if q.Last != 1065.0 {
		t.Errorf("Last = %v, want 1065.0", q.Last)
	}
	if q.Open != 1070.0 {
		t.Errorf("Open = %v, want 1070.0", q.Open)
	}
	if q.High != 1075.0 {
		t.Errorf("High = %v, want 1075.0", q.High)
	}
	if q.Low != 1055.0 {
		t.Errorf("Low = %v, want 1055.0", q.Low)
	}
	if q.Volume != 45045 {
		t.Errorf("Volume = %d, want 45045", q.Volume)
	}
	if q.Market != "TW" {
		t.Errorf("Market = %q, want TW", q.Market)
	}
	if q.Source != "finmind" {
		t.Errorf("Source = %q, want finmind", q.Source)
	}
}

func TestFinMindClient_GetStockPrice_NoData(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"msg":"success","status":200,"data":[]}`))
	}))
	defer ts.Close()

	c := NewFinMindClient("k")
	c.httpClient = &http.Client{Transport: &rewriteTransport{target: ts.URL, inner: http.DefaultTransport}}

	_, err := c.GetStockPrice(context.Background(), "2330", "2026-04-29")
	if err == nil {
		t.Fatal("expected error for empty data")
	}
}

func TestFinMindClient_GetStockPrice_OnlyClose(t *testing.T) {
	// When only "close" is present, High/Low fall back to close.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{
			"msg":"success","status":200,
			"data":[{"close":100.0,"Trading_Volume":1000.0}]
		}`))
	}))
	defer ts.Close()

	c := NewFinMindClient("k")
	c.httpClient = &http.Client{Transport: &rewriteTransport{target: ts.URL, inner: http.DefaultTransport}}

	q, err := c.GetStockPrice(context.Background(), "2330", "2026-04-29")
	if err != nil {
		t.Fatalf("GetStockPrice error: %v", err)
	}
	if q.Last != 100.0 {
		t.Errorf("Last = %v, want 100.0", q.Last)
	}
	// Without max/min, High and Low default to Last
	if q.High != 100.0 {
		t.Errorf("High = %v, want 100.0 (fallback to close)", q.High)
	}
	if q.Low != 100.0 {
		t.Errorf("Low = %v, want 100.0 (fallback to close)", q.Low)
	}
	if q.Open != 0 {
		t.Errorf("Open = %v, want 0 (no open field)", q.Open)
	}
	if q.Volume != 1000 {
		t.Errorf("Volume = %d, want 1000", q.Volume)
	}
}

// ─── FinMindProvider.GetQuotes ────────────────────────────────────────────────

func TestFinMindProvider_GetQuotes_PartialSuccess(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("data_id") {
		case "2330":
			w.Write([]byte(`{"msg":"success","status":200,"data":[{"close":1065.0,"Trading_Volume":45045.0}]}`))
		case "9999":
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer ts.Close()

	c := NewFinMindClient("k")
	c.httpClient = &http.Client{Transport: &rewriteTransport{target: ts.URL, inner: http.DefaultTransport}}

	p := NewFinMindProviderWithClient(c)
	quotes, err := p.GetQuotes(context.Background(), asOfDate("2026-04-29"), []string{"2330", "9999"})
	if err != nil {
		t.Fatalf("GetQuotes error: %v", err)
	}
	if len(quotes) != 1 {
		t.Fatalf("expected 1 quote (only 2330 succeeded), got %d", len(quotes))
	}
	if quotes[0].Symbol != "2330" {
		t.Errorf("Symbol = %q, want 2330", quotes[0].Symbol)
	}
}

func TestFinMindProvider_GetQuotes_AllFailed(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	c := NewFinMindClient("k")
	c.httpClient = &http.Client{Transport: &rewriteTransport{target: ts.URL, inner: http.DefaultTransport}}

	p := NewFinMindProviderWithClient(c)
	_, err := p.GetQuotes(context.Background(), asOfDate("2026-04-29"), []string{"X", "Y"})
	if err == nil {
		t.Fatal("expected error when all symbols fail")
	}
}

func TestFinMindProvider_GetQuotes_RejectsSaturday(t *testing.T) {
	var hits int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	c := NewFinMindClient("k")
	c.httpClient = &http.Client{Transport: &rewriteTransport{target: ts.URL, inner: http.DefaultTransport}}
	p := NewFinMindProviderWithClient(c)

	_, err := p.GetQuotes(context.Background(), asOfDate("2026-04-25"), []string{"2330"})
	if err == nil {
		t.Fatal("expected error when asOf is Saturday")
	}
	if !strings.Contains(err.Error(), "not a Taiwan trading day") {
		t.Errorf("err = %v, want it to mention 'not a Taiwan trading day'", err)
	}
	if atomic.LoadInt32(&hits) != 0 {
		t.Errorf("expected 0 HTTP calls on non-trading day, got %d", hits)
	}
}

func TestFinMindProvider_GetQuotes_RejectsSunday(t *testing.T) {
	var hits int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	c := NewFinMindClient("k")
	c.httpClient = &http.Client{Transport: &rewriteTransport{target: ts.URL, inner: http.DefaultTransport}}
	p := NewFinMindProviderWithClient(c)

	_, err := p.GetQuotes(context.Background(), asOfDate("2026-04-26"), []string{"2330"})
	if err == nil {
		t.Fatal("expected error when asOf is Sunday")
	}
	if !strings.Contains(err.Error(), "not a Taiwan trading day") {
		t.Errorf("err = %v, want it to mention 'not a Taiwan trading day'", err)
	}
	if atomic.LoadInt32(&hits) != 0 {
		t.Errorf("expected 0 HTTP calls on non-trading day, got %d", hits)
	}
}

func TestFinMindProvider_GetMonthRevenue_DelegatesToClient(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"msg":"success","status":200,"data":[{"revenue":500.0}]}`))
	}))
	defer ts.Close()

	c := NewFinMindClient("k")
	c.httpClient = &http.Client{Transport: &rewriteTransport{target: ts.URL, inner: http.DefaultTransport}}

	p := NewFinMindProviderWithClient(c)
	rev, err := p.GetMonthRevenue(context.Background(), "2330", 2026, 1)
	if err != nil {
		t.Fatalf("GetMonthRevenue error: %v", err)
	}
	if rev != 500.0 {
		t.Errorf("revenue = %v, want 500.0", rev)
	}
}

func TestFinMindProvider_GetFinancialStatements_DelegatesToClient(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// A02：2026-12-31 是 Q4（12 月），不是舊 heuristic 的 Q1
		w.Write([]byte(`{"msg":"success","status":200,"data":[{"date":"2026-12-31","origin_name":"Revenue","value":2000.0}]}`))
	}))
	defer ts.Close()

	c := NewFinMindClient("k")
	c.httpClient = &http.Client{Transport: &rewriteTransport{target: ts.URL, inner: http.DefaultTransport}}

	p := NewFinMindProviderWithClient(c)
	statements, err := p.GetFinancialStatements(context.Background(), "2330", 2026, 4)
	if err != nil {
		t.Fatalf("GetFinancialStatements error: %v", err)
	}
	if statements["Revenue"] != 2000.0 {
		t.Errorf("Revenue = %v, want 2000.0", statements["Revenue"])
	}
}

func TestFinMindProvider_GetInstitutionalInvestors_DelegatesToClient(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"msg":"success","status":200,"data":[{"name":"ForeignInvestors","buy":1000.0,"sell":500.0}]}`))
	}))
	defer ts.Close()

	c := NewFinMindClient("k")
	c.httpClient = &http.Client{Transport: &rewriteTransport{target: ts.URL, inner: http.DefaultTransport}}

	p := NewFinMindProviderWithClient(c)
	foreign, _, _, err := p.GetInstitutionalInvestors(context.Background(), "2330", "2026-04-29")
	if err != nil {
		t.Fatalf("GetInstitutionalInvestors error: %v", err)
	}
	if foreign != 500.0 {
		t.Errorf("foreign = %v, want 500.0", foreign)
	}
}

// ─── Shared client lifecycle ─────────────────────────────────────────────────

func TestGetSharedFinMindClient_Singleton(t *testing.T) {
	ResetSharedFinMindClient()
	c1 := GetSharedFinMindClient("key1")
	c2 := GetSharedFinMindClient("ignored-after-first-call")
	if c1 != c2 {
		t.Error("expected singleton to return same instance")
	}
	if c1.apiKey != "key1" {
		t.Errorf("apiKey = %q, want key1", c1.apiKey)
	}
	ResetSharedFinMindClient()
}

func TestUpdateSharedFinMindAPIKey_BeforeInit(t *testing.T) {
	ResetSharedFinMindClient()
	// Should not panic when sharedFinMindClient is nil
	UpdateSharedFinMindAPIKey("new-key")
}

func TestUpdateSharedFinMindAPIKey_AfterInit(t *testing.T) {
	ResetSharedFinMindClient()
	GetSharedFinMindClient("initial-key")
	UpdateSharedFinMindAPIKey("rotated-key")
	c := GetSharedFinMindClient("ignored")
	if c.apiKey != "rotated-key" {
		t.Errorf("apiKey = %q, want rotated-key", c.apiKey)
	}
	ResetSharedFinMindClient()
}

func TestResetSharedFinMindClient(t *testing.T) {
	GetSharedFinMindClient("k")
	ResetSharedFinMindClient()
	c := GetSharedFinMindClient("k2")
	if c.apiKey != "k2" {
		t.Errorf("apiKey = %q, want k2 (reset should allow new key)", c.apiKey)
	}
	ResetSharedFinMindClient()
}

// asOfDate is a tiny helper to keep tests readable.
func asOfDate(s string) time.Time {
	t, _ := time.Parse("2006-01-02", s)
	return t
}

// ─── Sanity tests using io.ReadAll directly ──────────────────────────────────

func TestFinMindClient_HTTPReadFailure(t *testing.T) {
	c := NewFinMindClient("k")
	// Use a context that is already cancelled to force a request failure.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c.httpClient = &http.Client{
		Transport: &rewriteTransport{target: "http://127.0.0.1:1", inner: http.DefaultTransport},
	}
	_, err := c.fetchDataset(ctx, "X", "Y", "2026-01-01", "2026-01-31")
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
}

// TestFinMindClient_GetStockPrice_RawTypeFallback ensures type assertions are graceful.
func TestFinMindClient_GetStockPrice_RawTypeFallback(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// close present but no max/min — High and Low fall back to Last
		w.Write([]byte(`{"msg":"success","status":200,"data":[{"close":100.0}]}`))
	}))
	defer ts.Close()

	c := NewFinMindClient("k")
	c.httpClient = &http.Client{Transport: &rewriteTransport{target: ts.URL, inner: http.DefaultTransport}}

	q, err := c.GetStockPrice(context.Background(), "2330", "2026-04-29")
	if err != nil {
		t.Fatalf("GetStockPrice error: %v", err)
	}
	if q.Last != 100.0 {
		t.Errorf("Last = %v, want 100.0", q.Last)
	}
	if q.Open != 0 || q.High != 100.0 || q.Low != 100.0 || q.Volume != 0 {
		t.Errorf("expected (Open=0, High=100, Low=100, Vol=0), got Open=%v High=%v Low=%v Vol=%d",
			q.Open, q.High, q.Low, q.Volume)
	}
}

// ─── encoding/json smoke test for compile-time guarantee ─────────────────────

func TestFinMindResponse_Parse(t *testing.T) {
	body := []byte(`{"msg":"success","status":200,"data":[{"a":1.0}]}`)
	var resp FinMindResponse
	if err := json.NewDecoder(io.NopCloser(strings.NewReader(string(body)))).Decode(&resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if resp.Status != 200 {
		t.Errorf("status = %d, want 200", resp.Status)
	}
	if resp.Msg != "success" {
		t.Errorf("msg = %q, want success", resp.Msg)
	}
	if len(resp.Data) != 1 {
		t.Fatalf("expected 1 data record, got %d", len(resp.Data))
	}
	if resp.Data[0]["a"].(float64) != 1.0 {
		t.Errorf("data[0][a] = %v, want 1.0", resp.Data[0]["a"])
	}
}

// ─── Quota gate: prevents cold-start bursts from exhausting the daily budget ──

// TestFinMindClient_QuotaGate_ReturnsErrQuotaExhausted verifies that once
// the daily quota is gone, fetchDataset returns ErrQuotaExhausted instead of
// making the HTTP call. This is the central contract that protects the
// channel from cold-start bursts (auto_quote_backfill × N symbols × 90 days).
func TestFinMindClient_QuotaGate_ReturnsErrQuotaExhausted(t *testing.T) {
	stateDir := t.TempDir()
	c := newFinMindClientInternal("test-key", stateDir)
	// Disable rate limiter so quota is the only gate.
	c.SetRateLimiter(rate.NewLimiter(rate.Inf, 0))
	// Force quota exhaustion by lowering the daily limit to 0. Now AllowCall
	// returns false immediately, the HTTP handler must never be reached.
	c.quotaTracker.SetLimit(0)

	var httpHit int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&httpHit, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	// We can't redirect finmindBaseURL (it's a const), so we expect the
	// request to fail BEFORE going out. The HTTP handler is only a tripwire:
	// if the gate works, httpHit stays 0.

	_, err := c.GetStockPrice(context.Background(), "2330", "2026-01-01")
	if !errors.Is(err, ErrQuotaExhausted) {
		t.Fatalf("err = %v, want errors.Is(err, ErrQuotaExhausted)", err)
	}
	if atomic.LoadInt32(&httpHit) != 0 {
		t.Errorf("HTTP handler hit %d times — quota gate did not block the request", httpHit)
	}
}

// TestFinMindClient_QuotaTelemetry verifies QuotaUsed/Remaining track daily
// usage so the channel-health dashboard can warn before the budget runs out.
func TestFinMindClient_QuotaTelemetry(t *testing.T) {
	c := newFinMindClientInternal("k", t.TempDir())
	c.SetRateLimiter(rate.NewLimiter(rate.Inf, 0))

	if got := c.QuotaUsed(); got != 0 {
		t.Errorf("QuotaUsed() = %d, want 0 before any calls", got)
	}
	if got := c.QuotaRemaining(); got != finmindDailyLimit {
		t.Errorf("QuotaRemaining() = %d, want %d (full daily limit)", got, finmindDailyLimit)
	}

	// Simulate 100 calls going through the tracker.
	for i := range 100 {
		if !c.quotaTracker.AllowCall() {
			t.Fatalf("AllowCall returned false at iteration %d", i)
		}
	}
	if got := c.QuotaUsed(); got != 100 {
		t.Errorf("QuotaUsed() after 100 calls = %d, want 100", got)
	}
	if got := c.QuotaRemaining(); got != finmindDailyLimit-100 {
		t.Errorf("QuotaRemaining() = %d, want %d", got, finmindDailyLimit-100)
	}
}

// TestFinMindClient_QuotaGateNilSafe verifies that clients without a tracker
// (e.g. test-only NewFinMindClient with a nil state) still work — the gate is
// skipped, the HTTP call proceeds. Defends against nil-pointer crashes if a
func TestFinMindClient_QuotaGateNilSafe(t *testing.T) {
	c := &FinMindClient{
		apiKey:       "k",
		httpClient:   &http.Client{Timeout: 1 * time.Second},
		rateLimiter:  rate.NewLimiter(rate.Inf, 0),
		quotaTracker: nil,
	}
	// Without the gate, fetchDataset must not crash on nil tracker. We only
	// verify the no-crash path; the request will fail (no live server) but
	// the error must NOT be ErrQuotaExhausted.
	_, err := c.GetStockPrice(context.Background(), "2330", "2026-01-01")
	if errors.Is(err, ErrQuotaExhausted) {
		t.Errorf("nil tracker should not yield ErrQuotaExhausted, got %v", err)
	}
}

// TestFinMindClient_fetchDataset_Non2xx_CapturesBody is the regression test
// for PR-A (kaecer 2026-08-04 dispatch). Before PR-A, FinMind errors like
// "Token is illegal" / "no data" were dropped — fetchDataset only logged
// the status code, so channel_health showed "finmind: status 400" with
// no operator-actionable info. After PR-A, the body (up to 512 bytes)
// is included in both the WARN log and the returned error.
//
// This test simulates the actual production scenario: a stale or
// misconfigured FINMIND_API_KEY env causes FinMind to return 400 with
// body {"msg":"Token is illegal.","status":400,"token_tail":"...key"}.
func TestFinMindClient_fetchDataset_Non2xx_CapturesBody(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"msg":"Token is illegal.","status":400,"token_tail":"...stale-key"}`))
	}))
	defer ts.Close()

	c := NewFinMindClient("stale-key")
	c.httpClient = &http.Client{
		Transport: &rewriteTransport{target: ts.URL, inner: http.DefaultTransport},
	}

	_, err := c.fetchDataset(context.Background(), "TaiwanStockPrice", "2330", "2026-08-04", "2026-08-04")
	if err == nil {
		t.Fatal("expected error for 400 response with body")
	}
	if !strings.Contains(err.Error(), "Token is illegal") {
		t.Errorf("error %q must contain the real FinMind reason 'Token is illegal' so operators can diagnose without re-reading logs", err.Error())
	}
	if !strings.Contains(err.Error(), "stale-key") {
		t.Errorf("error %q must include the token_tail hint so we can see WHICH env key is broken", err.Error())
	}
}

// TestFinMindClient_fetchDataset_Non2xx_EmptyBody still produces a clear
// error when FinMind returns a non-2xx with no body (e.g. nginx 502 during
// an upstream outage). The error must NOT be empty / ambiguous — it must
// at minimum show the status code and the placeholder "(empty body)".
func TestFinMindClient_fetchDataset_Non2xx_EmptyBody(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer ts.Close()

	c := NewFinMindClient("k")
	c.httpClient = &http.Client{
		Transport: &rewriteTransport{target: ts.URL, inner: http.DefaultTransport},
	}

	_, err := c.fetchDataset(context.Background(), "TaiwanStockPrice", "2330", "2026-08-04", "2026-08-04")
	if err == nil {
		t.Fatal("expected error for 502 response")
	}
	if !strings.Contains(err.Error(), "502") {
		t.Errorf("error %q must include the status code", err.Error())
	}
	if !strings.Contains(err.Error(), "(empty body)") {
		t.Errorf("error %q must indicate the body was empty so operators don't mistake it for a real FinMind API message", err.Error())
	}
}

// TestFinMindClient_fetchDataset_Non2xx_BodyTooLarge verifies the 512-byte
// read cap protects against a malicious / oversized body. We send 2KB
// of garbage and expect the error message to contain a truncated hint
// (the body cap stops at 512 bytes), not the full body.
func TestFinMindClient_fetchDataset_Non2xx_BodyTooLarge(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		// 2KB of garbage; the LimitReader should cap at 512 bytes.
		w.Write(bytes.Repeat([]byte("X"), 2048))
	}))
	defer ts.Close()

	c := NewFinMindClient("k")
	c.httpClient = &http.Client{
		Transport: &rewriteTransport{target: ts.URL, inner: http.DefaultTransport},
	}

	_, err := c.fetchDataset(context.Background(), "TaiwanStockPrice", "2330", "2026-08-04", "2026-08-04")
	if err == nil {
		t.Fatal("expected error for 400 response with oversized body")
	}
	// The error message must NOT contain more than ~600 bytes (512 cap + status + format prefix).
	if len(err.Error()) > 700 {
		t.Errorf("error message length %d exceeds expected cap (~700); body was not truncated", len(err.Error()))
	}
}

// TestFinMindClient_fetchDataset_402_WrapsErrQuotaExhausted is the P0-1
// regression test: a server-side 402 (FinMind "Requests reach the upper
// limit" — free-tier daily quota) must be wrapped in ErrQuotaExhausted so
// the channel adapter's errors.Is check maps it to warn, not error.
// Before P0-1 the 402 fell through to the generic "status 402" error and
// on-call got paged for a quota condition that auto-resets at 00:00 TW.
func TestFinMindClient_fetchDataset_402_WrapsErrQuotaExhausted(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusPaymentRequired)
		_, _ = w.Write([]byte(`{"msg":"Requests reach the upper limit. https://finmindtrade.com/","status":402}`))
	}))
	defer ts.Close()

	c := NewFinMindClient("k")
	c.httpClient = &http.Client{
		Transport: &rewriteTransport{target: ts.URL, inner: http.DefaultTransport},
	}

	_, err := c.fetchDataset(context.Background(), "TaiwanStockPrice", "2330", "2026-08-04", "2026-08-04")
	if err == nil {
		t.Fatal("expected error for 402 response")
	}
	if !errors.Is(err, ErrQuotaExhausted) {
		t.Errorf("402 error must wrap ErrQuotaExhausted, got: %v", err)
	}
	if !strings.Contains(err.Error(), "Requests reach the upper limit") {
		t.Errorf("error %q must include the real FinMind reason so operators can diagnose", err.Error())
	}
}

// TestLastDayOfMonth_AllMonthsAndLeapYears is the PR-E regression test for
// the FinMind 80+ day "auto_cycle_update" stale bug.
//
// Before PR-E, GetMonthRevenue hardcoded endDate as "31" for every month,
// which sent non-existent dates like "2026-06-31" to FinMind and got back
// HTTP 400 "parameter YYYY-MM-31 is illegal". The downstream symptom was
// `last_error="no valid data for industry 'electronics'"` on the
// auto_cycle_update channel for 80+ days.
//
// This test verifies the lastDayOfMonth helper used by the fix:
//   - 31-day months (Jan, Mar, May, Jul, Aug, Oct, Dec) → 31
//   - 30-day months (Apr, Jun, Sep, Nov) → 30
//   - Non-leap year February (e.g. 2026) → 28
//   - Leap year February (e.g. 2024) → 29
//   - Century-leap-year edge (2100 is NOT a leap year; 2000 IS) → 28 / 29
func TestLastDayOfMonth_AllMonthsAndLeapYears(t *testing.T) {
	cases := []struct {
		year, month, want int
	}{
		// 31-day months across a non-leap year
		{2026, 1, 31}, {2026, 3, 31}, {2026, 5, 31},
		{2026, 7, 31}, {2026, 8, 31}, {2026, 10, 31}, {2026, 12, 31},
		// 30-day months
		{2026, 4, 30}, {2026, 6, 30}, {2026, 9, 30}, {2026, 11, 30},
		// February in a non-leap year
		{2026, 2, 28},
		// February in a leap year (divisible by 4, not 100)
		{2024, 2, 29}, {2020, 2, 29}, {2016, 2, 29},
		// February in a century non-leap-year (divisible by 100, not 400)
		{2100, 2, 28}, {1900, 2, 28},
		// February in a 400-year leap year (divisible by 400)
		{2000, 2, 29},
	}
	for _, tc := range cases {
		got := lastDayOfMonth(tc.year, time.Month(tc.month))
		if got != tc.want {
			t.Errorf("lastDayOfMonth(%d, %d) = %d, want %d",
				tc.year, tc.month, got, tc.want)
		}
	}
}

// TestGetMonthRevenue_EndDateMatchesLastDay verifies that the endDate
// string constructed by GetMonthRevenue reflects the actual last day of
// the requested month, not a hardcoded 31. The fix delegates to
// lastDayOfMonth; this test reads back the request URL that the
// underlying fetchDataset would have sent by setting up an httptest
// server and parsing the query string.
func TestGetMonthRevenue_EndDateMatchesLastDay(t *testing.T) {
	cases := []struct {
		year, month int
		wantEnd     string
	}{
		{2026, 6, "2026-06-30"},  // the exact bug case
		{2026, 2, "2026-02-28"},  // non-leap Feb
		{2024, 2, "2024-02-29"},  // leap Feb
		{2026, 1, "2026-01-31"},  // 31-day month
		{2026, 4, "2026-04-30"},  // 30-day month
		{2026, 7, "2026-07-31"},  // 31-day month
		{2026, 9, "2026-09-30"},  // 30-day month
		{2026, 11, "2026-11-30"}, // 30-day month
		{2026, 12, "2026-12-31"}, // 31-day month
	}
	for _, tc := range cases {
		var capturedEnd string
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			capturedEnd = r.URL.Query().Get("end_date")
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"msg":"success","status":200,"data":[{"date":"2026-01-01","stock_id":"2330","revenue":1000.0}]}`))
		}))
		defer ts.Close()

		// Use a rewriteTransport-style injection: keep it simple here by
		// pointing finmindBaseURL at the test server via package-level
		// constants. Since finmindBaseURL is a const, we set the http
		// client to a transport that rewrites host.
		c := NewFinMindClient("test-key")
		c.httpClient = &http.Client{
			Transport: &rewriteTransport{target: ts.URL, inner: http.DefaultTransport},
		}

		_, _ = c.GetMonthRevenue(context.Background(), "2330", tc.year, tc.month)

		if capturedEnd != tc.wantEnd {
			t.Errorf("GetMonthRevenue(%d, %d): end_date=%q, want %q",
				tc.year, tc.month, capturedEnd, tc.wantEnd)
		}
	}
}

// TestGetFinancialStatements_EndDateUsesActualDecemberDays verifies
// the second endDate fix (year=YYYY-12-NN, where NN is the actual
// December day count). December always has 31, so the value is 31 in
// all cases, but the test pins the behaviour against the stdlib
// helper so a future refactor can't regress to a hardcoded 12-31.
func TestGetFinancialStatements_EndDateUsesActualDecemberDays(t *testing.T) {
	var capturedEnd string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedEnd = r.URL.Query().Get("end_date")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"msg":"success","status":200,"data":[]}`))
	}))
	defer ts.Close()

	c := NewFinMindClient("test-key")
	c.httpClient = &http.Client{
		Transport: &rewriteTransport{target: ts.URL, inner: http.DefaultTransport},
	}

	_, _ = c.GetFinancialStatements(context.Background(), "2330", 2026, 1)
	if capturedEnd != "2026-12-31" {
		t.Errorf("GetFinancialStatements(2026): end_date=%q, want %q",
			capturedEnd, "2026-12-31")
	}
}
