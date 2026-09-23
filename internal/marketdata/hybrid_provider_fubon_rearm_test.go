package marketdata

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/time/rate"
)

// newArmingProvider builds a HybridProvider whose fubon reachability probe is
// scripted and whose TWSE fallback points at a local server that always fails,
// so the tests never touch the network.
func newArmingProvider(t *testing.T, reachable *atomic.Bool, probes *atomic.Int64) *HybridProvider {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(ResetSharedFubonClient)

	return &HybridProvider{
		twseClient: &TWSEClient{
			httpClient:  srv.Client(),
			baseURL:     srv.URL,
			rateLimiter: rate.NewLimiter(rate.Inf, 0),
			breaker:     newProviderBreaker("twse", defaultCircuitBreakerConfig()),
		},
		breakers: map[string]*providerBreaker{
			"fugle": newProviderBreaker("fugle", defaultCircuitBreakerConfig()),
			"fubon": newProviderBreaker("fubon", defaultCircuitBreakerConfig()),
		},
		fubonProbe: func() bool {
			probes.Add(1)
			return reachable.Load()
		},
	}
}

// TestHybridProvider_MaybeArmFubon_ArmsWhenProxyBecomesReachable covers the
// 2026-09-21 failure mode on the hybrid path: the startup probe used to be a
// permanent verdict, so a proxy that started later was never used again by the
// process (runLiveTrading keeps ONE HybridProvider for its whole lifetime).
func TestHybridProvider_MaybeArmFubon_ArmsWhenProxyBecomesReachable(t *testing.T) {
	var probeCount atomic.Int64
	reachable := &atomic.Bool{}
	p := newArmingProvider(t, reachable, &probeCount)

	// 1. Proxy down: the primary stays unarmed and the probe is rate-limited.
	p.maybeArmFubon()
	if p.fubon() != nil {
		t.Fatal("fubon primary armed while the probe reports unreachable")
	}
	if got := probeCount.Load(); got != 1 {
		t.Fatalf("probe count = %d after the first tick, want 1", got)
	}
	p.maybeArmFubon()
	if got := probeCount.Load(); got != 1 {
		t.Fatalf("probe count = %d after a second immediate tick, want 1 (reprobe must be rate-limited)", got)
	}
	if name := p.Name(); name != "hybrid-twse" {
		t.Fatalf("Name() = %q with an unarmed fubon primary and no other provider, want hybrid-twse", name)
	}

	// 2. Proxy comes up: the next allowed tick arms the primary in-process.
	reachable.Store(true)
	p.nextFubonArmAt = time.Time{} // simulate the cooldown having elapsed
	p.maybeArmFubon()

	if p.fubon() == nil {
		t.Fatal("fubon primary not armed after the proxy became reachable (self-heal failed)")
	}
	if p.GetFubonClient() == nil {
		t.Error("GetFubonClient() = nil although the fubon primary is armed")
	}
	if name := p.Name(); name != "hybrid-fubon" {
		t.Errorf("Name() = %q after arming, want hybrid-fubon", name)
	}

	// 3. Already armed: no further probing.
	before := probeCount.Load()
	p.maybeArmFubon()
	if got := probeCount.Load(); got != before {
		t.Errorf("probe count = %d after arming, want %d (no re-probe once armed)", got, before)
	}
}

// TestHybridProvider_GetQuotes_RetriesFubonArming pins the wiring: the quote
// path is what retries the probe, so a long-lived provider recovers without a
// process restart.
func TestHybridProvider_GetQuotes_RetriesFubonArming(t *testing.T) {
	var probeCount atomic.Int64
	reachable := &atomic.Bool{}
	p := newArmingProvider(t, reachable, &probeCount)

	ctx := context.Background()
	now := time.Now()

	// First quote request: proxy down → one probe, fubon stays unarmed, the
	// existing TWSE fallback chain still answers (here with an error, which is
	// the fallback server's answer, not a panic).
	_, _ = p.GetQuotes(ctx, now, []string{"2330"})
	if got := probeCount.Load(); got != 1 {
		t.Fatalf("probe count = %d after the first GetQuotes, want 1", got)
	}
	if p.fubon() != nil {
		t.Fatal("fubon primary armed although the probe reported unreachable")
	}

	// Second request inside the cooldown window: no extra dial.
	_, _ = p.GetQuotes(ctx, now, []string{"2330"})
	if got := probeCount.Load(); got != 1 {
		t.Fatalf("probe count = %d after a second GetQuotes inside the cooldown, want 1", got)
	}

	// Proxy comes up: the next GetQuotes arms the primary.
	reachable.Store(true)
	p.nextFubonArmAt = time.Time{}
	_, _ = p.GetQuotes(ctx, now, []string{"2330"})

	if p.fubon() == nil {
		t.Fatal("GetQuotes did not arm the fubon primary after the proxy became reachable")
	}
	if got := probeCount.Load(); got != 2 {
		t.Errorf("probe count = %d after the successful re-probe, want 2", got)
	}
}
