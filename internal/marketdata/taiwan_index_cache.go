package marketdata

import (
	"sync"
	"time"
)

// taiwanIndexCache avoids duplicate Yahoo Finance API calls for ^TWII
// by both TAIEXIndexProvider and TaiwanVolatilityProvider during the
// same refresh cycle. Cache TTL is 60 seconds — longer than the typical
// us_market_refresh batch duration (cold start ~5s), so only one Yahoo
// call per refresh cycle regardless of how many adapters consume ^TWII.
//
// P1 B03: taiex_index + tw_vol channel consolidation.
type taiwanIndexCacheEntry struct {
	data    []byte
	expires time.Time
}

type taiwanIndexCache struct {
	mu    sync.RWMutex
	items map[string]taiwanIndexCacheEntry // key: "interval|range"
}

var twiiCache = &taiwanIndexCache{
	items: make(map[string]taiwanIndexCacheEntry),
}

const twiiCacheTTL = 60 * time.Second

// twiiNowFunc is overridable in tests so cache-freshness checks can be
// deterministic regardless of the wall clock.
var twiiNowFunc = time.Now

// twiiCacheTimestampIsCurrentTradingDay reports whether a cached Yahoo ^TWII
// response timestamp corresponds to the expected latest Taiwan trading day.
// On weekends/holidays the expectation rewinds to the most recent trading day
// using the package's existing isTaiwanTradingDay helper (currently weekends only).
//
// A04（2026-08-10 audit）：交易日盤前（09:00 CST 開盤前）Yahoo 尚未產生當日
// daily bar，最近有效資料是前一交易日。舊邏輯在平日 08:00 就要求當日 bar，
// 把「資料時間戳是昨天」誤判為 stale，進而加速 circuit breaker 開啟。
// 盤前容許前一交易日；盤中/盤後（09:00 之後）才要求當日 bar。
func twiiCacheTimestampIsCurrentTradingDay(ts int64) bool {
	dataDate := time.Unix(ts, 0).In(twseLocation).Truncate(24 * time.Hour)
	now := twiiNowFunc().In(twseLocation)
	expected := latestTaiwanTradingDay(now)
	if isTaiwanTradingDay(now) && now.Hour() < twseMarketOpenHour {
		// 盤前：最新已完成交易日是昨天（或上個交易日）
		expected = latestTaiwanTradingDay(now.AddDate(0, 0, -1))
	}
	return sameDate(dataDate, expected)
}

// twseMarketOpenHour 是台灣集中市場開盤時間（09:00 CST）。盤前 Yahoo 的
// ^TWII daily bar 仍是前一交易日的 close。
const twseMarketOpenHour = 9

// latestTaiwanTradingDay returns the most recent Taiwan trading day on or before t.
func latestTaiwanTradingDay(t time.Time) time.Time {
	for !isTaiwanTradingDay(t) {
		t = t.AddDate(0, 0, -1)
	}
	return t
}

// sameDate reports whether two times represent the same calendar date in twseLocation.
func sameDate(a, b time.Time) bool {
	return a.In(twseLocation).Format("20060102") == b.In(twseLocation).Format("20060102")
}

// key builds a composite key from interval and range.
func twiiKey(interval, rng string) string {
	return interval + "|" + rng
}

// get returns cached data if still valid for the given interval/range.
// Returns nil if cache is empty or expired.
func (c *taiwanIndexCache) get(interval, rng string) []byte {
	c.mu.RLock()
	defer c.mu.RUnlock()
	entry, ok := c.items[twiiKey(interval, rng)]
	if !ok || time.Now().After(entry.expires) {
		return nil
	}
	return entry.data
}

// invalidate removes the cached entry for the given interval/range.
// Used by TaiwanVolatilityProvider when it sees the cached Yahoo ^TWII
// response carries a RegularMarketTime from a previous trading day (e.g.
// Monday's close cached at 9:00, re-read at 9:01 on Tuesday still hits
// the 60s TTL but the data is no longer current). Without invalidate,
// every FetchSnapshot in the next 60s returns the stale error from
// twiiCacheTimestampIsCurrentTradingDay.
func (c *taiwanIndexCache) invalidate(interval, rng string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.items, twiiKey(interval, rng))
}

// set stores data with a TTL expiration.
func (c *taiwanIndexCache) set(data []byte, interval, rng string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items[twiiKey(interval, rng)] = taiwanIndexCacheEntry{
		data:    data,
		expires: time.Now().Add(twiiCacheTTL),
	}
}

// reset clears all cached data. Used by tests to ensure isolation.
func (c *taiwanIndexCache) reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items = make(map[string]taiwanIndexCacheEntry)
}
