package marketdata

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/time/rate"
)

// ---------------------------------------------------------------------------
// P0-5: shared fetchWithRetry — retry only 429/5xx, honor Retry-After,
// exponential backoff, and stop at maxAttempts.
// ---------------------------------------------------------------------------

// newRetryTestServer returns a server whose responses come from a queue.
// Each entry: status code + optional body + optional Retry-After header.
func newRetryTestServer(t *testing.T, responses []struct {
	status     int
	body       string
	retryAfter string
}) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := int(calls.Add(1)) - 1
		if n >= len(responses) {
			n = len(responses) - 1
		}
		resp := responses[n]
		if resp.retryAfter != "" {
			w.Header().Set("Retry-After", resp.retryAfter)
		}
		if resp.body != "" {
			w.Header().Set("Content-Type", "application/json")
		}
		w.WriteHeader(resp.status)
		if resp.body != "" {
			_, _ = w.Write([]byte(resp.body))
		}
	}))
	return srv, &calls
}

func TestFetchWithRetry_Retries5xxThenSucceeds(t *testing.T) {
	srv, calls := newRetryTestServer(t, []struct {
		status     int
		body       string
		retryAfter string
	}{
		{status: http.StatusServiceUnavailable, body: "boom"},
		{status: http.StatusServiceUnavailable, body: "boom"},
		{status: http.StatusOK, body: `{"ok":true}`},
	})
	defer srv.Close()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	cfg := retryConfig{maxAttempts: 3, baseBackoff: time.Millisecond}
	resp, err := fetchWithRetry(context.Background(), srv.Client(), req, cfg)
	if err != nil {
		t.Fatalf("fetchWithRetry after 2x503 then 200 = error %v, want success", err)
	}
	defer resp.Body.Close()
	if got := calls.Load(); got != 3 {
		t.Errorf("HTTP attempts = %d, want 3 (2 retries)", got)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "ok") {
		t.Errorf("body = %q, want the 200 payload", body)
	}
}

func TestFetchWithRetry_HonorsRetryAfter(t *testing.T) {
	// Retry-After: 1 overrides the (huge) exponential backoff — the retry
	// must happen ~1s later, not 1h later.
	srv, calls := newRetryTestServer(t, []struct {
		status     int
		body       string
		retryAfter string
	}{
		{status: http.StatusTooManyRequests, body: `{"statusCode":429}`, retryAfter: "1"},
		{status: http.StatusOK, body: `{"ok":true}`},
	})
	defer srv.Close()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	// baseBackoff of 1h proves the test only passes if Retry-After is used.
	cfg := retryConfig{maxAttempts: 3, baseBackoff: time.Hour}
	start := time.Now()
	resp, err := fetchWithRetry(context.Background(), srv.Client(), req, cfg)
	if err != nil {
		t.Fatalf("fetchWithRetry = error %v, want success after Retry-After", err)
	}
	defer resp.Body.Close()
	elapsed := time.Since(start)
	if elapsed > 10*time.Second {
		t.Errorf("retry waited %v, want ~1s (Retry-After must override exponential backoff)", elapsed)
	}
	if got := calls.Load(); got != 2 {
		t.Errorf("HTTP attempts = %d, want 2 (429 + 1 retry)", got)
	}
}

func TestFetchWithRetry_ExhaustsAttempts(t *testing.T) {
	srv, calls := newRetryTestServer(t, []struct {
		status     int
		body       string
		retryAfter string
	}{
		{status: http.StatusInternalServerError, body: "err"},
		{status: http.StatusInternalServerError, body: "err"},
		{status: http.StatusInternalServerError, body: "err"},
	})
	defer srv.Close()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	cfg := retryConfig{maxAttempts: 3, baseBackoff: time.Millisecond}
	_, err = fetchWithRetry(context.Background(), srv.Client(), req, cfg)
	if err == nil {
		t.Fatal("fetchWithRetry = nil error, want error after exhausting attempts")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("error %q should include the final status", err.Error())
	}
	if got := calls.Load(); got != 3 {
		t.Errorf("HTTP attempts = %d, want 3 (exactly maxAttempts)", got)
	}
}

