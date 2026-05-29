package marketdata

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"golang.org/x/time/rate"

	"github.com/kaecer68/atlas-go/internal/apigateway/httpclient"
	"github.com/kaecer68/atlas-go/internal/logging"
)

// Source indicates which tier provided the ETF NAV value.
type Source int

const (
	// SourceTWSE means the NAV came from a real TWSE data source.
	SourceTWSE Source = iota
	// SourceProxy means the NAV is a close-price proxy (market price
	// used in lieu of real NAV data).
	SourceProxy
)

// String returns a human-readable source label.
func (s Source) String() string {
	switch s {
	case SourceTWSE:
		return "twse"
	case SourceProxy:
		return "proxy"
	default:
		return "unknown"
	}
}

// TWSEETFNAVScraper fetches ETF Net Asset Value using a tiered strategy:
//
//	Tier 1 — TWSE HTML scraping (attempts real NAV when an endpoint exists).
//	Tier 2 — Close-price proxy via the configured QuoteFetcher (always works).
//
// If Tier 1 fails for any reason, the scraper silently falls back to Tier 2
// so that callers always receive a best-effort NAV value.
// TWSEETFNAVScraper attempts to fetch real ETF NAV from available data sources,
// falling back to a close-price proxy when no real NAV is available.
//
// Data source investigation (2026-05-29):
//
//   Priority 1 — Fubon (富邦證券): sdk.marketdata.rest_client.stock only provides
//     intraday OHLCV via client.intraday.quote(). No ETF NAV/fund API in the proxy.
//     Confirmed: 4 endpoints (/health, /quote, /quotes, /market-status), all OHLCV.
//
//   Priority 2 — TWSE OpenAPI: openapi.twse.com.tw/v1/ETFReport/ETFNAV → 302 HTML.
//     www.twse.com.tw/fund/BFIBMS → 302 redirect. mis.twse.com.tw/stock/api/
//     getETFNetValue.jsp → HTML only. No free REST API for ETF NAV exists.
//
//   Priority 3 — Fugle: fugle_client.go provides intraday quote/meta only. No NAV.
//
//   Priority 4 — TEJ (台灣經濟新報): tej_provider.go only implements TRAIL/TAPRCD
//     (stock OHLCV) and TWN/AFINA (financial statements). No ETF NAV dataset.
//
//   Priority 5 — FinMind: finmind_client.go uses TaiwanStockPrice (OHLCV), no
//     TaiwanStockETF dataset is implemented. TaiwanStockETF exists in FinMind's
//     catalog but requires a paid token refreshed every 7 days.
//
// Future iteration: When FinMind paid registration is completed, implement
//   finmind_client.GetETFNAV() → TWSEETFNAVScraper → SourceFinMind path.
//   The ETFNAVFetcher interface and TWSEETFNAVScraper are designed for this:
//   1. Add TaiwanStockETF dataset to FinMindClient
//   2. Implement attemptFinMindFetch() in this file
//   3. Add SourceFinMind to the Source enum
//   4. Update priority in FetchNAV() to try FinMind before close-price proxy
//
// Current strategy: Tier 1 (TWSE scrape) is a documented stub. Tier 2 (close-price
// proxy) uses the configured QuoteFetcher and is the only working path today.
// Taiwan ETFs trade within 0.1–0.5% of NAV, making close prices a reliable proxy.
type TWSEETFNAVScraper struct {
	client    *http.Client
	limiter   *rate.Limiter
	quoteProv QuoteFetcher
}

// NewTWSEETFNAVScraper creates a scraper with the given QuoteFetcher as the
// Tier-2 fallback. The QuoteFetcher is typically the system's primary market
// data provider (HybridProvider or TWSEOpenAPIProvider).
func NewTWSEETFNAVScraper(quoteProv QuoteFetcher) *TWSEETFNAVScraper {
	return &TWSEETFNAVScraper{
		client:    httpclient.NewFactory().NewClient(15 * time.Second),
		limiter:   rate.NewLimiter(rate.Every(5*time.Second), 1),
		quoteProv: quoteProv,
	}
}

