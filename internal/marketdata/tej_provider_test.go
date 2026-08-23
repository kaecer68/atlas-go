package marketdata

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func cleanupTEJQuotaState() {
	// P1-12: TEJ quota tracker is file-based (data/state/tej_daily_quota.json).
	// Tests share the same file via NewTEJClient → calls accumulate across
	// tests and hit the 500/day limit. Remove the state file so each test
	// starts at zero.
	_ = os.Remove(filepath.Join("data", "state", "tej_daily_quota.json"))
}

func TestTEJClient_GetStockPriceDaily_Success(t *testing.T) {
	cleanupTEJQuotaState()
	t.Cleanup(cleanupTEJQuotaState)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("api_key") == "" {
			t.Error("expected api_key query parameter")
		}
		if r.Header.Get("Accept") != "application/json" {
			t.Errorf("expected Accept: application/json, got %s", r.Header.Get("Accept"))
		}

		resp := map[string]any{
			"datatable": map[string]any{
				"data": [][]any{
					{"2330", "2025-01-02", 1070.0, 1075.0, 1055.0, 1065.0, 45045.0, 47883206.0},
					{"2330", "2025-01-03", 1065.0, 1070.0, 1050.0, 1060.0, 40000.0, 42400000.0},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewTEJClient("test-key")
	client.baseURL = server.URL // Point to test server.

	rows, err := client.GetStockPriceDaily(context.Background(), "2330", "2024-01-01", "2024-01-31")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	if rows[0].Close != 1065.0 {
		t.Errorf("expected close 1065.0, got %f", rows[0].Close)
	}
	if rows[1].CoID != "2330" {
		t.Errorf("expected coid 2330, got %s", rows[1].CoID)
	}
}

func TestTEJClient_GetStockPriceDaily_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"error":{"code":"Forbidden","message":"Access denied"}}`))
	}))
	defer server.Close()

	client := NewTEJClient("bad-key")
	client.baseURL = server.URL
	_, err := client.GetStockPriceDaily(context.Background(), "2330", "2024-01-01", "2024-01-31")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestTEJClient_GetStockPriceDaily_EmptyResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"datatable": map[string]any{
				"data": [][]any{},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewTEJClient("test-key")
	client.baseURL = server.URL
	rows, err := client.GetStockPriceDaily(context.Background(), "9999", "2024-01-01", "2024-01-31")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("expected 0 rows, got %d", len(rows))
	}
}

func TestToFloat64(t *testing.T) {
	tests := []struct {
		input    any
		expected float64
	}{
		{float64(123.45), 123.45},
		{int64(100), 100.0},
		{"1,234.56", 1234.56},
		{"NA", 0},
		{"-", 0},
		{"(100.5)", -100.5},
	}
	for _, tt := range tests {
		got := toFloat64(tt.input)
		if got != tt.expected {
			t.Errorf("toFloat64(%v) = %f, want %f", tt.input, got, tt.expected)
		}
	}
}

func TestToInt64(t *testing.T) {
	tests := []struct {
		input    any
		expected int64
	}{
		{int64(12345), 12345},
		{float64(67890.0), 67890},
		{"35,000", 35000},
		{"NA", 0},
	}
	for _, tt := range tests {
		got := toInt64(tt.input)
		if got != tt.expected {
			t.Errorf("toInt64(%v) = %d, want %d", tt.input, got, tt.expected)
		}
	}
}

func TestTEJClient_GetFinancialStatements(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"datatable": map[string]any{
				"data": [][]any{
					{"2330", "2024-Q1", 500000.0},
					{"2330", "2024-Q2", 600000.0},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewTEJClient("test-key")
	client.baseURL = server.URL
	rows, err := client.GetFinancialStatements(context.Background(), "2330", "TWN/AFINA", "2024-01-01", "2024-12-31")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rows) != 2 {
		t.Errorf("expected 2 rows, got %d", len(rows))
	}
}

func TestTEJClient_RateLimiter(t *testing.T) {
	client := NewTEJClient("test-key")
	if client.rateLimiter == nil {
		t.Error("expected rateLimiter to be set")
	}
	allowed := client.rateLimiter.AllowN(time.Now(), 1)
	if !allowed {
		t.Error("expected first request to be allowed")
	}
}

func TestTEJClient_Ping_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("api_key") == "" {
			t.Error("expected api_key query parameter")
		}
		resp := map[string]any{
			"datatable": map[string]any{
				"data": [][]any{
					{"2330", "2025-01-03", 1080.0, 1085.0, 1075.0, 1075.0, 31244.0, 33728652.0},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewTEJClient("test-key")
	client.baseURL = server.URL

	err := client.Ping(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestTEJClient_Ping_NoAPIKey(t *testing.T) {
	client := NewTEJClient("")
	err := client.Ping(context.Background())
	if err == nil {
		t.Fatal("expected error for missing API key")
	}
}

func TestTEJClient_Ping_EmptyResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"datatable": map[string]any{
				"data": [][]any{},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewTEJClient("test-key")
	client.baseURL = server.URL

	err := client.Ping(context.Background())
	if err == nil {
		t.Fatal("expected error for empty response")
	}
}
