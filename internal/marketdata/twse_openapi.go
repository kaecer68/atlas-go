package marketdata

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"github.com/kaecer68/atlas-go/internal/apigateway/httpclient"
	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/constants"
	"github.com/kaecer68/atlas-go/internal/domain"
)

const (
	twseAPIBaseURL = constants.TWSEBaseURL
)

// ─── shared TWSE token bucket (P1-13) ───────────────────────────────────────
//
// Previously 11 providers each built their OWN rate limiter against
// www.twse.com.tw (openapi 0.6/s, capital_flow 1/5s, margin 1/5s, oddlot 1/s,
// day_trading 1/5s, market_volume 1/5s, calendar 0.6/s, insider 1/5s, etf 1/s,
// sector_index 0.6/s, sbl 1/2s) — a busy cycle could collectively hit TWSE
// several times per second. One shared bucket enforces the documented policy
// (marketdata.twse_api_rate_limit / twse_api_rate_burst, default 3 req per
// 5s) across ALL call sites, including the taiex_twse_fallback path that
// previously skipped rate limiting entirely.
var (
	twseSharedLimiterMu sync.Mutex
	twseSharedLimiter   *rate.Limiter
)

// getTWSESharedLimiter lazily constructs the shared bucket from the
// parameter system (lazy so config overrides are honored at first use).
func getTWSESharedLimiter() *rate.Limiter {
	twseSharedLimiterMu.Lock()
	defer twseSharedLimiterMu.Unlock()
	if twseSharedLimiter == nil {
		params := config.GetParametersConfig()
		twseSharedLimiter = rate.NewLimiter(
			rate.Limit(params.Marketdata.TWSEAPIRateLimit.Value),
			params.Marketdata.TWSEAPIRateBurst.Value,
		)
	}
	return twseSharedLimiter
}

// SetTWSESharedLimiterForTest replaces the shared bucket. Returns the
// previous limiter for the caller to restore with defer/Cleanup.
func SetTWSESharedLimiterForTest(l *rate.Limiter) *rate.Limiter {
	twseSharedLimiterMu.Lock()
	defer twseSharedLimiterMu.Unlock()
	old := twseSharedLimiter
	twseSharedLimiter = l
	return old
}

// TWSEClient TWSE OpenAPI 客户端
type TWSEClient struct {
	httpClient  *http.Client
	baseURL     string
	rateLimiter *rate.Limiter
	// breaker is the client-level circuit breaker (P1-7). All TWSE call
	// sites (hybrid provider, gateway adapters, GetQuote/GetQuotes) share
	// this client via GetSharedTWSEClient, so one breaker covers the whole
	// host. ErrTWSEEmptyData (holiday/no-data) does NOT trip it.
	breaker *providerBreaker
}

// TWSEQuote TWSE 行情数据结构
type TWSEQuote struct {
	Code         string `json:"Code"`
	Name         string `json:"Name"`
	TradeVolume  string `json:"TradeVolume"`
	TradeValue   string `json:"TradeValue"`
	OpeningPrice string `json:"OpeningPrice"`
	HighestPrice string `json:"HighestPrice"`
	LowestPrice  string `json:"LowestPrice"`
	ClosingPrice string `json:"ClosingPrice"`
	Change       string `json:"Change"`
	Transaction  string `json:"Transaction"`
}

// TWSEDailyResponse TWSE 每月股票數據 API 回應結構
type TWSEDailyResponse struct {
	Stat   string     `json:"stat"`
	Date   string     `json:"date"`
	Title  string     `json:"title"`
	Fields []string   `json:"fields"`
	Data   [][]string `json:"data"`
}

var (
	sharedTWSEClient     *TWSEClient
	sharedTWSEClientOnce sync.Once
	sharedTWSEClientMu   sync.RWMutex
)

// GetSharedTWSEClient returns a singleton TWSEClient that all components share.
// Using a single client ensures one rate limiter enforces the rate limit across
// all call sites (hybrid provider, gateway adapters, daily-replay-sync, etc.).
func GetSharedTWSEClient() *TWSEClient {
	sharedTWSEClientOnce.Do(func() {
		sharedTWSEClient = &TWSEClient{
			httpClient:  httpclient.NewFactory().NewClient(time.Duration(config.GetParametersConfig().Marketdata.TWSEAPITimeoutSec.Value) * time.Second),
			baseURL:     twseAPIBaseURL,
			rateLimiter: getTWSESharedLimiter(),
			breaker:     newProviderBreaker("twse", defaultCircuitBreakerConfig()),
		}
	})
	return sharedTWSEClient
}