// FetchNAV attempts to retrieve the ETF NAV for symbol. Returns the NAV
// value, the source that provided it, and any error. A non-nil error means
// both tiers failed; otherwise at least the close-price proxy is returned.
func (s *TWSEETFNAVScraper) FetchNAV(ctx context.Context, symbol string) (float64, Source, error) {
	// Rate limit before any external call.
	if err := s.limiter.Wait(ctx); err != nil {
		return 0, SourceProxy, fmt.Errorf("etf_nav_scraper: rate limit wait: %w", err)
	}

	// Tier 1: attempt TWSE scrape.
	if nav, err := s.attemptTWSEFetch(ctx, symbol); err == nil {
		logging.Info("etf_nav_scraper", "nav_from_twse",
			logging.FStr("symbol", symbol),
			logging.FStr("source", SourceTWSE.String()))
		return nav, SourceTWSE, nil
	}

	// Tier 2: fall back to close-price proxy.
	nav, err := s.proxyFallback(ctx, symbol)
	if err != nil {
		return 0, SourceProxy, fmt.Errorf("etf_nav_scraper: proxy fallback for %s: %w", symbol, err)
	}

	logging.Info("etf_nav_scraper", "nav_from_proxy",
		logging.FStr("symbol", symbol),
		logging.FStr("source", SourceProxy.String()))

	return nav, SourceProxy, nil
}

// attemptTWSEFetch tries to retrieve real ETF NAV from TWSE.
//
// Currently a stub — TWSE does not expose ETF NAV through a free REST API.
// When a working endpoint is discovered, this function should be updated
// to perform HTML scraping or API calls against that endpoint.
//
// TODO(FINMIND): When FinMind paid registration is completed, add
//   attemptFinMindFetch() using TaiwanStockETF dataset, and wire it as
//   a new tier between TWSE and the close-price proxy. See the type-level
//   doc comment for the 4-step iteration plan.
func (s *TWSEETFNAVScraper) attemptTWSEFetch(ctx context.Context, symbol string) (float64, error) {
	// Strip .TW suffix for TWSE API calls (TWSE uses numeric-only symbols).
	twseSymbol := strings.TrimSuffix(symbol, ".TW")

	_ = twseSymbol

	// TODO: implement when a working TWSE ETF NAV endpoint is found.
	// Candidates tested and known NOT to work (all redirect to HTML or 404):
	//   - openapi.twse.com.tw/v1/ETFReport/ETFNAV
	//   - www.twse.com.tw/fund/BFIBMS
	//   - mis.twse.com.tw/stock/api/getETFNetValue.jsp
	//   - data.gov.tw dataset #11109

	return 0, fmt.Errorf("etf_nav_scraper: no working TWSE ETF NAV endpoint for %s", symbol)
}

// proxyFallback retrieves the current market close price for symbol via
// the configured QuoteFetcher and uses it as a NAV proxy.
func (s *TWSEETFNAVScraper) proxyFallback(ctx context.Context, symbol string) (float64, error) {
	quotes, err := s.quoteProv.GetQuotes(ctx, time.Now(), []string{symbol})
	if err != nil {
		return 0, fmt.Errorf("proxy fallback: get quotes: %w", err)
	}

	for _, q := range quotes {
		if q.Symbol == symbol && q.Last > 0 {
			return q.Last, nil
		}
	}

	return 0, fmt.Errorf("proxy fallback: no quote for %s", symbol)
}

// SetQuoteFetcher replaces the Tier-2 fallback quote provider at runtime.
// This is useful when the primary market data provider changes after startup
// (e.g., HybridProvider switching between Fugle and TWSE).
func (s *TWSEETFNAVScraper) SetQuoteFetcher(f QuoteFetcher) {
	s.quoteProv = f
}
