package marketdata

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"github.com/kaecer68/atlas-go/internal/apigateway/httpclient"
	"github.com/kaecer68/atlas-go/internal/constants"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/logging"
)

// FinMind API client for Taiwan stock data.
// NOTE: FinMind requires API key rotation every 7 days.
// This client is intended as a manual backup / fallback only.
// For production, prefer Fugle (real-time) or TWSE OpenAPI (free, no key).
// To rotate key: update FINMIND_API_KEY in .env and restart the service.
const (
	finmindBaseURL   = constants.FinMindBaseURL
	finmindRateLimit = 600 // 600 requests per hour for free tier
	finmindBurst     = 60
)

// finmindDailyLimit is the daily quota ceiling for the FinMind free tier.
// 600/hr × 24 = 14,400/day. We track this with DailyQuotaTracker so concurrent
// callers (auto_cycle_update, auto_quote_backfill, channel_health_finmind,
// tsmc_revenue, ad-hoc lookups) don't collectively exceed it. Without the
// tracker, cold-start backfill of N symbols × 90 days can blow the daily
// quota in a single scheduled run, leaving the channel dead for the rest of
// the day (regression: commit 35642c13 switched auto_quote_backfill from Fugle
// to FinMind, multiplying the call volume against this single channel).
const finmindDailyLimit = 14400

// ErrQuotaExhausted is returned by fetchDataset when the daily quota is gone.
// Callers should treat this as a transient, scheduled-skippable condition —
// distinct from API auth/quota errors that need human intervention. The
// channel adapter maps this to a "warn" status (not "error") so on-call
// doesn't get paged just because the daily budget ran out.
var ErrQuotaExhausted = fmt.Errorf("finmind: daily quota exhausted")

type FinMindClient struct {
	apiKey       string
	httpClient   *http.Client
	rateLimiter  *rate.Limiter
	quotaTracker *DailyQuotaTracker
}

type FinMindResponse struct {
	Msg    string           `json:"msg"`
	Status int              `json:"status"`
	Data   []map[string]any `json:"data"`
}

type StockInfo struct {
	StockID          string `json:"stock_id"`
	StockName        string `json:"stock_name"`
	IndustryCategory string `json:"industry_category"`
	Type             string `json:"type"`
}

var (
	sharedFinMindClient     *FinMindClient
	sharedFinMindClientOnce sync.Once
	sharedFinMindClientMu   sync.RWMutex
)

// GetSharedFinMindClient returns a singleton FinMindClient that all components
// share. Using a single client ensures one token bucket enforces the 600 req/hr
// limit across all call sites (gateway channels, TSMC revenue, cycle aggregator).
// The apiKey is used only on first call; subsequent calls ignore it. The
// stateDir is also captured once for the shared DailyQuotaTracker; callers
// that need a different state directory should use NewFinMindClient directly.
func GetSharedFinMindClient(apiKey string, stateDir ...string) *FinMindClient {
	dir := "data/state"
	if len(stateDir) > 0 && stateDir[0] != "" {
		dir = stateDir[0]
	}
	sharedFinMindClientOnce.Do(func() {
		sharedFinMindClient = newFinMindClientInternal(apiKey, dir)
	})
	return sharedFinMindClient
}

// UpdateSharedFinMindAPIKey replaces the API key on the shared client without
// recreating the rate limiter. Use after rotating the FinMind token at runtime.
func UpdateSharedFinMindAPIKey(apiKey string) {
	sharedFinMindClientMu.Lock()
	defer sharedFinMindClientMu.Unlock()
	if sharedFinMindClient != nil {
		sharedFinMindClient.apiKey = apiKey
	}
}

// ResetSharedFinMindClient clears the singleton (for tests).
func ResetSharedFinMindClient() {
	sharedFinMindClientMu.Lock()
	defer sharedFinMindClientMu.Unlock()
	sharedFinMindClient = nil
	sharedFinMindClientOnce = sync.Once{}
}

// NewFinMindClient creates a standalone FinMindClient with its own rate limiter.
// Prefer GetSharedFinMindClient in production to avoid multiple independent
// token buckets that can collectively exceed the free-tier limit.
func NewFinMindClient(apiKey string) *FinMindClient {
	return newFinMindClientInternal(apiKey, "data/state")
}

// NewFinMindClientWithStateDir creates a standalone FinMindClient whose
// DailyQuotaTracker persists under the given stateDir instead of the
// default "data/state". Test-only convenience — production callers
// should use GetSharedFinMindClient (which routes through
// newFinMindClientInternal with the configured WorkDir) so the quota
// state file lives next to the other runtime state.
func NewFinMindClientWithStateDir(apiKey, stateDir string) *FinMindClient {
	return newFinMindClientInternal(apiKey, stateDir)
}

