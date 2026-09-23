package apigateway

import (
	"fmt"
	"sync"
	"time"
)

// CacheLayer provides in-memory caching with TTL.
type CacheLayer struct {
	mu      sync.RWMutex
	entries map[string]*cacheEntry
	ttl     time.Duration
	// channelTTL overrides ttl for individual channels whose upstream cadence
	// is far slower than the 5-minute default (e.g. us_cpi: CPI-U is published
	// monthly, and its adapter carries a 1-request/hour limiter — see
	// NewGateway). Empty/nil means "use the default ttl for every channel".
	channelTTL map[string]time.Duration
}

type cacheEntry struct {
	result   *FetchResult
	cachedAt time.Time
}

// NewCacheLayer creates a cache with default 5-minute TTL.
func NewCacheLayer() *CacheLayer {
	return &CacheLayer{
		entries:    make(map[string]*cacheEntry),
		ttl:        5 * time.Minute,
		channelTTL: make(map[string]time.Duration),
	}
}

// SetChannelTTL overrides the cache TTL for one channel. A non-positive ttl
// restores the default. Callers use this to align the cache lifetime with the
// channel contract's ExpectedRefresh so a slow upstream is not re-fetched on
// every fan-out batch.
func (c *CacheLayer) SetChannelTTL(channelID string, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.channelTTL == nil {
		c.channelTTL = make(map[string]time.Duration)
	}
	if ttl <= 0 {
		delete(c.channelTTL, channelID)
		return
	}
	c.channelTTL[channelID] = ttl
}

// TTLFor reports the effective TTL for a channel (the per-channel override
// when present, otherwise the layer default).
func (c *CacheLayer) TTLFor(channelID string) time.Duration {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.ttlFor(channelID)
}

// ttlFor returns the effective TTL for a channel. Callers must hold at least
// the read lock.
func (c *CacheLayer) ttlFor(channelID string) time.Duration {
	if ttl, ok := c.channelTTL[channelID]; ok && ttl > 0 {
		return ttl
	}
	return c.ttl
}

// Get retrieves a cached result if not expired.
func (c *CacheLayer) Get(channelID string) *FetchResult {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, ok := c.entries[channelID]
	if !ok {
		return nil
	}

	if time.Since(entry.cachedAt) > c.ttlFor(channelID) {
		return nil
	}

	return entry.result
}

// Set stores a result in the cache.
func (c *CacheLayer) Set(channelID string, result *FetchResult) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries[channelID] = &cacheEntry{
		result:   result,
		cachedAt: time.Now(),
	}
}

// Invalidate removes a channel from the cache.
func (c *CacheLayer) Invalidate(channelID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, channelID)
}

// InvalidateAll clears all cached entries.
func (c *CacheLayer) InvalidateAll() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[string]*cacheEntry)
}

// ChannelRegistry holds all registered data providers.
type ChannelRegistry struct {
	providers map[string]DataProvider
	limiters  *RateLimitManager
	breakers  *CircuitBreakerManager
	mu        sync.RWMutex
}

// NewChannelRegistry creates a registry.
func NewChannelRegistry(limiters *RateLimitManager, breakers *CircuitBreakerManager) *ChannelRegistry {
	return &ChannelRegistry{
		providers: make(map[string]DataProvider),
		limiters:  limiters,
		breakers:  breakers,
	}
}

// Register adds a provider for a channel.
func (r *ChannelRegistry) Register(channelID string, provider DataProvider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.providers[channelID] = provider
}

// Get returns the provider for a channel.
func (r *ChannelRegistry) Get(channelID string) (DataProvider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.providers[channelID]
	if !ok {
		return nil, fmt.Errorf("channel not registered: %s", channelID)
	}
	return p, nil
}

// List returns all registered channel IDs.
func (r *ChannelRegistry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ids := make([]string, 0, len(r.providers))
	for id := range r.providers {
		ids = append(ids, id)
	}
	return ids
}
