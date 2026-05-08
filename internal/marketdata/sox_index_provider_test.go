package marketdata

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSOXIndexProvider_Name(t *testing.T) {
	p := NewSOXIndexProvider()
	if got := p.Name(); got != "sox_index" {
		t.Errorf("Name() = %q, want %q", got, "sox_index")
	}
}

func TestSOXIndexProvider_FetchSnapshot_Success(t *testing.T) {
	mockResponse := `{
		"chart": {
			"result": [
				{
					"meta": {
						"regularMarketTime": 1234567890
					},
					"indicators": {
						"quote": [
							{
								"close": [5000.0, 5100.0]
							}
						]
					}
				}
			]
		}
	}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v8/finance/chart/^SOX" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(mockResponse))
	}))
	defer server.Close()

	provider := &SOXIndexProvider{
		httpClient: server.Client(),
		baseURL:    server.URL,
	}

	ctx := context.Background()
	snap, err := provider.FetchSnapshot(ctx)
	if err != nil {
		t.Fatalf("FetchSnapshot() error = %v", err)
	}

	if snap.SOXIndex.Symbol != "^SOX" {
		t.Errorf("SOXIndex.Symbol = %q, want %q", snap.SOXIndex.Symbol, "^SOX")
	}

	if snap.SOXIndex.Value != 5100.0 {
		t.Errorf("SOXIndex.Value = %v, want %v", snap.SOXIndex.Value, 5100.0)
	}

	if snap.SOXIndex.ChangePct != 2.0 {
		t.Errorf("SOXIndex.ChangePct = %v, want %v", snap.SOXIndex.ChangePct, 2.0)
	}
}

func TestSOXIndexProvider_FetchSnapshot_APIFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	provider := &SOXIndexProvider{
		httpClient: server.Client(),
		baseURL:    server.URL,
	}

	ctx := context.Background()
	snap, err := provider.FetchSnapshot(ctx)
	if err != nil {
		t.Fatalf("FetchSnapshot() error = %v", err)
	}

	if snap.SOXIndex.Symbol != "" {
		t.Errorf("SOXIndex.Symbol = %q, want empty string on failure", snap.SOXIndex.Symbol)
	}
}

func TestSOXIndexProvider_FetchSnapshot_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`invalid json`))
	}))
	defer server.Close()

	provider := &SOXIndexProvider{
		httpClient: server.Client(),
		baseURL:    server.URL,
	}

	ctx := context.Background()
	snap, err := provider.FetchSnapshot(ctx)
	if err != nil {
		t.Fatalf("FetchSnapshot() error = %v", err)
	}

	if snap.SOXIndex.Symbol != "" {
		t.Errorf("SOXIndex.Symbol = %q, want empty string on invalid JSON", snap.SOXIndex.Symbol)
	}
}

func TestSOXIndexProvider_FetchSnapshot_EmptyResult(t *testing.T) {
	mockResponse := `{"chart": {"result": []}}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(mockResponse))
	}))
	defer server.Close()

	provider := &SOXIndexProvider{
		httpClient: server.Client(),
		baseURL:    server.URL,
	}

	ctx := context.Background()
	snap, err := provider.FetchSnapshot(ctx)
	if err != nil {
		t.Fatalf("FetchSnapshot() error = %v", err)
	}

	if snap.SOXIndex.Symbol != "" {
		t.Errorf("SOXIndex.Symbol = %q, want empty string on empty result", snap.SOXIndex.Symbol)
	}
}

func TestSOXIndexProvider_FetchSnapshot_NaNPrice(t *testing.T) {
	mockResponse := `{
		"chart": {
			"result": [
				{
					"meta": {
						"regularMarketTime": 1234567890
					},
					"indicators": {
						"quote": [
							{
								"close": [null, null]
							}
						]
					}
				}
			]
		}
	}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(mockResponse))
	}))
	defer server.Close()

	provider := &SOXIndexProvider{
		httpClient: server.Client(),
		baseURL:    server.URL,
	}

	ctx := context.Background()
	snap, err := provider.FetchSnapshot(ctx)
	if err != nil {
		t.Fatalf("FetchSnapshot() error = %v", err)
	}

	if snap.SOXIndex.Symbol != "" {
		t.Errorf("SOXIndex.Symbol = %q, want empty string on NaN price", snap.SOXIndex.Symbol)
	}
}

func TestSOXIndexProvider_CompositeMerge(t *testing.T) {
	mockSOX := MacroDataSnapshot{
		SOXIndex: MacroDataPoint{
			Symbol:    "^SOX",
			Value:     5000.0,
			ChangePct: 1.5,
			Timestamp: 1234567890,
		},
	}

	mockOther := MacroDataSnapshot{
		US10Y: MacroDataPoint{
			Symbol:    "^TNX",
			Value:     4.5,
			ChangePct: 0.1,
			Timestamp: 1234567890,
		},
	}

	composite := NewCompositeMacroProvider(
		&MockMacroProvider{Snapshot: mockOther},
		&MockMacroProvider{Snapshot: mockSOX},
	)

	ctx := context.Background()
	merged, err := composite.FetchSnapshot(ctx)
	if err != nil {
		t.Fatalf("FetchSnapshot() error = %v", err)
	}

	if merged.SOXIndex.Symbol != "^SOX" {
		t.Errorf("merged.SOXIndex.Symbol = %q, want %q", merged.SOXIndex.Symbol, "^SOX")
	}
	if merged.SOXIndex.Value != 5000.0 {
		t.Errorf("merged.SOXIndex.Value = %v, want %v", merged.SOXIndex.Value, 5000.0)
	}
	if merged.US10Y.Symbol != "^TNX" {
		t.Errorf("merged.US10Y.Symbol = %q, want %q", merged.US10Y.Symbol, "^TNX")
	}
}
