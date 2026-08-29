package marketdata

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// ─── P1-14: Yahoo negative cache ────────────────────────────────────────────
//
// fetchFromHost on 429/HTML marks the whole session blocked (Retry-After,
// clamped to [5,10] min); fetchWithFallback short-circuits while blocked so
// every Yahoo channel stops hammering the same IP.

func TestYahooNegativeCache_429HonorsRetryAfterAndShortCircuits(t *testing.T) {
	hit := 0
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit++
		w.Header().Set("Retry-After", "600") // 10 min
		http.Error(w, "blocked", http.StatusTooManyRequests)
	}))
	defer ts.Close()

	origHosts := yahooHosts
	yahooHosts = []string{strings.TrimPrefix(ts.URL, "https://")}
	defer func() { yahooHosts = origHosts }()
	SetYahooSessionClient(ts.Client())
	s := getYahooSession()
	defer resetYahooSessionState()

	ctx := context.Background()
	_, err := s.fetchWithFallback(ctx, "^TWII", map[string]string{"interval": "1d", "range": "5d"})
	if err == nil {
		t.Fatal("expected 429 error")
	}
	if hit != 1 {
		t.Fatalf("hit = %d, want 1", hit)
	}

	// Block must be active: clamped to the 10-minute max.
	if s.blockedUntil.IsZero() || !time.Now().Before(s.blockedUntil) {
		t.Fatal("negative cache block not set")
	}
	maxWait := time.Until(s.blockedUntil)
	if maxWait > negativeCacheBlockMax+time.Second || maxWait < 9*time.Minute {
		t.Fatalf("block duration %v out of [5,10] min range", maxWait)
	}

	// Every subsequent call short-circuits WITHOUT touching the network.
	for i := range 3 {
		_, err = s.fetchWithFallback(ctx, "AAPL", map[string]string{"interval": "1d", "range": "5d"})
		if err == nil || !strings.Contains(err.Error(), "negative-cache") {
			t.Fatalf("call %d: expected negative-cache short-circuit, got %v", i+1, err)
		}
	}
	if hit != 1 {
		t.Fatalf("hit = %d after short-circuits, want 1 (all channels must be suppressed)", hit)
	}
}

func TestYahooNegativeCache_HTMLResponseDoesNotBlock(t *testing.T) {
	hit := 0
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit++
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html><body>temporary error</body></html>"))
	}))
	defer ts.Close()

	origHosts := yahooHosts
	yahooHosts = []string{strings.TrimPrefix(ts.URL, "https://")}
	defer func() { yahooHosts = origHosts }()
	SetYahooSessionClient(ts.Client())
	s := getYahooSession()
	defer resetYahooSessionState()

	// HTML error page is a transient Yahoo error, NOT a definitive IP block:
	// the request fails (this host) but the negative cache must NOT be set,
	// so fetchWithFallback can still try the next host in the chain.
	_, err := s.fetchWithFallback(context.Background(), "^VIX", map[string]string{"interval": "1d", "range": "5d"})
	if err == nil {
		t.Fatal("expected error for HTML response")
	}
	if time.Now().Before(s.blockedUntil) {
		t.Fatal("HTML response must NOT set the negative cache (only 429 does)")
	}
}

func TestYahooNegativeCache_ShortRetryAfterClampsToFiveMinutes(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "5") // 5s — below the 5min floor
		http.Error(w, "blocked", http.StatusTooManyRequests)
	}))
	defer ts.Close()

	origHosts := yahooHosts
	yahooHosts = []string{strings.TrimPrefix(ts.URL, "https://")}
	defer func() { yahooHosts = origHosts }()
	SetYahooSessionClient(ts.Client())
	s := getYahooSession()
	defer resetYahooSessionState()

	_, err := s.fetchWithFallback(context.Background(), "^TWII", map[string]string{"interval": "1d", "range": "5d"})
	if err == nil {
		t.Fatal("expected error")
	}
	if wait := time.Until(s.blockedUntil); wait < negativeCacheBlockMin-time.Second {
		t.Fatalf("block %v below the 5-minute floor", wait)
	}
}

func TestYahooNegativeCache_Expires(t *testing.T) {
	hit := 0
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"chart":{"result":[]}}`))
	}))
	defer ts.Close()

	origHosts := yahooHosts
	yahooHosts = []string{strings.TrimPrefix(ts.URL, "https://")}
	defer func() { yahooHosts = origHosts }()
	SetYahooSessionClient(ts.Client())
	s := getYahooSession()
	defer resetYahooSessionState()

	// Force an expired block window.
	s.markBlocked(-time.Minute)
	_, err := s.fetchWithFallback(context.Background(), "^TWII", map[string]string{"interval": "1d", "range": "5d"})
	if err != nil {
		t.Fatalf("expired block must not suppress requests: %v", err)
	}
	if hit != 1 {
		t.Fatalf("hit = %d, want 1 (request must resume after block expiry)", hit)
	}
}

func TestYahooNegativeCache_RetryAfterHTTPDate(t *testing.T) {
	// Retry-After as an HTTP-date must also be honored (not just seconds).
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", time.Now().Add(time.Hour).Format(http.TimeFormat))
		http.Error(w, "blocked", http.StatusTooManyRequests)
	}))
	defer ts.Close()

	origHosts := yahooHosts
	yahooHosts = []string{strings.TrimPrefix(ts.URL, "https://")}
	defer func() { yahooHosts = origHosts }()
	SetYahooSessionClient(ts.Client())
	s := getYahooSession()
	defer resetYahooSessionState()

	_, err := s.fetchWithFallback(context.Background(), "^TWII", map[string]string{"interval": "1d", "range": "5d"})
	if err == nil {
		t.Fatal("expected error")
	}
	wait := time.Until(s.blockedUntil)
	if wait > negativeCacheBlockMax+time.Second {
		t.Fatalf("HTTP-date Retry-After not clamped to max: %v", wait)
	}
	if wait < negativeCacheBlockMin-time.Second {
		t.Fatalf("HTTP-date Retry-After not honored (below floor): %v", wait)
	}
}

// resetYahooSessionState clears the shared session's negative-cache block and
// breaker so a test that intentionally trips them does not pollute later
// tests (the singleton session is shared package-wide).
func resetYahooSessionState() {
	s := getYahooSession()
	s.blockedUntil = time.Time{}
	if s.breaker != nil {
		s.breaker.reset()
	}
}