// ResetSharedTWSEClient clears the singleton (for tests).
func ResetSharedTWSEClient() {
	sharedTWSEClientMu.Lock()
	defer sharedTWSEClientMu.Unlock()
	sharedTWSEClient = nil
	sharedTWSEClientOnce = sync.Once{}
	// Also clear the shared token bucket so the next GetSharedTWSEClient
	// constructs a fresh one (tests must not inherit drained tokens).
	twseSharedLimiterMu.Lock()
	twseSharedLimiter = nil
	twseSharedLimiterMu.Unlock()
}

// NewTWSEClient 创建 TWSE OpenAPI 客户端
// Deprecated: Prefer GetSharedTWSEClient() to share one rate-limited client across all call sites.
func NewTWSEClient() *TWSEClient {
	params := config.GetParametersConfig()
	return &TWSEClient{
		httpClient:  httpclient.NewFactory().NewClient(time.Duration(params.Marketdata.TWSEAPITimeoutSec.Value) * time.Second),
		baseURL:     twseAPIBaseURL,
		rateLimiter: getTWSESharedLimiter(),
		breaker:     newProviderBreaker("twse", defaultCircuitBreakerConfig()),
	}
}

// SetHTTPClient 设置自定义 HTTP 客户端
func (c *TWSEClient) SetHTTPClient(client *http.Client) {
	c.httpClient = client
}

// HTTPClient returns the underlying HTTP client (shared by the TAIEX
// fallback path, P1-13).
func (c *TWSEClient) HTTPClient() *http.Client {
	return c.httpClient
}

// RateLimiter returns the shared TWSE token bucket (P1-13: every
// www.twse.com.tw caller — providers and the TAIEX fallback — shares one
// limiter instead of 11 independent buckets).
func (c *TWSEClient) RateLimiter() *rate.Limiter {
	return c.rateLimiter
}

// GetQuotes 批量获取当日所有上市股票行情
func (c *TWSEClient) GetQuotes(ctx context.Context) ([]domain.Quote, error) {
	// P1-7: client-level breaker — open 時不發 HTTP。ErrTWSEEmptyData
	// (假日/no-data) 不計 failure（見下方）。
	if c.breaker != nil && !c.breaker.shouldTry() {
		return nil, fmt.Errorf("%w: twse circuit breaker open", ErrUpstream)
	}
	waitCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := c.rateLimiter.Wait(waitCtx); err != nil {
		return nil, fmt.Errorf("rate limit wait (10s timeout): %w", err)
	}

	endpoint := fmt.Sprintf("%s/exchangeReport/STOCK_DAY_ALL", c.baseURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.breakerRecordFailure()
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		c.breakerRecordFailure()
		return nil, fmt.Errorf("api error: status %d", resp.StatusCode)
	}

	// Buffer body to bytes (bytes-read pattern) so the CSV fallback below can
	// re-parse from a fresh reader. Previously, DecodeJSON's json.NewDecoder
	// would partially consume resp.Body on failure, leaving the CSV fallback
	// to choke on a truncated header → "csv header: parse error on line 1,
	// column 3: extraneous or missing \" in quoted-field".
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	var twseResp TWSEDailyResponse
	contentType := resp.Header.Get("Content-Type")
	decodeErr := DecodeJSON(bytes.NewReader(body), contentType, &twseResp)
	if decodeErr != nil {
		// TWSE changed STOCK_DAY_ALL from JSON to CSV (2026-06-30).
		// Fallback: parse CSV rows directly instead of failing.
		if isCSVContentType(contentType) {
			return c.parseStockCSV(bytes.NewReader(body))
		}
		c.breakerRecordFailure()
		return nil, fmt.Errorf("decode response: %w", decodeErr)
	}

	// P0-4: surface a non-OK upstream status instead of silently treating
	// the payload as an empty success (aligned with GetDailyQuote).
	if twseResp.Stat != "OK" {
		c.breakerRecordFailure()
		return nil, fmt.Errorf("twse: STOCK_DAY_ALL stat=%q (expected OK)", twseResp.Stat)
	}

	twseQuotes := make([]TWSEQuote, 0, len(twseResp.Data))
	for _, row := range twseResp.Data {
		if len(row) < 9 {
			continue
		}
		twseQuotes = append(twseQuotes, TWSEQuote{
			Code:         row[0],
			Name:         row[1],
			TradeVolume:  row[2],
			TradeValue:   row[3],
			OpeningPrice: row[4],
			HighestPrice: row[5],
			LowestPrice:  row[6],
			ClosingPrice: row[7],
			Change:       row[8],
			Transaction:  rowAt(row, 9, ""),
		})
	}

	// P0-4: zero rows → typed ErrTWSEEmptyData so adapters can distinguish
	// holiday/no-data from a genuine upstream failure. Previously this was
	// an empty success slice, so the gateway breaker never tripped and
	// channel-health showed ok while every quote was missing.
	if len(twseQuotes) == 0 {
		// P1-7: no-data (holiday / market closed) must NOT trip the breaker.
		c.breakerRecordSuccess()
		return nil, fmt.Errorf("%w: STOCK_DAY_ALL returned no rows", ErrTWSEEmptyData)
	}

	quotes := make([]domain.Quote, 0, len(twseQuotes))
	parseFailures := 0
	for _, q := range twseQuotes {
		quote, err := c.convertToQuote(q)
		if err != nil {
			parseFailures++
			continue
		}
		quotes = append(quotes, quote)
	}

	// P0-4: every row failed to parse = upstream schema change (renamed /
	// re-typed columns), NOT a silent empty success.
	if len(quotes) == 0 {
		// P1-7: all-rows-parse-failed is a schema change (upstream problem),
		// NOT a no-data condition — it must trip the breaker.
		c.breakerRecordFailure()
		return nil, fmt.Errorf("twse: all %d rows failed to parse (schema change?)", parseFailures)
	}

	c.breakerRecordSuccess()
	return quotes, nil
}

