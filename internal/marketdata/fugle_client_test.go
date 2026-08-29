package marketdata

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/time/rate"
)

func TestFugleClient_SetHTTPClient(t *testing.T) {
	c := NewFugleClient("test-key")
	c.SetHTTPClient(nil)
	if c.httpClient != nil {
		t.Error("expected httpClient nil after SetHTTPClient(nil)")
	}
}

func TestFugleClient_RateLimiter(t *testing.T) {
	c := NewFugleClient("test-key")
	if c.RateLimiter() == nil {
		t.Fatal("expected non-nil rate limiter")
	}
}

func TestGetFugleRateLimit(t *testing.T) {
	// Default free tier — conservative 30/min (below the measured ~39/min
	// 429 point, manifest F2/A2) instead of the published 60/min.
	if got := getFugleRateLimit(); got != 30 {
		t.Errorf("getFugleRateLimit() = %d, want 30", got)
	}
}

func TestFugleClient_GetMeta_Success(t *testing.T) {
	payload := FugleMetaResponse{
		Symbol: "0050",
		Name:   "元大台灣50",
	}
	body, _ := json.Marshal(payload)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/intraday/ticker/0050" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("X-API-KEY") != "test-key" {
			t.Errorf("X-API-KEY header missing or wrong")
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(body)
	}))
	defer srv.Close()

	c := NewFugleClient("test-key")
	c.baseURL = srv.URL
	c.rateLimiter = rate.NewLimiter(rate.Inf, 1)

	meta, err := c.GetMeta(context.Background(), "0050")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if meta.Symbol != "0050" {
		t.Errorf("Symbol = %q, want 0050", meta.Symbol)
	}
}

func TestFugleClient_CheckMarketStatus_Open(t *testing.T) {
	payload := FugleMetaResponse{
		Symbol:         "0050",
		SecurityStatus: "NORMAL",
	}
	body, _ := json.Marshal(payload)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(body)
	}))
	defer srv.Close()

	c := NewFugleClient("test-key")
	c.baseURL = srv.URL
	c.rateLimiter = rate.NewLimiter(rate.Inf, 1)

	open, err := c.CheckMarketStatus(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !open {
		t.Error("expected market open")
	}
}

func TestFugleClient_CheckMarketStatus_Suspended(t *testing.T) {
	payload := FugleMetaResponse{
		Symbol:         "0050",
		SecurityStatus: "SUSPENDED",
	}
	body, _ := json.Marshal(payload)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(body)
	}))
	defer srv.Close()

	c := NewFugleClient("test-key")
	c.baseURL = srv.URL
	c.rateLimiter = rate.NewLimiter(rate.Inf, 1)

	open, err := c.CheckMarketStatus(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if open {
		t.Error("expected market not open when suspended")
	}
}

// TestFugleClient_CheckMarketStatus_EmptyStatusFailClosed verifies that an
// empty/missing securityStatus (abnormal response) is treated as not-open
// (fail-closed) rather than open — a correction from the initial migration
// which treated "" as open (2026-08-03 live verification: NORMAL is the
// documented open value).
func TestFugleClient_CheckMarketStatus_EmptyStatusFailClosed(t *testing.T) {
	payload := FugleMetaResponse{
		Symbol: "0050",
	}
	body, _ := json.Marshal(payload)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(body)
	}))
	defer srv.Close()

	c := NewFugleClient("test-key")
	c.baseURL = srv.URL
	c.rateLimiter = rate.NewLimiter(rate.Inf, 1)

	open, err := c.CheckMarketStatus(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if open {
		t.Error("expected market not open when securityStatus is empty (fail-closed)")
	}
}

func TestNewFugleProviderWithClient(t *testing.T) {
	c := NewFugleClient("test-key")
	p := NewFugleProviderWithClient(c)
	if p.GetClient() != c {
		t.Error("expected provider client to equal injected client")
	}
}

