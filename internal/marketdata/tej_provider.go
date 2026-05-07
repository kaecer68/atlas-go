package marketdata

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/time/rate"
)

// TEJ API configuration.
// Free tier: 500 calls/day, 10,000 rows/call.
// Docs: https://api.tej.com.tw/
const (
	tejAPIBaseURL = "https://api.tej.com.tw"
	// Free tier daily call limit (conservative, leave headroom).
	tejCallsPerDay    = 450
	tejCallsPerSecond = 5 // burst limit for rate limiter
)

// TEJClient fetches data from TEJ API.
// Requires free trial API key from https://api.tej.com.tw/
type TEJClient struct {
	apiKey      string
	httpClient  *http.Client
	rateLimiter *rate.Limiter // per-second limiter derived from daily budget
	baseURL     string        // defaults to tejAPIBaseURL, overridable for tests
}

// TEJ API response wrapper (generic).
type tejResponse struct {
	Stat  string  `json:"stat"`
	Data  [][]any `json:"data"`
	Meta  any     `json:"meta"`
	Pages int     `json:"pages,omitempty"`
}

// TEJStockPriceRow represents one row from TRAIL/TAPRCD dataset.
type TEJStockPriceRow struct {
	CoID     string  `json:"co_id"`     // 證券代號
	Date     string  `json:"date"`      // 日期 (YYYY-MM-DD)
	Open     float64 `json:"open"`      // 開盤價(元)
	High     float64 `json:"high"`      // 最高價(元)
	Low      float64 `json:"low"`       // 最低價(元)
	Close    float64 `json:"close"`     // 收盤價(元)
	Volume   int64   `json:"volume"`    // 成交量(千股)
	TradeVal float64 `json:"trade_val"` // 成交值(千元)
}

// NewTEJClient creates a TEJ API client.
// apiKey: obtained from TEJ website (free trial key).
func NewTEJClient(apiKey string) *TEJClient {
	return &TEJClient{
		apiKey:  apiKey,
		baseURL: tejAPIBaseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		rateLimiter: rate.NewLimiter(rate.Limit(tejCallsPerSecond), tejCallsPerSecond),
	}
}

// Ping performs a lightweight health check against the TEJ API.
// It fetches a single day of data for a test symbol to verify connectivity and auth.
func (c *TEJClient) Ping(ctx context.Context) error {
	if c.apiKey == "" {
		return fmt.Errorf("TEJ API key not configured")
	}
	rows, err := c.GetStockPriceDaily(ctx, "2330", "2024-01-02", "2024-01-02")
	if err != nil {
		return fmt.Errorf("ping stock price: %w", err)
	}
	if len(rows) == 0 {
		return fmt.Errorf("TEJ returned no data for test query")
	}
	return nil
}

// GetStockPriceDaily fetches unadjusted daily OHLCV for a given stock ID.
// Uses TRAIL/TAPRCD dataset (上市(櫃)未調整股價(日)).
// startDate / endDate in YYYY-MM-DD format.
func (c *TEJClient) GetStockPriceDaily(ctx context.Context, stockID, startDate, endDate string) ([]TEJStockPriceRow, error) {
	if err := c.rateLimiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("tej rate limit wait: %w", err)
	}

	endpoint := fmt.Sprintf("%s/v1/data/TRAIL/TAPRCD", c.baseURL)

	params := url.Values{}
	params.Set("coid", stockID)
	params.Set("mdate", fmt.Sprintf("gte:%s,lte:%s", startDate, endDate))
	params.Set("opts.columns", "coid,mdate,open_d,high_d,low_d,close_d,volume,trade_value")
	params.Set("paginate", "true")
	params.Set("per_page", "10000")

	reqURL := fmt.Sprintf("%s?%s", endpoint, params.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("tej create request: %w", err)
	}
	req.Header.Set("X-API-Key", c.apiKey)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("tej http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("tej api error: status %d, body: %s", resp.StatusCode, string(body))
	}

	var apiResp tejResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("tej decode response: %w", err)
	}

	if apiResp.Stat != "OK" && apiResp.Stat != "200" && apiResp.Stat != "" {
		return nil, fmt.Errorf("tej stat not OK: %s", apiResp.Stat)
	}

	rows := make([]TEJStockPriceRow, 0, len(apiResp.Data))
	for _, row := range apiResp.Data {
		if len(row) < 8 {
			continue
		}
		r := TEJStockPriceRow{
			CoID:     toString(row[0]),
			Date:     toString(row[1]),
			Open:     toFloat64(row[2]),
			High:     toFloat64(row[3]),
			Low:      toFloat64(row[4]),
			Close:    toFloat64(row[5]),
			Volume:   toInt64(row[6]),
			TradeVal: toFloat64(row[7]),
		}
		rows = append(rows, r)
	}

	return rows, nil
}