// breakerRecordSuccess / breakerRecordFailure are nil-safe breaker wrappers
// (hand-constructed TWSEClient values in tests may have a nil breaker).
func (c *TWSEClient) breakerRecordSuccess() {
	if c.breaker != nil {
		c.breaker.recordSuccess()
	}
}

func (c *TWSEClient) breakerRecordFailure() {
	if c.breaker != nil {
		c.breaker.recordFailure()
	}
}

// BreakerInfo exposes the breaker state for tests and observability.
func (c *TWSEClient) BreakerInfo() ProviderBreakerInfo {
	if c.breaker == nil {
		return ProviderBreakerInfo{Name: "twse", State: ProviderCircuitClosed}
	}
	return c.breaker.stateSnapshot()
}

// GetQuote 获取单个股票行情
func (c *TWSEClient) GetQuote(ctx context.Context, symbol string) (domain.Quote, error) {
	// TWSE OpenAPI 只提供批量接口，我们获取全部后过滤
	quotes, err := c.GetQuotes(ctx)
	if err != nil {
		return domain.Quote{}, err
	}

	for _, q := range quotes {
		if q.Symbol == symbol {
			return q, nil
		}
	}

	return domain.Quote{}, fmt.Errorf("%w: %s", ErrTWSEQuoteNotFound, symbol)
}

// GetQuotesBySymbols 获取指定股票列表的行情
func (c *TWSEClient) GetQuotesBySymbols(ctx context.Context, symbols []string) ([]domain.Quote, error) {
	allQuotes, err := c.GetQuotes(ctx)
	if err != nil {
		return nil, err
	}

	symbolMap := make(map[string]bool)
	for _, s := range symbols {
		symbolMap[s] = true
	}

	filtered := make([]domain.Quote, 0)
	for _, q := range allQuotes {
		if symbolMap[q.Symbol] {
			filtered = append(filtered, q)
		}
	}

	return filtered, nil
}

