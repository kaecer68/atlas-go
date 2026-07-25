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
