// Maturity: experimental

package marketdata

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"golang.org/x/time/rate"

	"github.com/kaecer68/atlas-go/internal/apigateway/httpclient"
)

// TWSEOddLotStats holds after-hours odd-lot (零股) trading statistics from TWSE.
type TWSEOddLotStats struct {
	Date             string  `json:"date"`
	OddLotBuyVolume  float64 `json:"odd_lot_buy_volume"`
	OddLotSellVolume float64 `json:"odd_lot_sell_volume"`
	OddLotNetVolume  float64 `json:"odd_lot_net_volume"`
	OddLotRatio      float64 `json:"odd_lot_ratio"`
}

// TWSEOddLotProvider fetches after-hours odd-lot (零股) trading data from TWSE.
type TWSEOddLotProvider struct {
	client      *http.Client
	baseURL     string
	rateLimiter *rate.Limiter
}

// NewTWSEOddLotProvider creates a new TWSE odd-lot provider.
func NewTWSEOddLotProvider() *TWSEOddLotProvider {
	return &TWSEOddLotProvider{
		client:      httpclient.NewFactory().NewClient(20 * time.Second),
		baseURL:     "https://www.twse.com.tw",
		rateLimiter: rate.NewLimiter(rate.Every(2*time.Second), 1),
	}
}

// Name returns the provider name.
func (o *TWSEOddLotProvider) Name() string {
	return "twse_oddlot"
}

// FetchLatest retrieves the most recent after-hours odd-lot trading statistics.
func (o *TWSEOddLotProvider) FetchLatest(ctx context.Context) (*TWSEOddLotStats, error) {
	now := time.Now().UTC()
	for i := range 7 {
		dateStr := now.AddDate(0, 0, -i).Format("20060102")
		stats, err := o.fetchDate(ctx, dateStr)
		if err == nil {
			return stats, nil
		}
	}
	return nil, fmt.Errorf("no TWSE odd-lot data available in the last 7 days")
}

func (o *TWSEOddLotProvider) fetchDate(ctx context.Context, dateStr string) (*TWSEOddLotStats, error) {
	if err := o.rateLimiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limit wait: %w", err)
	}

	url := fmt.Sprintf("%s/rwd/zh/afterTrading/STOCK_DAY?response=json&date=%s", o.baseURL, dateStr)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")

	resp, err := o.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	var apiResp twseOddLotResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	if apiResp.Stat != "OK" || len(apiResp.Tables) == 0 {
		return nil, fmt.Errorf("TWSE API returned no data: stat=%s tables=%d", apiResp.Stat, len(apiResp.Tables))
	}

	oddTable := apiResp.Tables[0]
	if len(oddTable.Data) == 0 || len(oddTable.Data[0]) < 4 {
		return nil, fmt.Errorf("TWSE API returned empty odd-lot data")
	}

	row := oddTable.Data[0]
	stats := &TWSEOddLotStats{
		Date:             dateStr,
		OddLotBuyVolume:  parseTWSEFloat(row[0]),
		OddLotSellVolume: parseTWSEFloat(row[1]),
		OddLotNetVolume:  parseTWSEFloat(row[2]),
		OddLotRatio:      parseTWSEFloat(row[3]),
	}

	return stats, nil
}

type twseOddLotResponse struct {
	Stat   string            `json:"stat"`
	Date   string            `json:"date"`
	Title  string            `json:"title"`
	Tables []twseOddLotTable `json:"tables"`
}

type twseOddLotTable struct {
	Title  string     `json:"title"`
	Fields []string   `json:"fields"`
	Data   [][]string `json:"data"`
	Notes  []string   `json:"notes"`
	Total  int        `json:"total"`
}

func parseTWSEFloat(s string) float64 {
	s = strings.ReplaceAll(s, ",", "")
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return v
}
