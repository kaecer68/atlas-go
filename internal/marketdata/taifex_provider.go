// Package marketdata: TAIFEX data provider.
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

// TaifexDailyStats holds PCR (Put/Call Ratio) and retail futures OI data from TAIFEX.
type TaifexDailyStats struct {
	Date          string  `json:"date"`
	PutCallRatio  float64 `json:"put_call_ratio"`
	RetailLongOI  int64   `json:"retail_long_oi"`
	RetailShortOI int64   `json:"retail_short_oi"`
	RetailOIRatio float64 `json:"retail_oi_ratio"`
}

// TaifexProvider fetches Taiwan futures exchange daily market statistics.
type TaifexProvider struct {
	client      *http.Client
	baseURL     string
	rateLimiter *rate.Limiter
}

// NewTaifexProvider creates a new TAIFEX data provider.
func NewTaifexProvider() *TaifexProvider {
	return &TaifexProvider{
		client:      httpclient.NewFactory().NewClient(20 * time.Second),
		baseURL:     "https://www.taifex.com.tw",
		rateLimiter: rate.NewLimiter(rate.Every(3*time.Second), 1),
	}
}

// Name returns the provider name.
func (p *TaifexProvider) Name() string {
	return "taifex_daily"
}

// FetchDailyStats retrieves TAIFEX daily market report for the given date.
func (p *TaifexProvider) FetchDailyStats(ctx context.Context, date string) (*TaifexDailyStats, error) {
	if err := p.rateLimiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limit wait: %w", err)
	}

	url := fmt.Sprintf("%s/cht/3/futDailyMarketReport?response=json&date=%s", p.baseURL, date)
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

	var apiResp taifexDailyResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	if apiResp.Stat != "OK" || len(apiResp.Tables) == 0 {
		return nil, fmt.Errorf("TAIFEX API returned no data: stat=%s tables=%d", apiResp.Stat, len(apiResp.Tables))
	}

	stats := &TaifexDailyStats{
		Date: date,
	}

	// Iterate tables to extract PCR and retail OI data.
	// TAIFEX daily report returns multiple named tables.
	for _, table := range apiResp.Tables {
		if len(table.Data) == 0 {
			continue
		}
		title := strings.TrimSpace(table.Title)

		// Put/Call Ratio table — typically titled with "PC比值" or "Put/Call"
		if strings.Contains(title, "PC") || strings.Contains(title, "Put") || strings.Contains(title, "Call") {
			stats.PutCallRatio = extractPCRFromTable(table)
		}

		// Retail futures open interest — look for 散戶 or 未平倉 data.
		// TAIFEX provides "三大法人" table with proprietary vs retail breakdown.
		if strings.Contains(title, "法人") || strings.Contains(title, "OI") || strings.Contains(title, "未平倉") {
			long, short, ratio := extractRetailOIFromTable(table)
			if long > 0 || short > 0 {
				stats.RetailLongOI = long
				stats.RetailShortOI = short
				stats.RetailOIRatio = ratio
			}
		}
	}

	// If no structured data found, try the first table which sometimes
	// contains PCR as a summary row.
	if stats.PutCallRatio == 0 && len(apiResp.Tables) > 0 {
		stats.PutCallRatio = extractPCRFromTable(apiResp.Tables[0])
	}

	return stats, nil
}

// extractPCRFromTable scans table rows for Put/Call Ratio values.
func extractPCRFromTable(table taifexDailyTable) float64 {
	for _, row := range table.Data {
		for i, cell := range row {
			val := strings.TrimSpace(cell)
			if val == "" {
				continue
			}
			// PCR is typically expressed as a ratio or percentage.
			// Try parsing as float; if it looks like a ratio (0.5–3.0), return it.
			f := parseTaifexFloat(val)
			if i > 0 && f > 0.1 && f < 10.0 {
				return f
			}
		}
	}
	return 0
}

// extractRetailOIFromTable extracts long/short OI from institutional data.
func extractRetailOIFromTable(table taifexDailyTable) (int64, int64, float64) {
	var long, short int64

	for _, row := range table.Data {
		for i, cell := range row {
			val := strings.TrimSpace(cell)
			if val == "" {
				continue
			}

			// Look for retail/proprietary OI values.
			// TAIFEX institutional table typically has columns: 交易人, 多方, 空方, 淨額
			if strings.Contains(val, "散戶") || strings.Contains(val, "自然人") || strings.Contains(val, "個別") {
				if i+1 < len(row) {
					long = parseTaifexInt(row[i+1])
				}
				if i+2 < len(row) {
					short = parseTaifexInt(row[i+2])
				}
				break
			}
		}
		if long > 0 || short > 0 {
			break
		}
	}

	var ratio float64
	if short > 0 {
		ratio = float64(long) / float64(short)
	}

	return long, short, ratio
}

type taifexDailyResponse struct {
	Stat   string             `json:"stat"`
	Date   string             `json:"date"`
	Tables []taifexDailyTable `json:"tables"`
}

type taifexDailyTable struct {
	Title  string     `json:"title"`
	Fields []string   `json:"fields"`
	Data   [][]string `json:"data"`
	Notes  []string   `json:"notes"`
	Total  int        `json:"total"`
}

func parseTaifexFloat(s string) float64 {
	s = strings.ReplaceAll(s, ",", "")
	s = strings.ReplaceAll(s, "%", "")
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

func parseTaifexInt(s string) int64 {
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
