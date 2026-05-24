package marketdata

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/internal/apigateway/httpclient"
	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
	"golang.org/x/time/rate"
)

const (
	twseAPIBaseURL = "https://www.twse.com.tw"
)

// TWSEClient TWSE OpenAPI 客户端
type TWSEClient struct {
	httpClient  *http.Client
	baseURL     string
	rateLimiter *rate.Limiter
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

// NewTWSEClient 创建 TWSE OpenAPI 客户端
func NewTWSEClient() *TWSEClient {
	params := config.GetParametersConfig()
	return &TWSEClient{
		httpClient:  httpclient.NewFactory().NewClient(time.Duration(params.Marketdata.TWSEAPITimeoutSec.Value) * time.Second),
		baseURL:     twseAPIBaseURL,
		rateLimiter: rate.NewLimiter(rate.Limit(params.Marketdata.TWSEAPIRateLimit.Value), params.Marketdata.TWSEAPIRateBurst.Value),
	}
}

// SetHTTPClient 设置自定义 HTTP 客户端
func (c *TWSEClient) SetHTTPClient(client *http.Client) {
	c.httpClient = client
}

// GetQuotes 批量获取当日所有上市股票行情
func (c *TWSEClient) GetQuotes(ctx context.Context) ([]domain.Quote, error) {
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
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("api error: status %d", resp.StatusCode)
	}

	var twseResp TWSEDailyResponse
	if err := json.NewDecoder(resp.Body).Decode(&twseResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
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

	quotes := make([]domain.Quote, 0, len(twseQuotes))
	for _, q := range twseQuotes {
		quote, err := c.convertToQuote(q)
		if err != nil {
			// 跳过解析失败的记录
			continue
		}
		quotes = append(quotes, quote)
	}

	return quotes, nil
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

	return domain.Quote{}, fmt.Errorf("symbol %s not found", symbol)
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
		return domain.Quote{}, fmt.Errorf("http request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return domain.Quote{}, fmt.Errorf("api error: status %d", resp.StatusCode)
	}

	var dailyResp TWSEDailyResponse
	if err := json.NewDecoder(resp.Body).Decode(&dailyResp); err != nil {
		return domain.Quote{}, fmt.Errorf("decode response: %w", err)
	}

	if dailyResp.Stat != "OK" || len(dailyResp.Data) == 0 {
		return domain.Quote{}, fmt.Errorf("no data for %s on %s", symbol, date)
	}

	return c.convertDailyRowToQuote(dailyResp.Data[0], symbol)
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
		client: NewTWSEClient(),
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
