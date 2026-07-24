package apigateway

import (
	"context"
	"path/filepath"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Gateway is the unified entry point for all data channels.
type Gateway struct {
	registry *ChannelRegistry
	limiters *RateLimitManager
	breakers *CircuitBreakerManager
	health   *UnifiedHealthStore
	cache    *CacheLayer
}

// NewGateway creates a fully initialized gateway.
func NewGateway(workDir string, pool *pgxpool.Pool) (*Gateway, error) {
	// Initialize in dependency order:
	// 1. Cache (no deps)
	// 2. Health store (no deps)
	// 3. Rate limiters (no deps)
	// 4. Circuit breakers (no deps)
	// 5. Channel registry (needs limiters + breakers)
	// 6. Gateway (needs all)

	cache := NewCacheLayer()
	health := NewUnifiedHealthStore(filepath.Join(workDir, "data/state"), pool)
	limiters := NewRateLimitManager()
	breakers := NewCircuitBreakerManagerWithThresholds(map[string]int{
		"twse_capital_flow": 5,
		"twse_margin":       5,
		"twse_replay":       5,
		"twse_oddlot":       5,
		"twse_etf":          5,
		"export_statistics": 5,
		"tsmc_revenue":      5,
	}, CircuitBreakerFailureThreshold, channelIDs())
	registry := NewChannelRegistry(limiters, breakers)

	return &Gateway{
		registry: registry,
		limiters: limiters,
		breakers: breakers,
		health:   health,
		cache:    cache,
	}, nil
}

// Fetch retrieves data from a channel with rate limiting, circuit breaking,
// health tracking, and caching.
func (g *Gateway) Fetch(ctx context.Context, channelID string) (*FetchResult, error) {
	// 1. Check circuit breaker
	breaker, err := g.breakers.Get(channelID)
	if err != nil {
		return nil, err
	}

	var result *FetchResult
	callErr := breaker.Call(func() error {
		// 2. Check cache first
		if cached := g.cache.Get(channelID); cached != nil && !cached.Stale {
			result = cached
			return nil
		}

		// 3. Fetch from provider
		provider, err := g.registry.Get(channelID)
		if err != nil {
			return err
		}

		result, err = provider.Fetch(ctx)
		if err != nil {
			return err
		}

		// 4. Update cache
		g.cache.Set(channelID, result)

		// 5. Record health
		_ = g.health.Record(channelID, "ok", "", WithLatencyMs(result.Meta.LatencyMs))

		return nil
	})

	if callErr != nil {
		if breaker.IsOpen() {
			// Return stale cache if available, marked as fallback so callers know
			// the data is last-known-good and not a fresh fetch (Layer 1 of the
			// 4-layer data-visibility safeguard).
			if stale := g.cache.Get(channelID); stale != nil {
				// Update last_fetch_at on the health record so stale
				// threshold (PR #844) can determine record freshness.
				// Without this, CB-open channels with transient failures
				// retain ancient last_fetch_at timestamps that look
				// like current alerts forever.
				_ = g.health.Record(channelID, "warn", callErr.Error())
				stale.Stale = true
				stale.Fallback = true
				stale.LastError = callErr.Error()
				stale.Meta.Stale = true
				stale.Meta.Fallback = true
				stale.Meta.LastError = callErr.Error()
				return stale, nil
			}
		}
		_ = g.health.Record(channelID, "error", callErr.Error())
		return nil, callErr
	}

	return result, nil
}

// HealthCheck performs a health check for a channel.
func (g *Gateway) HealthCheck(ctx context.Context, channelID string) (HealthStatus, error) {
	provider, err := g.registry.Get(channelID)
	if err != nil {
		return HealthStatus{}, err
	}
	return provider.HealthCheck(ctx)
}

// RateLimitStatus returns rate limit status for all channels.
func (g *Gateway) RateLimitStatus() map[string]RateLimitStatus {
	return g.limiters.Status()
}

// BreakerStatus returns circuit breaker status for all channels.
func (g *Gateway) BreakerStatus() map[string]CircuitBreakerStatus {
	return g.breakers.Status()
}

// ForceOpenChannel manually opens a channel's circuit breaker.
// Used for crisis-driven emergency halt (e.g., MacroRiskAssessment.CrisisActive).
func (g *Gateway) ForceOpenChannel(channelID string) error {
	return g.breakers.ForceOpen(channelID)
}

// Health returns the unified health store.
func (g *Gateway) Health() *UnifiedHealthStore {
	return g.health
}

// HasChannel reports whether a channel is registered.
func (g *Gateway) HasChannel(channelID string) bool {
	_, err := g.registry.Get(channelID)
	return err == nil
}

// Summary returns a concise channelID→status map for monitoring purposes.
func (g *Gateway) Summary() map[string]string {
	statuses := g.health.StatusSummary()
	result := make(map[string]string, len(statuses))
	for id, s := range statuses {
		result[id] = s.Status
	}
	return result
}

// ChannelIDs returns all registered channel IDs.
func (g *Gateway) ChannelIDs() []string {
	return channelIDs()
}

func channelIDs() []string {
	return []string{
		"us_yahoo",
		"twse_replay",
		"twse_capital_flow",
		"fugle",
		"fubon",
		"finmind",
		"frankfurter_fx",
		"geopolitical",
		"twse_margin",
		"export_statistics",
		"tsmc_revenue",
		"geopolitical_taiwan",
		"janus_regime",
		"tej",
		"exchange_rate",
		"sox_index",
		"dram_spot_price",
		"twse_sector_index",
		"sector_data",
		"day_trading",
		"bdi",
		"taifex_daily",
		"taifex_institutional",
		"tdcc_equity_dispersion",
		"twse_oddlot",
		"twse_sbl",
		"government_flow",
		"twse_etf",
		"us_spx",
		"us_ndx",
		"us_dji",
		"taiex_index",
		"tw_vol",
		"us_nvda",
		"us_aapl",
		"us_msft",
		"tsm_adr",
	}
}
