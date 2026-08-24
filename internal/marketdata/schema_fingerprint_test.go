package marketdata

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"golang.org/x/time/rate"
)

// ─── generic fingerprint helper ─────────────────────────────────────────────

func TestCheckFingerprint_OK(t *testing.T) {
	fp := responseFingerprint{
		provider: "test",
		endpoint: "e",
		fields: []fingerprintField{
			{name: "name", kind: fingerprintString},
			{name: "count", kind: fingerprintNumber},
			{name: "tags", kind: fingerprintArray},
		},
	}
	problems := checkFingerprint(fp, map[string]any{
		"name":  "2330",
		"count": float64(3),
		"tags":  []any{"a", "b"},
	})
	if len(problems) != 0 {
		t.Fatalf("expected no problems, got %v", problems)
	}
}

func TestCheckFingerprint_MissingAndWrongType(t *testing.T) {
	fp := responseFingerprint{
		provider: "test",
		endpoint: "e",
		fields: []fingerprintField{
			{name: "name", kind: fingerprintString},
			{name: "count", kind: fingerprintNumber},
		},
	}
	// "count" is a string → wrong type; "name" missing entirely.
	problems := checkFingerprint(fp, map[string]any{"count": "lots"})
	joined := strings.Join(problems, " | ")
	if !strings.Contains(joined, `missing field "name"`) {
		t.Errorf("problems = %q, want missing-field problem for name", joined)
	}
	if !strings.Contains(joined, "want number") {
		t.Errorf("problems = %q, want wrong-type problem for count", joined)
	}
}

func TestCheckFingerprint_NumberKinds(t *testing.T) {
	fp := responseFingerprint{
		provider: "test",
		endpoint: "e",
		fields:   []fingerprintField{{name: "v", kind: fingerprintNumber}},
	}
	for _, tc := range []struct {
		val  any
		want int // 0 = ok, else problems
	}{
		{float64(1.5), 0},
		{int64(7), 0},
		{int(7), 0},
		{json.Number("12.5"), 0},
		{"12.5", 1},
		{true, 1},
	} {
		got := checkFingerprint(fp, map[string]any{"v": tc.val})
		if len(got) != tc.want {
			t.Errorf("checkFingerprint(value=%T) = %v, want %d problems", tc.val, got, tc.want)
		}
	}
}

// ─── FinMind dataset fingerprint ────────────────────────────────────────────

func TestWarnFinMindDatasetFingerprint_NoOpOnUnknownOrEmpty(t *testing.T) {
	// Unknown dataset / empty data must not panic and must not warn.
	warnFinMindDatasetFingerprint("SomeNewDataset", []map[string]any{{"x": 1}})
	warnFinMindDatasetFingerprint("TaiwanStockPrice", nil)
}

func TestCheckFingerprint_FinMindRevenueRow(t *testing.T) {
	fp := responseFingerprint{
		provider: "finmind",
		endpoint: "TaiwanStockMonthRevenue",
		fields:   finmindDatasetFields["TaiwanStockMonthRevenue"],
	}
	// Upstream renamed revenue → revenue_usd: fingerprint must flag it.
	problems := checkFingerprint(fp, map[string]any{"date": "2026-08-01", "revenue_usd": float64(1e6)})
	joined := strings.Join(problems, " | ")
	if !strings.Contains(joined, `missing field "revenue"`) {
		t.Errorf("problems = %q, want missing-field problem for revenue", joined)
	}
}

// ─── Yahoo chart fingerprint ────────────────────────────────────────────────

// TestUnmarshalYahooChart_EmptyIndicatorsQuote (P2-15) verifies that a chart
// result WITHOUT indicators.quote returns a typed ErrSchema error instead of
// letting consumers panic on Result[0].Indicators.Quote[0] (4 of 5 consumers
// index it unguarded).
func TestUnmarshalYahooChart_EmptyIndicatorsQuote(t *testing.T) {
	body := []byte(`{"chart":{"result":[{"meta":{"regularMarketTime":0,"regularMarketPrice":100},"indicators":{"quote":[]}}]}}`)
	_, err := UnmarshalYahooChart(body)
	if err == nil {
		t.Fatal("expected ErrSchema error for empty indicators.quote")
	}
	if !errors.Is(err, ErrSchema) {
		t.Fatalf("err = %v, want errors.Is(err, ErrSchema)", err)
	}
}

