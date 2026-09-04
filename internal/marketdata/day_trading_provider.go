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

// SetBaseURL overrides the TWSE base URL (tests only).
func (d *DayTradingProvider) SetBaseURL(u string) {
	if u != "" {
		d.baseURL = u
	}
}

// FetchLatest retrieves the most recent day trading statistics.
// Calendar-aware scan (#1767): walk back over EXPECTED Taiwan trading days
// only — the blind 7-calendar-day scan wasted rate-limiter tokens on empty
// weekend queries, and under the shared TWSE bucket the queued waits exceeded
// the caller's context deadline ("rate limit wait: rate: Wait(n=1) would
// exceed context deadline") even though Friday's data was available.
func (d *DayTradingProvider) FetchLatest(ctx context.Context) (*DayTradingStats, error) {
	const maxAttempts = 3
	var lastErr error
	for _, day := range RecentTradingDays(time.Now().UTC(), maxAttempts) {
		stats, err := d.fetchDate(ctx, day.Format("20060102"))
		if err == nil {
			return stats, nil
		}
		lastErr = err
	}
	// Keep the stable prefix (channel-health text matches on it) but attach
	// the last underlying error so transient upstream failures (rate-limit
	// bursts after startup, TWSE maintenance) are diagnosable from the
	// channel record instead of surfacing as an opaque "no data" message.
	return nil, fmt.Errorf("no TWSE day trading data available in the last 7 days: last error: %w", lastErr)
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

	if apiResp.Stat != "OK" {
		return nil, fmt.Errorf("TWSE API returned no data: stat=%s", apiResp.Stat)
	}

	// PR-3: locate the aggregate statistics table robustly. TWSE moved the
	// response to a `tables` array (BFI84U precedent) and the layout varies:
	//   - tables[0] = 「當日沖銷交易統計資訊」(the 6-column aggregate row this
	//     parser consumes) and tables[1] = the eligible-securities list
	//     (2026-09-01/09-03 live shape), OR
	//   - tables[0] = an EMPTY object and tables[1] = the securities list
	//     with only 3 columns — the day's statistics are simply not
	//     published yet (2026-09-04 live shape, verified 18:40 TW: the
	//     ratio lands after TWSE's evening processing), OR
	//   - legacy pre-tables shape: a top-level `data` array.
	// Dispatch on table identity (fields of the stats table), not on
	// position, so a future reorder cannot silently parse the securities
	// list as statistics.
	var row []string
	for i := range apiResp.Tables {
		table := &apiResp.Tables[i]
		if isDayTradingStatsTable(table) {
			if len(table.Data) > 0 && len(table.Data[0]) >= 6 {
				row = table.Data[0]
			}
			break
		}
	}
	// Fallback 1 — unlabeled aggregate row: some payloads (and legacy test
	// fixtures) carry a field-less table whose first row is the 6-column
	// aggregate. Accept the first such table, skipping the eligible-
	// securities list (identified by its 證券代號 field) so a volume-less or
	// 6-column securities table can never be mis-parsed as statistics.
	if row == nil {
		for i := range apiResp.Tables {
			table := &apiResp.Tables[i]
			if isSecuritiesListTable(table) {
				continue
			}
			if len(table.Data) > 0 && len(table.Data[0]) >= 6 {
				row = table.Data[0]
				break
			}
		}
	}
	// Fallback 2 — legacy dual-format: no tables payload → top-level data
	// rows carry the same 6 columns.
	if row == nil && len(apiResp.Tables) == 0 && len(apiResp.Data) > 0 {
		if len(apiResp.Data[0]) >= 6 {
			row = apiResp.Data[0]
		}
	}
	if row == nil {
		// Covers both "statistics not published yet" (empty tables[0] on
		// fresh dates) and genuine schema breakage. Wrapping ErrNoData keeps
		// gateway/channel-health classification (waiting, not outage) and
		// FetchLatest's calendar walk-back intact.
		return nil, fmt.Errorf("%w: TWSE day trading statistics not in response (stat=%s tables=%d top_data=%d) — not published yet or schema moved",
			ErrNoData, apiResp.Stat, len(apiResp.Tables), len(apiResp.Data))
	}

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

// isSecuritiesListTable reports whether a `tables` entry is the
// eligible-securities list (證券代號 field present) rather than the aggregate
// statistics row.
func isSecuritiesListTable(t *twseDayTradingTable) bool {
	for _, f := range t.Fields {
		if strings.Contains(f, "證券代號") {
			return true
		}
	}
	return false
}

// isDayTradingStatsTable reports whether a `tables` entry is the aggregate
// 當日沖銷交易統計資訊 table (as opposed to the eligible-securities list).
// Field-matching survives reordering and the volume-less securities table
// that TWSE serves before the day's statistics are published.
func isDayTradingStatsTable(t *twseDayTradingTable) bool {
	for _, f := range t.Fields {
		if strings.Contains(f, "當日沖銷交易總成交股數") {
			return true
		}
	}
	return false
}

type twseDayTradingResponse struct {
	Stat   string                `json:"stat"`
	Date   string                `json:"date"`
	Tables []twseDayTradingTable `json:"tables"`
	// Data is the legacy (pre-tables) top-level row array, kept for the
	// dual-format fallback in fetchDate.
	Data [][]string `json:"data"`
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
