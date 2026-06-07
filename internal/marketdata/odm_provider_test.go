package marketdata

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type odmMockRevenue struct {
	symbol     string
	startDate  string
	revenue    float64
	returnData bool
}

// newODMMockServer returns a FinMind-shaped test server. The handler matches
// requests by (data_id, start_date) so multiple symbols / month windows can
// be served from a single test.
func newODMMockServer(t *testing.T, responses []odmMockRevenue) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/data" {
			t.Errorf("expected path /api/v4/data, got %s", r.URL.Path)
		}
		if dataset := r.URL.Query().Get("dataset"); dataset != "TaiwanStockMonthRevenue" {
			t.Errorf("expected dataset TaiwanStockMonthRevenue, got %s", dataset)
		}
		dataID := r.URL.Query().Get("data_id")
		startDate := r.URL.Query().Get("start_date")

		for _, resp := range responses {
			if resp.symbol == dataID && resp.startDate == startDate {
				finmindResp := FinMindResponse{Status: 200, Msg: "OK"}
				if resp.returnData {
					finmindResp.Data = []map[string]any{{"revenue": resp.revenue}}
				} else {
					finmindResp.Data = []map[string]any{}
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(finmindResp)
				return
			}
		}
		t.Errorf("unexpected FinMind request: data_id=%s start_date=%s", dataID, startDate)
	}))
}

// newODMTestProvider wires a fresh FinMind client to the test server using
// the same transport-rewrite pattern as TestTSMCRevenueProvider.
func newODMTestProvider(t *testing.T, server *httptest.Server) *ODMRevenueProvider {
	t.Helper()
	client := NewFinMindClient("test-key")
	transport := &mockFinMindTransport{serverURL: strings.TrimPrefix(server.URL, "http://")}
	client.SetHTTPClient(&http.Client{Transport: transport})
	return &ODMRevenueProvider{client: client}
}

func TestODMRevenueProvider_Name(t *testing.T) {
	p := NewODMRevenueProvider("test-key")
	if got := p.Name(); got != "odm_revenue" {
		t.Errorf("Name() = %q, want %q", got, "odm_revenue")
	}
}

func TestODMRevenueProvider_FetchODMRevenue_HappyPath(t *testing.T) {
	now := time.Now()
	year, month := now.Year(), int(now.Month())
	currentStart := fmt.Sprintf("%d-%02d-01", year, month)
	priorStart := fmt.Sprintf("%d-%02d-01", year-1, month)

	currentRevenue := 500e9
	priorRevenue := 400e9
	expectedYoY := (currentRevenue - priorRevenue) / priorRevenue * 100

	server := newODMMockServer(t, []odmMockRevenue{
		{symbol: "2317", startDate: currentStart, revenue: currentRevenue, returnData: true},
		{symbol: "2317", startDate: priorStart, revenue: priorRevenue, returnData: true},
	})
	defer server.Close()

	p := newODMTestProvider(t, server)

	point, err := p.FetchODMRevenue(context.Background(), "2317")
	if err != nil {
		t.Fatalf("FetchODMRevenue() error = %v", err)
	}

	if point.Symbol != "2317" {
		t.Errorf("Symbol = %q, want %q", point.Symbol, "2317")
	}
	if point.Revenue <= 0 {
		t.Errorf("Revenue = %f, want > 0", point.Revenue)
	}
	if point.Revenue != currentRevenue {
		t.Errorf("Revenue = %f, want %f", point.Revenue, currentRevenue)
	}
	if point.YoYPct != expectedYoY {
		t.Errorf("YoYPct = %f, want %f", point.YoYPct, expectedYoY)
	}
	if point.Timestamp == 0 {
		t.Error("Timestamp should not be zero")
	}
}

func TestODMRevenueProvider_FetchODMRevenue_ExtremeValues(t *testing.T) {
	now := time.Now()
	year, month := now.Year(), int(now.Month())
	currentStart := fmt.Sprintf("%d-%02d-01", year, month)
	priorStart := fmt.Sprintf("%d-%02d-01", year-1, month)

	currentRevenue := 1e12
	priorRevenue := 5e11
	expectedYoY := (currentRevenue - priorRevenue) / priorRevenue * 100

	server := newODMMockServer(t, []odmMockRevenue{
		{symbol: "2382", startDate: currentStart, revenue: currentRevenue, returnData: true},
		{symbol: "2382", startDate: priorStart, revenue: priorRevenue, returnData: true},
	})
	defer server.Close()

	p := newODMTestProvider(t, server)

	point, err := p.FetchODMRevenue(context.Background(), "2382")
	if err != nil {
		t.Fatalf("FetchODMRevenue() error = %v", err)
	}

	if point.Revenue != currentRevenue {
		t.Errorf("Revenue = %e, want %e (overflow check)", point.Revenue, currentRevenue)
	}
	if point.YoYPct != expectedYoY {
		t.Errorf("YoYPct = %f, want %f (extreme value math)", point.YoYPct, expectedYoY)
	}
}

