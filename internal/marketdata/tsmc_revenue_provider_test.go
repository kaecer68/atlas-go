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
