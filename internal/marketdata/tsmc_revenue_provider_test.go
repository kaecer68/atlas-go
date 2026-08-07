package marketdata

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type mockFinMindTransport struct {
	serverURL string
}

func (t *mockFinMindTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.URL.Scheme = "http"
	req.URL.Host = t.serverURL
	return http.DefaultTransport.RoundTrip(req)
}

func TestTSMCRevenueProvider_Name(t *testing.T) {
	p := NewTSMCRevenueProvider("test-key")
	if got := p.Name(); got != "tsmc_revenue" {
		t.Errorf("Name() = %q, want %q", got, "tsmc_revenue")
	}
}

func TestTSMCRevenueProvider_FetchSnapshot_ValidData(t *testing.T) {
	now := time.Now()
	year, month := now.Year(), int(now.Month())

	currentRevenue := 25000000000.0
	priorRevenue := 20000000000.0
	expectedYoY := (currentRevenue - priorRevenue) / priorRevenue * 100

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/data" {
			t.Errorf("expected path /api/v4/data, got %s", r.URL.Path)
		}

		dataset := r.URL.Query().Get("dataset")
		if dataset != "TaiwanStockMonthRevenue" {
			t.Errorf("expected dataset TaiwanStockMonthRevenue, got %s", dataset)
		}

		dataID := r.URL.Query().Get("data_id")
		if dataID != "2330" {
			t.Errorf("expected data_id 2330, got %s", dataID)
		}

		startDate := r.URL.Query().Get("start_date")

		resp := FinMindResponse{Status: 200, Msg: "OK"}

		expectedCurrentStart := fmt.Sprintf("%d-%02d-01", year, month)
		expectedPriorStart := fmt.Sprintf("%d-%02d-01", year-1, month)

		if startDate == expectedCurrentStart {
			resp.Data = []map[string]any{{"revenue": currentRevenue}}
		} else if startDate == expectedPriorStart {
			resp.Data = []map[string]any{{"revenue": priorRevenue}}
		} else {
			t.Errorf("unexpected start_date: %s", startDate)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewFinMindClient("test-key")
	transport := &mockFinMindTransport{serverURL: strings.TrimPrefix(server.URL, "http://")}
	client.SetHTTPClient(&http.Client{Transport: transport})

	p := &TSMCRevenueProvider{client: client}

	ctx := context.Background()
	snap, err := p.FetchSnapshot(ctx)
	if err != nil {
		t.Fatalf("FetchSnapshot() error = %v", err)
	}

	if snap.TSMCRevenue.Symbol != "2330.TW" {
		t.Errorf("TSMCRevenue.Symbol = %q, want %q", snap.TSMCRevenue.Symbol, "2330.TW")
	}

	if snap.TSMCRevenue.Value != currentRevenue {
		t.Errorf("TSMCRevenue.Value = %f, want %f", snap.TSMCRevenue.Value, currentRevenue)
	}

	if snap.TSMCRevenue.ChangePct != expectedYoY {
		t.Errorf("TSMCRevenue.ChangePct = %f, want %f", snap.TSMCRevenue.ChangePct, expectedYoY)
	}

	if snap.TSMCRevenue.Timestamp == 0 {
		t.Error("TSMCRevenue.Timestamp should not be zero")
	}
}

func TestTSMCRevenueProvider_OnDegradedCalledOnFallback(t *testing.T) {
	now := time.Now()
	year, month := now.Year(), int(now.Month())

	// Server returns 400 for all requests (simulate API failure)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()

	client := NewFinMindClient("test-key")
	transport := &mockFinMindTransport{serverURL: strings.TrimPrefix(server.URL, "http://")}
	client.SetHTTPClient(&http.Client{Transport: transport})

	storageDir := t.TempDir()

	// Pre-seed cache with valid revenue data
	rocYear := year - 1911
	cacheDate := fmt.Sprintf("%03d%02d", rocYear-1, month) // prior month
	cacheFile := filepath.Join(storageDir, cacheDate+"_revenue.json")
	_ = os.MkdirAll(storageDir, 0o755)
	cacheRecord := tsmcRevenueRecord{
		Date:      cacheDate,
		Revenue:   18000000000.0,
		YoYPct:    15.0,
		Timestamp: now.Unix(),
	}
	data, _ := json.Marshal(cacheRecord)
	if err := os.WriteFile(cacheFile, data, 0o644); err != nil {
		t.Fatalf("seed cache: %v", err)
	}

	var degradedChannel, degradedReason string
	p := &TSMCRevenueProvider{
		client:     client,
		storageDir: storageDir,
		OnDegraded: func(channelID, reason string) {
			degradedChannel = channelID
			degradedReason = reason
		},
	}

	ctx := context.Background()
	snap, err := p.FetchSnapshot(ctx)
	if err != nil {
		t.Fatalf("FetchSnapshot() error = %v", err)
	}

	// Must have served from cache
	if snap.TSMCRevenue.Symbol == "" {
		t.Fatal("expected cached data from loadLatestSnapshot")
	}
	if snap.TSMCRevenue.Value != 18000000000.0 {
		t.Fatalf("expected cached revenue 18000000000.0, got %f", snap.TSMCRevenue.Value)
	}

	// OnDegraded must have been called
	if degradedChannel != "tsmc_revenue" {
		t.Fatalf("expected degraded channel 'tsmc_revenue', got %q", degradedChannel)
	}
	if degradedReason != "cache_fallback" {
		t.Fatalf("expected degraded reason 'cache_fallback', got %q", degradedReason)
	}
}