func TestResetSharedFugleClient(t *testing.T) {
	ResetSharedFugleClient()
	c1 := GetSharedFugleClient("key1")
	ResetSharedFugleClient()
	c2 := GetSharedFugleClient("key2")
	if c1 == c2 {
		t.Error("expected different shared client instances after reset")
	}
}

// TestGetSharedFugleClient_SameInstance verifies the singleton returns the
// same *FugleClient (and thus the same rate limiter) across repeated calls —
// the invariant that keeps all Fugle call sites under one 60/min budget
// instead of each site minting its own limiter and blowing past the free
// tier (SK-22 Fugle audit).
func TestGetSharedFugleClient_SameInstance(t *testing.T) {
	ResetSharedFugleClient()
	defer ResetSharedFugleClient()
	c1 := GetSharedFugleClient("test-key")
	c2 := GetSharedFugleClient("test-key")
	if c1 != c2 {
		t.Fatal("GetSharedFugleClient must return the same instance across calls")
	}
	if c1.RateLimiter() != c2.RateLimiter() {
		t.Fatal("both calls must share the same rate limiter")
	}
}

// TestHybridProvider_UsesSharedFugleClient verifies NewHybridProvider wires
// the Fugle provider through the shared singleton client — one limiter for
// all Fugle call sites — rather than a per-instance NewFugleClient with its
// own token bucket (SK-22 Fugle audit regression guard).
func TestHybridProvider_UsesSharedFugleClient(t *testing.T) {
	ResetSharedFugleClient()
	defer ResetSharedFugleClient()
	p := NewHybridProvider("", "test-key")
	if p.fugleProvider == nil {
		t.Skip("fugle provider not configured (fubon proxy reachable?)")
	}
	got := p.fugleProvider.GetClient()
	shared := GetSharedFugleClient("test-key")
	if got != shared {
		t.Fatal("hybrid provider Fugle client must be the shared singleton")
	}
}

