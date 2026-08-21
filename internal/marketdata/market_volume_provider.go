package marketdata

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"golang.org/x/time/rate"

	"github.com/kaecer68/atlas-go/internal/apigateway/httpclient"
	"github.com/kaecer68/atlas-go/internal/constants"
)

// twseBrowserUserAgent is a full browser User-Agent required to pass the
// TWSE WAF (short UAs such as "Mozilla/5.0" get 403 Forbidden).
const twseBrowserUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/115.0 Safari/537.36"

// MarketVolumeResult holds the parsed 集中市場成交金額 from TWSE MI_INDEX.
// MarketVolume is in 億元 (hundred million NTD), matching the applyMarketVolume contract.
type MarketVolumeResult struct {
	MarketVolume float64 `json:"market_volume"` // 億元
	Date         string  `json:"date"`          // YYYYMMDD
}

// MarketVolumeProvider fetches 集中市場大盤統計資訊 (成交金額) from TWSE.
//
// Data source: TWSE exchangeReport/MI_INDEX?type=MS.
// Returns the 一般股票 成交金額 converted from 元 to 億元.
type MarketVolumeProvider struct {
	client      *http.Client
	baseURL     string
	rateLimiter *rate.Limiter
}

// NewMarketVolumeProvider creates a new TWSE market volume provider.
func NewMarketVolumeProvider() *MarketVolumeProvider {
	return &MarketVolumeProvider{
		client:      httpclient.NewFactory().NewClient(20 * time.Second),
		baseURL:     constants.TWSEBaseURL,
		rateLimiter: rate.NewLimiter(rate.Every(5*time.Second), 1),
	}
}

// SetHTTPClient sets a custom HTTP client for tests.
func (p *MarketVolumeProvider) SetHTTPClient(client *http.Client) {
	if client != nil {
		p.client = client
	}
}

// SetRateLimiter overrides the rate limiter (for testing).
func (p *MarketVolumeProvider) SetRateLimiter(lim *rate.Limiter) {
	if lim != nil {
		p.rateLimiter = lim
	}
}

// Name returns the provider name.
func (p *MarketVolumeProvider) Name() string {
	return "market_volume"
}

// FetchLatest retrieves the most recent 集中市場成交金額.
// Scans up to 7 days back to find the latest available trading day.
func (p *MarketVolumeProvider) FetchLatest(ctx context.Context) (*MarketVolumeResult, error) {
	now := time.Now().UTC()
	for i := range 7 {
		dateStr := now.AddDate(0, 0, -i).Format("20060102")
		result, err := p.fetchDate(ctx, dateStr)
		if err == nil {
			return result, nil
		}
	}
	return nil, fmt.Errorf("no TWSE market volume data available in the last 7 days")
}

// FetchDate retrieves 集中市場成交金額 for a specific trading date (YYYYMMDD).
// Non-trading days (weekends/holidays) return an error so callers can skip them.
// Exported for historical backfill (cmd/backfill-market-volume); FetchLatest
// keeps scanning the recent window via the private fetchDate.
func (p *MarketVolumeProvider) FetchDate(ctx context.Context, dateStr string) (*MarketVolumeResult, error) {
	return p.fetchDate(ctx, dateStr)
}

func (p *MarketVolumeProvider) fetchDate(ctx context.Context, dateStr string) (*MarketVolumeResult, error) {
	if err := p.rateLimiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limit wait: %w", err)
	}

	url := fmt.Sprintf("%s/exchangeReport/MI_INDEX?response=json&date=%s&type=MS", p.baseURL, dateStr)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	// Full browser UA: TWSE's WAF returns 403 for short UAs (observed
	// 2026-08-21 during R4 backfill). Matches government_broker_aggregator.
	req.Header.Set("User-Agent", twseBrowserUserAgent)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	var apiResp twseMIIndexResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	if apiResp.Stat != "OK" || len(apiResp.Tables) < 7 {
		return nil, fmt.Errorf("TWSE MI_INDEX returned no data: stat=%s tables=%d", apiResp.Stat, len(apiResp.Tables))
	}

	// Table index 6 = 大盤統計資訊
	marketTable := apiResp.Tables[6]
	if len(marketTable.Data) == 0 {
		return nil, fmt.Errorf("TWSE MI_INDEX returned empty market stats")
	}

	// Row 0 = "1.一般股票", column 1 = 成交金額(元)
	row := marketTable.Data[0]
	if len(row) < 2 {
		return nil, fmt.Errorf("TWSE MI_INDEX market stats: insufficient columns")
	}

	amountYuan := parseTWSEFloat(row[1])
	if amountYuan <= 0 {
		return nil, fmt.Errorf("TWSE MI_INDEX market stats: non-positive amount: %.0f", amountYuan)
	}

	// Convert 元 → 億元
	amountYi := amountYuan / 100_000_000

	return &MarketVolumeResult{
		MarketVolume: amountYi,
		Date:         dateStr,
	}, nil
}

// twseMIIndexResponse mirrors the TWSE MI_INDEX JSON response.
type twseMIIndexResponse struct {
	Stat   string        `json:"stat"`
	Date   string        `json:"date"`
	Tables []twseMITable `json:"tables"`
}

type twseMITable struct {
	Title  string     `json:"title"`
	Fields []string   `json:"fields"`
	Data   [][]string `json:"data"`
	Notes  []string   `json:"notes"`
}
