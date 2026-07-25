package marketdata

import (
	"sync"
	"time"
)

// usMarketCache avoids duplicate Yahoo Finance API calls for US market
// symbols during the same refresh batch cycle. Each entry is keyed by a
// composite of (symbol, interval, range) — the same symbol fetched with
// different parameters (e.g. 2d vs 5d range) produces separate cache
// entries. Cache TTL is 60 seconds.
//
// P1 B01+B02: us_spx/us_ndx/us_dji + us_nvda/us_aapl/us_msft/tsm_adr
// channel consolidation.
//
// P2: upgraded from symbol-only to param-keyed to align with twiiCache
// and support YahooFinanceMacroProvider.fetchIndicator cache integration.
type usMarketCacheEntry struct {
	data    []byte
	expires time.Time
}

type usMarketCache struct {
	mu    sync.RWMutex
	items map[string]usMarketCacheEntry
}

var usCache = &usMarketCache{
	items: make(map[string]usMarketCacheEntry),
}

const usCacheTTL = 60 * time.Second

// usKey builds a composite key from symbol, interval, and range.
func usKey(symbol, interval, rng string) string {
	return symbol + "|" + interval + "|" + rng
}

// get returns cached data for a symbol+params combination if still valid.
// Returns nil if cache is empty or expired.
func (c *usMarketCache) get(symbol, interval, rng string) []byte {
	c.mu.RLock()
	defer c.mu.RUnlock()
	entry, ok := c.items[usKey(symbol, interval, rng)]
	if !ok || time.Now().After(entry.expires) {
		return nil
	}
	return entry.data
}

// set stores data for a symbol+params combination with TTL expiration.
func (c *usMarketCache) set(symbol, interval, rng string, data []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items[usKey(symbol, interval, rng)] = usMarketCacheEntry{
		data:    data,
		expires: time.Now().Add(usCacheTTL),
	}
}

// reset clears all cached entries. Used by tests to ensure isolation.
func (c *usMarketCache) reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items = make(map[string]usMarketCacheEntry)
}