// TestFugleClient_GetQuote_429Retry verifies the v1.0 client retries a 429
// response (rate limit exceeded) instead of failing immediately — the
// free-tier sliding window can 429 before the token bucket drains, and
// GetQuote must honor it (SK-22 live probe: 27/65 calls 429'd without retry).
func TestFugleClient_GetQuote_429Retry(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte(`{"statusCode":429,"message":"Rate limit exceeded"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		// v1.0 flat shape
		w.Write([]byte(`{"date":"2026-07-07","symbol":"2330","name":"台積電","closePrice":680,"openPrice":670,"highPrice":685,"lowPrice":668,"lastPrice":680,"total":{"tradeVolume":12345}}`))
	}))
	defer srv.Close()

	c := NewFugleClient("test-key")
	c.baseURL = srv.URL
	c.rateLimiter = rate.NewLimiter(rate.Inf, 1)

	q, err := c.GetQuote(context.Background(), "2330")
	if err != nil {
		t.Fatalf("GetQuote after 429 retry: %v", err)
	}
	if attempts < 2 {
		t.Errorf("expected at least 2 attempts (1x 429 + retry), got %d", attempts)
	}
	if q.Last != 680 {
		t.Errorf("Last = %v, want 680", q.Last)
	}
	if q.Source != "fugle" {
		t.Errorf("Source = %q, want fugle", q.Source)
	}
}

// TestFugleClient_GetMeta_429Retry verifies GetMeta also retries 429.
func TestFugleClient_GetMeta_429Retry(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte(`{"statusCode":429}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"symbol":"0050","securityStatus":"NORMAL"}`))
	}))
	defer srv.Close()

	c := NewFugleClient("test-key")
	c.baseURL = srv.URL
	c.rateLimiter = rate.NewLimiter(rate.Inf, 1)

	meta, err := c.GetMeta(context.Background(), "0050")
	if err != nil {
		t.Fatalf("GetMeta after 429 retry: %v", err)
	}
	if attempts < 2 {
		t.Errorf("expected at least 2 attempts, got %d", attempts)
	}
	if meta.SecurityStatus != "NORMAL" {
		t.Errorf("SecurityStatus = %q, want NORMAL", meta.SecurityStatus)
	}
}

// ─── Daily-quota gate (mirrors the FinMind gate) ────────────────────────────

// TestFugleClient_QuotaGate_ReturnsErrFugleQuotaExhausted verifies that once
// the Fugle daily quota is gone, doGet short-circuits with ErrFugleQuotaExhausted
// before making any HTTP request. This is the symmetric counterpart to the
// FinMind quota gate; together they let the channel-health dashboard present
// a unified "which providers are out of budget today" view.
func TestFugleClient_QuotaGate_ReturnsErrFugleQuotaExhausted(t *testing.T) {
	c := newFugleClient("test-key", t.TempDir())
	// Disable rate limiter so the quota gate is the only block.
	c.rateLimiter = rate.NewLimiter(rate.Inf, 0)
	// Force the daily limit to 0 so AllowCall returns false immediately.
	c.quotaTracker.SetLimit(0)

	var httpHit int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&httpHit, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	c.baseURL = srv.URL

	_, err := c.GetQuote(context.Background(), "2330")
	if !errors.Is(err, ErrFugleQuotaExhausted) {
		t.Fatalf("err = %v, want errors.Is(err, ErrFugleQuotaExhausted)", err)
	}
	if atomic.LoadInt32(&httpHit) != 0 {
		t.Errorf("HTTP handler hit %d times — quota gate did not block the request", httpHit)
	}
}

// TestFugleClient_QuotaTelemetry verifies the QuotaUsed/Remaining accessors
// expose the same counter as the FinMind ones, so a future /api/dashboard/quota
// endpoint can render both providers from one Snapshot() call.
func TestFugleClient_QuotaTelemetry(t *testing.T) {
	c := newFugleClient("k", t.TempDir())
	c.rateLimiter = rate.NewLimiter(rate.Inf, 0)

	if got := c.QuotaUsed(); got != 0 {
		t.Errorf("QuotaUsed() = %d, want 0 before any calls", got)
	}
	if got := c.QuotaRemaining(); got != fugleDailyLimit {
		t.Errorf("QuotaRemaining() = %d, want %d (full daily limit)", got, fugleDailyLimit)
	}

	for i := range 100 {
		if !c.quotaTracker.AllowCall() {
			t.Fatalf("AllowCall returned false at iteration %d", i)
		}
	}
	if got := c.QuotaUsed(); got != 100 {
		t.Errorf("QuotaUsed() after 100 calls = %d, want 100", got)
	}
	if got := c.QuotaRemaining(); got != fugleDailyLimit-100 {
		t.Errorf("QuotaRemaining() = %d, want %d", got, fugleDailyLimit-100)
	}
}

// TestFugleClient_QuotaGateNilSafe mirrors the FinMind test: clients without
// a tracker (nil state) must not crash. Defends against future refactors
// that might forget to wire the tracker.
func TestFugleClient_QuotaGateNilSafe(t *testing.T) {
	c := &FugleClient{
		apiKey:       "k",
		httpClient:   &http.Client{Timeout: 1 * time.Second},
		rateLimiter:  rate.NewLimiter(rate.Inf, 0),
		quotaTracker: nil,
	}
	// Without the gate, doGet must not crash on nil tracker.
	_, err := c.GetQuote(context.Background(), "2330")
	if errors.Is(err, ErrFugleQuotaExhausted) {
		t.Errorf("nil tracker should not yield ErrFugleQuotaExhausted, got %v", err)
	}
}

// TestFugleClient_401_ReturnsErrFugleUnauthorized verifies that a 401
// response maps to the typed ErrFugleUnauthorized (free-tier quota-lock
// and invalid-key both surface as 401 — manifest F3/D5). Without the
// typed mapping, HandleQuote treats it as a generic failure and the
// quota-lock stays invisible in channel health.
func TestFugleClient_401_ReturnsErrFugleUnauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"message":"Unauthorized","statusCode":401}`))
	}))
	defer srv.Close()

	c := NewFugleClient("test-key")
	c.baseURL = srv.URL
	c.rateLimiter = rate.NewLimiter(rate.Inf, 1)

	_, err := c.GetQuote(context.Background(), "2330")
	if !errors.Is(err, ErrFugleUnauthorized) {
		t.Fatalf("err = %v, want errors.Is(err, ErrFugleUnauthorized)", err)
	}
}