func TestFetchWithRetry_DoesNotRetry4xx(t *testing.T) {
	srv, calls := newRetryTestServer(t, []struct {
		status     int
		body       string
		retryAfter string
	}{
		{status: http.StatusBadRequest, body: `{"msg":"bad"}`},
	})
	defer srv.Close()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	cfg := retryConfig{maxAttempts: 3, baseBackoff: time.Millisecond}
	resp, err := fetchWithRetry(context.Background(), srv.Client(), req, cfg)
	if err != nil {
		t.Fatalf("fetchWithRetry on 400 = error %v, want the response as-is", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 (returned untouched for non-transient 4xx)", resp.StatusCode)
	}
	if got := calls.Load(); got != 1 {
		t.Errorf("HTTP attempts = %d, want 1 (4xx must not be retried)", got)
	}
}

func TestFetchWithRetry_TransportErrorReturnsImmediately(t *testing.T) {
	// Closed server → connection refused on first attempt; no retry.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := srv.URL
	srv.Close()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	cfg := retryConfig{maxAttempts: 3, baseBackoff: time.Millisecond}
	_, err = fetchWithRetry(context.Background(), srv.Client(), req, cfg)
	if err == nil {
		t.Fatal("fetchWithRetry = nil error, want transport error")
	}
}

// TestDefaultRetryConfig_ParameterSystem pins the P0-5 contract: the shared
// helper consumes marketdata.max_retry_attempts / retry_backoff_ms (defaults
// 3 attempts / 1000ms base backoff), which previously existed only in the
// parameter system with zero production consumers.
func TestDefaultRetryConfig_ParameterSystem(t *testing.T) {
	cfg := defaultRetryConfig()
	if cfg.maxAttempts < 1 {
		t.Errorf("maxAttempts = %d, want >= 1", cfg.maxAttempts)
	}
	if cfg.baseBackoff <= 0 {
		t.Errorf("baseBackoff = %v, want > 0", cfg.baseBackoff)
	}
	if cfg.maxAttempts != 3 {
		t.Errorf("maxAttempts = %d, want 3 (marketdata.max_retry_attempts default)", cfg.maxAttempts)
	}
	if cfg.baseBackoff != time.Second {
		t.Errorf("baseBackoff = %v, want 1s (marketdata.retry_backoff_ms default)", cfg.baseBackoff)
	}
}

// ---------------------------------------------------------------------------
// P0-5 integration: the shared helper wired into FinMind / TAIFEX / Fugle.
// ---------------------------------------------------------------------------

// TestFinMindClient_fetchDataset_Retries429ThenSucceeds verifies the FinMind
// main data path now retries a 429 and succeeds (previously no retry).
func TestFinMindClient_fetchDataset_Retries429ThenSucceeds(t *testing.T) {
	var calls atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"msg":"Requests reach the upper limit","status":429}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"msg":"success","status":200,"data":[{"revenue":289420000000.0}]}`))
	}))
	defer ts.Close()

	c := NewFinMindClient("test-key")
	c.retryCfg = retryConfig{maxAttempts: 3, baseBackoff: 2 * time.Millisecond}
	c.httpClient = &http.Client{
		Transport: &rewriteTransport{target: ts.URL, inner: http.DefaultTransport},
	}

	data, err := c.fetchDataset(context.Background(), "TaiwanStockMonthRevenue", "2330", "2026-04-01", "2026-04-30")
	if err != nil {
		t.Fatalf("fetchDataset after 429 retry: %v", err)
	}
	if got := calls.Load(); got != 2 {
		t.Errorf("HTTP attempts = %d, want 2 (429 + 1 retry)", got)
	}
	if len(data) != 1 {
		t.Fatalf("data len = %d, want 1", len(data))
	}
}

// TestTAIFEXFetchPCR_Retries503ThenSucceeds verifies the TAIFEX PutCallRatio
// endpoint retries a 503 and succeeds (previously the whole cycle failed).
func TestTAIFEXFetchPCR_Retries503ThenSucceeds(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/PutCallRatio" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if calls.Add(1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("upstream busy"))
			return
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
	p.rateLimiter = rate.NewLimiter(rate.Inf, 1)
	p.retryCfg = retryConfig{maxAttempts: 3, baseBackoff: 2 * time.Millisecond}

	stats, err := p.FetchPCR(context.Background())
	if err != nil {
		t.Fatalf("FetchPCR after 503 retry: %v", err)
	}
	if got := calls.Load(); got != 2 {
		t.Errorf("HTTP attempts = %d, want 2 (503 + 1 retry)", got)
	}
	if stats.PutCallVolumeRatio != 1.1043 {
		t.Errorf("PutCallVolumeRatio = %v, want 1.1043", stats.PutCallVolumeRatio)
	}
}

// TestFugleClient_GetHistoricalCandles_Retries429ThenSucceeds verifies the
// Fugle candles path (GetHistoricalCandles) retries a 429 honoring
// Retry-After — previously a single 429 failed the fetch outright.
func TestFugleClient_GetHistoricalCandles_Retries429ThenSucceeds(t *testing.T) {
	var calls atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"statusCode":429}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[
			{"date":"2026-07-07","open":100,"high":105,"low":99,"close":103,"volume":1000},
			{"date":"2026-07-06","open":98,"high":102,"low":97,"close":101,"volume":900}
		]}`))
	}))
	defer ts.Close()

	c := NewFugleClient("test-key")
	// GetHistoricalCandles hardcodes the api.fugle.tw host in its URL, so
	// the mock must rewrite that host to the test server.
	c.httpClient = &http.Client{
		Transport: &fugleHostRewriteTransport{target: ts.URL},
	}
	c.rateLimiter = rate.NewLimiter(rate.Inf, 1)
	c.retryCfg = retryConfig{maxAttempts: 3, baseBackoff: 2 * time.Millisecond}

	bars, err := c.GetHistoricalCandles(context.Background(), "2330", "2026-07-01", "2026-07-07")
	if err != nil {
		t.Fatalf("GetHistoricalCandles after 429 retry: %v", err)
	}
	if got := calls.Load(); got != 2 {
		t.Errorf("HTTP attempts = %d, want 2 (429 + 1 retry)", got)
	}
	if len(bars) != 2 {
		t.Fatalf("bars len = %d, want 2", len(bars))
	}
	if bars[0].Symbol != "2330.TW" || bars[0].Close != 103 {
		t.Errorf("bars[0] = %+v, want 2330.TW close=103", bars[0])
	}
}

// fugleHostRewriteTransport redirects api.fugle.tw requests to the test
// server (the candles URL is hardcoded to the production host).
type fugleHostRewriteTransport struct{ target string }

func (t *fugleHostRewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if !strings.HasPrefix(req.URL.Host, "api.fugle.tw") {
		return http.DefaultTransport.RoundTrip(req)
	}
	r := req.Clone(req.Context())
	r.URL.Scheme = "http"
	r.URL.Host = strings.TrimPrefix(t.target, "http://")
	return http.DefaultTransport.RoundTrip(r)
}
