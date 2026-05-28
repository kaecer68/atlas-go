package marketdata

import (
	"context"
	"net/http"
	"net/http/httptest"
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
	found := false
	for _, ua := range modernUserAgents {
		if captured == ua {
			found = true
			break
		}
	}
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
