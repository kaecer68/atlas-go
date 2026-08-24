package marketdata

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"github.com/kaecer68/atlas-go/internal/apigateway/httpclient"
	"github.com/kaecer68/atlas-go/internal/config"
)

// TEJ API configuration.
// Free tier: 500 calls/day, 10,000 rows/call.
// Docs: https://api.tej.com.tw/
const (
	tejAPIBaseURL = "https://api.tej.com.tw"
)

// ErrTEJQuotaExhausted is returned when TEJ's daily quota is gone.
// Mirrors ErrQuotaExhausted (FinMind) / ErrFugleQuotaExhausted so callers
// can `errors.Is` any provider's budget-exhausted condition without
// coupling to the specific provider (P2-18). TEJ is currently DISABLED in
// production (API key expired 2026-08-03); the daily gate + registry
// registration below are the revival checklist — the quota tracker and
// sentinel must be in place BEFORE the key is re-enabled so a revived
// channel cannot blow the 500/day trial budget.
var ErrTEJQuotaExhausted = fmt.Errorf("tej: daily quota exhausted")

// getTEJDailyLimit returns the daily call limit based on TEJ_TIER env var.
// trial: 500/day (default), paid: 2000/day
func getTEJDailyLimit() int {
	if config.GetSecret("TEJ_TIER") == "paid" {
		return 2000
	}
	return 500 // trial tier
}

// TEJClient fetches data from TEJ API.
// Requires free trial API key from https://api.tej.com.tw/
type TEJClient struct {
	apiKey       string
	httpClient   *http.Client
	rateLimiter  *rate.Limiter      // per-second limiter
	quotaTracker *DailyQuotaTracker // daily quota tracker
	baseURL      string             // defaults to tejAPIBaseURL, overridable for tests
}

