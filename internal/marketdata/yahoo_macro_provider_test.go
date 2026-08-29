package marketdata

import (
	"context"
	"math"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
)

func TestYahooFinanceMacroProvider_UserAgent(t *testing.T) {
	var capturedUA atomic.Value
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedUA.Store(r.Header.Get("User-Agent"))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"chart":{"result":[{"meta":{"regularMarketTime":1700000000,"regularMarketPrice":150.0},"indicators":{"quote":[{"close":[149.0,150.0]}]}}]}}`))
	}))
	defer ts.Close()

	origHosts := yahooHosts
	yahooHosts = []string{strings.TrimPrefix(ts.URL, "https://")}
	defer func() { yahooHosts = origHosts }()
	SetYahooSessionClient(ts.Client())

	provider := NewYahooFinanceMacroProvider()
	_, err := provider.FetchSnapshot(context.Background())
	if err != nil {
		t.Fatalf("FetchSnapshot failed: %v", err)
	}

	captured := capturedUA.Load().(string)
	if captured == "" {
		t.Fatal("User-Agent header was not sent")
	}
	found := slices.Contains(modernUserAgents, captured)
	if !found {
		t.Errorf("User-Agent %q not in modern list", captured)
	}
}

func TestYahooFinanceMacroProvider_HTMLFallback(t *testing.T) {
	tsHTML := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`<html><body>Error</body></html>`))
	}))
	defer tsHTML.Close()

	tsJSON := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"chart":{"result":[{"meta":{"regularMarketTime":1700000000,"regularMarketPrice":150.0},"indicators":{"quote":[{"close":[149.0,150.0]}]}}]}}`))
	}))
	defer tsJSON.Close()

	hostHTML := strings.TrimPrefix(tsHTML.URL, "https://")
	hostJSON := strings.TrimPrefix(tsJSON.URL, "https://")

	origHosts := yahooHosts
	yahooHosts = []string{hostHTML, hostJSON}
	defer func() { yahooHosts = origHosts }()
	// Use the JSON server's client (both are TLS, same client works)
	SetYahooSessionClient(tsJSON.Client())

	provider := NewYahooFinanceMacroProvider()
	snap, err := provider.FetchSnapshot(context.Background())
	if err != nil {
		t.Logf("FetchSnapshot returned errors (expected partial): %v", err)
	}

	if snap.DXY.Value == 0 {
		t.Error("expected DXY to be populated after host fallback, got zero")
	}
	resetYahooSessionState() // P1-14: HTML response tripped the negative cache; don't pollute later tests
}

func TestYahooFinanceMacroProvider_AllHostsFail(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer ts.Close()

	host := strings.TrimPrefix(ts.URL, "https://")
	origHosts := yahooHosts
	yahooHosts = []string{host, host}
	defer func() { yahooHosts = origHosts }()
	SetYahooSessionClient(ts.Client())

	provider := NewYahooFinanceMacroProvider()
	_, err := provider.FetchSnapshot(context.Background())
	if err == nil {
		t.Fatal("expected error when all hosts fail")
	}
	resetYahooSessionState() // P1-14: 429 tripped the negative cache; don't pollute later tests
}

