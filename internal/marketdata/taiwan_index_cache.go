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
type taiwanIndexCache struct {
	mu       sync.RWMutex
	data     []byte
	expires  time.Time
	interval string // e.g. "1d"
	rng      string // e.g. "3mo"
}

var twiiCache = &taiwanIndexCache{}

const twiiCacheTTL = 60 * time.Second

// get returns cached data if still valid for the given interval/range.
// Returns nil if cache is empty or expired.
func (c *taiwanIndexCache) get(interval, rng string) []byte {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.data == nil || time.Now().After(c.expires) {
		return nil
	}
	if c.interval != interval || c.rng != rng {
		return nil
	}
	return c.data
}

// set stores data with a TTL expiration.
func (c *taiwanIndexCache) set(data []byte, interval, rng string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data = data
	c.interval = interval
	c.rng = rng
	c.expires = time.Now().Add(twiiCacheTTL)
}