// TEJ API response wrapper (REST API format).
// The REST API returns data in a "datatable" wrapper.
type tejResponse struct {
	Datatable struct {
		Data    [][]any `json:"data"`
		Columns []struct {
			Name string `json:"name"`
		} `json:"columns"`
	} `json:"datatable"`
	Error *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
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

var (
	sharedTEJClient     *TEJClient
	sharedTEJClientOnce sync.Once
	sharedTEJClientMu   sync.RWMutex
)

// GetSharedTEJClient returns a singleton TEJClient that all components
// share. Using a single client ensures one token bucket enforces the
// rate limit across all call sites. The apiKey is used only on first
// call; subsequent calls ignore it.
func GetSharedTEJClient(apiKey string) *TEJClient {
	sharedTEJClientOnce.Do(func() {
		sharedTEJClient = newTEJClientInternal(apiKey)
	})
	return sharedTEJClient
}

// ResetSharedTEJClient clears the singleton (for tests).
func ResetSharedTEJClient() {
	sharedTEJClientMu.Lock()
	defer sharedTEJClientMu.Unlock()
	sharedTEJClient = nil
	sharedTEJClientOnce = sync.Once{}
}

// NewTEJClient creates a standalone TEJClient with its own rate limiter
// and quota tracker. Prefer GetSharedTEJClient in production to avoid
// multiple independent token buckets that can collectively exceed the
// API tier limit.
//
// Deprecated: use GetSharedTEJClient for production code.
func NewTEJClient(apiKey string) *TEJClient {
	return newTEJClientInternal(apiKey)
}

// SetHTTPClient sets a custom HTTP client for tests.
func (c *TEJClient) SetHTTPClient(client *http.Client) {
	if client != nil {
		c.httpClient = client
	}
}

// newTEJClientInternal creates a TEJ API client (shared implementation).
func newTEJClientInternal(apiKey string) *TEJClient {
	params := config.GetParametersConfig()
	dailyLimit := getTEJDailyLimit()
	tracker := NewDailyQuotaTracker("tej", "data/state", dailyLimit)
	// P2-18: register the tracker with the global QuotaRegistry so the
	// dashboard / channel-health page shows TEJ alongside FinMind/Fugle in
	// one Snapshot() once the key is revived. FinMind and Fugle register in
	// their constructors; TEJ was the only quota-tracked provider missing
	// this wiring.
	GlobalQuotaRegistry().Register("tej", tracker)
	return &TEJClient{
		apiKey:       apiKey,
		baseURL:      tejAPIBaseURL,
		httpClient:   httpclient.NewFactory().NewClient(time.Duration(params.Marketdata.TEJAPITimeoutSec.Value) * time.Second),
		rateLimiter:  rate.NewLimiter(rate.Limit(params.Marketdata.TEJCallsPerSecond.Value), params.Marketdata.TEJCallsPerSecond.Value),
		quotaTracker: tracker,
	}
}

// Ping performs a lightweight health check against the TEJ API.
// It fetches a single day of data for a test symbol to verify connectivity and auth.
func (c *TEJClient) Ping(ctx context.Context) error {
	if c.apiKey == "" {
		return fmt.Errorf("TEJ API key not configured")
	}
	rows, err := c.GetStockPriceDaily(ctx, "2330", "2025-01-03", "2025-01-03")
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
	if !c.quotaTracker.AllowCall() {
		// P2-18: wrap ErrTEJQuotaExhausted so errors.Is at the adapter /
		// monitoring layer maps this to warn/quotas instead of a plain
		// string that paged on-call (FinMind 402 lesson, P0-1).
		return nil, fmt.Errorf("%w (used=%d, remaining=%d)", ErrTEJQuotaExhausted, c.quotaTracker.CallsToday(), c.quotaTracker.Remaining())
	}
	if err := c.rateLimiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("tej rate limit wait: %w", ErrRateLimited)
	}

	endpoint := fmt.Sprintf("%s/api/datatables/TRAIL/TAPRCD.json", c.baseURL)

	params := url.Values{}
	params.Set("api_key", c.apiKey)
	params.Set("coid", stockID)
	params.Set("mdate.gte", startDate)
	params.Set("mdate.lte", endDate)
	params.Set("opts.columns", "coid,mdate,open_d,high_d,low_d,close_d,volume,amount")

	reqURL := fmt.Sprintf("%s?%s", endpoint, params.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("tej create request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("tej http request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("tej api error: status %d, body: %s", resp.StatusCode, string(body))
	}

	var apiResp tejResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("tej decode response: %w", err)
	}

	if apiResp.Error != nil {
		return nil, fmt.Errorf("tej api error: %s - %s", apiResp.Error.Code, apiResp.Error.Message)
	}

	rows := make([]TEJStockPriceRow, 0, len(apiResp.Datatable.Data))
	for _, row := range apiResp.Datatable.Data {
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
	if !c.quotaTracker.AllowCall() {
		// P2-18: same sentinel as GetStockPriceDaily.
		return nil, fmt.Errorf("%w (used=%d, remaining=%d)", ErrTEJQuotaExhausted, c.quotaTracker.CallsToday(), c.quotaTracker.Remaining())
	}
	if err := c.rateLimiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("tej rate limit wait: %w", ErrRateLimited)
	}

	// tableCode examples:
	//  TWN/AFINA  — 綜合損益表 (trial)
	//  TWN/ABINA  — 資產負債表 (trial)
	endpoint := fmt.Sprintf("%s/api/datatables/%s.json", c.baseURL, tableCode)

	params := url.Values{}
	params.Set("api_key", c.apiKey)
	params.Set("coid", stockID)
	params.Set("mdate.gte", startDate)
	params.Set("mdate.lte", endDate)
	params.Set("paginate", "true")
	params.Set("per_page", "10000")

	reqURL := fmt.Sprintf("%s?%s", endpoint, params.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("tej create request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("tej http request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("tej api error: status %d, body: %s", resp.StatusCode, string(body))
	}

	var apiResp tejResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("tej decode response: %w", err)
	}

	if apiResp.Error != nil {
		return nil, fmt.Errorf("tej api error: %s - %s", apiResp.Error.Code, apiResp.Error.Message)
	}

	// Return as []map for flexibility; caller knows column order.
	rows := make([]map[string]any, 0, len(apiResp.Datatable.Data))
	for _, row := range apiResp.Datatable.Data {
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
