package marketdata

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
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

// fugleDailyLimit is the daily quota ceiling enforced locally before any
// request reaches the Fugle upstream (manifest F1/A1). P2-21 researched the
// official docs (developer.fugle.tw/docs/pricing/, 2026-08-24):
//
//	基本用戶 (free)   : intraday 60/min, historical 60/min, snapshot 不支援
//	開發者 (NT$1499/m): intraday 600/min, historical 60/min
//	進階用戶 (NT$2999/m): intraday 2000/min, historical 60/min
//
// The docs publish per-MINUTE caps only — there is NO official daily cap for
// the marketdata API (the "2000/day" figure in older comments matched the
// TRADING API's 委託下單 daily limit, not the行情 API). So there is no official
// daily number to align with; 2000/day stays as a deliberately conservative
// LOCAL runaway-burst gate (≈33 min of sustained free-tier max rate):
// normal usage (warmup ~32 candles + on-demand technical + quotes) is well
// under 500/day, so 2000 only trips during runaway bursts, giving
// channel-health a warn signal instead of an invisible 401 lockout.
// Actual free-tier tolerance should be re-validated by live measurement
// before tuning the constant.
const fugleDailyLimit = 2000

// ErrFugleQuotaExhausted is returned by FugleClient.doGet when the daily
// quota is gone. Mirrors marketdata.ErrQuotaExhausted (FinMind) so callers
// can `errors.Is` either one to detect "budget ran out" without coupling
// to the specific provider. The shared QuotaRegistry surfaces both.
var ErrFugleQuotaExhausted = fmt.Errorf("fugle: daily quota exhausted")

// ErrFugleUnauthorized is returned by FugleClient.doGet on HTTP 401.
// The free tier surfaces both quota-lockout and invalid-key as 401
// (manifest F3/D5) — it is treated as a quota/credential event (visible
// warn + breaker trip) rather than a silent generic failure, so a locked
// key can never masquerade as "Fugle is fine, we fell back to TWSE".
var ErrFugleUnauthorized = fmt.Errorf("fugle: unauthorized (quota locked or invalid key)")

// ErrFugleBreakerOpen is returned by FugleClient.doGet when the client-level
// breaker is open (consecutive upstream failures, manifest Phase D). The
// breaker lives on the shared singleton client, so every consumer layer
// (gateway channel, stocktools, hybrid provider, warmup) short-circuits
// together instead of hammering the upstream per-layer.
var ErrFugleBreakerOpen = fmt.Errorf("fugle: circuit breaker open")

// FugleClient Fugle API 客户端
type FugleClient struct {
	apiKey       string
	httpClient   *http.Client
	baseURL      string
	rateLimiter  *rate.Limiter
	quotaTracker *DailyQuotaTracker
	// breaker 是 client 級熔斷器（manifest Phase D）：單一 shared client
	// 讓所有消費層（gateway/stocktools/hybrid/warmup）共享同一 breaker —
	// 連續失敗後統一短路，不再逐層重試上游。
	breaker *providerBreaker
	// retryCfg is the shared fetchWithRetry policy (P0-5) used by the
	// candles path — doGet already had its own 429 retry loop, but
	// GetHistoricalCandles had none (a single 429 failed the fetch).
	retryCfg retryConfig
}

