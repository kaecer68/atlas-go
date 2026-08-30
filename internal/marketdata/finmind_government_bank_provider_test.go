package marketdata

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/time/rate"
)

// newGovBankTestProvider wires a FinMindGovernmentBankProvider against an
// httptest server that replays a canned TaiwanStockGovernmentBankBuySell
// response. The sponsor client's 600ms token bucket is replaced with
// rate.Inf so tests don't pace.
func newGovBankTestProvider(t *testing.T, handler http.HandlerFunc) (*FinMindGovernmentBankProvider, *httptest.Server, string) {
	t.Helper()
	ts := httptest.NewServer(handler)
	client := NewFinMindClientWithStateDir("test-token", t.TempDir())
	// rewriteTransport (defined in finmind_client_extra_test.go) redirects
	// finmindBaseURL requests to the test server.
	client.SetHTTPClient(&http.Client{
		Transport: &rewriteTransport{target: ts.URL, inner: http.DefaultTransport},
	})
	client.SetRateLimiter(rate.NewLimiter(rate.Inf, 1))
	p := NewFinMindGovernmentBankProviderWithClient(client)
	return p, ts, t.TempDir()
}

// cannedGovBankRows returns a small whole-market fixture for 2021-06-30:
// 2330 (TW50) across 3 banks + 0050 (non-TW50, must be filtered out) + a
// non-trading 0060?? unknown stock. Values in TWD (buy_amount/sell_amount).
func cannedGovBankRows() []map[string]any {
	return []map[string]any{
		// 2330 — in tw50Symbols
		{"date": "2021-06-30", "stock_id": "2330", "bank_name": "合庫", "buy": 1000, "sell": 500, "buy_amount": 600000.0, "sell_amount": 300000.0},
		{"date": "2021-06-30", "stock_id": "2330", "bank_name": "兆豐", "buy": 2000, "sell": 800, "buy_amount": 1200000.0, "sell_amount": 480000.0},
		// 2317 — in tw50Symbols
		{"date": "2021-06-30", "stock_id": "2317", "bank_name": "合庫", "buy": 300, "sell": 700, "buy_amount": 90000.0, "sell_amount": 210000.0},
		// 0050 — NOT in tw50Symbols → must be filtered out
		{"date": "2021-06-30", "stock_id": "0050", "bank_name": "合庫", "buy": 99999, "sell": 0, "buy_amount": 99999999.0, "sell_amount": 0.0},
		// unknown bank → must be skipped with a warning
		{"date": "2021-06-30", "stock_id": "2330", "bank_name": "某銀行", "buy": 1, "sell": 1, "buy_amount": 1.0, "sell_amount": 1.0},
	}
}

func govBankOKHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(FinMindResponse{
			Msg:    "success",
			Status: 200,
			Data:   cannedGovBankRows(),
		})
	}
}

func TestFinMindGovernmentBankProvider_FetchDay_AggregatesByBank(t *testing.T) {
	p, ts, _ := newGovBankTestProvider(t, govBankOKHandler())
	defer ts.Close()

	day, err := p.FetchDay(context.Background(), time.Date(2021, 6, 30, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("FetchDay: %v", err)
	}
	if day == nil {
		t.Fatal("expected a non-nil day")
	}
	if day.Date != "20210630" {
		t.Errorf("Date = %q, want 20210630", day.Date)
	}

	// Expected: 合庫 = 600000-300000 (2330) + 90000-210000 (2317) = 300000 - 120000 = 180000
	//          兆豐 = 1200000-480000 = 720000
	// Total = 900000. 0050 and unknown bank filtered out.
	if day.TotalNet != 900000 {
		t.Errorf("TotalNet = %d, want 900000", day.TotalNet)
	}
	if len(day.Banks) != 2 {
		t.Fatalf("len(Banks) = %d, want 2 (0050 + unknown bank filtered)", len(day.Banks))
	}
	byCode := map[string]BrokerDailyDetail{}
	for _, b := range day.Banks {
		byCode[b.Code] = b
	}
	if b, ok := byCode["8060"]; !ok {
		t.Error("missing bank 8060 (合庫)")
	} else {
		if b.Name != "合作金庫" {
			t.Errorf("8060 Name = %q, want 合作金庫", b.Name)
		}
		if b.Buy != 690000 || b.Sell != 510000 || b.Net != 180000 {
			t.Errorf("8060 buy/sell/net = %d/%d/%d, want 690000/510000/180000", b.Buy, b.Sell, b.Net)
		}
	}
	if b, ok := byCode["8061"]; !ok {
		t.Error("missing bank 8061 (兆豐)")
	} else {
		if b.Buy != 1200000 || b.Sell != 480000 || b.Net != 720000 {
			t.Errorf("8061 buy/sell/net = %d/%d/%d, want 1200000/480000/720000", b.Buy, b.Sell, b.Net)
		}
	}
}

func TestFinMindGovernmentBankProvider_FetchDay_PartialRowsNoUniverseNoData(t *testing.T) {
	// API returns 200 with rows, but none match tw50Symbols x 8 banks
	// (upstream partial data, verified 2023-04-06 / 2023-10-25) → must be
	// treated as no-data, NOT a fake total_net=0 day.
	p, ts, _ := newGovBankTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(FinMindResponse{Msg: "success", Status: 200, Data: []map[string]any{
			{"date": "2023-04-06", "stock_id": "6509", "bank_name": "兆豐", "buy": 0, "sell": 1300, "buy_amount": 0.0, "sell_amount": 58745.0},
			{"date": "2023-04-06", "stock_id": "00877", "bank_name": "合庫", "buy": 0, "sell": 1, "buy_amount": 0.0, "sell_amount": 1.0},
		}})
	})
	defer ts.Close()

	day, err := p.FetchDay(context.Background(), time.Date(2023, 4, 6, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("FetchDay: %v", err)
	}
	if day != nil {
		t.Errorf("expected nil day for partial data with no universe match, got %+v", day)
	}
}

