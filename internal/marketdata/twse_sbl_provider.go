package marketdata

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"golang.org/x/time/rate"

	"github.com/kaecer68/atlas-go/internal/apigateway/httpclient"
	"github.com/kaecer68/atlas-go/internal/constants"
)

// SBLStats holds securities borrowing & lending statistics for a single stock.
type SBLStats struct {
	Date             string `json:"date"`
	Symbol           string `json:"symbol"`
	SBLShortBalance  int64  `json:"sbl_short_balance"`  // 借券賣出餘額（股）
	SBLShortVolume   int64  `json:"sbl_short_volume"`   // 當日借券賣出股數
	SBLReturnVolume  int64  `json:"sbl_return_volume"`  // 當日還券股數
	SBLBorrowBalance int64  `json:"sbl_borrow_balance"` // 借券餘額（股）
}

// TWSESBLProvider fetches securities borrowing & lending data from TWSE.
// Data source: TWSE 借券賣出餘額表（TWT93U）。
//
// The TWSE endpoint for SBL data uses a different URL pattern than the
// standard OpenAPI. Currently the provider records a placeholder until
// the exact endpoint is confirmed. When the TWSE SBL API is live:
//   - Set endpoint via SetBaseURL
//   - Implement FetchSBL() with actual HTTP call + JSON parsing
type TWSESBLProvider struct {
	client  *http.Client
	baseURL string
	limiter *rate.Limiter
}

// NewTWSESBLProvider creates a TWSE SBL data provider.
// ratePerSec defaults to 0.5 (1 request per 2 seconds) per TWSE rate-limiting.
func NewTWSESBLProvider(ratePerSec float64) *TWSESBLProvider {
	// ratePerSec is accepted for API compatibility but the provider now
	// shares the single TWSE token bucket (P1-13): 11 independent limiters
	// against the same host could collectively exceed the documented policy.
	_ = ratePerSec
	return &TWSESBLProvider{
		client:  httpclient.NewFactory().NewClient(20 * time.Second),
		baseURL: constants.TWSEBaseURL,
		limiter: getTWSESharedLimiter(),
	}
}

// SetHTTPClient overrides the HTTP client (for testing).
func (p *TWSESBLProvider) SetHTTPClient(c *http.Client) { p.client = c }

// SetBaseURL overrides the base URL (for testing).
func (p *TWSESBLProvider) SetBaseURL(u string) { p.baseURL = u }

// Name identifies this provider.
func (p *TWSESBLProvider) Name() string { return "twse_sbl" }

// SetRateLimiter overrides the rate limiter (tests only; P1-13 shared-bucket
// tests use SetTWSESharedLimiterForTest instead).
func (p *TWSESBLProvider) SetRateLimiter(l *rate.Limiter) {
	if l != nil {
		p.limiter = l
	}
}

// RateLimiter returns the per-provider rate limiter.
func (p *TWSESBLProvider) RateLimiter() *rate.Limiter { return p.limiter }

// FetchSBLSummary fetches the daily SBL summary data from TWSE.
// Returns SBLStats for all listed stocks.
//
// TODO(G02): Implement actual HTTP fetch against TWSE SBL endpoint.
// The TWSE endpoint is likely of the form:
//
//	https://www.twse.com.tw/rwd/zh/lending/TWT93U?date=YYYYMMDD&response=json
//
// Confirmation is needed on the exact endpoint and response format.
// Until then, this returns a placeholder indicating the data is unavailable.
func (p *TWSESBLProvider) FetchSBLSummary(ctx context.Context, date string) ([]SBLStats, error) {
	_ = ctx
	_ = date
	// Placeholder: return empty results until TWSE endpoint is confirmed.
	// The caller (ChannelAdapter) will record the fetch attempt and surface
	// the "data unavailable" status through the channel health system.
	return nil, fmt.Errorf("twse_sbl: endpoint not yet confirmed; see G02 implementation notes in internal/marketdata/twse_sbl_provider.go")
}

// Ensure time import is used.
var _ = time.Now