// newFinMindClientInternal is the shared constructor used by both the
// singleton accessor and the standalone constructor. It wires the shared
// DailyQuotaTracker so all call sites share one daily counter.
func newFinMindClientInternal(apiKey, stateDir string) *FinMindClient {
	tracker := NewDailyQuotaTracker("finmind", stateDir, finmindDailyLimit)
	// Register the tracker with the global QuotaRegistry so the dashboard's
	// channel-health page and the future /api/dashboard/quota endpoint see
	// FinMind alongside Fugle in one Snapshot() — addressing kaecer's
	// 2026-08-04 feedback to manage FinMind + Fugle together.
	GlobalQuotaRegistry().Register("finmind", tracker)
	return &FinMindClient{
		apiKey:       apiKey,
		httpClient:   httpclient.NewFactory().NewClient(30 * time.Second),
		rateLimiter:  rate.NewLimiter(rate.Every(time.Hour/finmindRateLimit), finmindBurst),
		quotaTracker: tracker,
	}
}
func (c *FinMindClient) SetHTTPClient(client *http.Client) {
	c.httpClient = client
}

// SetRateLimiter overrides the rate limiter (tests only; use rate.Inf to disable pacing).
func (c *FinMindClient) SetRateLimiter(limiter *rate.Limiter) {
	c.rateLimiter = limiter
}

// RateLimiter returns the rate limiter for Gateway adapter registration.
func (c *FinMindClient) RateLimiter() *rate.Limiter {
	return c.rateLimiter
}

// QuotaUsed returns the number of FinMind API calls made today (across all
// callers sharing the DailyQuotaTracker). Surfaced by the channel adapter
// health record so the dashboard can warn before the budget runs out.
func (c *FinMindClient) QuotaUsed() int {
	if c.quotaTracker == nil {
		return 0
	}
	return c.quotaTracker.CallsToday()
}

// QuotaRemaining returns the unused portion of today's FinMind budget.
// Returns the full daily limit when no tracker is configured.
func (c *FinMindClient) QuotaRemaining() int {
	if c.quotaTracker == nil {
		return finmindDailyLimit
	}
	return c.quotaTracker.Remaining()
}

func (c *FinMindClient) fetchDataset(ctx context.Context, dataset string, dataId string, startDate string, endDate string) ([]map[string]any, error) {
	if err := c.rateLimiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("finmind: rate limit wait: %w", ErrRateLimited)
	}
	// Daily-quota gate: every call site funnels through fetchDataset, so a
	// single AllowCall check protects the whole channel from cold-start
	// bursts (commit 35642c13 switched auto_quote_backfill to FinMind, which
	// can hit 1000s of calls per cycle without this gate). When the daily
	// budget is gone we return ErrQuotaExhausted rather than letting the
	// HTTP request fail with a misleading 400 status.
	if c.quotaTracker != nil && !c.quotaTracker.AllowCall() {
		return nil, fmt.Errorf("finmind: %w (used=%d, remaining=%d)", ErrQuotaExhausted, c.quotaTracker.CallsToday(), c.quotaTracker.Remaining())
	}

	endpoint := fmt.Sprintf("%s/data", finmindBaseURL)
	params := url.Values{}
	params.Set("dataset", dataset)
	params.Set("data_id", dataId)
	params.Set("start_date", startDate)
	params.Set("end_date", endDate)

	reqURL := fmt.Sprintf("%s?%s", endpoint, params.Encode())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("finmind: create request: %w", err)
	}

	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("finmind: http request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Capture response body on non-200 so channel_health surfaces the real
	// reason. Without this, the FinMind API's "Token is illegal" / "no
	// data" / "rate limit exceeded" messages get dropped, and operators
	// only see "finmind: status 400" — which forced hermes + this agent to
	// debug in circles before realising the real issue (FINMIND_API_KEY
	// env mismatch, NOT quota exhaustion, on 2026-08-04).
	// Limit read to 512 bytes: enough for FinMind's JSON error envelope,
	// bounded so a malicious / oversized body can't blow memory.
	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		bodyStr := strings.TrimSpace(string(bodyBytes))
		if bodyStr == "" {
			bodyStr = "(empty body)"
		}
		logging.Warn("finmind", "fetch_non_2xx",
			"status", resp.StatusCode,
			"body", bodyStr,
			"dataset", dataset,
			"data_id", dataId,
		)
		return nil, fmt.Errorf("finmind: status %d, body: %s", resp.StatusCode, bodyStr)
	}

	var finmindResp FinMindResponse
	if err := json.NewDecoder(resp.Body).Decode(&finmindResp); err != nil {
		return nil, fmt.Errorf("finmind: decode response: %w", err)
	}

	if finmindResp.Status != 200 {
		return nil, fmt.Errorf("finmind: API error: %s", finmindResp.Msg)
	}

	return finmindResp.Data, nil
}

