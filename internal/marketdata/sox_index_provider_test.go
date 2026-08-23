package marketdata

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
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

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "SOX") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(mockResponse))
	}))
	defer server.Close()

	origHosts := yahooHosts
	yahooHosts = []string{strings.TrimPrefix(server.URL, "https://")}
	defer func() { yahooHosts = origHosts }()
	SetYahooSessionClient(server.Client())

	ctx := context.Background()
	snap, err := NewSOXIndexProvider().FetchSnapshot(ctx)
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
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	origHosts := yahooHosts
	yahooHosts = []string{strings.TrimPrefix(server.URL, "https://")}
	defer func() { yahooHosts = origHosts }()
	SetYahooSessionClient(server.Client())

	ctx := context.Background()
	_, err := NewSOXIndexProvider().FetchSnapshot(ctx)
	if err == nil {
		t.Fatal("expected error on API failure")
	}
	if !strings.Contains(err.Error(), "http 500") {
		t.Errorf("expected status error, got: %v", err)
	}
}

func TestSOXIndexProvider_FetchSnapshot_InvalidJSON(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`invalid json`))
	}))
	defer server.Close()

	origHosts := yahooHosts
	yahooHosts = []string{strings.TrimPrefix(server.URL, "https://")}
	defer func() { yahooHosts = origHosts }()
	SetYahooSessionClient(server.Client())

	ctx := context.Background()
	_, err := NewSOXIndexProvider().FetchSnapshot(ctx)
	if err == nil {
		t.Fatal("expected error on invalid JSON")
	}
	if !strings.Contains(err.Error(), "unmarshal") {
		t.Errorf("expected unmarshal error, got: %v", err)
	}
}

func TestSOXIndexProvider_FetchSnapshot_EmptyResult(t *testing.T) {
	mockResponse := `{"chart": {"result": []}}`

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(mockResponse))
	}))
	defer server.Close()

	origHosts := yahooHosts
	yahooHosts = []string{strings.TrimPrefix(server.URL, "https://")}
	defer func() { yahooHosts = origHosts }()
	SetYahooSessionClient(server.Client())

	ctx := context.Background()
	_, err := NewSOXIndexProvider().FetchSnapshot(ctx)
	if err == nil {
		t.Fatal("expected error on empty result")
	}
	if !strings.Contains(err.Error(), "no chart result") {
		t.Errorf(`expected "no chart result" error, got: %v`, err)
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

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(mockResponse))
	}))
	defer server.Close()

	origHosts := yahooHosts
	yahooHosts = []string{strings.TrimPrefix(server.URL, "https://")}
	defer func() { yahooHosts = origHosts }()
	SetYahooSessionClient(server.Client())

	ctx := context.Background()
	_, err := NewSOXIndexProvider().FetchSnapshot(ctx)
	if err == nil {
		t.Fatal("expected error on all-NaN price array")
	}
	// P0-6: the unified findLastValidClose reports "no valid close prices"
	// when the entire closes array is zero/NaN (previously "invalid latest").
	if !strings.Contains(err.Error(), "no valid close prices") {
		t.Errorf(`expected error containing "no valid close prices", got: %v`, err)
	}
}

func TestSOXIndexProvider_FetchSnapshot_HTMLResponse(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`<html><body>Rate Limited</body></html>`))
	}))
	defer server.Close()

	origHosts := yahooHosts
	yahooHosts = []string{strings.TrimPrefix(server.URL, "https://")}
	defer func() { yahooHosts = origHosts }()
	SetYahooSessionClient(server.Client())

	ctx := context.Background()
	_, err := NewSOXIndexProvider().FetchSnapshot(ctx)
	if err == nil {
		t.Fatal("expected error on HTML response")
	}
	if !strings.Contains(err.Error(), "HTML response") {
		t.Errorf(`expected "HTML response" error, got: %v`, err)
	}
}

func TestSOXIndexProvider_FetchSnapshot_HostFallback(t *testing.T) {
	calls := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"chart":{"result":[{"meta":{"regularMarketTime":1234567890},"indicators":{"quote":[{"close":[5000.0,5100.0]}]}}]}}`))
	}))
	defer server.Close()

	host := strings.TrimPrefix(server.URL, "https://")
	origHosts := yahooHosts
	yahooHosts = []string{host, host}
	defer func() { yahooHosts = origHosts }()
	SetYahooSessionClient(server.Client())

	ctx := context.Background()
	snap, err := NewSOXIndexProvider().FetchSnapshot(ctx)
	if err != nil {
		t.Fatalf("FetchSnapshot() error after fallback = %v", err)
	}
	if snap.SOXIndex.Value != 5100.0 {
		t.Errorf("SOXIndex.Value = %v, want %v", snap.SOXIndex.Value, 5100.0)
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
