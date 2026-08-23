package marketdata

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"golang.org/x/time/rate"
)

// ─── P1-13: shared TWSE token bucket ────────────────────────────────────────
//
// All www.twse.com.tw callers (the 11 twse_* providers + taiex_twse_fallback)
// must share ONE token bucket instead of each building its own limiter.

func TestTWSEProviders_ShareSingleLimiter(t *testing.T) {
	// Fresh deterministic bucket: 1 token per second, burst 1.
	// Reset the singleton so GetSharedTWSEClient binds to the bucket created
	// below (the shared client persists across test iterations).
	ResetSharedTWSEClient()
	old := SetTWSESharedLimiterForTest(rate.NewLimiter(rate.Every(time.Second), 1))
	defer SetTWSESharedLimiterForTest(old)

	capital := NewTWSECapitalFlowProvider(t.TempDir())
	margin := NewTWSEMarginBalanceProvider("")
	oddlot := NewTWSEOddLotProvider()
	calendar := NewTWSECalendarProvider()
	etf := NewTWSEETFProvider()
	sector := NewTWSESectorIndexProvider(t.TempDir())
	sbl := NewTWSESBLProvider(0.5)
	insider := NewTWSEInsiderProvider("")
	marketVol := NewMarketVolumeProvider()
	dayTrading := NewDayTradingProvider()
	client := GetSharedTWSEClient()

	// Every provider field must point at the SAME limiter instance.
	buckets := []*rate.Limiter{
		capital.limiter, margin.rateLimiter, oddlot.rateLimiter,
		calendar.rateLimiter, etf.rateLimiter, sector.limiter,
		sbl.limiter, insider.limiter, marketVol.rateLimiter,
		dayTrading.rateLimiter, client.rateLimiter,
	}
	first := buckets[0]
	for i, b := range buckets {
		if b != first {
			t.Fatalf("bucket %d is a different limiter instance (P1-13 requires one shared bucket)", i)
		}
	}

	// Cross-provider exhaustion: consuming tokens through one provider must
	// deplete the bucket visible to every other provider.
	if !capital.limiter.Allow() {
		t.Fatal("expected first token to be allowed")
	}
	if oddlot.rateLimiter.Allow() {
		t.Fatal("second token must be refused (burst=1 shared bucket)")
	}
}

func TestTWSETAIEXFallback_WaitsOnSharedLimiter(t *testing.T) {
	// The fallback must route through the shared bucket: with the bucket
	// exhausted, a fallback call must fail at the rate-limit wait and must
	// NOT hit the upstream (proving it no longer builds a fresh raw client
	// that bypasses the limiter).
	hit := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit++
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"stat":"OK","tables":[]}`))
	}))
	defer ts.Close()

	oldURL := taiexTWSEBaseURL
	taiexTWSEBaseURL = ts.URL
	defer func() { taiexTWSEBaseURL = oldURL }()

	// Reset the singleton so GetSharedTWSEClient binds its limiter field to
	// the bucket created below (otherwise a client built by an earlier test
	// keeps pointing at a stale bucket).
	ResetSharedTWSEClient()
	old := SetTWSESharedLimiterForTest(rate.NewLimiter(rate.Every(time.Hour), 1))
	defer SetTWSESharedLimiterForTest(old)
	// Drain the single token so the next Wait blocks.
	if !GetSharedTWSEClient().RateLimiter().Allow() {
		t.Fatal("expected to drain the shared token")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	_, err := fetchTWSETAIEXFallback(ctx)
	if err == nil || !strings.Contains(err.Error(), "rate limit wait") {
		t.Fatalf("expected rate-limit-wait error via shared bucket, got %v", err)
	}
	if hit != 0 {
		t.Fatalf("upstream was hit %d times — fallback bypassed the shared limiter", hit)
	}
}
