package marketdata

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewTWSESectorIndexProvider(t *testing.T) {
	provider := NewTWSESectorIndexProvider("")
	if provider == nil {
		t.Fatal("expected non-nil provider")
	}
	if provider.Name() != "twse_sector_index" {
		t.Errorf("expected name 'twse_sector_index', got %s", provider.Name())
	}
}

func TestTWSESectorIndexProvider_FetchSectorIndices(t *testing.T) {
	mockData := map[string]interface{}{
		"stat":   "OK",
		"date":   "20240102",
		"title":  "Industry Index",
		"fields": []string{"sector", "index"},
		"data": [][]string{
			{"Semiconductors", "1,250.50"},
			{"Financial and Insurance", "850.30"},
			{"Electric Machinery", "420.15"},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(mockData)
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	provider := NewTWSESectorIndexProvider(tmpDir)
	provider.baseURL = server.URL

	startDate := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
	endDate := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)

	ctx := context.Background()
	result, err := provider.FetchSectorIndices(ctx, startDate, endDate)
	if err != nil {
		t.Fatalf("fetch sector indices: %v", err)
	}

	if len(result) == 0 {
		t.Fatal("expected non-empty result")
	}

	// Check semiconductor data
	if semis, ok := result["semiconductor"]; !ok {
		t.Error("expected semiconductor data")
	} else if len(semis) != 1 {
		t.Errorf("expected 1 data point for semiconductor, got %d", len(semis))
	} else if semis[0].Index != 1250.50 {
		t.Errorf("expected index 1250.50, got %.2f", semis[0].Index)
	}

	// Check financials data
	if fins, ok := result["financials"]; !ok {
		t.Error("expected financials data")
	} else if len(fins) != 1 {
		t.Errorf("expected 1 data point for financials, got %d", len(fins))
	}
}

func TestTWSESectorIndexProvider_Cache(t *testing.T) {
	tmpDir := t.TempDir()
	provider := NewTWSESectorIndexProvider(tmpDir)

	startDate := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	endDate := time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC)

	data := map[string][]SectorIndexData{
		"semiconductor": {
			{Date: "2024-01-01", Industry: "semiconductor", Index: 1000.0},
			{Date: "2024-01-02", Industry: "semiconductor", Index: 1020.0},
			{Date: "2024-01-03", Industry: "semiconductor", Index: 1010.0},
		},
	}

	// Save to cache
	if err := provider.saveToCache(data, startDate, endDate); err != nil {
		t.Fatalf("save to cache: %v", err)
	}

	// Load from cache
	cached, err := provider.loadFromCache(startDate, endDate)
	if err != nil {
		t.Fatalf("load from cache: %v", err)
	}

	if len(cached) != len(data) {
		t.Fatalf("expected %d industries, got %d", len(data), len(cached))
	}

	if len(cached["semiconductor"]) != 3 {
		t.Errorf("expected 3 data points, got %d", len(cached["semiconductor"]))
	}
}

func TestTWSESectorIndexProvider_CalculateReturns(t *testing.T) {
	provider := NewTWSESectorIndexProvider("")

	data := map[string][]SectorIndexData{
		"semiconductor": {
			{Date: "2024-01-01", Industry: "semiconductor", Index: 1000.0},
			{Date: "2024-01-02", Industry: "semiconductor", Index: 1020.0},
			{Date: "2024-01-03", Industry: "semiconductor", Index: 1010.0},
		},
	}

	returns := provider.CalculateReturns(data)

	if len(returns["semiconductor"]) != 2 {
		t.Fatalf("expected 2 returns, got %d", len(returns["semiconductor"]))
	}

	// Day 1: (1020 - 1000) / 1000 = 0.02
	if returns["semiconductor"][0] != 0.02 {
		t.Errorf("expected return 0.02, got %.4f", returns["semiconductor"][0])
	}

	// Day 2: (1010 - 1020) / 1020 = -0.0098...
	expected := -10.0 / 1020.0
	if returns["semiconductor"][1] != expected {
		t.Errorf("expected return %.4f, got %.4f", expected, returns["semiconductor"][1])
	}
}

func TestMapIndustryName(t *testing.T) {
	provider := NewTWSESectorIndexProvider("")

	tests := []struct {
		input    string
		expected string
	}{
		{"Semiconductors", "semiconductor"},
		{"Financial and Insurance", "financials"},
		{"Electric Machinery", "robotics"},
		{"Unknown Sector", ""},
	}

	for _, tt := range tests {
		result := provider.mapIndustryName(tt.input)
		if result != tt.expected {
			t.Errorf("mapIndustryName(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestTWSESectorIndexProvider_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	provider := NewTWSESectorIndexProvider("")
	provider.baseURL = server.URL

	startDate := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
	endDate := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)

	ctx := context.Background()
	result, err := provider.FetchSectorIndices(ctx, startDate, endDate)
	// FetchSectorIndices skips individual day errors and continues
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Error("expected empty result for API failure")
	}
}
