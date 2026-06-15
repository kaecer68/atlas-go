package marketdata

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// US tech providers use Yahoo Finance as the source.
// Reference prices (2026-04-29, illustrative):
//   - NVDA: $950.00 → $1010.00 (+6.32% over 1y window)
//   - AAPL: $195.00 → $215.00 (+10.26%)
//   - MSFT: $410.00 → $445.00 (+8.54%)

func TestNVDAProvider_Name(t *testing.T) {
	p := NewNVDAProvider()
	if got := p.Name(); got != "us_nvda" {
		t.Errorf("Name() = %q, want %q", got, "us_nvda")
	}
}

func TestNVDAProvider_FetchSnapshot_Success(t *testing.T) {
	mockResponse := `{
		"chart": {
			"result": [
				{
					"meta": {"regularMarketTime": 1714500000, "regularMarketPrice": 1010.0},
					"indicators": {"quote": [{"close": [950.0, 1010.0]}]}
				}
			]
		}
	}`

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "NVDA") {
			t.Errorf("unexpected path: %s, expected NVDA", r.URL.Path)
		}
		if r.URL.Query().Get("range") != "1y" {
			t.Errorf("expected range=1y, got %s", r.URL.Query().Get("range"))
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

	snap, err := NewNVDAProvider().FetchSnapshot(context.Background())
	if err != nil {
		t.Fatalf("FetchSnapshot() error = %v", err)
	}
	if snap.NVDA.Symbol != "NVDA" {
		t.Errorf("Symbol = %q, want NVDA", snap.NVDA.Symbol)
	}
	if snap.NVDA.Value != 1010.0 {
		t.Errorf("Value = %v, want 1010.0", snap.NVDA.Value)
	}
	// (1010-950)/950*100 = 6.315... 
	expectedPct := (1010.0 - 950.0) / 950.0 * 100
	if snap.NVDA.ChangePct != expectedPct {
		t.Errorf("ChangePct = %v, want %v", snap.NVDA.ChangePct, expectedPct)
	}
	if snap.RecordedAt == 0 {
		t.Error("RecordedAt should be set")
	}
}

func TestNVDAProvider_FetchSnapshot_NoResult(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"chart":{"result":[]}}`))
	}))
	defer server.Close()

	origHosts := yahooHosts
	yahooHosts = []string{strings.TrimPrefix(server.URL, "https://")}
	defer func() { yahooHosts = origHosts }()
	SetYahooSessionClient(server.Client())

	if _, err := NewNVDAProvider().FetchSnapshot(context.Background()); err == nil {
		t.Fatal("expected error when no chart result")
	}
}

func TestNVDAProvider_FetchSnapshot_NoClosePrices(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"chart":{"result":[{"indicators":{"quote":[{"close":[]}]}}]}}`))
	}))
	defer server.Close()

	origHosts := yahooHosts
	yahooHosts = []string{strings.TrimPrefix(server.URL, "https://")}
	defer func() { yahooHosts = origHosts }()
	SetYahooSessionClient(server.Client())

	if _, err := NewNVDAProvider().FetchSnapshot(context.Background()); err == nil {
		t.Fatal("expected error when close prices empty")
	}
}

func TestAAPLProvider_Name(t *testing.T) {
	if got := NewAAPLProvider().Name(); got != "us_aapl" {
		t.Errorf("Name() = %q, want us_aapl", got)
	}
}

func TestAAPLProvider_FetchSnapshot_Success(t *testing.T) {
	mockResponse := `{
		"chart": {
			"result": [
				{
					"meta": {"regularMarketTime": 1714500000, "regularMarketPrice": 215.0},
					"indicators": {"quote": [{"close": [195.0, 215.0]}]}
				}
			]
		}
	}`

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "AAPL") {
			t.Errorf("unexpected path: %s, expected AAPL", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(mockResponse))
	}))
	defer server.Close()

	origHosts := yahooHosts
	yahooHosts = []string{strings.TrimPrefix(server.URL, "https://")}
	defer func() { yahooHosts = origHosts }()
	SetYahooSessionClient(server.Client())

	snap, err := NewAAPLProvider().FetchSnapshot(context.Background())
	if err != nil {
		t.Fatalf("FetchSnapshot() error = %v", err)
	}
	if snap.AAPL.Symbol != "AAPL" {
		t.Errorf("Symbol = %q, want AAPL", snap.AAPL.Symbol)
	}
	if snap.AAPL.Value != 215.0 {
		t.Errorf("Value = %v, want 215.0", snap.AAPL.Value)
	}
	// (215-195)/195*100 = 10.256410...
	expectedPct := (215.0 - 195.0) / 195.0 * 100
	if diff := snap.AAPL.ChangePct - expectedPct; diff > 1e-9 || diff < -1e-9 {
		t.Errorf("ChangePct = %v, want %v (diff %v)", snap.AAPL.ChangePct, expectedPct, diff)
	}
}

func TestAAPLProvider_FetchSnapshot_ZeroLatestRejected(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"chart":{"result":[{"indicators":{"quote":[{"close":[0, 0]}]}}]}}`))
	}))
	defer server.Close()

	origHosts := yahooHosts
	yahooHosts = []string{strings.TrimPrefix(server.URL, "https://")}
	defer func() { yahooHosts = origHosts }()
	SetYahooSessionClient(server.Client())

	if _, err := NewAAPLProvider().FetchSnapshot(context.Background()); err == nil {
		t.Fatal("expected error when latest price is 0")
	}
}

func TestMSFTProvider_Name(t *testing.T) {
	if got := NewMSFTProvider().Name(); got != "us_msft" {
		t.Errorf("Name() = %q, want us_msft", got)
	}
}

func TestMSFTProvider_FetchSnapshot_Success(t *testing.T) {
	mockResponse := `{
		"chart": {
			"result": [
				{
					"meta": {"regularMarketTime": 1714500000, "regularMarketPrice": 445.0},
					"indicators": {"quote": [{"close": [410.0, 445.0]}]}
				}
			]
		}
	}`

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "MSFT") {
			t.Errorf("unexpected path: %s, expected MSFT", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(mockResponse))
	}))
	defer server.Close()

	origHosts := yahooHosts
	yahooHosts = []string{strings.TrimPrefix(server.URL, "https://")}
	defer func() { yahooHosts = origHosts }()
	SetYahooSessionClient(server.Client())

	snap, err := NewMSFTProvider().FetchSnapshot(context.Background())
	if err != nil {
		t.Fatalf("FetchSnapshot() error = %v", err)
	}
	if snap.MSFT.Symbol != "MSFT" {
		t.Errorf("Symbol = %q, want MSFT", snap.MSFT.Symbol)
	}
	if snap.MSFT.Value != 445.0 {
		t.Errorf("Value = %v, want 445.0", snap.MSFT.Value)
	}
	// (445-410)/410*100 = 8.536...
	expectedPct := (445.0 - 410.0) / 410.0 * 100
	if snap.MSFT.ChangePct != expectedPct {
		t.Errorf("ChangePct = %v, want %v", snap.MSFT.ChangePct, expectedPct)
	}
}

func TestMSFTProvider_FetchSnapshot_NoResult(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"chart":{"result":[]}}`))
	}))
	defer server.Close()

	origHosts := yahooHosts
	yahooHosts = []string{strings.TrimPrefix(server.URL, "https://")}
	defer func() { yahooHosts = origHosts }()
	SetYahooSessionClient(server.Client())

	if _, err := NewMSFTProvider().FetchSnapshot(context.Background()); err == nil {
		t.Fatal("expected error when no chart result")
	}
}