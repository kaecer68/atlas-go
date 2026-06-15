package marketdata

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// ─── DRAMSpotPriceProvider ───────────────────────────────────────────────────

// DRAMSpotPriceProvider uses Micron Technology (MU) as a ~85% correlated proxy
// for DRAM spot prices (DRAMeXchange/InSpectrum). Reference: 2026-04-29 close
// around $87.50 USD, prior close $86.10, implying +1.62% day-over-day change.

func TestDRAMSpotPriceProvider_Name(t *testing.T) {
	p := NewDRAMSpotPriceProvider()
	if got := p.Name(); got != "dram_spot_price" {
		t.Errorf("Name() = %q, want %q", got, "dram_spot_price")
	}
}

func TestDRAMSpotPriceProvider_FetchSnapshot_Success(t *testing.T) {
	mockResponse := `{
		"chart": {
			"result": [
				{
					"meta": {"regularMarketTime": 1714500000},
					"indicators": {
						"quote": [
							{
								"close": [86.10, 87.50]
							}
						]
					}
				}
			]
		}
	}`

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "MU") {
			t.Errorf("unexpected path: %s, expected MU symbol", r.URL.Path)
		}
		if r.URL.Query().Get("interval") != "1d" {
			t.Errorf("expected interval=1d, got %s", r.URL.Query().Get("interval"))
		}
		if r.URL.Query().Get("range") != "2d" {
			t.Errorf("expected range=2d, got %s", r.URL.Query().Get("range"))
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
	snap, err := NewDRAMSpotPriceProvider().FetchSnapshot(ctx)
	if err != nil {
		t.Fatalf("FetchSnapshot() error = %v", err)
	}

	if snap.DRAMSpotPrice.Symbol != "MU" {
		t.Errorf("Symbol = %q, want MU", snap.DRAMSpotPrice.Symbol)
	}
	if snap.DRAMSpotPrice.Value != 87.50 {
		t.Errorf("Value = %v, want 87.50", snap.DRAMSpotPrice.Value)
	}
	// (87.50 - 86.10) / 86.10 * 100 = 1.6254... → math.Round(162.54)/100 = 1.63
	expectedPct := 1.63
	if snap.DRAMSpotPrice.ChangePct != expectedPct {
		t.Errorf("ChangePct = %v, want %v", snap.DRAMSpotPrice.ChangePct, expectedPct)
	}
	if snap.DRAMSpotPrice.Timestamp == 0 {
		t.Error("Timestamp should be populated")
	}
	if time.Since(time.Unix(snap.DRAMSpotPrice.Timestamp, 0)) > 5*time.Second {
		t.Error("Timestamp should be near now")
	}
}

func TestDRAMSpotPriceProvider_FetchSnapshot_NoChartResult(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		// Empty chart result list
		w.Write([]byte(`{"chart":{"result":[]}}`))
	}))
	defer server.Close()

	origHosts := yahooHosts
	yahooHosts = []string{strings.TrimPrefix(server.URL, "https://")}
	defer func() { yahooHosts = origHosts }()
	SetYahooSessionClient(server.Client())

	_, err := NewDRAMSpotPriceProvider().FetchSnapshot(context.Background())
	if err == nil {
		t.Fatal("expected error when chart result is empty")
	}
	if !strings.Contains(err.Error(), "no chart result") {
		t.Errorf("error %q should mention 'no chart result'", err.Error())
	}
}

func TestDRAMSpotPriceProvider_FetchSnapshot_NoClosePrices(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"chart":{"result":[{"indicators":{"quote":[{"close":[]}]}}]}}`))
	}))
	defer server.Close()

	origHosts := yahooHosts
	yahooHosts = []string{strings.TrimPrefix(server.URL, "https://")}
	defer func() { yahooHosts = origHosts }()
	SetYahooSessionClient(server.Client())

	_, err := NewDRAMSpotPriceProvider().FetchSnapshot(context.Background())
	if err == nil {
		t.Fatal("expected error when close prices are empty")
	}
}

func TestDRAMSpotPriceProvider_FetchSnapshot_InvalidLatestPrice(t *testing.T) {
	// NaN isn't representable in JSON, but 0 is. Latest = 0 must be rejected.
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"chart":{"result":[{"indicators":{"quote":[{"close":[0, 0]}]}}]}}`))
	}))
	defer server.Close()

	origHosts := yahooHosts
	yahooHosts = []string{strings.TrimPrefix(server.URL, "https://")}
	defer func() { yahooHosts = origHosts }()
	SetYahooSessionClient(server.Client())

	_, err := NewDRAMSpotPriceProvider().FetchSnapshot(context.Background())
	if err == nil {
		t.Fatal("expected error when latest price is 0")
	}
}

func TestDRAMSpotPriceProvider_FetchSnapshot_OnlyLatest(t *testing.T) {
	// Single close value → prev fallback to latest → changePct = 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"chart":{"result":[{"indicators":{"quote":[{"close":[95.0]}]}}]}}`))
	}))
	defer server.Close()

	origHosts := yahooHosts
	yahooHosts = []string{strings.TrimPrefix(server.URL, "https://")}
	defer func() { yahooHosts = origHosts }()
	SetYahooSessionClient(server.Client())

	snap, err := NewDRAMSpotPriceProvider().FetchSnapshot(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if snap.DRAMSpotPrice.Value != 95.0 {
		t.Errorf("Value = %v, want 95.0", snap.DRAMSpotPrice.Value)
	}
	if snap.DRAMSpotPrice.ChangePct != 0.0 {
		t.Errorf("ChangePct = %v, want 0.0 (no prior data)", snap.DRAMSpotPrice.ChangePct)
	}
}
