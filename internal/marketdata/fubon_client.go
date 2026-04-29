package marketdata

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"golang.org/x/time/rate"
)

const (
	fubonAPIBaseURL = "https://api.fubon.com.tw/market-data/v1"
	// 即時行情限制: 300 requests/minute
	fubonIntradayRateLimit = 300
	fubonIntradayBurst     = 30
	// 歷史行情限制: 60 requests/minute
	fubonHistoricalRateLimit = 60
	fubonHistoricalBurst     = 10
)

type FubonClient struct {
	authToken         string
	httpClient        *http.Client
	baseURL           string
	intradayLimiter   *rate.Limiter
	historicalLimiter *rate.Limiter
}

type FubonQuoteResponse struct {
	APIVersion string `json:"api_version"`
	Data       struct {
		Info struct {
			Date        string `json:"date"`
			Time        string `json:"time"`
			Symbol      string `json:"symbol"`
			Name        string `json:"name"`
			CountryCode string `json:"country_code"`
			TimeZone    string `json:"time_zone"`
		} `json:"info"`
		Quote struct {
			Trade struct {
				Price float64 `json:"price"`
			} `json:"trade"`
			PriceOpen struct {
				Price float64 `json:"price"`
			} `json:"price_open"`
			PriceHigh struct {
				Price float64 `json:"price"`
			} `json:"price_high"`
			PriceLow struct {
				Price float64 `json:"price"`
			} `json:"price_low"`
			Total struct {
				TradeVolume int64 `json:"trade_volume"`
			} `json:"total"`
		} `json:"quote"`
	} `json:"data"`
}

func NewFubonClient(authToken string) *FubonClient {
	return &FubonClient{
		authToken: authToken,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		baseURL:           fubonAPIBaseURL,
		intradayLimiter:   rate.NewLimiter(rate.Every(time.Minute/fubonIntradayRateLimit), fubonIntradayBurst),
		historicalLimiter: rate.NewLimiter(rate.Every(time.Minute/fubonHistoricalRateLimit), fubonHistoricalBurst),
	}
}

func (c *FubonClient) SetHTTPClient(client *http.Client) {
	c.httpClient = client
}

func (c *FubonClient) GetQuote(ctx context.Context, symbol string) (domain.Quote, error) {
	if err := c.intradayLimiter.Wait(ctx); err != nil {
		return domain.Quote{}, fmt.Errorf("fubon api: rate limit wait: %w", err)
	}

	endpoint := fmt.Sprintf("%s/intraday/quote", c.baseURL)
	params := url.Values{}
	params.Set("symbol_id", symbol)

	reqURL := fmt.Sprintf("%s?%s", endpoint, params.Encode())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return domain.Quote{}, fmt.Errorf("fubon api: create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.authToken)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return domain.Quote{}, fmt.Errorf("fubon api: http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return domain.Quote{}, fmt.Errorf("fubon api: status %d", resp.StatusCode)
	}

	var fubonResp FubonQuoteResponse
	if err := json.NewDecoder(resp.Body).Decode(&fubonResp); err != nil {
		return domain.Quote{}, fmt.Errorf("fubon api: decode response: %w", err)
	}

	quote := domain.Quote{
		Symbol:     symbol,
		Last:       fubonResp.Data.Quote.Trade.Price,
		Open:       fubonResp.Data.Quote.PriceOpen.Price,
		High:       fubonResp.Data.Quote.PriceHigh.Price,
		Low:        fubonResp.Data.Quote.PriceLow.Price,
		Volume:     fubonResp.Data.Quote.Total.TradeVolume,
		Market:     "TW",
		AsOf:       time.Now(),
		IsTradable: true,
		Source:     "fubon",
	}

	return quote, nil
}

func (c *FubonClient) GetQuotes(ctx context.Context, symbols []string) ([]domain.Quote, error) {
	quotes := make([]domain.Quote, 0, len(symbols))

	for _, symbol := range symbols {
		quote, err := c.GetQuote(ctx, symbol)
		if err != nil {
			fmt.Printf("[Fubon] Error fetching %s: %v\n", symbol, err)
			continue
		}
		quotes = append(quotes, quote)
	}

	return quotes, nil
}

func (c *FubonClient) GetHistoricalQuote(ctx context.Context, symbol string, date time.Time) (domain.Quote, error) {
	if err := c.historicalLimiter.Wait(ctx); err != nil {
		return domain.Quote{}, fmt.Errorf("fubon api: historical rate limit wait: %w", err)
	}
	return domain.Quote{}, fmt.Errorf("fubon api: historical quote not yet implemented")
}

func (c *FubonClient) CheckMarketStatus(ctx context.Context) (bool, error) {
	_, err := c.GetQuote(ctx, "0050")
	if err != nil {
		return false, fmt.Errorf("fubon api: check market status: %w", err)
	}
	return true, nil
}
