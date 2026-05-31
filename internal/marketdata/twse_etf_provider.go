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
)

// ETFStats holds daily ETF net subscription statistics from TWSE.
type ETFStats struct {
	Date             string  `json:"date"`
	NetSubscription  int64   `json:"net_subscription"`
	TotalNAV         int64   `json:"total_nav"`
	SubscriberCount  int64   `json:"subscriber_count"`
}

// TWSEETFProvider fetches Taiwan ETF net subscription data from TWSE.
type TWSEETFProvider struct {
	client      *http.Client
	baseURL     string
	rateLimiter *rate.Limiter
}

// NewTWSEETFProvider creates a new TWSE ETF provider.
func NewTWSEETFProvider() *TWSEETFProvider {
	return &TWSEETFProvider{
		client:      httpclient.NewFactory().NewClient(20 * time.Second),
		baseURL:     "https://www.twse.com.tw",
		rateLimiter: rate.NewLimiter(rate.Every(1*time.Second), 1),
	}
}

// Name returns the provider name.
func (p *TWSEETFProvider) Name() string {
	return "twse_etf"
}

// FetchLatest retrieves the most recent ETF net subscription statistics.
func (p *TWSEETFProvider) FetchLatest(ctx context.Context) (*ETFStats, error) {
	now := time.Now().UTC()
	for i := range 7 {
		dateStr := now.AddDate(0, 0, -i).Format("20060102")
		stats, err := p.fetchDate(ctx, dateStr)
		if err == nil {
			return stats, nil
		}
	}
	return nil, fmt.Errorf("no TWSE ETF data available in the last 7 days")
}

func (p *TWSEETFProvider) fetchDate(ctx context.Context, dateStr string) (*ETFStats, error) {
	if err := p.rateLimiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limit wait: %w", err)
	}

	url := fmt.Sprintf("%s/exchangeReport/TWT44U?response=json&date=%s", p.baseURL, dateStr)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	var apiResp twseETFResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	if apiResp.Stat != "OK" || len(apiResp.Tables) == 0 {
		return nil, fmt.Errorf("TWSE API returned no data: stat=%s tables=%d", apiResp.Stat, len(apiResp.Tables))
	}

	// ETF net subscription data is typically in the summary row of the first table.
	marketTable := apiResp.Tables[0]
	if len(marketTable.Data) == 0 {
		return nil, fmt.Errorf("TWSE API returned empty ETF data")
	}

	// Aggregate across all ETF rows to compute totals.
	var netSubTotal int64
	var navTotal int64
	var subscriberTotal int64

	for _, row := range marketTable.Data {
		if len(row) < 4 {
			continue
		}
		// Typical TWSE TWT44U columns:
		// [0] ETF name, [1] Net Subscription (units),
		// [2] Total NAV, [3] Subscriber Count
		netSubTotal += parseTWSEInt(row[1])
		navTotal += parseTWSEInt(row[2])

		// Subscriber count from a later column if available
		if len(row) > 3 {
			subscriberTotal += parseTWSEInt(row[3])
		}
	}

	stats := &ETFStats{
		Date:            dateStr,
		NetSubscription: netSubTotal,
		TotalNAV:        navTotal,
		SubscriberCount: subscriberTotal,
	}

	return stats, nil
}

type twseETFResponse struct {
	Stat   string           `json:"stat"`
	Date   string           `json:"date"`
	Tables []twseETFTable   `json:"tables"`
}

type twseETFTable struct {
	Title  string     `json:"title"`
	Fields []string   `json:"fields"`
	Data   [][]string `json:"data"`
	Notes  []string   `json:"notes"`
	Total  int        `json:"total"`
}


