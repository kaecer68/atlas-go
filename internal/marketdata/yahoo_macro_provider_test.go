package marketdata

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestYahooFinanceMacroProvider_UserAgent(t *testing.T) {
	var capturedUA string
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedUA = r.Header.Get("User-Agent")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"chart":{"result":[{"meta":{"regularMarketTime":1700000000,"regularMarketPrice":150.0},"indicators":{"quote":[{"close":[149.0,150.0]}]}}]}}`))
	}))
	defer ts.Close()

	origHosts := yahooHosts
	yahooHosts = []string{strings.TrimPrefix(ts.URL, "https://")}
	defer func() { yahooHosts = origHosts }()

	provider := NewYahooFinanceMacroProvider()
	provider.client = ts.Client()

	_, err := provider.FetchSnapshot(context.Background())
	if err != nil {
		t.Fatalf("FetchSnapshot failed: %v", err)
	}

	if capturedUA == "" {
		t.Fatal("User-Agent header was not sent")
	}
	found := false
	for _, ua := range modernUserAgents {
		if capturedUA == ua {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("User-Agent %q not in modern list", capturedUA)
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

	provider := NewYahooFinanceMacroProvider()
	provider.client = tsHTML.Client()

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

	provider := NewYahooFinanceMacroProvider()
	provider.client = ts.Client()

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

	provider := NewYahooFinanceMacroProvider()
	provider.client = ts.Client()

	host := strings.TrimPrefix(ts.URL, "https://")
	point, err := provider.fetchFromHost(context.Background(), host, "TEST")
	if err != nil {
		t.Fatalf("fetchFromHost failed: %v", err)
	}
	if point.Value != 150.0 {
		t.Errorf("expected value 150.0, got %v", point.Value)
	}
	if point.ChangePct < 0.67 || point.ChangePct > 0.68 {
		t.Errorf("expected pct change ~0.671, got %v", point.ChangePct)
	}
}

func TestYahooFetchFromHost_HTMLResponse(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`<html><body>Rate Limited</body></html>`))
	}))
	defer ts.Close()

	provider := NewYahooFinanceMacroProvider()
	provider.client = ts.Client()

	host := strings.TrimPrefix(ts.URL, "https://")
	_, err := provider.fetchFromHost(context.Background(), host, "TEST")
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

	provider := NewYahooFinanceMacroProvider()
	provider.client = ts.Client()

	host := strings.TrimPrefix(ts.URL, "https://")
	_, err := provider.fetchFromHost(context.Background(), host, "TEST")
	if err == nil {
		t.Fatal("expected error for non-OK status")
	}
	if !strings.Contains(err.Error(), "http status 429") {
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

	provider := NewYahooFinanceMacroProvider()
	provider.client = ts.Client()

	host := strings.TrimPrefix(ts.URL, "https://")
	_, err := provider.fetchFromHost(context.Background(), host, "TEST")
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
	if !strings.Contains(err.Error(), "unmarshal") {
		t.Errorf("expected unmarshal error, got: %v", err)
	}
}

func TestYahooBDISymbol(t *testing.T) {
	p := NewYahooFinanceMacroProvider()

	if !strings.Contains(p.bdiSymbol, "BDI") && !strings.Contains(p.bdiSymbol, "BALTT") {
		t.Errorf("expected bdiSymbol to contain BDI or BALTT, got %s", p.bdiSymbol)
	}

	// Override
	p.SetBDISymbol("BALTT")
	if p.bdiSymbol != "BALTT" {
		t.Errorf("expected bdiSymbol to be BALTT after SetBDISymbol, got %s", p.bdiSymbol)
	}
}
