package marketdata

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// US index providers use Yahoo Finance for ^GSPC, ^IXIC, ^DJI.
// Reference levels (2026-04-29 close):
//   - S&P 500 (^GSPC): 5,400 → 5,820 (+7.78% YoY)
//   - Nasdaq Composite (^IXIC): 16,200 → 17,500 (+8.02%)
//   - Dow Jones (^DJI): 38,500 → 41,200 (+7.01%)

func TestSPXIndexProvider_Name(t *testing.T) {
	if got := NewSPXIndexProvider().Name(); got != "us_spx" {
		t.Errorf("Name() = %q, want us_spx", got)
	}
}

func TestSPXIndexProvider_FetchSnapshot_Success(t *testing.T) {
	mockResponse := `{
		"chart": {
			"result": [
				{
					"meta": {"regularMarketTime": 1714500000, "regularMarketPrice": 5820.0},
					"indicators": {"quote": [{"close": [5400.0, 5820.0]}]}
				}
			]
		}
	}`

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "^GSPC") {
			t.Errorf("unexpected path: %s, expected ^GSPC", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(mockResponse))
	}))
	defer server.Close()

	origHosts := yahooHosts
	yahooHosts = []string{strings.TrimPrefix(server.URL, "https://")}
	defer func() { yahooHosts = origHosts }()
	SetYahooSessionClient(server.Client())

	snap, err := NewSPXIndexProvider().FetchSnapshot(context.Background())
	if err != nil {
		t.Fatalf("FetchSnapshot() error = %v", err)
	}
	if snap.SPXIndex.Symbol != "^GSPC" {
		t.Errorf("Symbol = %q, want ^GSPC", snap.SPXIndex.Symbol)
	}
	if snap.SPXIndex.Value != 5820.0 {
		t.Errorf("Value = %v, want 5820.0", snap.SPXIndex.Value)
	}
	// (5820-5400)/5400*100 = 7.7777..., rounds to 7.78
	expectedPct := 7.78
	if snap.SPXIndex.ChangePct != expectedPct {
		t.Errorf("ChangePct = %v, want %v", snap.SPXIndex.ChangePct, expectedPct)
	}
	if snap.SPXIndex.Timestamp != 1714500000 {
		t.Errorf("Timestamp = %v, want 1714500000", snap.SPXIndex.Timestamp)
	}
}

func TestSPXIndexProvider_FetchSnapshot_NoChartResult(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"chart":{"result":[]}}`))
	}))
	defer server.Close()

	origHosts := yahooHosts
	yahooHosts = []string{strings.TrimPrefix(server.URL, "https://")}
	defer func() { yahooHosts = origHosts }()
	SetYahooSessionClient(server.Client())

	if _, err := NewSPXIndexProvider().FetchSnapshot(context.Background()); err == nil {
		t.Fatal("expected error when chart result is empty")
	}
}

func TestSPXIndexProvider_FetchSnapshot_NoClosePrices(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"chart":{"result":[{"indicators":{"quote":[{"close":[]}]}}]}}`))
	}))
	defer server.Close()

	origHosts := yahooHosts
	yahooHosts = []string{strings.TrimPrefix(server.URL, "https://")}
	defer func() { yahooHosts = origHosts }()
	SetYahooSessionClient(server.Client())

	if _, err := NewSPXIndexProvider().FetchSnapshot(context.Background()); err == nil {
		t.Fatal("expected error when close prices are empty")
	}
}