func TestFinMindGovernmentBankProvider_FetchDay_EmptyRowsNoData(t *testing.T) {
	p, ts, _ := newGovBankTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(FinMindResponse{Msg: "success", Status: 200, Data: []map[string]any{}})
	})
	defer ts.Close()

	day, err := p.FetchDay(context.Background(), time.Date(2021, 6, 15, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("FetchDay: %v", err)
	}
	if day != nil {
		t.Errorf("expected nil day for empty data, got %+v", day)
	}
}

func TestFinMindGovernmentBankProvider_FetchDay_402Quota(t *testing.T) {
	// Server-side 402 must map to ErrQuotaExhausted and NOT trip the breaker.
	p, ts, _ := newGovBankTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusPaymentRequired)
		_, _ = w.Write([]byte(`{"msg":"Requests reach the upper limit","status":402,"data":[]}`))
	})
	defer ts.Close()

	_, err := p.FetchDay(context.Background(), time.Date(2021, 7, 1, 0, 0, 0, 0, time.UTC))
	if err == nil {
		t.Fatal("expected error on 402")
	}
	if !errors.Is(err, ErrQuotaExhausted) {
		t.Errorf("expected ErrQuotaExhausted, got %v", err)
	}
	info := p.Client().BreakerInfo()
	if info.State != ProviderCircuitClosed {
		t.Errorf("breaker state = %v, want closed (402 must not trip breaker)", info.State)
	}
}

func TestFinMindGovernmentBankProvider_BackfillDay_WritesCompatibleFiles(t *testing.T) {
	p, ts, outDir := newGovBankTestProvider(t, govBankOKHandler())
	defer ts.Close()

	reading, err := p.BackfillDay(context.Background(), time.Date(2021, 6, 30, 0, 0, 0, 0, time.UTC), outDir)
	if err != nil {
		t.Fatalf("BackfillDay: %v", err)
	}
	if reading == nil {
		t.Fatal("expected non-nil reading")
	}
	if reading.Date != "20210630" || reading.TotalNet != 900000 {
		t.Errorf("reading = %+v, want date=20210630 total_net=900000", reading)
	}
	if reading.Source != "broker-aggregate" {
		t.Errorf("Source = %q, want broker-aggregate", reading.Source)
	}

	// YYYYMMDD.json must be a valid GovernmentFlowReading (provider-compatible).
	raw, err := os.ReadFile(filepath.Join(outDir, "20210630.json"))
	if err != nil {
		t.Fatalf("read reading file: %v", err)
	}
	var r GovernmentFlowReading
	if err := json.Unmarshal(raw, &r); err != nil {
		t.Fatalf("decode reading: %v", err)
	}
	if r.Date != "20210630" || r.TotalNet != 900000 {
		t.Errorf("file reading = %+v", r)
	}
	if !GovernmentFlowAllowedSources[r.Source] {
		t.Errorf("source %q not in allowed set", r.Source)
	}

	// YYYYMMDD_brokers.json must decode with the BrokerDailyDetail shape.
	rawB, err := os.ReadFile(filepath.Join(outDir, "20210630_brokers.json"))
	if err != nil {
		t.Fatalf("read brokers file: %v", err)
	}
	var payload struct {
		Date    string              `json:"date"`
		Source  string              `json:"source"`
		Brokers []BrokerDailyDetail `json:"brokers"`
	}
	if err := json.Unmarshal(rawB, &payload); err != nil {
		t.Fatalf("decode brokers: %v", err)
	}
	if len(payload.Brokers) != 2 {
		t.Errorf("brokers len = %d, want 2", len(payload.Brokers))
	}
}

func TestFinMindGovernmentBankProvider_BackfillDay_NoDataWritesNothing(t *testing.T) {
	p, ts, outDir := newGovBankTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(FinMindResponse{Msg: "success", Status: 200, Data: []map[string]any{}})
	})
	defer ts.Close()

	reading, err := p.BackfillDay(context.Background(), time.Date(2021, 6, 15, 0, 0, 0, 0, time.UTC), outDir)
	if err != nil {
		t.Fatalf("BackfillDay: %v", err)
	}
	if reading != nil {
		t.Errorf("expected nil reading, got %+v", reading)
	}
	if _, err := os.Stat(filepath.Join(outDir, "20210615.json")); !os.IsNotExist(err) {
		t.Errorf("expected no reading file for no-data day, stat err = %v", err)
	}
}
