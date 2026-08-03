package marketdata

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"github.com/kaecer68/atlas-go/internal/apigateway/httpclient"
	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/logging"
)

const (
	// v1.0 marketdata API (2026-08-03 migration). The legacy realtime/v0.3
	// endpoint is retired — see developer.fugle.tw/docs/data/migration-guide/.
	// The API key itself is version-agnostic (base64 in .env); only the
	// endpoint changed. Verified live 2026-08-03: stock_get_quote 2330/2317/
	// 0050 all return 200 via Fugle v1.0 with the existing key.
	fugleAPIBaseURL = "https://api.fugle.tw/marketdata/v1.0/stock"
)

// FugleClient Fugle API 客户端
type FugleClient struct {
	apiKey      string
	httpClient  *http.Client
	baseURL     string
	rateLimiter *rate.Limiter
}

// FugleQuoteResponse Fugle v1.0 行情响应（扁平結構）
// 對照 v0.3 巢狀 data.quote.* 結構，v1.0 將欄位提升到頂層。
type FugleQuoteResponse struct {
	APIVersion string `json:"apiVersion"`
	Date       string `json:"date"`
	Type       string `json:"type"`
	Exchange   string `json:"exchange"`
	Market     string `json:"market"`
	Symbol     string `json:"symbol"`
	Name       string `json:"name"`

	ClosePrice float64 `json:"closePrice"`
	OpenPrice  float64 `json:"openPrice"`
	HighPrice  float64 `json:"highPrice"`
	LowPrice   float64 `json:"lowPrice"`
	LastPrice  float64 `json:"lastPrice"`

	Total struct {
		TradeVolume int64 `json:"tradeVolume"`
	} `json:"total"`
}

// getFugleRateLimit returns the rate limit based on FUGLE_TIER env var.
// free: 60/min (default), developer: 600/min, advanced: 2000/min
func getFugleRateLimit() int {
	switch config.GetSecret("FUGLE_TIER") {
	case "developer":
		return 600
	case "advanced":
		return 2000
	default:
		return 60 // free tier
	}
}

var (
	sharedFugleClient     *FugleClient
	sharedFugleClientOnce sync.Once
	sharedFugleClientMu   sync.RWMutex
)

// GetSharedFugleClient returns a singleton FugleClient shared across all
// components (gateway channel, hybrid provider). Using one client ensures
// a single rate limiter enforces the Fugle API tier limit globally.
func GetSharedFugleClient(apiKey string) *FugleClient {
	sharedFugleClientOnce.Do(func() {
		sharedFugleClient = newFugleClient(apiKey)
	})
	return sharedFugleClient
}

// ResetSharedFugleClient clears the singleton (for tests).
func ResetSharedFugleClient() {
	sharedFugleClientMu.Lock()
	defer sharedFugleClientMu.Unlock()
	sharedFugleClient = nil
	sharedFugleClientOnce = sync.Once{}
}

// NewFugleClient creates a standalone FugleClient with its own rate limiter.
// Prefer GetSharedFugleClient in production to avoid multiple independent
// rate limiter token buckets.
func NewFugleClient(apiKey string) *FugleClient {
	return newFugleClient(apiKey)
}

func newFugleClient(apiKey string) *FugleClient {
	params := config.GetParametersConfig()
	limit := getFugleRateLimit()
	// Override with parameters config if explicitly set higher than default free tier
	if params.Marketdata.FugleRateLimit.Value > 60 {
		limit = params.Marketdata.FugleRateLimit.Value
	}
	logging.Info("fugle", "client_initialized", "tier", config.GetSecret("FUGLE_TIER"), "rate_limit", limit)
	timeout := time.Duration(params.Marketdata.FugleAPITimeoutSec.Value) * time.Second
	return &FugleClient{
		apiKey:      apiKey,
		httpClient:  httpclient.NewFactory().NewClient(timeout),
		baseURL:     fugleAPIBaseURL,
		rateLimiter: rate.NewLimiter(rate.Every(time.Minute/time.Duration(limit)), limit),
	}
}