// GetDailyQuote 获取指定日期和股票的行情
func (c *TWSEClient) GetDailyQuote(ctx context.Context, date string, symbol string) (domain.Quote, error) {
	// P1-7: client-level breaker (same shared breaker as GetQuotes).
	if c.breaker != nil && !c.breaker.shouldTry() {
		return domain.Quote{}, fmt.Errorf("%w: twse circuit breaker open", ErrUpstream)
	}
	waitCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := c.rateLimiter.Wait(waitCtx); err != nil {
		return domain.Quote{}, fmt.Errorf("rate limit wait (10s timeout): %w", ErrRateLimited)
	}

	endpoint := fmt.Sprintf("%s/exchangeReport/STOCK_DAY", c.baseURL)
	params := url.Values{}
	params.Set("response", "json")
	params.Set("date", date)
	params.Set("stockNo", symbol)

	reqURL := fmt.Sprintf("%s?%s", endpoint, params.Encode())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return domain.Quote{}, fmt.Errorf("create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.breakerRecordFailure()
		return domain.Quote{}, fmt.Errorf("http request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		c.breakerRecordFailure()
		return domain.Quote{}, fmt.Errorf("api error: status %d", resp.StatusCode)
	}

	// Buffer body to bytes (bytes-read pattern) so charset transcoding via
	// DecodeJSON uses a fresh reader. Mirrors twse_margin_provider.go:110-118.
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		c.breakerRecordFailure()
		return domain.Quote{}, fmt.Errorf("read body: %w", err)
	}

	var dailyResp TWSEDailyResponse
	if err := DecodeJSON(bytes.NewReader(body), resp.Header.Get("Content-Type"), &dailyResp); err != nil {
		c.breakerRecordFailure()
		return domain.Quote{}, fmt.Errorf("decode response: %w", err)
	}

	if dailyResp.Stat != "OK" || len(dailyResp.Data) == 0 {
		// P1-7: no-data for the requested date (holiday/weekend) is expected
		// — must NOT trip the breaker.
		c.breakerRecordSuccess()
		return domain.Quote{}, fmt.Errorf("no data for %s on %s", symbol, date)
	}

	// TWSE STOCK_DAY returns the whole MONTH for the requested date (the day
	// component is ignored server-side), so Data[0] is the month's first
	// trading day, not the requested day. Find the row matching the requested
	// date (ROC calendar, e.g. "115/08/20"); a weekday that never traded
	// (holiday) simply has no matching row → "no data", which callers treat
	// as a non-trading day. Same date-matching pattern as
	// cmd/extend-replay-etf/main.go:fetchStockDay.
	t, err := time.Parse("20060102", date)
	if err != nil {
		return domain.Quote{}, fmt.Errorf("invalid date %q: %w", date, err)
	}
	wantROC := fmt.Sprintf("%d/%02d/%02d", t.Year()-1911, t.Month(), t.Day())
	for _, row := range dailyResp.Data {
		if len(row) > 0 && strings.TrimSpace(row[0]) == wantROC {
			quote, err := c.convertDailyRowToQuote(row, symbol)
			if err != nil {
				c.breakerRecordFailure()
				return domain.Quote{}, err
			}
			c.breakerRecordSuccess()
			return quote, nil
		}
	}

	// Date not present in the month's rows = that day never traded (holiday).
	c.breakerRecordSuccess()
	return domain.Quote{}, fmt.Errorf("no data for %s on %s", symbol, date)
}

// convertToQuote 将 TWSE 数据转换为 domain.Quote
func (c *TWSEClient) convertToQuote(twse TWSEQuote) (domain.Quote, error) {
	last, err := strconv.ParseFloat(twse.ClosingPrice, 64)
	if err != nil {
		return domain.Quote{}, fmt.Errorf("parse closing price: %w", err)
	}

	open, _ := strconv.ParseFloat(twse.OpeningPrice, 64)
	high, _ := strconv.ParseFloat(twse.HighestPrice, 64)
	low, _ := strconv.ParseFloat(twse.LowestPrice, 64)
	volume, _ := strconv.ParseInt(twse.TradeVolume, 10, 64)

	return domain.Quote{
		Symbol:     twse.Code,
		Last:       last,
		Open:       open,
		High:       high,
		Low:        low,
		Volume:     volume,
		Market:     "TW",
		AsOf:       time.Now(),
		IsTradable: true,
		Source:     "twse",
	}, nil
}

// convertDailyRowToQuote 將 TWSE 每日數據陣列轉換為 domain.Quote
// data row: [日期, 成交股數, 成交金額, 開盤價, 最高價, 最低價, 收盤價, 漲跌價差, 成交筆數, 註記]
func (c *TWSEClient) convertDailyRowToQuote(row []string, symbol string) (domain.Quote, error) {
	if len(row) < 7 {
		return domain.Quote{}, fmt.Errorf("invalid row length: %d", len(row))
	}

	closeStr := stripCommas(row[6])
	last, err := strconv.ParseFloat(closeStr, 64)
	if err != nil {
		return domain.Quote{}, fmt.Errorf("parse close price: %w", err)
	}

	open, _ := strconv.ParseFloat(stripCommas(row[3]), 64)
	high, _ := strconv.ParseFloat(stripCommas(row[4]), 64)
	low, _ := strconv.ParseFloat(stripCommas(row[5]), 64)
	volume, _ := strconv.ParseInt(stripCommas(row[1]), 10, 64)

	return domain.Quote{
		Symbol:     symbol,
		Last:       last,
		Open:       open,
		High:       high,
		Low:        low,
		Volume:     volume,
		Market:     "TW",
		AsOf:       time.Now(),
		IsTradable: true,
		Source:     "twse",
	}, nil
}

