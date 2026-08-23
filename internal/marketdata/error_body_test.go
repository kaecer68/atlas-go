package marketdata

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ─── P1-10: bounded upstream error-body capture ─────────────────────────────
//
// TWSE / Fubon / export / BDI previously surfaced only the HTTP status code
// in LastError; a 500 with a meaningful JSON body ("rate limited", "invalid
// date", WAF page) was invisible. All four now capture a bounded (512B) body
// snippet, mirroring the FinMind pattern.

func TestTWSE_ErrorBodyCaptured(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"stat":"fail","message":"upstream schema changed"}`))
	}))
	defer ts.Close()

	ResetSharedTWSEClient()
	c := GetSharedTWSEClient()
	c.baseURL = ts.URL
	c.SetHTTPClient(ts.Client())
	c.rateLimiter = newUnlimitedLimiter()

	_, err := c.GetQuotes(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), `"upstream schema changed"`) {
		t.Errorf("error body not captured in TWSE error: %v", err)
	}
	if !strings.Contains(err.Error(), "502") {
		t.Errorf("status code missing from TWSE error: %v", err)
	}
}

func TestBDI_ErrorBodyCaptured(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte("rate limit exceeded by CNBC"))
	}))
	defer ts.Close()

	old := SetBDILimiterForTest(newUnlimitedLimiter())
	defer SetBDILimiterForTest(old)

	p := NewBDIProvider()
	p.endpoint = ts.URL
	p.client = ts.Client()

	_, err := p.FetchSnapshot(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "rate limit exceeded by CNBC") {
		t.Errorf("error body not captured in BDI error: %v", err)
	}
}

func TestExport_ErrorBodyCaptured(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("customs portal maintenance window"))
	}))
	defer ts.Close()

	p := NewExportStatisticsProvider(t.TempDir())
	p.client = ts.Client()
	p.limiter = newUnlimitedLimiter()
	p.baseURL = ts.URL

	_, err := p.FetchSnapshot(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "customs portal maintenance window") {
		t.Errorf("error body not captured in export error: %v", err)
	}
}

func TestFubon_ErrorBodyCaptured(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"error":"upstream quote provider unreachable"}`))
	}))
	defer ts.Close()

	c := NewFubonClient()
	c.proxyURL = ts.URL
	c.SetHTTPClient(ts.Client())
	c.intradayLimiter = newUnlimitedLimiter()

	_, err := c.GetQuote(context.Background(), "2330")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), `"upstream quote provider unreachable"`) {
		t.Errorf("error body not captured in Fubon error: %v", err)
	}
	// Bounded: a large body must be truncated to ~512 bytes, never read fully.
	big := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(strings.Repeat("x", 100_000)))
	}))
	defer big.Close()
	c2 := NewFubonClient()
	c2.proxyURL = big.URL
	c2.SetHTTPClient(big.Client())
	c2.intradayLimiter = newUnlimitedLimiter()
	_, err = c2.GetQuote(context.Background(), "2330")
	if err == nil {
		t.Fatal("expected error")
	}
	if len(err.Error()) > 2_000 {
		t.Errorf("error body not bounded: len=%d", len(err.Error()))
	}
}

func TestFinMind_ErrorBody_Includes402Quota(t *testing.T) {
	// Regression guard for the P0-1 typed 402 path (body must reach LastError).
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusPaymentRequired)
		_, _ = w.Write([]byte(`{"msg":"Requests reach the upper limit"}`))
	}))
	defer ts.Close()

	c := NewFinMindClient("k")
	c.httpClient = &http.Client{Transport: &rewriteTransport{target: ts.URL, inner: http.DefaultTransport}}
	c.rateLimiter = newUnlimitedLimiter()
	c.retryCfg = retryConfig{maxAttempts: 1}

	_, err := c.fetchDataset(context.Background(), "TaiwanStockPrice", "2330", "2026-01-01", "2026-01-01")
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrQuotaExhausted) {
		t.Errorf("expected ErrQuotaExhausted, got %v", err)
	}
	if !strings.Contains(err.Error(), "upper limit") {
		t.Errorf("402 body not captured: %v", err)
	}
}
