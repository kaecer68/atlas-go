package marketdata

import (
	"sync"
	"time"
)

// usMarketCache avoids duplicate Yahoo Finance API calls for US market
// symbols during the same us_market_refresh batch cycle. Each symbol's
// response is cached for 60 seconds — longer than the typical cold-start
// duration (~5s), so all 7 US index/tech/ADR channels share results
// within a single refresh.
//
// P1 B01+B02: us_spx/us_ndx/us_dji + us_nvda/us_aapl/us_msft/tsm_adr
// channel consolidation.
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

// get returns cached data for a symbol if still valid.
// Returns nil if cache is empty or expired.
func (c *usMarketCache) get(symbol string) []byte {
	c.mu.RLock()
	defer c.mu.RUnlock()
	entry, ok := c.items[symbol]
	if !ok || time.Now().After(entry.expires) {
		return nil
	}
	return entry.data
}

// set stores data for a symbol with TTL expiration.
func (c *usMarketCache) set(symbol string, data []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items[symbol] = usMarketCacheEntry{
		data:    data,
		expires: time.Now().Add(usCacheTTL),
	}
}
