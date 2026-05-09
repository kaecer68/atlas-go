package marketdata

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/logging"
	"golang.org/x/time/rate"
)

const (
	fugleAPIBaseURL = "https://api.fugle.tw/realtime/v0.3"
)

// FugleClient Fugle API 客户端
type FugleClient struct {
	apiKey      string
	httpClient  *http.Client
	baseURL     string
	rateLimiter *rate.Limiter
}

// FugleQuoteResponse Fugle 行情响应
type FugleQuoteResponse struct {
	APIVersion string `json:"apiVersion"`
	Data       struct {
		Info struct {
			Date        string `json:"date"`
			Time        string `json:"time"`
			Symbol      string `json:"symbol"`
			Name        string `json:"name"`
			CountryCode string `json:"countryCode"`
			TimeZone    string `json:"timeZone"`
		} `json:"info"`
		Quote struct {
			Trade struct {
				Price float64 `json:"price"`
			} `json:"trade"`
			PriceOpen struct {
				Price float64 `json:"price"`
			} `json:"priceOpen"`
			PriceHigh struct {
				Price float64 `json:"price"`
			} `json:"priceHigh"`
			PriceLow struct {
				Price float64 `json:"price"`
			} `json:"priceLow"`
			Total struct {
				TradeVolume int64 `json:"tradeVolume"`
			} `json:"total"`
		} `json:"quote"`
	} `json:"data"`
}

// NewFugleClient 创建 Fugle 客户端
func NewFugleClient(apiKey string) *FugleClient {
	params := config.GetParametersConfig()
	return &FugleClient{
		apiKey: apiKey,
		httpClient: &http.Client{
			Timeout: time.Duration(params.Marketdata.FugleAPITimeoutSec.Value) * time.Second,
		},
		baseURL:     fugleAPIBaseURL,
		rateLimiter: rate.NewLimiter(rate.Every(time.Minute/time.Duration(params.Marketdata.FugleRateLimit.Value)), params.Marketdata.FugleRateLimit.Value),
	}
}

// SetHTTPClient 设置自定义 HTTP 客户端
func (c *FugleClient) SetHTTPClient(client *http.Client) {
	c.httpClient = client
}

// GetQuote 获取单个股票行情
func (c *FugleClient) GetQuote(ctx context.Context, symbol string) (domain.Quote, error) {
	// 等待速率限制
	if err := c.rateLimiter.Wait(ctx); err != nil {
		return domain.Quote{}, fmt.Errorf("rate limit wait: %w", err)
	}

	// 构建 URL
	endpoint := fmt.Sprintf("%s/intraday/quote", c.baseURL)
	params := url.Values{}
	params.Set("symbolId", symbol)
	params.Set("apiToken", c.apiKey)

	reqURL := fmt.Sprintf("%s?%s", endpoint, params.Encode())

	// 发送请求
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return domain.Quote{}, fmt.Errorf("create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return domain.Quote{}, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return domain.Quote{}, fmt.Errorf("api error: status %d, body: %s", resp.StatusCode, string(body))
	}

	// 解析响应
	var fugleResp FugleQuoteResponse
	if err := json.NewDecoder(resp.Body).Decode(&fugleResp); err != nil {
		return domain.Quote{}, fmt.Errorf("decode response: %w", err)
	}

	// 转换为 domain.Quote
	quote := domain.Quote{
		Symbol:     symbol,
		Last:       fugleResp.Data.Quote.Trade.Price,
		Open:       fugleResp.Data.Quote.PriceOpen.Price,
		High:       fugleResp.Data.Quote.PriceHigh.Price,
		Low:        fugleResp.Data.Quote.PriceLow.Price,
		Volume:     fugleResp.Data.Quote.Total.TradeVolume,
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

// GetMeta 获取股票元数据
func (c *FugleClient) GetMeta(ctx context.Context, symbol string) (*FugleMetaResponse, error) {
	if err := c.rateLimiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limit wait: %w", err)
	}

	endpoint := fmt.Sprintf("%s/intraday/meta", c.baseURL)
	params := url.Values{}
	params.Set("symbolId", symbol)
	params.Set("apiToken", c.apiKey)

	reqURL := fmt.Sprintf("%s?%s", endpoint, params.Encode())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

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
	APIVersion string `json:"apiVersion"`
	Data       struct {
		Info struct {
			Date        string `json:"date"`
			Time        string `json:"time"`
			Symbol      string `json:"symbol"`
			Name        string `json:"name"`
			CountryCode string `json:"countryCode"`
			TimeZone    string `json:"timeZone"`
		} `json:"info"`
		Meta struct {
			Market         string  `json:"market"`
			NameZhTW       string  `json:"nameZhTw"`
			IndustryZhTW   string  `json:"industryZhTw"`
			TypeZhTW       string  `json:"typeZhTw"`
			IsIndex        bool    `json:"isIndex"`
			IsWarrant      bool    `json:"isWarrant"`
			IsSuspended    bool    `json:"isSuspended"`
			IsDelisted     bool    `json:"isDelisted"`
			ReferencePrice float64 `json:"referencePrice"`
			LimitUpPrice   float64 `json:"limitUpPrice"`
			LimitDownPrice float64 `json:"limitDownPrice"`
			PreviousClose  float64 `json:"previousClose"`
		} `json:"meta"`
	} `json:"data"`
}

// CheckMarketStatus 检查市场状态
func (c *FugleClient) CheckMarketStatus(ctx context.Context) (bool, error) {
	// 使用 0050 (元大台灣50) 作为市场指标
	meta, err := c.GetMeta(ctx, "0050")
	if err != nil {
		return false, err
	}

	// 检查是否为交易日且未停牌
	return !meta.Data.Meta.IsSuspended && !meta.Data.Meta.IsDelisted, nil
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
