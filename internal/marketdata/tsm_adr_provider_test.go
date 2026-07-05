package marketdata

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TSM ADR provider fetches from Yahoo Finance with range=5d
// and computes daily change (closes[len(closes)-2] vs closes[len(closes)-1]).

func TestTSMADRProvider_Name(t *testing.T) {
	p := NewTSMADRProvider()
	if got := p.Name(); got != "tsm_adr" {
		t.Errorf("Name() = %q, want %q", got, "tsm_adr")
	}
}

func TestTSMADRProvider_FetchSnapshot_DailyChange(t *testing.T) {
	// Simulate 2 trading days: yesterday $175, today $180
	// Daily change: (180-175)/175*100 = 2.857...
	mockResponse := `{
		"chart": {
			"result": [
				{
					"meta": {"regularMarketTime": 1714500000, "regularMarketPrice": 180.0},
					"indicators": {"quote": [{"close": [175.0, 180.0]}]}
				}
			]
		}
	}`

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "TSM") {
			t.Errorf("unexpected path: %s, expected TSM", r.URL.Path)
		}
		if r.URL.Query().Get("range") != "5d" {
			t.Errorf("expected range=5d, got %s", r.URL.Query().Get("range"))
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

	snap, err := NewTSMADRProvider().FetchSnapshot(context.Background())
	if err != nil {
		t.Fatalf("FetchSnapshot() error = %v", err)
	}
	if snap.TSMADR.Symbol != "TSM" {
		t.Errorf("Symbol = %q, want TSM", snap.TSMADR.Symbol)
	}
	if snap.TSMADR.Value != 180.0 {
		t.Errorf("Value = %v, want 180.0", snap.TSMADR.Value)
	}
	// (180-175)/175*100 = 2.857..., rounded to 2 decimal places
	expectedPct := 2.86
	if snap.TSMADR.ChangePct != expectedPct {
		t.Errorf("ChangePct = %v, want %v (daily change, rounded to 2dp)", snap.TSMADR.ChangePct, expectedPct)
	}
	if snap.RecordedAt == 0 {
		t.Error("RecordedAt should be set")
	}
}