// stripCommas removes commas from numeric strings (e.g., "1,940.00" -> "1940.00")
func stripCommas(s string) string {
	return strings.ReplaceAll(s, ",", "")
}

// TWSEOpenAPIProvider 实现 marketdata.Provider 接口
type TWSEOpenAPIProvider struct {
	client *TWSEClient
}

// NewTWSEOpenAPIProvider 创建 TWSE OpenAPI Provider
func NewTWSEOpenAPIProvider() *TWSEOpenAPIProvider {
	return &TWSEOpenAPIProvider{
		client: GetSharedTWSEClient(),
	}
}

// Name 返回 Provider 名称
func (p *TWSEOpenAPIProvider) Name() string {
	return "twse-openapi"
}

// GetQuotes 实现 Provider 接口
func (p *TWSEOpenAPIProvider) GetQuotes(ctx context.Context, asOf time.Time, symbols []string) ([]domain.Quote, error) {
	if len(symbols) == 1 {
		quote, err := p.client.GetQuote(ctx, symbols[0])
		if err != nil {
			return nil, err
		}
		return []domain.Quote{quote}, nil
	}
	return p.client.GetQuotesBySymbols(ctx, symbols)
}

// CheckMarketStatus 检查市场状态（通过获取数据判断是否开市）
func (c *TWSEClient) CheckMarketStatus(ctx context.Context) (bool, error) {
	_, err := c.GetQuotes(ctx)
	if err != nil {
		return false, err
	}
	return true, nil
}

// rowAt returns the element at index if within bounds, otherwise fallback.
func rowAt(row []string, idx int, fallback string) string {
	if idx < len(row) {
		return row[idx]
	}
	return fallback
}

// isCSVContentType reports whether the Content-Type header indicates a CSV
// response. TWSE changed STOCK_DAY_ALL from JSON to CSV (2026-06-30);
// GetQuotes uses this as a fallback trigger when DecodeJSON fails.
func isCSVContentType(contentType string) bool {
	return strings.HasPrefix(strings.ToLower(contentType), "text/csv")
}

// parseStockCSV parses a TWSE STOCK_DAY_ALL CSV response into domain.Quote
// records. TWSE uses standard CSV quoting (double-quote, RFC 4180) with
// columns: 日期,證券代號,證券名稱,成交股數,成交金額,開盤價,最高價,最低價,
// 收盤價,漲跌價差,成交筆數.
func (c *TWSEClient) parseStockCSV(body io.Reader) ([]domain.Quote, error) {
	r := csv.NewReader(body)
	r.FieldsPerRecord = -1

	headers, err := r.Read()
	if err != nil {
		return nil, fmt.Errorf("csv header: %w", err)
	}
	col := make(map[string]int)
	for i, h := range headers {
		col[h] = i
	}
	symbolIdx, ok := col["證券代號"]
	if !ok {
		return nil, fmt.Errorf("csv missing column 證券代號")
	}

	var quotes []domain.Quote
	parseFailures := 0
	rows := 0
	for {
		row, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("csv row: %w", err)
		}
		rows++
		symbol := rowAt(row, symbolIdx, "")
		if symbol == "" {
			continue
		}
		q := TWSEQuote{
			Code:         symbol,
			Name:         rowAt(row, col["證券名稱"], ""),
			TradeVolume:  rowAt(row, col["成交股數"], ""),
			TradeValue:   rowAt(row, col["成交金額"], ""),
			OpeningPrice: rowAt(row, col["開盤價"], ""),
			HighestPrice: rowAt(row, col["最高價"], ""),
			LowestPrice:  rowAt(row, col["最低價"], ""),
			ClosingPrice: rowAt(row, col["收盤價"], ""),
			Change:       rowAt(row, col["漲跌價差"], ""),
			Transaction:  rowAt(row, col["成交筆數"], ""),
		}
		quote, err := c.convertToQuote(q)
		if err != nil {
			parseFailures++
			continue
		}
		quotes = append(quotes, quote)
	}
	// P0-4: mirror the JSON path — empty CSV or all-rows-unparseable must
	// not surface as an empty success.
	if rows == 0 {
		// P1-7: no-data must not trip the breaker.
		c.breakerRecordSuccess()
		return nil, fmt.Errorf("%w: STOCK_DAY_ALL CSV returned no rows", ErrTWSEEmptyData)
	}
	if len(quotes) == 0 {
		c.breakerRecordFailure()
		return nil, fmt.Errorf("twse: all %d CSV rows failed to parse (schema change?)", parseFailures)
	}
	c.breakerRecordSuccess()
	return quotes, nil
}
