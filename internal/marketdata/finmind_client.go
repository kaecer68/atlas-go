package marketdata

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
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

type FinMindClient struct {
	apiKey      string
	httpClient  *http.Client
	rateLimiter *rate.Limiter
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
// The apiKey is used only on first call; subsequent calls ignore it.
func GetSharedFinMindClient(apiKey string) *FinMindClient {
	sharedFinMindClientOnce.Do(func() {
		sharedFinMindClient = &FinMindClient{
			apiKey:      apiKey,
			httpClient:  httpclient.NewFactory().NewClient(30 * time.Second),
			rateLimiter: rate.NewLimiter(rate.Every(time.Hour/finmindRateLimit), finmindBurst),
		}
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
	return &FinMindClient{
		apiKey:      apiKey,
		httpClient:  httpclient.NewFactory().NewClient(30 * time.Second),
		rateLimiter: rate.NewLimiter(rate.Every(time.Hour/finmindRateLimit), finmindBurst),
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

func (c *FinMindClient) fetchDataset(ctx context.Context, dataset string, dataId string, startDate string, endDate string) ([]map[string]any, error) {
	if err := c.rateLimiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("finmind: rate limit wait: %w", ErrRateLimited)
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

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("finmind: status %d", resp.StatusCode)
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

func (c *FinMindClient) GetMonthRevenue(ctx context.Context, symbol string, year int, month int) (float64, error) {
	startDate := fmt.Sprintf("%d-%02d-01", year, month)
	endDate := fmt.Sprintf("%d-%02d-31", year, month)

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
	endDate := fmt.Sprintf("%d-12-31", year)

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
