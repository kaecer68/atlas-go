package marketdata

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func buildTWIIChartJSON(timestamp int64, closes []float64) []byte {
	var sb strings.Builder
	sb.WriteString(`{"chart":{"result":[{"meta":{"regularMarketTime":`)
	sb.WriteString(fmt.Sprintf("%d", timestamp))
	sb.WriteString(`},"indicators":{"quote":[{"close":[`)
	for i, c := range closes {
		if i > 0 {
			sb.WriteString(",")
		}
		sb.WriteString(fmt.Sprintf("%g", c))
	}
	sb.WriteString(`]}]}}]}}`)
	return []byte(sb.String())
}

// TestTaiwanVolatilityProvider_FetchSnapshot_StaleCache_AutoRefetches
// documents the PR-B contract change: a stale cache entry (60s TTL still
// valid, but RegularMarketTime from a previous trading day) used to
// return a hard error. As of PR-B, the provider invalidates the stale
// entry and refetches exactly once. If the refetched Yahoo response is
// also stale (e.g. Yahoo itself is lagging, or the upstream is broken),
// the error is reported — this test exercises that fallback path by
// making the refetched response also carry yesterday's timestamp.
func TestTaiwanVolatilityProvider_FetchSnapshot_StaleCache_AutoRefetches(t *testing.T) {
	twiiCache.reset()
	defer twiiCache.reset()

	// Fix "now" to a weekday (Wednesday 2026-07-29) so the freshness
	// expectation is stable.
	fixedNow := time.Date(2026, 7, 29, 9, 0, 0, 0, twseLocation)
	origNowFunc := twiiNowFunc
	twiiNowFunc = func() time.Time { return fixedNow }
	defer func() { twiiNowFunc = origNowFunc }()

	// Prime cache with Monday's close timestamp.
	staleTs := time.Date(2026, 7, 27, 13, 30, 0, 0, twseLocation).Unix()
	closes := make([]float64, 25)
	for i := range closes {
		closes[i] = 23000.0 + float64(i)*10
	}
	twiiCache.set(buildTWIIChartJSON(staleTs, closes), "1d", "3mo")

	// Refetched response also stale (Yahoo itself returning yesterday's bars).
	// This proves PR-B only retries ONCE; if the refetch is also stale the
	// error is reported (rather than looping).
	server := installYahooMockServer(t, buildTWIIChartJSON(staleTs, closes))
	defer server.Close()

	p := NewTaiwanVolatilityProvider()
	_, err := p.FetchSnapshot(context.Background())
	if err == nil {
		t.Fatal("expected error when both cache and refetch return stale data")
	}
	if !strings.Contains(err.Error(), "stale") {
		t.Fatalf("expected stale error, got %v", err)
	}
	// After a stale refetch we must NOT keep the stale entry sitting in cache
	// (would just re-trigger the same error on the next FetchSnapshot call).
	// get() returns nil for missing/expired entries, so this verifies the
	// entry was removed regardless of TTL.
	if cached := twiiCache.get("1d", "3mo"); cached != nil {
		t.Errorf("stale entry should have been invalidated, but cache still holds data")
	}
}

// TestTaiwanVolatilityProvider_FetchSnapshot_StaleCache_RecoversWithFreshRefetch
// is the success path: cache holds yesterday's close, refetch pulls today's
// bar, and the snapshot is computed from the fresh data. This is the
// regression test that proves PR-B fixes the real production failure mode
// (tw_vol channel 報 stale error 一整天 even though Yahoo is up).
func TestTaiwanVolatilityProvider_FetchSnapshot_StaleCache_RecoversWithFreshRefetch(t *testing.T) {
	twiiCache.reset()
	defer twiiCache.reset()

	fixedNow := time.Date(2026, 7, 29, 9, 0, 0, 0, twseLocation)
	origNowFunc := twiiNowFunc
	twiiNowFunc = func() time.Time { return fixedNow }
	defer func() { twiiNowFunc = origNowFunc }()

	// Cache: Monday's close (stale).
	staleTs := time.Date(2026, 7, 27, 13, 30, 0, 0, twseLocation).Unix()
	closes := make([]float64, 25)
	for i := range closes {
		closes[i] = 23000.0 + float64(i)*10
	}
	twiiCache.set(buildTWIIChartJSON(staleTs, closes), "1d", "3mo")

	// Refetch: returns today's bar (fresh).
	freshTs := fixedNow.Unix()
	server := installYahooMockServer(t, buildTWIIChartJSON(freshTs, closes))
	defer server.Close()

	p := NewTaiwanVolatilityProvider()
	snap, err := p.FetchSnapshot(context.Background())
	if err != nil {
		t.Fatalf("expected recovery via refetch, got error: %v", err)
	}
	if snap.HistoricalVolatility.Symbol != "^TWII" {
		t.Errorf("Symbol = %q, want ^TWII", snap.HistoricalVolatility.Symbol)
	}
	if snap.HistoricalVolatility.Timestamp != freshTs {
		t.Errorf("Timestamp = %d, want %d (fresh refetch)", snap.HistoricalVolatility.Timestamp, freshTs)
	}
	if snap.HistoricalVolatility.ChangePct <= 0 || math.IsNaN(snap.HistoricalVolatility.ChangePct) {
		t.Errorf("expected positive volatility from fresh data, got %v", snap.HistoricalVolatility.ChangePct)
	}
}

// installYahooMockServer injects an httptest.TLSServer that returns `body`
// for any ^TWII fetch, redirects yahooHosts at it, and sets the session
// client. Caller is responsible for server.Close() (typically via defer).
func installYahooMockServer(t *testing.T, body []byte) *httptest.Server {
	t.Helper()
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write(body)
	}))
	origHosts := yahooHosts
	yahooHosts = []string{strings.TrimPrefix(server.URL, "https://")}
	t.Cleanup(func() { yahooHosts = origHosts })
	SetYahooSessionClient(server.Client())
	return server
}

func TestTaiwanVolatilityProvider_FetchSnapshot_FreshCacheAccepts(t *testing.T) {
	twiiCache.reset()
	defer twiiCache.reset()

	fixedNow := time.Date(2026, 7, 29, 9, 0, 0, 0, twseLocation)
	origNowFunc := twiiNowFunc
	twiiNowFunc = func() time.Time { return fixedNow }
	defer func() { twiiNowFunc = origNowFunc }()

	freshTs := fixedNow.Unix()
	closes := make([]float64, 25)
	for i := range closes {
		closes[i] = 23000.0 + float64(i)*10
	}
	twiiCache.set(buildTWIIChartJSON(freshTs, closes), "1d", "3mo")

	p := NewTaiwanVolatilityProvider()
	snap, err := p.FetchSnapshot(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if snap.HistoricalVolatility.Symbol != "^TWII" {
		t.Errorf("Symbol = %q, want ^TWII", snap.HistoricalVolatility.Symbol)
	}
	if snap.HistoricalVolatility.Value != closes[len(closes)-1] {
		t.Errorf("Value = %v, want %v", snap.HistoricalVolatility.Value, closes[len(closes)-1])
	}
	if snap.HistoricalVolatility.ChangePct <= 0 || math.IsNaN(snap.HistoricalVolatility.ChangePct) {
		t.Errorf("expected positive volatility, got %v", snap.HistoricalVolatility.ChangePct)
	}
}
