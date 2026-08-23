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
	"github.com/kaecer68/atlas-go/internal/constants"
)

// DayTradingStats holds daily day trading statistics from TWSE.
type DayTradingStats struct {
	Date                string  `json:"date"`
	DayTradingVolume    int64   `json:"day_trading_volume"`
	VolumeRatio         float64 `json:"volume_ratio"`
	DayTradingBuyValue  int64   `json:"day_trading_buy_value"`
	BuyValueRatio       float64 `json:"buy_value_ratio"`
	DayTradingSellValue int64   `json:"day_trading_sell_value"`
	SellValueRatio      float64 `json:"sell_value_ratio"`
}

// DayTradingProvider fetches Taiwan day trading statistics from TWSE.
type DayTradingProvider struct {
	client      *http.Client
	baseURL     string
	rateLimiter *rate.Limiter
}

// NewDayTradingProvider creates a new TWSE day trading provider.
func NewDayTradingProvider() *DayTradingProvider {
	return &DayTradingProvider{
		client:      httpclient.NewFactory().NewClient(20 * time.Second),
		baseURL:     constants.TWSEBaseURL,
		rateLimiter: getTWSESharedLimiter(), // P1-13: shared TWSE bucket
	}
}

// SetHTTPClient sets a custom HTTP client for tests.
func (d *DayTradingProvider) SetHTTPClient(client *http.Client) {
	if client != nil {
		d.client = client
	}
}

// Name returns the provider name.
func (d *DayTradingProvider) Name() string {
	return "twse_day_trading"
}

// SetRateLimiter overrides the rate limiter (tests only).
func (d *DayTradingProvider) SetRateLimiter(l *rate.Limiter) {
	if l != nil {
		d.rateLimiter = l
	}
}

// FetchLatest retrieves the most recent day trading statistics.
func (d *DayTradingProvider) FetchLatest(ctx context.Context) (*DayTradingStats, error) {
	now := time.Now().UTC()
	for i := range 7 {
		dateStr := now.AddDate(0, 0, -i).Format("20060102")
		stats, err := d.fetchDate(ctx, dateStr)
		if err == nil {
			return stats, nil
		}
	}
	return nil, fmt.Errorf("no TWSE day trading data available in the last 7 days")
}

func (d *DayTradingProvider) fetchDate(ctx context.Context, dateStr string) (*DayTradingStats, error) {
	if err := d.rateLimiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limit wait: %w", err)
	}

	url := fmt.Sprintf("%s/exchangeReport/TWTB4U?response=json&date=%s", d.baseURL, dateStr)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")

	resp, err := d.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	var apiResp twseDayTradingResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	if apiResp.Stat != "OK" || len(apiResp.Tables) == 0 {
		return nil, fmt.Errorf("TWSE API returned no data: stat=%s tables=%d", apiResp.Stat, len(apiResp.Tables))
	}

	marketTable := apiResp.Tables[0]
	if len(marketTable.Data) == 0 || len(marketTable.Data[0]) < 6 {
		return nil, fmt.Errorf("TWSE API returned empty market data")
	}

	row := marketTable.Data[0]
	stats := &DayTradingStats{
		Date:                dateStr,
		DayTradingVolume:    parseTWSEInt(row[0]),
		VolumeRatio:         parseTWSEPercent(row[1]),
		DayTradingBuyValue:  parseTWSEInt(row[2]),
		BuyValueRatio:       parseTWSEPercent(row[3]),
		DayTradingSellValue: parseTWSEInt(row[4]),
		SellValueRatio:      parseTWSEPercent(row[5]),
	}

	return stats, nil
}

type twseDayTradingResponse struct {
	Stat   string                `json:"stat"`
	Date   string                `json:"date"`
	Tables []twseDayTradingTable `json:"tables"`
}

type twseDayTradingTable struct {
	Title  string     `json:"title"`
	Fields []string   `json:"fields"`
	Data   [][]string `json:"data"`
	Notes  []string   `json:"notes"`
	Total  int        `json:"total"`
}

func parseTWSEInt(s string) int64 {
	s = strings.ReplaceAll(s, ",", "")
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0
	}
	return v
}

func parseTWSEPercent(s string) float64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return v / 100.0
}