// SetHTTPClient 设置自定义 HTTP 客户端
func (c *FugleClient) SetHTTPClient(client *http.Client) {
	c.httpClient = client
}

// RateLimiter returns the rate limiter for Gateway adapter registration.
func (c *FugleClient) RateLimiter() *rate.Limiter {
	return c.rateLimiter
}

// GetQuote 获取单个股票行情（v1.0 API）
func (c *FugleClient) GetQuote(ctx context.Context, symbol string) (domain.Quote, error) {
	// 等待速率限制
	if err := c.rateLimiter.Wait(ctx); err != nil {
		return domain.Quote{}, fmt.Errorf("rate limit wait: %w", ErrRateLimited)
	}

	// v1.0: symbol 在 URL path，key 在 X-API-KEY header（非 query param）
	endpoint := fmt.Sprintf("%s/intraday/quote/%s", c.baseURL, url.PathEscape(symbol))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return domain.Quote{}, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("X-API-KEY", c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return domain.Quote{}, fmt.Errorf("http request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return domain.Quote{}, fmt.Errorf("api error: status %d, body: %s", resp.StatusCode, string(body))
	}

	// 解析响应
	var fugleResp FugleQuoteResponse
	if err := json.NewDecoder(resp.Body).Decode(&fugleResp); err != nil {
		return domain.Quote{}, fmt.Errorf("decode response: %w", err)
	}

	// v1.0 扁平欄位 → domain.Quote（lastPrice 優先，fallback closePrice）
	last := fugleResp.LastPrice
	if last == 0 {
		last = fugleResp.ClosePrice
	}
	quote := domain.Quote{
		Symbol:     symbol,
		Last:       last,
		Open:       fugleResp.OpenPrice,
		High:       fugleResp.HighPrice,
		Low:        fugleResp.LowPrice,
		Volume:     fugleResp.Total.TradeVolume,
		Market:     "TW",
		AsOf:       time.Now(),
		IsTradable: true,
		Source:     "fugle",
	}

	return quote, nil
}

// GetQuotes 批量获取股票行情
func (c *FugleClient) GetQuotes(ctx context.Context, symbols []string) ([]domain.Quote, error) {
	quotes := make([]domain.Quote, 0, len(symbols))

	for _, symbol := range symbols {
		quote, err := c.GetQuote(ctx, symbol)
		if err != nil {
			// 记录错误但继续获取其他股票
			logging.Error("fugle", "fetch_failed", "symbol", symbol, logging.Err(err))
			continue
		}
		quotes = append(quotes, quote)
	}

	return quotes, nil
}

// GetMeta 获取股票元数据（v1.0: GET /intraday/ticker/{symbol}）
// v0.3 Meta → v1.0 Ticker（官方 migration-guide）
func (c *FugleClient) GetMeta(ctx context.Context, symbol string) (*FugleMetaResponse, error) {
	if err := c.rateLimiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limit wait: %w", ErrRateLimited)
	}

	endpoint := fmt.Sprintf("%s/intraday/ticker/%s", c.baseURL, url.PathEscape(symbol))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("X-API-KEY", c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("api error: status %d, body: %s", resp.StatusCode, string(body))
	}

	var metaResp FugleMetaResponse
	if err := json.NewDecoder(resp.Body).Decode(&metaResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &metaResp, nil
}

// FugleMetaResponse 元数据响应
type FugleMetaResponse struct {
	Date         string `json:"date"`
	Type         string `json:"type"`
	Exchange     string `json:"exchange"`
	Market       string `json:"market"`
	Symbol       string `json:"symbol"`
	Name         string `json:"name"`
	Industry     string `json:"industry"`
	SecurityType string `json:"securityType"`

	// v1.0: securityStatus 取代 v0.3 的 isSuspended/isDelisted
	SecurityStatus string `json:"securityStatus"` // NORMAL | TERMINATED | SUSPENDED

	ReferencePrice float64 `json:"referencePrice"`
	LimitUpPrice   float64 `json:"limitUpPrice"`
	LimitDownPrice float64 `json:"limitDownPrice"`
	PreviousClose  float64 `json:"previousClose"`
}

// CheckMarketStatus 检查市场状态
func (c *FugleClient) CheckMarketStatus(ctx context.Context) (bool, error) {
	// 使用 0050 (元大台灣50) 作为市场指标
	meta, err := c.GetMeta(ctx, "0050")
	if err != nil {
		return false, err
	}

	// v1.0: 檢查 securityStatus 而非 v0.3 的 isSuspended/isDelisted
	// NORMAL = 正常交易；TERMINATED/SUSPENDED = 不可交易
	return meta.SecurityStatus == "NORMAL" || meta.SecurityStatus == "", nil
}

// fugleCandlesResponse is the JSON shape returned by Fugle's historical candles endpoint.
type fugleCandlesResponse struct {
	Data []struct {
		Date   string  `json:"date"`
		Open   float64 `json:"open"`
		High   float64 `json:"high"`
		Low    float64 `json:"low"`
		Close  float64 `json:"close"`
		Volume int64   `json:"volume"`
	} `json:"data"`
}

// GetHistoricalCandles fetches daily candlestick data from Fugle for a single symbol
// over the given date range (inclusive, YYYY-MM-DD). Returns bars with ".TW"-suffixed
// symbols. Respects the FugleClient rate limiter.
//
// Data source priority (CONSTITUTION.md):
//  1. TWSE OpenAPI — no historical range API (intraday quotes only)
//  2. Fubon — no historical candles API (intraday quotes only)
//  3. FinMind — per-day GetStockPrice (too slow for on-demand; used in BTM backfill)
//  4. Fugle — THIS method (one call covers full date range; used for on-demand + warmup)
func (c *FugleClient) GetHistoricalCandles(ctx context.Context, symbol, from, to string) ([]domain.DailyBar, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	if err := c.rateLimiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("fugle rate limit: %w", err)
	}

	fugleURL := fmt.Sprintf(
		"https://api.fugle.tw/marketdata/v1.0/stock/historical/candles/%s?from=%s&to=%s&fields=open,high,low,close,volume",
		symbol, from, to,
	)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fugleURL, nil)
	if err != nil {
		return nil, fmt.Errorf("fugle candles request: %w", err)
	}
	req.Header.Set("X-API-KEY", c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fugle candles fetch: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fugle candles %s returned %d", symbol, resp.StatusCode)
	}

	var result fugleCandlesResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("fugle candles decode: %w", err)
	}

	bars := make([]domain.DailyBar, 0, len(result.Data))
	for _, bar := range result.Data {
		d, err := time.Parse("2006-01-02", bar.Date)
		if err != nil {
			logging.Warn("marketdata", "fugle_candles_date_parse", "symbol", symbol, "date", bar.Date, logging.Err(err))
			continue
		}
		bars = append(bars, domain.DailyBar{
			Symbol: symbol + ".TW",
			Date:   d,
			Open:   bar.Open,
			High:   bar.High,
			Low:    bar.Low,
			Close:  bar.Close,
			Volume: bar.Volume,
			Source: "fugle_candles",
		})
	}
	return bars, nil
}

// FugleProvider 实现 marketdata.Provider 接口
type FugleProvider struct {
	client *FugleClient
}

// NewFugleProviderWithClient 使用客户端创建 Provider
func NewFugleProviderWithClient(client *FugleClient) *FugleProvider {
	return &FugleProvider{client: client}
}

// NewFugleProviderWithAPIKey 使用 API Key 创建 Provider
func NewFugleProviderWithAPIKey(apiKey string) *FugleProvider {
	return &FugleProvider{
		client: NewFugleClient(apiKey),
	}
}

// Name 返回 Provider 名称
func (p *FugleProvider) Name() string {
	return "fugle"
}

// GetQuotes 实现 Provider 接口
func (p *FugleProvider) GetQuotes(ctx context.Context, asOf time.Time, symbols []string) ([]domain.Quote, error) {
	return p.client.GetQuotes(ctx, symbols)
}

// GetClient 获取底层客户端
func (p *FugleProvider) GetClient() *FugleClient {
	return p.client
}