func TestTSMCRevenueProvider_FetchSnapshot_MissingData(t *testing.T) {
	now := time.Now()
	year, month := now.Year(), int(now.Month())

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		startDate := r.URL.Query().Get("start_date")
		expectedCurrentStart := fmt.Sprintf("%d-%02d-01", year, month)

		resp := FinMindResponse{Status: 200, Msg: "OK"}

		if startDate == expectedCurrentStart {
			resp.Data = []map[string]any{}
		} else {
			resp.Data = []map[string]any{{"revenue": 20000000000.0}}
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewFinMindClient("test-key")
	transport := &mockFinMindTransport{serverURL: strings.TrimPrefix(server.URL, "http://")}
	client.SetHTTPClient(&http.Client{Transport: transport})

	p := &TSMCRevenueProvider{client: client}

	ctx := context.Background()
	snap, err := p.FetchSnapshot(ctx)
	if err != nil {
		t.Fatalf("FetchSnapshot() error = %v", err)
	}

	if snap.TSMCRevenue.Symbol != "" {
		t.Errorf("TSMCRevenue.Symbol = %q, want empty string for missing data", snap.TSMCRevenue.Symbol)
	}
}

// TestTSMCRevenueProvider_FetchSnapshotForSymbol_ValidData verifies the new
// per-symbol method (added 2026-08-07 for stock_get_monthly_revenue endpoint).
// Same shape as FetchSnapshot_ValidData but symbol = 3131 (TPEX-listed
// 弘塑) instead of 2330 — confirms the FinMind call carries the right
// data_id and the returned MacroDataSnapshot reflects that symbol.
func TestTSMCRevenueProvider_FetchSnapshotForSymbol_ValidData(t *testing.T) {
	now := time.Now()
	year, month := now.Year(), int(now.Month())

	currentRevenue := 631051000.0
	priorRevenue := 560000000.0
	expectedYoY := (currentRevenue - priorRevenue) / priorRevenue * 100

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/data" {
			t.Errorf("expected path /api/v4/data, got %s", r.URL.Path)
		}
		dataset := r.URL.Query().Get("dataset")
		if dataset != "TaiwanStockMonthRevenue" {
			t.Errorf("expected dataset TaiwanStockMonthRevenue, got %s", dataset)
		}
		// Critical: data_id must be the per-symbol argument, NOT 2330.
		dataID := r.URL.Query().Get("data_id")
		if dataID != "3131" {
			t.Errorf("expected data_id 3131, got %s", dataID)
		}
		resp := FinMindResponse{Status: 200, Msg: "OK"}
		startDate := r.URL.Query().Get("start_date")
		expectedCurrentStart := fmt.Sprintf("%d-%02d-01", year, month)
		expectedPriorStart := fmt.Sprintf("%d-%02d-01", year-1, month)
		if startDate == expectedCurrentStart {
			resp.Data = []map[string]any{{"revenue": currentRevenue, "date": expectedCurrentStart}}
		} else if startDate == expectedPriorStart {
			resp.Data = []map[string]any{{"revenue": priorRevenue, "date": expectedPriorStart}}
		} else {
			resp.Data = []map[string]any{}
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewFinMindClient("test-key")
	transport := &mockFinMindTransport{serverURL: strings.TrimPrefix(server.URL, "http://")}
	client.SetHTTPClient(&http.Client{Transport: transport})

	p := &TSMCRevenueProvider{client: client, storageDir: t.TempDir()}

	ctx := context.Background()
	snap, err := p.FetchSnapshotForSymbol(ctx, "3131")
	if err != nil {
		t.Fatalf("FetchSnapshotForSymbol(3131) error = %v", err)
	}
	if snap.TSMCRevenue.Symbol != "3131.TW" {
		t.Errorf("TSMCRevenue.Symbol = %q, want %q (per-symbol)", snap.TSMCRevenue.Symbol, "3131.TW")
	}
	if snap.TSMCRevenue.Value != currentRevenue {
		t.Errorf("TSMCRevenue.Value = %v, want %v", snap.TSMCRevenue.Value, currentRevenue)
	}
	if snap.TSMCRevenue.ChangePct != expectedYoY {
		t.Errorf("TSMCRevenue.ChangePct = %v, want %v", snap.TSMCRevenue.ChangePct, expectedYoY)
	}
}

// TestTSMCRevenueProvider_FetchSnapshot_StillDefaultsTo2330 is the
// backward-compatibility regression guard: the macro channel consumer
// (silicon_cycle.go:408) calls FetchSnapshot (no symbol argument) and
// expects the TSMC symbol "2330.TW". A naive refactor that routes
// FetchSnapshot through FetchSnapshotForSymbol with a different default
// would silently break silicon_cycle. This test pins the contract.
func TestTSMCRevenueProvider_FetchSnapshot_StillDefaultsTo2330(t *testing.T) {
	now := time.Now()
	year, month := now.Year(), int(now.Month())

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		dataID := r.URL.Query().Get("data_id")
		if dataID != "2330" {
			t.Errorf("FetchSnapshot default symbol = %q, want 2330 (macro channel contract)", dataID)
		}
		resp := FinMindResponse{Status: 200, Msg: "OK"}
		startDate := r.URL.Query().Get("start_date")
		expectedCurrentStart := fmt.Sprintf("%d-%02d-01", year, month)
		expectedPriorStart := fmt.Sprintf("%d-%02d-01", year-1, month)
		if startDate == expectedCurrentStart {
			resp.Data = []map[string]any{{"revenue": 25000000000.0, "date": expectedCurrentStart}}
		} else if startDate == expectedPriorStart {
			resp.Data = []map[string]any{{"revenue": 20000000000.0, "date": expectedPriorStart}}
		} else {
			resp.Data = []map[string]any{}
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewFinMindClient("test-key")
	transport := &mockFinMindTransport{serverURL: strings.TrimPrefix(server.URL, "http://")}
	client.SetHTTPClient(&http.Client{Transport: transport})

	p := &TSMCRevenueProvider{client: client, storageDir: t.TempDir()}

	snap, err := p.FetchSnapshot(context.Background())
	if err != nil {
		t.Fatalf("FetchSnapshot error = %v", err)
	}
	if snap.TSMCRevenue.Symbol != "2330.TW" {
		t.Errorf("TSMCRevenue.Symbol = %q, want %q (macro contract)", snap.TSMCRevenue.Symbol, "2330.TW")
	}
}

// TestTSMCRevenueProvider_QuotaRemaining verifies the new QuotaRemaining
// method (added 2026-08-07 for stocktools handler quota-aware fail-soft).
// Used by the /api/stock/monthly_revenue handler to 503 before exhausting
// the FinMind daily budget on a 3-call per-symbol lookup.
func TestTSMCRevenueProvider_QuotaRemaining(t *testing.T) {
	// Use a per-test TempDir so DailyQuotaTracker doesn't read the
	// production data/state/finmind_daily_quota.json (which would
	// cause cross-test contamination of remaining-count assertions).
	client := newFinMindClientInternal("test-key", t.TempDir())
	p := &TSMCRevenueProvider{client: client}
	remaining := p.QuotaRemaining()
	if remaining != 14400 {
		t.Errorf("QuotaRemaining() = %d, want 14400 (full daily limit on fresh tracker)", remaining)
	}

	// After one AllowCall, remaining should drop by 1.
	client.quotaTracker.AllowCall()
	if got := p.QuotaRemaining(); got != 14399 {
		t.Errorf("QuotaRemaining() after 1 call = %d, want 14399", got)
	}

	// Nil client must not panic — return 0 (signals "not configured" to
	// callers that want to fail-closed).
	p2 := &TSMCRevenueProvider{}
	if got := p2.QuotaRemaining(); got != 0 {
		t.Errorf("QuotaRemaining() with nil client = %d, want 0", got)
	}
}