// lastDayOfMonth returns the last calendar day of (year, month) — 28/29/30/31
// depending on the month and leap year. Uses Go's time.Date normalisation:
// time.Date(y, m+1, 0, ...) is the idiomatic way to get the last day of
// month m in year y.
//
// PR-E (kaecer 2026-08-05 dispatch). This replaces the previous hardcoded
// "31" in endDate construction, which caused FinMind to return
// "parameter YYYY-MM-31 is illegal" errors for any month with 30 or
// fewer days (Feb/Apr/Jun/Sep/Nov). The 80+ day auto_cycle_update stale
// issue documented in v3.0 §A 問題 5 is rooted in this bug, not in
// upstream TWSE as the v3.0 report assumed. See post-restart-e2e-
// verification-2026-08-05.md §3 for the full log evidence.
func lastDayOfMonth(year int, month time.Month) int {
	return time.Date(year, month+1, 0, 0, 0, 0, 0, time.UTC).Day()
}

func (c *FinMindClient) GetMonthRevenue(ctx context.Context, symbol string, year int, month int) (float64, error) {
	startDate := fmt.Sprintf("%d-%02d-01", year, month)
	endDate := fmt.Sprintf("%d-%02d-%02d", year, month, lastDayOfMonth(year, time.Month(month)))

	data, err := c.fetchDataset(ctx, "TaiwanStockMonthRevenue", symbol, startDate, endDate)
	if err != nil {
		return 0, err
	}

	if len(data) == 0 {
		return 0, fmt.Errorf("finmind: no month revenue data for %s %d-%02d", symbol, year, month)
	}

	revenue, ok := data[0]["revenue"].(float64)
	if !ok {
		return 0, fmt.Errorf("finmind: cannot parse revenue from response")
	}

	return revenue, nil
}

func (c *FinMindClient) GetFinancialStatements(ctx context.Context, symbol string, year int, quarter int) (map[string]float64, error) {
	startDate := fmt.Sprintf("%d-01-01", year)
	endDate := fmt.Sprintf("%d-12-%02d", year, lastDayOfMonth(year, time.December))

	data, err := c.fetchDataset(ctx, "TaiwanStockFinancialStatements", symbol, startDate, endDate)
	if err != nil {
		return nil, err
	}

	result := make(map[string]float64)
	for _, item := range data {
		dateStr, ok := item["date"].(string)
		if !ok {
			continue
		}
		if len(dateStr) >= 7 {
			q := int(dateStr[5] - '0')
			if q == quarter {
				if val, ok := item["value"].(float64); ok {
					originName, _ := item["origin_name"].(string)
					result[originName] = val
				}
			}
		}
	}

	return result, nil
}

func (c *FinMindClient) GetInstitutionalInvestors(ctx context.Context, symbol string, date string) (foreign, domestic, dealer float64, err error) {
	data, err := c.fetchDataset(ctx, "TaiwanStockInstitutionalInvestorsBuySell", symbol, date, date)
	if err != nil {
		return 0, 0, 0, err
	}

	for _, item := range data {
		name, ok := item["name"].(string)
		if !ok {
			continue
		}
		buy, _ := item["buy"].(float64)
		sell, _ := item["sell"].(float64)
		net := buy - sell

		switch name {
		case "ForeignInvestors", "ForeignDealer":
			foreign += net
		case "InvestmentTrust", "DomesticInstitution":
			domestic += net
		case "Dealer":
			dealer += net
		}
	}

	return foreign, domestic, dealer, nil
}

func (c *FinMindClient) GetStockPrice(ctx context.Context, symbol string, date string) (domain.Quote, error) {
	data, err := c.fetchDataset(ctx, "TaiwanStockPrice", symbol, date, date)
	if err != nil {
		return domain.Quote{}, err
	}

	if len(data) == 0 {
		return domain.Quote{}, fmt.Errorf("finmind: no price data for %s on %s", symbol, date)
	}

	item := data[0]
	quote := domain.Quote{
		Symbol: symbol,
		Market: "TW",
		AsOf:   time.Now(),
		Source: "finmind",
	}

	if v, ok := item["close"].(float64); ok {
		quote.Last = v
		quote.High = v
		quote.Low = v
	}
	if v, ok := item["open"].(float64); ok {
		quote.Open = v
	}
	if v, ok := item["max"].(float64); ok {
		quote.High = v
	}
	if v, ok := item["min"].(float64); ok {
		quote.Low = v
	}
	if v, ok := item["Trading_Volume"].(float64); ok {
		quote.Volume = int64(v)
	}

	return quote, nil
}

