package marketdata

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"path/filepath"
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
// entry and refetches exactly once.
//
// A04（2026-08-10 audit）：refetch 也 stale（Yahoo 尚未發布最新交易日 bar）
// 不再是 error — 這是「資料時間戳較舊」而非 transport failure，用最新可用
// bars 計算並回傳，避免誤觸 circuit breaker。
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
	// PR-B only retries ONCE; A04 accepts the stale bars instead of erroring.
	server := installYahooMockServer(t, buildTWIIChartJSON(staleTs, closes))
	defer server.Close()

	p := NewTaiwanVolatilityProvider()
	snap, err := p.FetchSnapshot(context.Background())
	if err != nil {
		t.Fatalf("stale refetch should be accepted (stale-data ≠ transport failure), got error: %v", err)
	}
	if snap.HistoricalVolatility.Timestamp != staleTs {
		t.Errorf("Timestamp = %d, want %d (stale data timestamp preserved)", snap.HistoricalVolatility.Timestamp, staleTs)
	}
	if snap.HistoricalVolatility.ChangePct <= 0 || math.IsNaN(snap.HistoricalVolatility.ChangePct) {
		t.Errorf("expected positive volatility from accepted bars, got %v", snap.HistoricalVolatility.ChangePct)
	}
	// After a stale refetch we must NOT keep the stale entry sitting in cache
	// (would just re-trigger the same path on the next FetchSnapshot call).
	// get() returns nil for missing/expired entries, so this verifies the
	// entry was removed regardless of TTL.
	if cached := twiiCache.get("1d", "3mo"); cached != nil {
		t.Errorf("stale entry should have been invalidated, but cache still holds data")
	}
}

// TestTaiwanVolatilityProvider_PreMarket_AcceptsPreviousDay 驗證 A04 盤前
// 寬限：交易日 09:00 前，Yahoo 的最新 bar 是前一交易日 close，應直接接受
// 而不觸發 stale 路徑。
func TestTaiwanVolatilityProvider_PreMarket_AcceptsPreviousDay(t *testing.T) {
	twiiCache.reset()
	defer twiiCache.reset()

	// Monday 08:00 CST（盤前）：expected = 上週五
	fixedNow := time.Date(2026, 8, 10, 8, 0, 0, 0, twseLocation)
	origNowFunc := twiiNowFunc
	twiiNowFunc = func() time.Time { return fixedNow }
	defer func() { twiiNowFunc = origNowFunc }()

	prevTs := time.Date(2026, 8, 7, 13, 30, 0, 0, twseLocation).Unix() // Friday close
	closes := make([]float64, 25)
	for i := range closes {
		closes[i] = 23000.0 + float64(i)*10
	}
	server := installYahooMockServer(t, buildTWIIChartJSON(prevTs, closes))
	defer server.Close()

	p := NewTaiwanVolatilityProvider()
	snap, err := p.FetchSnapshot(context.Background())
	if err != nil {
		t.Fatalf("pre-market previous-day bar should be accepted, got error: %v", err)
	}
	if snap.HistoricalVolatility.Timestamp != prevTs {
		t.Errorf("Timestamp = %d, want %d (Friday close)", snap.HistoricalVolatility.Timestamp, prevTs)
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

// B02：Yahoo transport error 時用 history store fallback 計算波動率。
func TestTaiwanVolatilityProvider_YahooDown_FallbackFromHistory(t *testing.T) {
	twiiCache.reset()
	defer twiiCache.reset()

	// Yahoo 全部失敗（500）
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	origHosts := yahooHosts
	yahooHosts = []string{strings.TrimPrefix(server.URL, "https://")}
	t.Cleanup(func() { yahooHosts = origHosts })
	SetYahooSessionClient(server.Client())

	// 預填 history store：25 筆升序 closes
	storePath := filepath.Join(t.TempDir(), "twii_history.json")
	p := NewTaiwanVolatilityProviderWithStore(storePath)
	base := time.Date(2026, 7, 1, 0, 0, 0, 0, twseLocation)
	for i := range 25 {
		p.history.Append(base.AddDate(0, 0, i), 23000.0+float64(i)*10)
	}

	snap, err := p.FetchSnapshot(context.Background())
	if err != nil {
		t.Fatalf("expected history fallback success, got error: %v", err)
	}
	if snap.HistoricalVolatility.Symbol != "^TWII" {
		t.Errorf("Symbol = %q, want ^TWII", snap.HistoricalVolatility.Symbol)
	}
	if snap.HistoricalVolatility.ChangePct <= 0 || math.IsNaN(snap.HistoricalVolatility.ChangePct) {
		t.Errorf("expected positive volatility from history fallback, got %v", snap.HistoricalVolatility.ChangePct)
	}
	// Timestamp 應為 store 最後一筆日期（7/25）
	wantTs := base.AddDate(0, 0, 24).Unix()
	if snap.HistoricalVolatility.Timestamp != wantTs {
		t.Errorf("Timestamp = %d, want %d (last history date)", snap.HistoricalVolatility.Timestamp, wantTs)
	}
}

// B02：Yahoo down 且 history 不足 21 筆 → 回原 error（fallback 不偽造資料）。
func TestTaiwanVolatilityProvider_YahooDown_HistoryInsufficient(t *testing.T) {
	twiiCache.reset()
	defer twiiCache.reset()

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	origHosts := yahooHosts
	yahooHosts = []string{strings.TrimPrefix(server.URL, "https://")}
	t.Cleanup(func() { yahooHosts = origHosts })
	SetYahooSessionClient(server.Client())

	p := NewTaiwanVolatilityProviderWithStore(filepath.Join(t.TempDir(), "empty.json"))
	// 只放 5 筆
	base := time.Date(2026, 7, 1, 0, 0, 0, 0, twseLocation)
	for i := range 5 {
		p.history.Append(base.AddDate(0, 0, i), 23000.0)
	}

	_, err := p.FetchSnapshot(context.Background())
	if err == nil {
		t.Fatal("expected error when history has <21 closes")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("err = %v, want upstream error preserved", err)
	}
}
