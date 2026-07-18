package marketdata

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"golang.org/x/time/rate"

	"github.com/kaecer68/atlas-go/internal/apigateway/httpclient"
)

// EquityDispersionRecord holds a single row from the TDCC equity dispersion table.
// Each row represents one shareholding tier for one stock.
type EquityDispersionRecord struct {
	Date       string  `json:"date"`
	Symbol     string  `json:"symbol"`
	Tier       string  `json:"tier"`        // e.g. "1-999", "1000-5000", ">400000"
	Holders    int     `json:"holders"`     // number of shareholders in this tier
	SharesHeld int64   `json:"shares_held"` // total shares held by this tier
	PctHeld    float64 `json:"pct_held"`    // percentage of total outstanding shares
}

// TDCClient is a placeholder provider for TDCC (臺灣集中保管結算所)
// equity dispersion data (G01).
//
// Data source: TDCC OpenAPI — 集保戶股權分散表查詢
// Base URL: https://openapi.tdcc.com.tw/
//
// NOTE: TDCC API integration requires:
//  1. A TDCC API key (application required)
//  2. Confirmation of the exact endpoint path
//  3. Understanding of the authentication mechanism
//
// Until the above are confirmed, this provider returns placeholder results.
// The channel is registered but not scheduled for automatic fetch.
type TDCClient struct {
	client  *http.Client
	baseURL string
	limiter *rate.Limiter
}

// NewTDCClient creates a TDCC equity dispersion provider.
func NewTDCClient() *TDCClient {
	return &TDCClient{
		client:  httpclient.NewFactory().NewClient(20 * time.Second),
		baseURL: "https://openapi.tdcc.com.tw",
		limiter: rate.NewLimiter(rate.Limit(0.2), 1), // conservative: 1 req / 5s
	}
}

// Name identifies this provider.
func (p *TDCClient) Name() string { return "tdcc_equity_dispersion" }

// RateLimiter returns the per-provider rate limiter.
func (p *TDCClient) RateLimiter() *rate.Limiter { return p.limiter }

// FetchDispersion fetches the weekly equity dispersion table from TDCC.
//
// TODO(G01): Implement when TDCC API access is confirmed.
func (p *TDCClient) FetchDispersion(ctx context.Context, date string) ([]EquityDispersionRecord, error) {
	_ = ctx
	_ = date
	return nil, fmt.Errorf("tdcc: API access not yet configured; see G01 implementation notes in internal/marketdata/tdcc_provider.go")
}

// SetHTTPClient overrides the HTTP client (for testing).
func (p *TDCClient) SetHTTPClient(c *http.Client) { p.client = c }
