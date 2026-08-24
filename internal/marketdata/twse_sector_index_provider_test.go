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
	if provider.baseURL != "https://openapi.twse.com.tw/v1" {
		t.Errorf("expected baseURL 'https://openapi.twse.com.tw/v1', got %s", provider.baseURL)
	}
}

func TestTWSESectorIndexProvider_FetchSectorIndices(t *testing.T) {
	withUnlimitedTWSELimiter(t)
	mockData := []twseIndexItem{
		{Index: "半導體類指數", CloseIndex: "1,250.50", Change: "+", ChangePts: "27.36", ChangePct: "1.94"},
		{Index: "金融保險類指數", CloseIndex: "850.30", Change: "+", ChangePts: "5.12", ChangePct: "0.60"},
		{Index: "電機機械類指數", CloseIndex: "420.15", Change: "-", ChangePts: "2.10", ChangePct: "0.50"},
		{Index: "電腦及週邊設備類指數", CloseIndex: "780.00", Change: "+", ChangePts: "15.00", ChangePct: "1.96"},
		{Index: "電子零組件類指數", CloseIndex: "340.50", Change: "+", ChangePts: "3.20", ChangePct: "0.95"},
		{Index: "其他電子類指數", CloseIndex: "210.00", Change: "-", ChangePts: "1.50", ChangePct: "0.71"},
		{Index: "航運類指數", CloseIndex: "560.75", Change: "+", ChangePts: "8.00", ChangePct: "1.45"},
		{Index: "油電燃氣類指數", CloseIndex: "920.10", Change: "+", ChangePts: "4.50", ChangePct: "0.49"},
		{Index: "發行量加權股價指數", CloseIndex: "23,998.95", Change: "+", ChangePts: "37.44", ChangePct: "0.16"},
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
	} else if fins[0].Index != 850.30 {
		t.Errorf("expected index 850.30, got %.2f", fins[0].Index)
	}

	// Check robotics data
	if robotics, ok := result["robotics"]; !ok {
		t.Error("expected robotics data")
	} else if robotics[0].Index != 420.15 {
		t.Errorf("expected index 420.15, got %.2f", robotics[0].Index)
	}

	// Check ai_supply_chain data
	if ai, ok := result["ai_supply_chain"]; !ok {
		t.Error("expected ai_supply_chain data")
	} else if ai[0].Index != 780.00 {
		t.Errorf("expected index 780.00, got %.2f", ai[0].Index)
	}

	// Check electronics data
	if elec, ok := result["electronics"]; !ok {
		t.Error("expected electronics data")
	} else if elec[0].Index != 340.50 {
		t.Errorf("expected index 340.50, got %.2f", elec[0].Index)
	}

	// Check other_electronics data
	if other, ok := result["other_electronics"]; !ok {
		t.Error("expected other_electronics data")
	} else if other[0].Index != 210.00 {
		t.Errorf("expected index 210.00, got %.2f", other[0].Index)
	}

	// Check shipping data
	if ship, ok := result["shipping"]; !ok {
		t.Error("expected shipping data")
	} else if ship[0].Index != 560.75 {
		t.Errorf("expected index 560.75, got %.2f", ship[0].Index)
	}

	// Check energy data
	if energy, ok := result["energy"]; !ok {
		t.Error("expected energy data")
	} else if energy[0].Index != 920.10 {
		t.Errorf("expected index 920.10, got %.2f", energy[0].Index)
	}

	// 8 mapped industries expected (weighted index should be filtered out)
	if len(result) != 8 {
		t.Errorf("expected 8 industries, got %d: %v", len(result), industryKeys(result))
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

	if err := provider.saveToCache(data, startDate, endDate); err != nil {
		t.Fatalf("save to cache: %v", err)
	}

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

	if returns["semiconductor"][0] != 0.02 {
		t.Errorf("expected return 0.02, got %.4f", returns["semiconductor"][0])
	}

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
		{"半導體類指數", "semiconductor"},
		{"電腦及週邊設備類指數", "ai_supply_chain"},
		{"電子零組件類指數", "electronics"},
		{"其他電子類指數", "other_electronics"},
		{"航運類指數", "shipping"},
		{"金融保險類指數", "financials"},
		{"油電燃氣類指數", "energy"},
		{"電機機械類指數", "robotics"},
		{"發行量加權股價指數", ""},
		{"紡織纖維類指數", ""},
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
	withUnlimitedTWSELimiter(t)
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
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Error("expected empty result for API failure")
	}
}

func TestTWSESectorIndexProvider_EmptyCloseIndex(t *testing.T) {
	withUnlimitedTWSELimiter(t)
	mockData := []twseIndexItem{
		{Index: "半導體類指數", CloseIndex: "", Change: "+", ChangePts: "27.36", ChangePct: "1.94"},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(mockData)
	}))
	defer server.Close()

	provider := NewTWSESectorIndexProvider("")
	provider.baseURL = server.URL

	startDate := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
	endDate := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)

	ctx := context.Background()
	result, err := provider.FetchSectorIndices(ctx, startDate, endDate)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Error("expected empty result for empty close index")
	}
}

func TestTWSESectorIndexProvider_ZeroCloseIndex(t *testing.T) {
	withUnlimitedTWSELimiter(t)
	mockData := []twseIndexItem{
		{Index: "半導體類指數", CloseIndex: "0", Change: "+", ChangePts: "0", ChangePct: "0"},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(mockData)
	}))
	defer server.Close()

	provider := NewTWSESectorIndexProvider("")
	provider.baseURL = server.URL

	startDate := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
	endDate := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)

	ctx := context.Background()
	result, err := provider.FetchSectorIndices(ctx, startDate, endDate)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Error("expected empty result for zero close index")
	}
}

func TestTWSESectorIndexProvider_InvalidJSON(t *testing.T) {
	withUnlimitedTWSELimiter(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("not valid json"))
	}))
	defer server.Close()

	provider := NewTWSESectorIndexProvider("")
	provider.baseURL = server.URL

	startDate := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
	endDate := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)

	ctx := context.Background()
	result, err := provider.FetchSectorIndices(ctx, startDate, endDate)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Error("expected empty result for invalid JSON")
	}
}

func industryKeys(m map[string][]SectorIndexData) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