// GetFinancialStatements fetches quarterly financial statements.
// Uses TWN/AFINA (income statement / balance sheet — trial database).
// Returns raw JSON rows; caller can parse specific tables.
func (c *TEJClient) GetFinancialStatements(ctx context.Context, stockID, tableCode, startDate, endDate string) ([]map[string]any, error) {
	if err := c.rateLimiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("tej rate limit wait: %w", err)
	}

	// tableCode examples:
	//  TWN/AFINA  — 綜合損益表 (trial)
	//  TWN/ABINA  — 資產負債表 (trial)
	endpoint := fmt.Sprintf("%s/v1/data/%s", c.baseURL, tableCode)

	params := url.Values{}
	params.Set("coid", stockID)
	params.Set("mdate", fmt.Sprintf("gte:%s,lte:%s", startDate, endDate))
	params.Set("paginate", "true")
	params.Set("per_page", "10000")

	reqURL := fmt.Sprintf("%s?%s", endpoint, params.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("tej create request: %w", err)
	}
	req.Header.Set("X-API-Key", c.apiKey)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("tej http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("tej api error: status %d, body: %s", resp.StatusCode, string(body))
	}

	var apiResp struct {
		Stat string  `json:"stat"`
		Data [][]any `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("tej decode response: %w", err)
	}

	if apiResp.Stat != "OK" && apiResp.Stat != "" {
		return nil, fmt.Errorf("tej stat not OK: %s", apiResp.Stat)
	}

	// Return as []map for flexibility; caller knows column order.
	rows := make([]map[string]any, 0, len(apiResp.Data))
	for _, row := range apiResp.Data {
		m := make(map[string]any)
		for i, col := range row {
			m[fmt.Sprintf("col_%d", i)] = col
		}
		rows = append(rows, m)
	}
	return rows, nil
}

// --- Type conversion helpers ---

func toString(v any) string {
	if s, ok := v.(string); ok {
		return strings.TrimSpace(s)
	}
	return fmt.Sprintf("%v", v)
}

func toFloat64(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case int64:
		return float64(x)
	case int:
		return float64(x)
	case string:
		s := strings.ReplaceAll(x, ",", "")
		s = strings.TrimSpace(s)
		if s == "" || s == "NA" || s == "-" {
			return 0
		}
		f, err := parseFloat(s)
		if err != nil {
			return 0
		}
		return f
	}
	return 0
}

func toInt64(v any) int64 {
	switch x := v.(type) {
	case int64:
		return x
	case int:
		return int64(x)
	case float64:
		return int64(x)
	case string:
		s := strings.ReplaceAll(x, ",", "")
		s = strings.TrimSpace(s)
		if s == "" || s == "NA" || s == "-" {
			return 0
		}
		f, err := parseFloat(s)
		if err != nil {
			return 0
		}
		return int64(f)
	}
	return 0
}

func parseFloat(s string) (float64, error) {
	var neg bool
	if strings.HasPrefix(s, "(") && strings.HasSuffix(s, ")") {
		neg = true
		s = strings.Trim(s, "()")
	}
	var val float64
	n, err := fmt.Sscanf(s, "%f", &val)
	if err != nil || n == 0 {
		return 0, fmt.Errorf("parse float failed: %s", s)
	}
	if neg {
		return -val, nil
	}
	return val, nil
}