// FugleQuoteResponse Fugle v1.0 行情响应（扁平結構）
// 對照 v0.3 巢狀 data.quote.* 結構，v1.0 將欄位提升到頂層。
type FugleQuoteResponse struct {
	Date     string `json:"date"`
	Type     string `json:"type"`
	Exchange string `json:"exchange"`
	Market   string `json:"market"`
	Symbol   string `json:"symbol"`
	Name     string `json:"name"`

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
// free: 30/min (default — conservative vs the measured ~39/min 429 point,
// manifest F2/A2), developer: 600/min, advanced: 2000/min
func getFugleRateLimit() int {
	switch config.GetSecret("FUGLE_TIER") {
	case "developer":
		return 600
	case "advanced":
		return 2000
	default:
		return 30 // free tier — deliberately below the measured 429 point
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
// stateDir is captured once for the shared DailyQuotaTracker; callers
// that need a different state directory should use NewFugleClient directly.
func GetSharedFugleClient(apiKey string, stateDir ...string) *FugleClient {
	dir := "data/state"
	if len(stateDir) > 0 && stateDir[0] != "" {
		dir = stateDir[0]
	}
	sharedFugleClientOnce.Do(func() {
		sharedFugleClient = newFugleClient(apiKey, dir)
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
	return newFugleClient(apiKey, "data/state")
}
func newFugleClient(apiKey string, stateDir string) *FugleClient {
	params := config.GetParametersConfig()
	limit := getFugleRateLimit()
	if params.Marketdata.FugleRateLimit.Value > 60 {
		limit = params.Marketdata.FugleRateLimit.Value
	}
	logging.Info("fugle", "client_initialized", "tier", config.GetSecret("FUGLE_TIER"), "rate_limit", limit)
	timeout := time.Duration(params.Marketdata.FugleAPITimeoutSec.Value) * time.Second
	// Burst is deliberately conservative: a burst == limit (e.g. 60) lets a
	// single caller fire 60 requests instantly, which the Fugle sliding
	// window rejects well before 60 (measured 429 at ~39 live calls,
	// 2026-08-03). Keep burst small (3) so the limiter actually throttles.
	burst := min(limit, 3)
	tracker := NewDailyQuotaTracker("fugle", stateDir, fugleDailyLimit)
	// Register the tracker with the global QuotaRegistry so the dashboard
	// sees Fugle alongside FinMind in one Snapshot(). Pairs with the
	// FinMind gate in finmind_client.go:131 so neither provider can silently
	// starve the other when their quota changes.
	GlobalQuotaRegistry().Register("fugle", tracker)
	return &FugleClient{
		apiKey:       apiKey,
		httpClient:   httpclient.NewFactory().NewClient(timeout),
		baseURL:      fugleAPIBaseURL,
		rateLimiter:  rate.NewLimiter(rate.Every(time.Minute/time.Duration(limit)), burst),
		quotaTracker: tracker,
		breaker:      newProviderBreaker("fugle", defaultCircuitBreakerConfig()),
		retryCfg:     defaultRetryConfig(),
	}
}

// QuotaUsed returns today's Fugle API call count (across all callers sharing
// the DailyQuotaTracker). Surfaced by the channel-health page so the
// dashboard can warn before the daily budget is gone.
func (c *FugleClient) QuotaUsed() int {
	if c.quotaTracker == nil {
		return 0
	}
	return c.quotaTracker.CallsToday()
}

// QuotaRemaining returns the unused portion of today's Fugle budget.
func (c *FugleClient) QuotaRemaining() int {
	if c.quotaTracker == nil {
		return fugleDailyLimit
	}
	return c.quotaTracker.Remaining()
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
// The free tier (60/min) is enforced by the shared limiter, but Fugle's
// sliding window can 429 before the token bucket drains — honor
// Retry-After (or a conservative backoff) instead of failing immediately.
// Mirrors finmind_backfill.go FetchWithRetry's 429 handling.
func (c *FugleClient) doGet(ctx context.Context, endpoint string) ([]byte, error) {
	// Phase D：client 級 breaker 短路 — open 時不發 HTTP，直接回
	// ErrFugleBreakerOpen（所有消費層共享此 client → 統一熔斷）。
	if c.breaker != nil && !c.breaker.shouldTry() {
		return nil, fmt.Errorf("fugle: %w", ErrFugleBreakerOpen)
	}
	const maxRetries = 3
	for attempt := 0; ; attempt++ {
		if err := c.rateLimiter.Wait(ctx); err != nil {
			return nil, fmt.Errorf("rate limit wait: %w", ErrRateLimited)
		}
		// Daily-quota gate: mirrors the FinMind gate so the channel-health
		// page and shared QuotaRegistry see one consistent view across
		// providers. Without it, a cold-start burst from any caller can
		// burn the day's Fugle budget in seconds (30/min × 1440 = 43,200).
		if c.quotaTracker != nil && !c.quotaTracker.AllowCall() {
			// P1-7 鏡像（FinMind 同修）：額度耗盡是預算條件（00:00 UTC 自動
			// 重置），不是上游故障 — 不得計入 breaker 連續失敗，否則 quota
			// 事件會把 breaker 打開、讓後續 ErrFugleBreakerOpen 以 error 級
			// 告警（實證 2026-09-04: calls_today=2000 觸發後 fugle 全日 error）。
			c.breakerRecordSuccess()
			return nil, fmt.Errorf("fugle: %w (used=%d, remaining=%d)", ErrFugleQuotaExhausted, c.quotaTracker.CallsToday(), c.quotaTracker.Remaining())
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, fmt.Errorf("create request: %w", err)
		}
		req.Header.Set("X-API-KEY", c.apiKey)

		resp, err := c.httpClient.Do(req)
		if err != nil {
			c.breakerRecordFailure()
			return nil, fmt.Errorf("http request: %w", err)
		}

		if resp.StatusCode == http.StatusTooManyRequests {
			// 429: Fugle sliding-window exceeded. Sleep Retry-After if present,
			// else conservative backoff; then retry.
			_ = resp.Body.Close()
			wait := time.Duration(1<<attempt) * time.Second
			if ra := resp.Header.Get("Retry-After"); ra != "" {
				if secs, parseErr := strconv.Atoi(ra); parseErr == nil && secs > 0 {
					wait = time.Duration(secs) * time.Second
				}
			}
			logging.Warn("fugle", "rate_limit_429", "endpoint", endpoint, "retry_in_s", int(wait.Seconds()))
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(wait):
			}
			if attempt >= maxRetries {
				c.breakerRecordFailure()
				return nil, fmt.Errorf("fugle: rate limited after %d retries", maxRetries)
			}
			continue
		}

		body, readErr := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			// 401: free tier 破表鎖定與 key 無效都表現為 401（manifest
			// F3）。記錄 warn 讓鎖定可見（401 不走上方 429 retry 分支，
			// 否則破表完全隱形）。
			if resp.StatusCode == http.StatusUnauthorized {
				logging.Warn("fugle", "rate_limit_401",
					"endpoint", endpoint,
					"note", "free-tier quota lock or invalid key; treating as quota event")
				c.breakerRecordFailure()
				return nil, fmt.Errorf("fugle: %w", ErrFugleUnauthorized)
			}
			c.breakerRecordFailure()
			return nil, fmt.Errorf("api error: status %d, body: %s", resp.StatusCode, string(body))
		}
		c.breakerRecordSuccess()
		return body, readErr
	}
}

// GetQuote 获取单个股票行情（v1.0 API）
func (c *FugleClient) GetQuote(ctx context.Context, symbol string) (domain.Quote, error) {
	// v1.0: symbol 在 URL path，key 在 X-API-KEY header（非 query param）
	endpoint := fmt.Sprintf("%s/intraday/quote/%s", c.baseURL, url.PathEscape(symbol))
	body, err := c.doGet(ctx, endpoint)
	if err != nil {
		return domain.Quote{}, err
	}

	// 解析响应
	var fugleResp FugleQuoteResponse
	if err := json.Unmarshal(body, &fugleResp); err != nil {
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
	endpoint := fmt.Sprintf("%s/intraday/ticker/%s", c.baseURL, url.PathEscape(symbol))
	body, err := c.doGet(ctx, endpoint)
	if err != nil {
		return nil, err
	}

	var metaResp FugleMetaResponse
	if err := json.Unmarshal(body, &metaResp); err != nil {
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
	// NORMAL = 正常交易；TERMINATED/SUSPENDED = 不可交易。
	// 實測（2026-08-03）正常股票回 "NORMAL"；空字串代表回應異常/缺失，
	// 保守視為不可交易（fail-closed）。
	return meta.SecurityStatus == "NORMAL", nil
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

	// Phase D：candles 路徑（warmup/technical on-demand）與 doGet 統一
	// breaker + quota gate — 此前只有 rateLimiter，warmup 32 calls 不受
	// 日額度 gate 保護（manifest D 缺口）。
	if c.breaker != nil && !c.breaker.shouldTry() {
		return nil, fmt.Errorf("fugle: %w", ErrFugleBreakerOpen)
	}
	if c.quotaTracker != nil && !c.quotaTracker.AllowCall() {
		// P1-7 鏡像：同 doGet — 額度耗盡不計 breaker 失敗。
		c.breakerRecordSuccess()
		return nil, fmt.Errorf("fugle: %w (used=%d, remaining=%d)", ErrFugleQuotaExhausted, c.quotaTracker.CallsToday(), c.quotaTracker.Remaining())
	}

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

	// P0-5: shared fetchWithRetry — 429/5xx on the candles path are retried
	// with Retry-After / exponential backoff (previously a single 429 failed
	// the fetch, inconsistent with doGet which already retried).
	resp, err := fetchWithRetry(ctx, c.httpClient, req, c.retryCfg)
	if err != nil {
		c.breakerRecordFailure()
		return nil, fmt.Errorf("fugle candles fetch: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		c.breakerRecordFailure()
		if resp.StatusCode == http.StatusUnauthorized {
			logging.Warn("fugle", "rate_limit_401",
				"endpoint", fugleURL,
				"note", "free-tier quota lock or invalid key; treating as quota event")
			return nil, fmt.Errorf("fugle: %w", ErrFugleUnauthorized)
		}
		return nil, fmt.Errorf("fugle candles %s returned %d", symbol, resp.StatusCode)
	}
	c.breakerRecordSuccess()

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

// breakerRecordFailure / breakerRecordSuccess 是 breaker 記錄的 nil-safe
// 包裝（部分測試與未來 refactor 可能以裸 struct 建 FugleClient，breaker
// 為 nil 時不得 panic）。
func (c *FugleClient) breakerRecordFailure() {
	if c.breaker != nil {
		c.breaker.recordFailure()
	}
}

func (c *FugleClient) breakerRecordSuccess() {
	if c.breaker != nil {
		c.breaker.recordSuccess()
	}
}