func TestODMRevenueProvider_FetchODMRevenue_NoData(t *testing.T) {
	now := time.Now()
	year, month := now.Year(), int(now.Month())
	currentStart := fmt.Sprintf("%d-%02d-01", year, month)

	server := newODMMockServer(t, []odmMockRevenue{
		{symbol: "6669", startDate: currentStart, returnData: false},
	})
	defer server.Close()

	p := newODMTestProvider(t, server)

	_, err := p.FetchODMRevenue(context.Background(), "6669")
	if err == nil {
		t.Fatal("FetchODMRevenue() expected error for empty dataset, got nil")
	}
}

func TestODMRevenueProvider_FetchODMRevenue_ZeroPriorRevenue(t *testing.T) {
	now := time.Now()
	year, month := now.Year(), int(now.Month())
	currentStart := fmt.Sprintf("%d-%02d-01", year, month)
	priorStart := fmt.Sprintf("%d-%02d-01", year-1, month)

	currentRevenue := 100.0
	priorRevenue := 0.0

	server := newODMMockServer(t, []odmMockRevenue{
		{symbol: "2317", startDate: currentStart, revenue: currentRevenue, returnData: true},
		{symbol: "2317", startDate: priorStart, revenue: priorRevenue, returnData: true},
	})
	defer server.Close()

	p := newODMTestProvider(t, server)

	point, err := p.FetchODMRevenue(context.Background(), "2317")
	if err != nil {
		t.Fatalf("FetchODMRevenue() error = %v (zero prior must not error)", err)
	}

	if point.YoYPct != 0 {
		t.Errorf("YoYPct = %f, want 0 (division by zero must be protected)", point.YoYPct)
	}
	if point.Revenue != currentRevenue {
		t.Errorf("Revenue = %f, want %f", point.Revenue, currentRevenue)
	}
}

func TestODMRevenueProvider_FetchAllODMRevenue_AllSuccess(t *testing.T) {
	now := time.Now()
	year, month := now.Year(), int(now.Month())
	currentStart := fmt.Sprintf("%d-%02d-01", year, month)
	priorStart := fmt.Sprintf("%d-%02d-01", year-1, month)

	server := newODMMockServer(t, []odmMockRevenue{
		{symbol: "2317", startDate: currentStart, revenue: 500e9, returnData: true},
		{symbol: "2317", startDate: priorStart, revenue: 400e9, returnData: true},
		{symbol: "2382", startDate: currentStart, revenue: 300e9, returnData: true},
		{symbol: "2382", startDate: priorStart, revenue: 250e9, returnData: true},
		{symbol: "6669", startDate: currentStart, revenue: 100e9, returnData: true},
		{symbol: "6669", startDate: priorStart, revenue: 80e9, returnData: true},
	})
	defer server.Close()

	p := newODMTestProvider(t, server)

	results, err := p.FetchAllODMRevenue(context.Background())
	if err != nil {
		t.Fatalf("FetchAllODMRevenue() error = %v", err)
	}

	if len(results) != 3 {
		t.Fatalf("FetchAllODMRevenue() returned %d symbols, want 3", len(results))
	}

	for _, symbol := range odmSymbols {
		point, ok := results[symbol]
		if !ok {
			t.Errorf("results missing symbol %s", symbol)
			continue
		}
		if point.Symbol != symbol {
			t.Errorf("results[%s].Symbol = %q, want %q", symbol, point.Symbol, symbol)
		}
		if point.Revenue <= 0 {
			t.Errorf("results[%s].Revenue = %f, want > 0", symbol, point.Revenue)
		}
	}
}

func TestODMRevenueProvider_FetchAllODMRevenue_PartialData(t *testing.T) {
	now := time.Now()
	year, month := now.Year(), int(now.Month())
	currentStart := fmt.Sprintf("%d-%02d-01", year, month)
	priorStart := fmt.Sprintf("%d-%02d-01", year-1, month)

	// 2317 succeeds, 2382 fails on current month (no prior request issued),
	// 6669 succeeds.
	server := newODMMockServer(t, []odmMockRevenue{
		{symbol: "2317", startDate: currentStart, revenue: 500e9, returnData: true},
		{symbol: "2317", startDate: priorStart, revenue: 400e9, returnData: true},
		{symbol: "2382", startDate: currentStart, returnData: false},
		{symbol: "6669", startDate: currentStart, revenue: 100e9, returnData: true},
		{symbol: "6669", startDate: priorStart, revenue: 80e9, returnData: true},
	})
	defer server.Close()

	p := newODMTestProvider(t, server)

	results, err := p.FetchAllODMRevenue(context.Background())
	if err != nil {
		t.Fatalf("FetchAllODMRevenue() error = %v (partial should not error)", err)
	}

	if len(results) != 2 {
		t.Fatalf("FetchAllODMRevenue() returned %d symbols, want 2 (partial)", len(results))
	}

	if _, ok := results["2317"]; !ok {
		t.Error("results missing 2317 (succeeded symbol)")
	}
	if _, ok := results["2382"]; ok {
		t.Error("results should not contain 2382 (failed symbol)")
	}
	if _, ok := results["6669"]; !ok {
		t.Error("results missing 6669 (succeeded symbol)")
	}
}
