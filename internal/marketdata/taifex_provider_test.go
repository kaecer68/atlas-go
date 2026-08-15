package marketdata

import (
	"bytes"
	"compress/gzip"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"golang.org/x/time/rate"
)

// TestTAIFEXFetchPCR_UnitConversion verifies that TAIFEX PutCallRatio API
// percentages (e.g. "110.43" = 110.43%) are converted to ratios (1.1043)
// before being stored in PCRStats. Audit A13 (2026-08-12): the raw API field
// name carries a "%" suffix; without /100 the retail subA5 threshold mapping
// (1.5/1.0/0.8) always matched the top band, pinning weekly_pcr to 0.9.
func TestTAIFEXFetchPCR_UnitConversion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/PutCallRatio" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{"Date":"20260811","PutVolume":"100","CallVolume":"200","PutCallVolumeRatio%":"110.43","PutOI":"50","CallOI":"40","PutCallOIRatio%":"123.58"}
		]`))
	}))
	defer server.Close()

	p := NewTAIFEXProvider()
	p.baseURL = server.URL
	p.SetHTTPClient(server.Client())
	p.rateLimiter = rate.NewLimiter(rate.Every(time.Second), 1)

	stats, err := p.FetchPCR(context.Background())
	if err != nil {
		t.Fatalf("FetchPCR error: %v", err)
	}
	if got, want := stats.PutCallVolumeRatio, 1.1043; got != want {
		t.Errorf("PutCallVolumeRatio = %v, want %v (110.43%% → 1.1043)", got, want)
	}
	if got, want := stats.PutCallOIRatio, 1.2358; got != want {
		t.Errorf("PutCallOIRatio = %v, want %v (123.58%% → 1.2358)", got, want)
	}
}

// TestTAIFEXFetchPCR_GzipResponse verifies that when the upstream returns a
// gzip-encoded body (as requested by Accept-Encoding: gzip), the provider
// transparently decompresses it before JSON parsing.
func TestTAIFEXFetchPCR_GzipResponse(t *testing.T) {
	payload := `[
		{"Date":"20260811","PutVolume":"100","CallVolume":"200","PutCallVolumeRatio%":"110.43","PutOI":"50","CallOI":"40","PutCallOIRatio%":"123.58"}
	]`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Accept-Encoding"); got != "gzip" {
			t.Errorf("Accept-Encoding = %q, want gzip", got)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Encoding", "gzip")
		var buf bytes.Buffer
		gz := gzip.NewWriter(&buf)
		_, _ = gz.Write([]byte(payload))
		_ = gz.Close()
		_, _ = w.Write(buf.Bytes())
	}))
	defer server.Close()

	p := NewTAIFEXProvider()
	p.baseURL = server.URL
	p.SetHTTPClient(server.Client())
	p.rateLimiter = rate.NewLimiter(rate.Every(time.Second), 1)

	stats, err := p.FetchPCR(context.Background())
	if err != nil {
		t.Fatalf("FetchPCR error: %v", err)
	}
	if got, want := stats.PutCallVolumeRatio, 1.1043; got != want {
		t.Errorf("PutCallVolumeRatio = %v, want %v (gzip decompressed)", got, want)
	}
}

// TestTAIFEXProvider_ClientTimeout verifies the upstream timeout was raised to
// 30s (openapi.taifex.com.tw can exceed the old 20s budget under load).
func TestTAIFEXProvider_ClientTimeout(t *testing.T) {
	p := NewTAIFEXProvider()
	if p.client == nil {
		t.Fatal("NewTAIFEXProvider client is nil")
	}
	if got, want := p.client.Timeout, 30*time.Second; got != want {
		t.Errorf("client.Timeout = %v, want %v", got, want)
	}
}

// TestTAIFEXFetchPCR_ZeroRatio ensures a "0.00" percentage maps to 0 ratio
// (not NaN or fallback-triggering large value).
func TestTAIFEXFetchPCR_ZeroRatio(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{"Date":"20260811","PutVolume":"0","CallVolume":"0","PutCallVolumeRatio%":"0.00","PutOI":"0","CallOI":"0","PutCallOIRatio%":"0.00"}
		]`))
	}))
	defer server.Close()

	p := NewTAIFEXProvider()
	p.baseURL = server.URL
	p.SetHTTPClient(server.Client())
	p.rateLimiter = rate.NewLimiter(rate.Every(time.Second), 1)

	stats, err := p.FetchPCR(context.Background())
	if err != nil {
		t.Fatalf("FetchPCR error: %v", err)
	}
	if stats.PutCallVolumeRatio != 0 {
		t.Errorf("PutCallVolumeRatio = %v, want 0", stats.PutCallVolumeRatio)
	}
}