func TestYahooFetchFromHost_Success(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"chart":{"result":[{"meta":{"regularMarketTime":1700000000,"regularMarketPrice":150.0},"indicators":{"quote":[{"close":[149.0,150.0]}]}}]}}`))
	}))
	defer ts.Close()

	origHosts := yahooHosts
	yahooHosts = []string{strings.TrimPrefix(ts.URL, "https://")}
	defer func() { yahooHosts = origHosts }()
	SetYahooSessionClient(ts.Client())

	// Test session.fetchFromHost directly
	ctx := context.Background()
	s := getYahooSession()
	body, err := s.fetchFromHost(ctx, yahooHosts[0], "TEST", map[string]string{"interval": "1d", "range": "2d"})
	if err != nil {
		t.Fatalf("fetchFromHost failed: %v", err)
	}

	chartResp, err := UnmarshalYahooChart(body)
	if err != nil {
		t.Fatalf("UnmarshalYahooChart failed: %v", err)
	}
	if chartResp.Chart.Result[0].Meta.RegularMarketPrice != 150.0 {
		t.Errorf("expected price 150.0, got %v", chartResp.Chart.Result[0].Meta.RegularMarketPrice)
	}
}

func TestYahooFetchFromHost_HTMLResponse(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`<html><body>Rate Limited</body></html>`))
	}))
	defer ts.Close()

	origHosts := yahooHosts
	yahooHosts = []string{strings.TrimPrefix(ts.URL, "https://")}
	defer func() { yahooHosts = origHosts }()
	SetYahooSessionClient(ts.Client())

	ctx := context.Background()
	s := getYahooSession()
	_, err := s.fetchFromHost(ctx, yahooHosts[0], "TEST", nil)
	if err == nil {
		t.Fatal("expected error for HTML response")
	}
	if !strings.Contains(err.Error(), "HTML response") {
		t.Errorf("expected HTML error, got: %v", err)
	}
}

func TestYahooFetchFromHost_NonOKStatus(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer ts.Close()

	origHosts := yahooHosts
	yahooHosts = []string{strings.TrimPrefix(ts.URL, "https://")}
	defer func() { yahooHosts = origHosts }()
	SetYahooSessionClient(ts.Client())

	ctx := context.Background()
	s := getYahooSession()
	_, err := s.fetchFromHost(ctx, yahooHosts[0], "TEST", nil)
	if err == nil {
		t.Fatal("expected error for non-OK status")
	}
	if !strings.Contains(err.Error(), "http 429") {
		t.Errorf("expected status error, got: %v", err)
	}
}

func TestYahooFetchFromHost_InvalidJSON(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{not json}`))
	}))
	defer ts.Close()

	origHosts := yahooHosts
	yahooHosts = []string{strings.TrimPrefix(ts.URL, "https://")}
	defer func() { yahooHosts = origHosts }()
	SetYahooSessionClient(ts.Client())

	ctx := context.Background()
	s := getYahooSession()
	body, err := s.fetchFromHost(ctx, yahooHosts[0], "TEST", nil)
	if err != nil {
		t.Fatalf("fetchFromHost should succeed on HTTP level: %v", err)
	}

	_, err = UnmarshalYahooChart(body)
	if err == nil {
		t.Fatal("expected unmarshal error for invalid JSON")
	}
	if !strings.Contains(err.Error(), "unmarshal") {
		t.Errorf("expected unmarshal error, got: %v", err)
	}
}

func TestFindLastValidClose_AllValid(t *testing.T) {
	closes := []float64{100.0, 101.0, 102.5}
	latest, prev := findLastValidClose(closes)
	if latest != 102.5 {
		t.Errorf("latest = %v, want 102.5", latest)
	}
	if prev != 101.0 {
		t.Errorf("prev = %v, want 101.0", prev)
	}
}

func TestFindLastValidClose_TrailingZeros(t *testing.T) {
	// Simulates Yahoo off-hours: [valid, ..., valid, 0.0, 0.0]
	closes := []float64{100.0, 101.0, 0.0, 0.0}
	latest, prev := findLastValidClose(closes)
	if latest != 101.0 {
		t.Errorf("latest = %v, want 101.0", latest)
	}
	if prev != 100.0 {
		t.Errorf("prev = %v, want 100.0", prev)
	}
}

func TestFindLastValidClose_AllZeros(t *testing.T) {
	closes := []float64{0.0, 0.0, 0.0}
	latest, prev := findLastValidClose(closes)
	if latest != 0 || prev != 0 {
		t.Errorf("latest=%v, prev=%v, want 0,0", latest, prev)
	}
}

func TestFindLastValidClose_SingleValid(t *testing.T) {
	// Only one non-zero value — prev should be 0 (no second valid close)
	closes := []float64{0.0, 0.0, 100.0}
	latest, prev := findLastValidClose(closes)
	if latest != 100.0 {
		t.Errorf("latest = %v, want 100.0", latest)
	}
	if prev != 0 {
		t.Errorf("prev = %v, want 0 (only one valid close)", prev)
	}
}

func TestFindLastValidClose_MidZeros(t *testing.T) {
	// Zeros in the middle, valid on both ends
	closes := []float64{50.0, 0.0, 0.0, 51.0, 0.0, 52.0}
	latest, prev := findLastValidClose(closes)
	if latest != 52.0 {
		t.Errorf("latest = %v, want 52.0", latest)
	}
	if prev != 51.0 {
		t.Errorf("prev = %v, want 51.0", prev)
	}
}

func TestFindLastValidClose_NaNSkip(t *testing.T) {
	// NaN between valid closes (parser glitch / off-hours) must be skipped
	// just like zeros — prev resolves to the nearest earlier valid value.
	closes := []float64{98.5, math.NaN(), 99.43}
	latest, prev := findLastValidClose(closes)
	if latest != 99.43 {
		t.Errorf("latest = %v, want 99.43", latest)
	}
	if prev != 98.5 {
		t.Errorf("prev = %v, want 98.5 (NaN must be skipped)", prev)
	}
}
