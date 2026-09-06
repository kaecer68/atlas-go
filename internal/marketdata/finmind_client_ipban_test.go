package marketdata

// 2026-09-06 IP-ban classification tests. Observed in production 04:10Z:
// FinMind answered HTTP 403 {"msg":"ip banned","retry_after":971} after
// multi-process cron containers collectively exceeded the per-IP rate on
// the sponsor token. The untyped 403 fell through to the generic error
// path → channel marked "error" for ~30m → ChannelHealthStatusError fired
// even though the ban self-healed. Contract under test:
//   1. 403 "ip banned" maps to the typed ErrIPBanned (warn family), and
//      records the ban window.
//   2. While the ban window is active, fetchDataset short-circuits without
//      an HTTP call (the ban is respected, not hammered).
//   3. The breaker does NOT trip on an IP ban (throttling, not outage).

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/time/rate"
)

func ipBanTestServer(t *testing.T, hits *int32) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(hits, 1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"msg":"ip banned","status":403,"retry_after":1,"token_tail":"...test"}`))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func newIPBanTestClient(t *testing.T, srv *httptest.Server) *FinMindClient {
	t.Helper()
	c := newFinMindClientInternal("test-key", t.TempDir())
	c.SetBaseURL(srv.URL)
	c.SetRateLimiter(rate.NewLimiter(rate.Inf, 0))
	return c
}

func TestFinMindClient_IPBan_403MapsToErrIPBanned(t *testing.T) {
	var hits int32
	c := newIPBanTestClient(t, ipBanTestServer(t, &hits))

	_, err := c.GetStockPrice(context.Background(), "2330", "2026-01-01")
	if !errors.Is(err, ErrIPBanned) {
		t.Fatalf("err = %v, want errors.Is(err, ErrIPBanned)", err)
	}
	if !strings.Contains(err.Error(), "retry_after=1s") {
		t.Errorf("err = %v, want retry_after echoed in message", err)
	}
	if atomic.LoadInt32(&hits) != 1 {
		t.Errorf("HTTP hits = %d, want 1", hits)
	}
	if c.ipBanUntilSec.Load() == 0 {
		t.Error("ipBanUntilSec not recorded — short-circuit gate will never engage")
	}
}

func TestFinMindClient_IPBan_ShortCircuitDuringBan(t *testing.T) {
	var hits int32
	c := newIPBanTestClient(t, ipBanTestServer(t, &hits))

	// First call trips the ban (403). Second call must be short-circuited
	// locally: ErrIPBanned again, but NO second HTTP request.
	if _, err := c.GetStockPrice(context.Background(), "2330", "2026-01-01"); !errors.Is(err, ErrIPBanned) {
		t.Fatalf("first call err = %v, want ErrIPBanned", err)
	}
	if _, err := c.GetStockPrice(context.Background(), "2330", "2026-01-01"); !errors.Is(err, ErrIPBanned) {
		t.Fatalf("second call err = %v, want ErrIPBanned (short-circuit)", err)
	}
	if got := atomic.LoadInt32(&hits); got != 1 {
		t.Errorf("HTTP hits = %d, want 1 (ban window must short-circuit without HTTP)", got)
	}
}

func TestFinMindClient_IPBan_DoesNotTripBreaker(t *testing.T) {
	var hits int32
	c := newIPBanTestClient(t, ipBanTestServer(t, &hits))

	if _, err := c.GetStockPrice(context.Background(), "2330", "2026-01-01"); !errors.Is(err, ErrIPBanned) {
		t.Fatalf("err = %v, want ErrIPBanned", err)
	}
	if c.breaker != nil && !c.breaker.shouldTry() {
		t.Error("breaker tripped by IP ban — throttling conditions must not count as outages")
	}
}

func TestFinMindClient_IPBan_ExpiresAfterRetryAfter(t *testing.T) {
	var hits int32
	c := newIPBanTestClient(t, ipBanTestServer(t, &hits)) // retry_after=1 in body

	if _, err := c.GetStockPrice(context.Background(), "2330", "2026-01-01"); !errors.Is(err, ErrIPBanned) {
		t.Fatalf("first call err = %v, want ErrIPBanned", err)
	}
	// Wait out the 1s ban, then the gate must reopen and the request go out.
	time.Sleep(1200 * time.Millisecond)
	if until := c.ipBanUntilSec.Load(); until > time.Now().Unix() {
		t.Fatalf("ban window still active after retry_after: %d", until)
	}
	if _, err := c.GetStockPrice(context.Background(), "2330", "2026-01-01"); !errors.Is(err, ErrIPBanned) {
		t.Fatalf("post-ban call err = %v, want ErrIPBanned (server still 403s)", err)
	}
	if got := atomic.LoadInt32(&hits); got != 2 {
		t.Errorf("HTTP hits = %d, want 2 (request must go out again after ban expiry)", got)
	}
}
