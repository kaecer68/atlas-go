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

// OddLotStats holds after-hours odd-lot trading statistics from TWSE.
type OddLotStats struct {
	Date           string  `json:"date"`
	BuyVolume      int64   `json:"buy_volume"`
	SellVolume     int64   `json:"sell_volume"`
	ImbalanceRatio float64 `json:"imbalance_ratio"`
}

// TWSEOddLotProvider fetches Taiwan after-hours odd-lot trading data from TWSE.
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
		rateLimiter: rate.NewLimiter(rate.Every(1*time.Second), 1),
	}
}

// Name returns the provider name.
func (p *TWSEOddLotProvider) Name() string {
	return "twse_oddlot"
}

// FetchLatest retrieves the most recent odd-lot trading statistics.
func (p *TWSEOddLotProvider) FetchLatest(ctx context.Context) (*OddLotStats, error) {
	now := time.Now().UTC()
	for i := range 7 {
		dateStr := now.AddDate(0, 0, -i).Format("20060102")
		stats, err := p.fetchDate(ctx, dateStr)
		if err == nil {
			return stats, nil
		}
	}
	return nil, fmt.Errorf("no TWSE odd-lot data available in the last 7 days")
}

func (p *TWSEOddLotProvider) fetchDate(ctx context.Context, dateStr string) (*OddLotStats, error) {
	if err := p.rateLimiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limit wait: %w", err)
	}

	url := fmt.Sprintf("%s/exchangeReport/BFI84U?response=json&date=%s", p.baseURL, dateStr)
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

	var apiResp twseOddLotResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	if apiResp.Stat != "OK" || len(apiResp.Tables) == 0 {
		return nil, fmt.Errorf("TWSE API returned no data: stat=%s tables=%d", apiResp.Stat, len(apiResp.Tables))
	}

	marketTable := apiResp.Tables[0]
	if len(marketTable.Data) == 0 {
		return nil, fmt.Errorf("TWSE API returned empty odd-lot data")
	}

	// Aggregate per-stock odd-lot trading volumes.
	// BFI84U per-stock rows typically contain: symbol, name, trade_volume, trade_value, open, high, low, close, ...
	// We aggregate trade_volume (column index 2) across all stocks as total volume,
	// and compute buy/sell split from close-to-open price direction as a heuristic.
	var buyVolume int64
	var sellVolume int64

	for _, row := range marketTable.Data {
		if len(row) < 9 {
			continue
		}
		vol := parseTWSEInt(row[2])
		if vol <= 0 {
			continue
		}

		// Heuristic: if close > open, the net volume is buy-dominant;
		// if close < open, sell-dominant. Proportional to close/open ratio.
		openPrice := parseTWSEFloat(row[5])
		closePrice := parseTWSEFloat(row[8])
		if closePrice > openPrice && openPrice > 0 {
			buyVolume += vol
		} else if closePrice < openPrice && closePrice > 0 {
			sellVolume += vol
		} else {
			// Flat: split evenly
			buyVolume += vol / 2
			sellVolume += vol - vol/2
		}
	}

	stats := &OddLotStats{
		Date:       dateStr,
		BuyVolume:  buyVolume,
		SellVolume: sellVolume,
	}

	if buyVolume+sellVolume > 0 {
		stats.ImbalanceRatio = float64(buyVolume-sellVolume) / float64(buyVolume+sellVolume)
	}

	return stats, nil
}

// parseTWSEFloat parses a TWSE-formatted float string (may contain commas).
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

type twseOddLotResponse struct {
	Stat   string            `json:"stat"`
	Date   string            `json:"date"`
	Tables []twseOddLotTable `json:"tables"`
}

type twseOddLotTable struct {
	Title  string     `json:"title"`
	Fields []string   `json:"fields"`
	Data   [][]string `json:"data"`
	Notes  []string   `json:"notes"`
	Total  int        `json:"total"`
}