func TestTSMADRProvider_FetchSnapshot_NegativeDailyChange(t *testing.T) {
	// Yesterday $180, today $170 → (170-180)/180*100 = -5.555...
	mockResponse := `{
		"chart": {
			"result": [
				{
					"meta": {"regularMarketTime": 1714500000, "regularMarketPrice": 170.0},
					"indicators": {"quote": [{"close": [180.0, 170.0]}]}
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

	snap, err := NewTSMADRProvider().FetchSnapshot(context.Background())
	if err != nil {
		t.Fatalf("FetchSnapshot() error = %v", err)
	}
	// (170-180)/180*100 = -5.555..., rounded to 2 decimal places
	expectedPct := -5.56
	if snap.TSMADR.ChangePct != expectedPct {
		t.Errorf("ChangePct = %v, want %v (daily change, rounded to 2dp)", snap.TSMADR.ChangePct, expectedPct)
	}
}

func TestTSMADRProvider_FetchSnapshot_BoundsCheck_RejectsExtreme(t *testing.T) {
	// Simulate an implausible value: yesterday $50, today $100 → +100% change
	mockResponse := `{
		"chart": {
			"result": [
				{
					"meta": {"regularMarketTime": 1714500000, "regularMarketPrice": 100.0},
					"indicators": {"quote": [{"close": [50.0, 100.0]}]}
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

	_, err := NewTSMADRProvider().FetchSnapshot(context.Background())
	if err == nil {
		t.Fatal("expected error for implausible daily change > 30%")
	}
	if !strings.Contains(err.Error(), "implausible daily change") {
		t.Errorf("error should mention implausible daily change, got: %v", err)
	}
}

func TestTSMADRProvider_FetchSnapshot_BoundsCheck_ExtremeButWithinLimit(t *testing.T) {
	// Simulate a large but technically possible daily change: +29%
	// Yesterday $100, today $129 → +29% (within the 30% cap)
	mockResponse := `{
		"chart": {
			"result": [
				{
					"meta": {"regularMarketTime": 1714500000, "regularMarketPrice": 129.0},
					"indicators": {"quote": [{"close": [100.0, 129.0]}]}
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

	snap, err := NewTSMADRProvider().FetchSnapshot(context.Background())
	if err != nil {
		t.Fatalf("FetchSnapshot() unexpected error for 29%% daily change: %v", err)
	}
	// (129-100)/100*100 = 29.0, rounded to 2 decimal places
	expectedPct := 29.0
	if snap.TSMADR.ChangePct != expectedPct {
		t.Errorf("ChangePct = %v, want %v (29%% daily change, rounded to 2dp)", snap.TSMADR.ChangePct, expectedPct)
	}
}

func TestTSMADRProvider_FetchSnapshot_SingleClose(t *testing.T) {
	// Only one close price available → prev = latest → changePct = 0
	mockResponse := `{
		"chart": {
			"result": [
				{
					"meta": {"regularMarketTime": 1714500000, "regularMarketPrice": 180.0},
					"indicators": {"quote": [{"close": [180.0]}]}
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

	snap, err := NewTSMADRProvider().FetchSnapshot(context.Background())
	if err != nil {
		t.Fatalf("FetchSnapshot() error = %v", err)
	}
	if snap.TSMADR.ChangePct != 0.0 {
		t.Errorf("ChangePct = %v, want 0.0 (single close, no previous day)", snap.TSMADR.ChangePct)
	}
}

func TestTSMADRProvider_FetchSnapshot_NoResult(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"chart":{"result":[]}}`))
	}))
	defer server.Close()

	origHosts := yahooHosts
	yahooHosts = []string{strings.TrimPrefix(server.URL, "https://")}
	defer func() { yahooHosts = origHosts }()
	SetYahooSessionClient(server.Client())

	if _, err := NewTSMADRProvider().FetchSnapshot(context.Background()); err == nil {
		t.Fatal("expected error when no chart result")
	}
}

func TestTSMADRProvider_FetchSnapshot_NoClosePrices(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"chart":{"result":[{"indicators":{"quote":[{"close":[]}]}}]}}`))
	}))
	defer server.Close()

	origHosts := yahooHosts
	yahooHosts = []string{strings.TrimPrefix(server.URL, "https://")}
	defer func() { yahooHosts = origHosts }()
	SetYahooSessionClient(server.Client())

	if _, err := NewTSMADRProvider().FetchSnapshot(context.Background()); err == nil {
		t.Fatal("expected error when close prices empty")
	}
}

func TestTSMADRProvider_FetchSnapshot_ZeroLatestRejected(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"chart":{"result":[{"indicators":{"quote":[{"close":[0, 0]}]}}]}}`))
	}))
	defer server.Close()

	origHosts := yahooHosts
	yahooHosts = []string{strings.TrimPrefix(server.URL, "https://")}
	defer func() { yahooHosts = origHosts }()
	SetYahooSessionClient(server.Client())

	if _, err := NewTSMADRProvider().FetchSnapshot(context.Background()); err == nil {
		t.Fatal("expected error when latest price is 0")
	}
}

func TestTSMADRProvider_FetchSnapshot_ZeroPrevFallback(t *testing.T) {
	// Previous day close is zero → fallback to latest → changePct = 0
	mockResponse := `{
		"chart": {
			"result": [
				{
					"meta": {"regularMarketTime": 1714500000, "regularMarketPrice": 180.0},
					"indicators": {"quote": [{"close": [0.0, 180.0]}]}
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

	snap, err := NewTSMADRProvider().FetchSnapshot(context.Background())
	if err != nil {
		t.Fatalf("FetchSnapshot() error = %v", err)
	}
	if snap.TSMADR.ChangePct != 0.0 {
		t.Errorf("ChangePct = %v, want 0.0 (zero prev falls back to latest)", snap.TSMADR.ChangePct)
	}
}
