package marketdata

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/logging"
	"golang.org/x/time/rate"
)

const (
	finmindBaseURL   = "https://api.finmindtrade.com/api/v4"
	finmindRateLimit = 600
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

func NewFinMindClient(apiKey string) *FinMindClient {
	return &FinMindClient{
		apiKey: apiKey,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		rateLimiter: rate.NewLimiter(rate.Every(time.Minute/finmindRateLimit), finmindBurst),
	}
}

func (c *FinMindClient) SetHTTPClient(client *http.Client) {
	c.httpClient = client
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
	defer resp.Body.Close()

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

type FinMindProvider struct {
	client *FinMindClient
}

func NewFinMindProviderWithClient(client *FinMindClient) *FinMindProvider {
	return &FinMindProvider{client: client}
}

func NewFinMindProvider(apiKey string) *FinMindProvider {
	return &FinMindProvider{client: NewFinMindClient(apiKey)}
}

func (p *FinMindProvider) Name() string {
	return "finmind"
}

func (p *FinMindProvider) GetQuotes(ctx context.Context, asOf time.Time, symbols []string) ([]domain.Quote, error) {
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
