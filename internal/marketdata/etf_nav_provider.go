package marketdata

import (
	"context"
	"fmt"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/logging"
)

// ETFNAVProvider fetches ETF Net Asset Value data for Taiwan-listed ETFs.
//
// Primary strategy: Uses ETF market closing prices as NAV proxy since
// Taiwanese ETFs trade within 0.1-0.5% of their NAV under normal conditions.
// This is acceptable for a simulation-first research system.
//
// When a real-time NAV data source becomes available (e.g., TWSE OpenAPI
// ETF NAV endpoint, FinMind TaiwanStockETF dataset, or fund company APIs),
// swap the fetcher implementation without changing the call sites.
type ETFNAVProvider struct {
	quoteFetcher QuoteFetcher
	cache        map[string]cachedNAV
	mu           sync.RWMutex
	cacheTTL     time.Duration
	limiter      *rate.Limiter
}

// QuoteFetcher fetches quotes for ETF symbols. Implemented by existing
// market data providers (TWSEClient, FugleClient, FinMindClient, etc.).
type QuoteFetcher interface {
	GetQuotes(ctx context.Context, asOf time.Time, symbols []string) ([]domain.Quote, error)
}

type cachedNAV struct {
	NAV       float64
	FetchedAt time.Time
}

// NewETFNAVProvider creates a new provider backed by a quote fetcher
// for market-price-as-NAV-proxy lookups.
func NewETFNAVProvider(fetcher QuoteFetcher) *ETFNAVProvider {
	return &ETFNAVProvider{
		quoteFetcher: fetcher,
		cache:        make(map[string]cachedNAV),
		cacheTTL:     4 * time.Hour,
		limiter:      rate.NewLimiter(rate.Every(2*time.Second), 5),
	}
}

// Name returns the provider name.
func (p *ETFNAVProvider) Name() string {
	return "etf_nav"
}

// FetchNAV fetches the NAV for a single ETF symbol.
// Uses cached value if fresh, otherwise fetches via the quote provider.
func (p *ETFNAVProvider) FetchNAV(ctx context.Context, symbol string) (float64, error) {
	if cached, ok := p.getCached(symbol); ok {
		return cached, nil
	}

	if err := p.limiter.Wait(ctx); err != nil {
		return 0, fmt.Errorf("etf_nav: rate limit wait: %w", err)
	}

	quotes, err := p.quoteFetcher.GetQuotes(ctx, time.Now(), []string{symbol})
	if err != nil {
		return 0, fmt.Errorf("etf_nav: fetch quote for %s: %w", symbol, err)
	}

	for _, q := range quotes {
		if q.Symbol == symbol && q.Last > 0 {
			p.setCached(symbol, q.Last)
			return q.Last, nil
		}
	}

	return 0, fmt.Errorf("etf_nav: no quote for %s", symbol)
}

// FetchNAVBatch fetches NAV for multiple ETF symbols in a single batch call.
// Returns a map of symbol → NAV. Symbols with errors are excluded.
func (p *ETFNAVProvider) FetchNAVBatch(ctx context.Context, symbols []string) (map[string]float64, error) {
	needsFetch := make([]string, 0, len(symbols))
	result := make(map[string]float64, len(symbols))

	for _, sym := range symbols {
		if cached, ok := p.getCached(sym); ok {
			result[sym] = cached
			continue
		}
		needsFetch = append(needsFetch, sym)
	}

	if len(needsFetch) == 0 {
		return result, nil
	}

	if err := p.limiter.Wait(ctx); err != nil {
		return result, fmt.Errorf("etf_nav: rate limit wait: %w", err)
	}

	quotes, err := p.quoteFetcher.GetQuotes(ctx, time.Now(), needsFetch)
	if err != nil {
		logging.Warn("etf_nav", "batch_fetch_failed", logging.Err(err))
		return result, nil
	}

	for _, q := range quotes {
		if q.Last > 0 {
			p.setCached(q.Symbol, q.Last)
			result[q.Symbol] = q.Last
		}
	}

	return result, nil
}

func (p *ETFNAVProvider) getCached(symbol string) (float64, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	c, ok := p.cache[symbol]
	if !ok || time.Since(c.FetchedAt) > p.cacheTTL {
		return 0, false
	}
	return c.NAV, true
}

func (p *ETFNAVProvider) setCached(symbol string, nav float64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.cache[symbol] = cachedNAV{NAV: nav, FetchedAt: time.Now()}
}

// ClearCache clears the in-memory NAV cache.
func (p *ETFNAVProvider) ClearCache() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.cache = make(map[string]cachedNAV)
}

// SetCacheTTL sets the cache TTL duration.
func (p *ETFNAVProvider) SetCacheTTL(ttl time.Duration) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.cacheTTL = ttl
}

// SetQuoteFetcher replaces the quote fetcher at runtime.
func (p *ETFNAVProvider) SetQuoteFetcher(f QuoteFetcher) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.quoteFetcher = f
}