func TestUnmarshalYahooChart_ValidChartStillWorks(t *testing.T) {
	body := []byte(`{"chart":{"result":[{"meta":{"regularMarketTime":1728000000,"regularMarketPrice":100},
		"indicators":{"quote":[{"close":[99.5,100.0]}]}}]}}`)
	res, err := UnmarshalYahooChart(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Chart.Result[0].Indicators.Quote[0].Close) != 2 {
		t.Fatalf("expected 2 closes, got %d", len(res.Chart.Result[0].Indicators.Quote[0].Close))
	}
}

func TestYahooChartFingerprintProblems_EmptyResultNoProblems(t *testing.T) {
	var res yahooChartResult
	if problems := yahooChartFingerprintProblems(&res); len(problems) != 0 {
		t.Fatalf("empty result should have no problems, got %v", problems)
	}
}

// ─── TWSE fingerprint ───────────────────────────────────────────────────────

func TestTWSEStockDayAllFingerprintProblems(t *testing.T) {
	// Healthy: 10 fields, all rows >= 9 columns.
	ok := &TWSEDailyResponse{
		Fields: []string{"Code", "Name", "TradeVolume", "TradeValue", "OpeningPrice", "HighestPrice", "LowestPrice", "ClosingPrice", "Change", "Transaction"},
		Data:   [][]string{{"2330", "台積電", "1", "2", "3", "4", "5", "6", "7", "8"}},
	}
	if problems := twseStockDayAllFingerprintProblems(ok); len(problems) != 0 {
		t.Fatalf("healthy response should have no problems, got %v", problems)
	}

	// Drift: fields header shortened (9 → 7) and one short row.
	drift := &TWSEDailyResponse{
		Fields: []string{"Code", "Name", "TradeVolume", "TradeValue", "OpeningPrice", "HighestPrice", "LowestPrice"},
		Data:   [][]string{{"2330", "台積電", "1", "2", "3", "4", "5", "6", "7", "8"}, {"2330", "台積電", "1", "2"}},
	}
	problems := twseStockDayAllFingerprintProblems(drift)
	joined := strings.Join(problems, " | ")
	if !strings.Contains(joined, "want >= 9") {
		t.Errorf("problems = %q, want fields-header problem", joined)
	}
	if !strings.Contains(joined, "1 of 2 data rows have < 9 columns") {
		t.Errorf("problems = %q, want short-row problem", joined)
	}
}

// TestTWSEGetQuotes_FingerprintDoesNotBreakHealthyFetch (P2-15) end-to-end:
// a healthy STOCK_DAY_ALL JSON still parses with the fingerprint wired in.
func TestTWSEGetQuotes_FingerprintDoesNotBreakHealthyFetch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"stat":"OK","date":"20260513","title":"","fields":["Code","Name","TradeVolume","TradeValue","OpeningPrice","HighestPrice","LowestPrice","ClosingPrice","Change","Transaction"],"data":[["2330","台積電","1000","2000","990","1005","985","1000","10","9"]]}`))
	}))
	defer server.Close()

	client := NewTWSEClient()
	client.baseURL = server.URL
	client.SetHTTPClient(server.Client())
	client.rateLimiter = rate.NewLimiter(rate.Inf, 1)

	quotes, err := client.GetQuotes(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(quotes) != 1 || quotes[0].Symbol != "2330" {
		t.Fatalf("expected 1 quote for 2330, got %+v", quotes)
	}
}

// ─── TAIFEX PCR fingerprint ─────────────────────────────────────────────────

// TestTAIFEXFetchPCR_SchemaFingerprintWarnsOnRenamedKey (P2-15) verifies the
// fingerprint re-parse path works: a renamed key is caught by the hard
// parseOK check (ErrTAIFEXSchema) — the fingerprint itself is warn-only, so
// the fetch still fails via the existing gate, just with a warn first.
func TestTAIFEXFetchPCR_SchemaFingerprintWarnsOnRenamedKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// PutVolume renamed → PutVol: hard parse gate fails with ErrTAIFEXSchema.
		_, _ = w.Write([]byte(`[{"Date":"20260811","PutVol":"100","CallVolume":"200","PutCallVolumeRatio%":"110.43","PutOI":"50","CallOI":"40","PutCallOIRatio%":"123.58"}]`))
	}))
	defer server.Close()

	p := NewTAIFEXProvider()
	p.baseURL = server.URL
	p.SetHTTPClient(server.Client())
	p.rateLimiter = rate.NewLimiter(rate.Every(time.Second), 1)

	_, err := p.FetchPCR(context.Background())
	if err == nil {
		t.Fatal("expected error for renamed PutVolume key")
	}
	if !errors.Is(err, ErrTAIFEXSchema) {
		t.Fatalf("err = %v, want errors.Is(err, ErrTAIFEXSchema)", err)
	}
}
