// Maturity: experimental
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

// TWSEETFStats holds aggregated TWSE ETF net subscription data.
type TWSEETFStats struct {
	NetSubscription float64 `json:"net_subscription"`
	NetRedemption   float64 `json:"net_redemption"`
	NetFlow         float64 `json:"net_flow"`
	ETFCount        int     `json:"etf_count"`
	Date            string  `json:"date"`
}

// TWSEETFProvider fetches Taiwan ETF net subscription/redemption flow from TWSE.
type TWSEETFProvider struct {
	client      *http.Client
	baseURL     string
	rateLimiter *rate.Limiter
}

// NewTWSEETFProvider creates a new TWSE ETF subscription provider.
func NewTWSEETFProvider() *TWSEETFProvider {
	return &TWSEETFProvider{
		client:      httpclient.NewFactory().NewClient(20 * time.Second),
		baseURL:     "https://www.twse.com.tw",
		rateLimiter: rate.NewLimiter(rate.Every(2*time.Second), 1),
	}
}

// Name returns the provider name.
func (p *TWSEETFProvider) Name() string {
	return "twse_etf"
}

// FetchNetSubscription retrieves ETF net subscription data for a given date.
func (p *TWSEETFProvider) FetchNetSubscription(ctx context.Context, date string) (*TWSEETFStats, error) {
	if err := p.rateLimiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limit wait: %w", err)
	}

	url := fmt.Sprintf("%s/rwd/zh/ETF/etfDailyNetFlow?response=json&date=%s", p.baseURL, date)
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
		return nil, fmt.Errorf("TWSE ETF API returned no data: stat=%s tables=%d", apiResp.Stat, len(apiResp.Tables))
	}

	mainTable := apiResp.Tables[0]
	if len(mainTable.Data) == 0 {
		return nil, fmt.Errorf("TWSE ETF API returned empty data")
	}

	stats := &TWSEETFStats{
		Date: date,
	}

	var totalSub, totalRed float64
	for _, row := range mainTable.Data {
		// Typical TWSE ETF flow row: [code, name, net_subscription, net_redemption, net_flow]
		// Columns may vary; try to parse what's available
		if len(row) >= 5 {
			totalSub += parseTWSEFloat(row[2])
			totalRed += parseTWSEFloat(row[3])
		} else if len(row) >= 3 {
			totalSub += parseTWSEFloat(row[1])
			totalRed += parseTWSEFloat(row[2])
		}
	}

	stats.NetSubscription = totalSub
	stats.NetRedemption = totalRed
	stats.NetFlow = totalSub - totalRed
	stats.ETFCount = len(mainTable.Data)

	return stats, nil
}

type twseETFResponse struct {
	Stat   string         `json:"stat"`
	Date   string         `json:"date"`
	Tables []twseETFTable `json:"tables"`
}

type twseETFTable struct {
	Title  string     `json:"title"`
	Fields []string   `json:"fields"`
	Data   [][]string `json:"data"`
	Notes  []string   `json:"notes"`
	Total  int        `json:"total"`
}