func TestSPXIndexProvider_FetchSnapshot_ZeroLatest(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"chart":{"result":[{"indicators":{"quote":[{"close":[0, 0]}]}}]}}`))
	}))
	defer server.Close()

	origHosts := yahooHosts
	yahooHosts = []string{strings.TrimPrefix(server.URL, "https://")}
	defer func() { yahooHosts = origHosts }()
	SetYahooSessionClient(server.Client())

	if _, err := NewSPXIndexProvider().FetchSnapshot(context.Background()); err == nil {
		t.Fatal("expected error when latest is 0")
	}
}

func TestNDXIndexProvider_Name(t *testing.T) {
	if got := NewNDXIndexProvider().Name(); got != "us_ndx" {
		t.Errorf("Name() = %q, want us_ndx", got)
	}
}

func TestNDXIndexProvider_FetchSnapshot_Success(t *testing.T) {
	mockResponse := `{
		"chart": {
			"result": [
				{
					"meta": {"regularMarketTime": 1714500000, "regularMarketPrice": 17500.0},
					"indicators": {"quote": [{"close": [16200.0, 17500.0]}]}
				}
			]
		}
	}`

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "^IXIC") {
			t.Errorf("unexpected path: %s, expected ^IXIC", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(mockResponse))
	}))
	defer server.Close()

	origHosts := yahooHosts
	yahooHosts = []string{strings.TrimPrefix(server.URL, "https://")}
	defer func() { yahooHosts = origHosts }()
	SetYahooSessionClient(server.Client())

	snap, err := NewNDXIndexProvider().FetchSnapshot(context.Background())
	if err != nil {
		t.Fatalf("FetchSnapshot() error = %v", err)
	}
	if snap.NDXIndex.Symbol != "^IXIC" {
		t.Errorf("Symbol = %q, want ^IXIC", snap.NDXIndex.Symbol)
	}
	if snap.NDXIndex.Value != 17500.0 {
		t.Errorf("Value = %v, want 17500.0", snap.NDXIndex.Value)
	}
	// (17500-16200)/16200*100 = 8.0246..., rounds to 8.02
	expectedPct := 8.02
	if snap.NDXIndex.ChangePct != expectedPct {
		t.Errorf("ChangePct = %v, want %v", snap.NDXIndex.ChangePct, expectedPct)
	}
}

func TestNDXIndexProvider_FetchSnapshot_NoChartResult(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"chart":{"result":[]}}`))
	}))
	defer server.Close()

	origHosts := yahooHosts
	yahooHosts = []string{strings.TrimPrefix(server.URL, "https://")}
	defer func() { yahooHosts = origHosts }()
	SetYahooSessionClient(server.Client())

	if _, err := NewNDXIndexProvider().FetchSnapshot(context.Background()); err == nil {
		t.Fatal("expected error when chart result is empty")
	}
}

func TestDJIIndexProvider_Name(t *testing.T) {
	if got := NewDJIIndexProvider().Name(); got != "us_dji" {
		t.Errorf("Name() = %q, want us_dji", got)
	}
}

func TestDJIIndexProvider_FetchSnapshot_Success(t *testing.T) {
	mockResponse := `{
		"chart": {
			"result": [
				{
					"meta": {"regularMarketTime": 1714500000, "regularMarketPrice": 41200.0},
					"indicators": {"quote": [{"close": [38500.0, 41200.0]}]}
				}
			]
		}
	}`

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "^DJI") {
			t.Errorf("unexpected path: %s, expected ^DJI", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(mockResponse))
	}))
	defer server.Close()

	origHosts := yahooHosts
	yahooHosts = []string{strings.TrimPrefix(server.URL, "https://")}
	defer func() { yahooHosts = origHosts }()
	SetYahooSessionClient(server.Client())

	snap, err := NewDJIIndexProvider().FetchSnapshot(context.Background())
	if err != nil {
		t.Fatalf("FetchSnapshot() error = %v", err)
	}
	if snap.DJIIndex.Symbol != "^DJI" {
		t.Errorf("Symbol = %q, want ^DJI", snap.DJIIndex.Symbol)
	}
	if snap.DJIIndex.Value != 41200.0 {
		t.Errorf("Value = %v, want 41200.0", snap.DJIIndex.Value)
	}
	// (41200-38500)/38500*100 = 7.0129..., rounds to 7.01
	expectedPct := 7.01
	if snap.DJIIndex.ChangePct != expectedPct {
		t.Errorf("ChangePct = %v, want %v", snap.DJIIndex.ChangePct, expectedPct)
	}
}

func TestDJIIndexProvider_FetchSnapshot_NoChartResult(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"chart":{"result":[]}}`))
	}))
	defer server.Close()

	origHosts := yahooHosts
	yahooHosts = []string{strings.TrimPrefix(server.URL, "https://")}
	defer func() { yahooHosts = origHosts }()
	SetYahooSessionClient(server.Client())

	if _, err := NewDJIIndexProvider().FetchSnapshot(context.Background()); err == nil {
		t.Fatal("expected error when chart result is empty")
	}
}

func TestDJIIndexProvider_FetchSnapshot_NoClosePrices(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"chart":{"result":[{"indicators":{"quote":[{"close":[]}]}}]}}`))
	}))
	defer server.Close()

	origHosts := yahooHosts
	yahooHosts = []string{strings.TrimPrefix(server.URL, "https://")}
	defer func() { yahooHosts = origHosts }()
	SetYahooSessionClient(server.Client())

	if _, err := NewDJIIndexProvider().FetchSnapshot(context.Background()); err == nil {
		t.Fatal("expected error when close prices empty")
	}
}