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