func parseStockInfo(item map[string]any) (StockInfo, error) {
	var info StockInfo

	stockID, ok := item["stock_id"].(string)
	if !ok {
		return info, fmt.Errorf("finmind: missing stock_id in TaiwanStockInfo")
	}
	info.StockID = stockID

	stockName, _ := item["stock_name"].(string)
	info.StockName = stockName

	industryCategory, _ := item["industry_category"].(string)
	info.IndustryCategory = industryCategory

	stockType, _ := item["type"].(string)
	info.Type = stockType

	return info, nil
}

func (c *FinMindClient) GetStockInfo(ctx context.Context) ([]StockInfo, error) {
	data, err := c.fetchDataset(ctx, "TaiwanStockInfo", "", "", "")
	if err != nil {
		return nil, fmt.Errorf("finmind: TaiwanStockInfo: %w", err)
	}

	if len(data) == 0 {
		return nil, fmt.Errorf("finmind: TaiwanStockInfo returned empty data")
	}

	infos := make([]StockInfo, 0, len(data))
	for _, item := range data {
		info, err := parseStockInfo(item)
		if err != nil {
			logging.Warn("finmind", "parse_stock_info_failed", logging.Err(err))
			continue
		}
		infos = append(infos, info)
	}

	if len(infos) == 0 {
		return nil, fmt.Errorf("finmind: TaiwanStockInfo: all items failed to parse")
	}

	return infos, nil
}

type FinMindProvider struct {
	client *FinMindClient
}

func NewFinMindProviderWithClient(client *FinMindClient) *FinMindProvider {
	return &FinMindProvider{client: client}
}

func NewFinMindProvider(apiKey string) *FinMindProvider {
	return &FinMindProvider{client: GetSharedFinMindClient(apiKey)}
}

func (p *FinMindProvider) Name() string {
	return "finmind"
}

// isTaiwanTradingDay 回傳 t 是否為台股交易日 (排除週末)。
// FinMind 在週末/假日回傳空資料,GetQuotes 必須在呼叫 FinMind API
// 之前先驗證 asOf 為交易日,否則會拿到空的 quotes 卻無明確錯誤,
// 讓下游誤以為「當天無報價」而非「查詢日期非交易日」。
// 國定假日清單暫不內建 (需每年維護),僅擋週末;後續可由
// globalmarket.TradingSchedule.Holidays 注入或讀 configs 擴充。
func isTaiwanTradingDay(t time.Time) bool {
	w := t.Weekday()
	return w != time.Saturday && w != time.Sunday
}

func (p *FinMindProvider) GetQuotes(ctx context.Context, asOf time.Time, symbols []string) ([]domain.Quote, error) {
	if !isTaiwanTradingDay(asOf) {
		return nil, fmt.Errorf("finmind: asOf %s is not a Taiwan trading day (weekend or holiday)", asOf.Format("2006-01-02"))
	}
	date := asOf.Format("2006-01-02")
	quotes := make([]domain.Quote, 0, len(symbols))

	var lastErr error
	for _, symbol := range symbols {
		quote, err := p.client.GetStockPrice(ctx, symbol, date)
		if err != nil {
			logging.Error("finmind", "fetch_failed", "symbol", symbol, logging.Err(err))
			lastErr = err
			continue
		}
		quotes = append(quotes, quote)
	}

	if len(quotes) == 0 && lastErr != nil {
		return nil, fmt.Errorf("finmind: all symbols failed: %w", lastErr)
	}
	return quotes, nil
}

func (p *FinMindProvider) GetClient() *FinMindClient {
	return p.client
}

func (p *FinMindProvider) GetMonthRevenue(ctx context.Context, symbol string, year int, month int) (float64, error) {
	return p.client.GetMonthRevenue(ctx, symbol, year, month)
}

func (p *FinMindProvider) GetFinancialStatements(ctx context.Context, symbol string, year int, quarter int) (map[string]float64, error) {
	return p.client.GetFinancialStatements(ctx, symbol, year, quarter)
}

func (p *FinMindProvider) GetInstitutionalInvestors(ctx context.Context, symbol string, date string) (float64, float64, float64, error) {
	return p.client.GetInstitutionalInvestors(ctx, symbol, date)
}