// ─── manifest Phase D — client 級 breaker（所有消費層共享）───────────────

// TestFugleClient_Breaker_OpensAfterConsecutiveFailures verifies the
// client-level breaker: 3 consecutive upstream failures trip it, after
// which doGet short-circuits with ErrFugleBreakerOpen and stops hitting
// the upstream (all consumers — gateway channel, stocktools, hybrid,
// warmup — share this single client).
func TestFugleClient_Breaker_OpensAfterConsecutiveFailures(t *testing.T) {
	var httpHits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		httpHits.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := NewFugleClient("test-key")
	c.baseURL = srv.URL
	c.rateLimiter = rate.NewLimiter(rate.Inf, 1)

	// 3 次失敗 → breaker open
	for i := range 3 {
		if _, err := c.GetQuote(context.Background(), "2330"); err == nil {
			t.Fatalf("attempt %d: expected error, got nil", i+1)
		}
	}
	if got := httpHits.Load(); got != 3 {
		t.Fatalf("http hits = %d, want 3 (breaker should not short-circuit before threshold)", got)
	}

	// 第 4 次 → breaker open，不發 HTTP
	_, err := c.GetQuote(context.Background(), "2330")
	if !errors.Is(err, ErrFugleBreakerOpen) {
		t.Fatalf("err = %v, want errors.Is(err, ErrFugleBreakerOpen)", err)
	}
	if got := httpHits.Load(); got != 3 {
		t.Errorf("http hits = %d after breaker open, want 3 (no upstream calls while open)", got)
	}
}

// TestFugleClient_Breaker_ResetsOnSuccess verifies a successful call after
// a failure resets the breaker to closed (single-failure does not trip).
func TestFugleClient_Breaker_ResetsOnSuccess(t *testing.T) {
	attempt := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempt++
		if attempt == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"symbol":"2330","closePrice":680,"openPrice":670,"highPrice":685,"lowPrice":668,"lastPrice":680,"total":{"tradeVolume":12345}}`))
	}))
	defer srv.Close()

	c := NewFugleClient("test-key")
	c.baseURL = srv.URL
	c.rateLimiter = rate.NewLimiter(rate.Inf, 1)

	if _, err := c.GetQuote(context.Background(), "2330"); err == nil {
		t.Fatal("expected error on first attempt")
	}
	if _, err := c.GetQuote(context.Background(), "2330"); err != nil {
		t.Fatalf("second attempt should succeed and reset breaker: %v", err)
	}
	if !c.breaker.shouldTry() {
		t.Fatal("breaker should be closed after success (shouldTry=true)")
	}
}

// TestFugleClient_GetHistoricalCandles_QuotaGate verifies the candles path
// (warmup + technical on-demand) is protected by the same daily-quota gate
// as doGet — previously it only passed the rate limiter, so warmup's ~32
// calls were invisible to the 2000/day budget (manifest Phase D gap).
func TestFugleClient_GetHistoricalCandles_QuotaGate(t *testing.T) {
	c := newFugleClient("test-key", t.TempDir())
	c.rateLimiter = rate.NewLimiter(rate.Inf, 0)
	c.quotaTracker.SetLimit(0)

	var httpHit int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&httpHit, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	c.baseURL = srv.URL

	_, err := c.GetHistoricalCandles(context.Background(), "2330", "2026-01-01", "2026-08-10")
	if !errors.Is(err, ErrFugleQuotaExhausted) {
		t.Fatalf("err = %v, want errors.Is(err, ErrFugleQuotaExhausted)", err)
	}
	if atomic.LoadInt32(&httpHit) != 0 {
		t.Errorf("HTTP handler hit %d times — quota gate did not block candles request", httpHit)
	}
}
