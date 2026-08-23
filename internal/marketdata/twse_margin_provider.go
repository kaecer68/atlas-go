package marketdata

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/time/rate"

	"github.com/kaecer68/atlas-go/internal/apigateway/httpclient"
	"github.com/kaecer68/atlas-go/internal/constants"
	"github.com/kaecer68/atlas-go/internal/logging"
)

// TWSEMarginBalanceProvider fetches Taiwan margin balance data from TWSE.
type TWSEMarginBalanceProvider struct {
	client      *http.Client
	baseURL     string
	rateLimiter *rate.Limiter
	storageDir  string
}

// NewTWSEMarginBalanceProvider creates a new TWSE margin balance provider.
// Pass an empty storageDir to skip saving margin data to disk.
func NewTWSEMarginBalanceProvider(storageDir string) *TWSEMarginBalanceProvider {
	return &TWSEMarginBalanceProvider{
		client:      httpclient.NewFactory().NewClient(20 * time.Second),
		baseURL:     constants.TWSEBaseURL,
		rateLimiter: getTWSESharedLimiter(), // P1-13: shared TWSE bucket
		storageDir:  storageDir,
	}
}

// SetHTTPClient sets a custom HTTP client for tests.
func (t *TWSEMarginBalanceProvider) SetHTTPClient(client *http.Client) {
	if client != nil {
		t.client = client
	}
}

// SetRateLimiter sets a custom rate limiter for tests.
func (t *TWSEMarginBalanceProvider) SetRateLimiter(l *rate.Limiter) {
	if l != nil {
		t.rateLimiter = l
	}
}

// Name returns the provider name.
func (t *TWSEMarginBalanceProvider) Name() string {
	return "twse_margin_balance"
}

// FetchSnapshot retrieves the latest margin balance data.
func (t *TWSEMarginBalanceProvider) FetchSnapshot(ctx context.Context) (MacroDataSnapshot, error) {
	return t.FetchSnapshotForDate(ctx, time.Now().UTC())
}

// FetchSnapshotForDate retrieves margin balance data for a specific date.
func (t *TWSEMarginBalanceProvider) FetchSnapshotForDate(ctx context.Context, date time.Time) (MacroDataSnapshot, error) {
	for i, attempts := 0, 0; attempts < 7; i++ {
		d := date.AddDate(0, 0, -i)
		if d.Weekday() == time.Saturday || d.Weekday() == time.Sunday {
			continue
		}
		attempts++
		dateStr := d.Format("20060102")
		balance, shortBalance, changePct, shortChangePct, err := t.fetchDateExpanded(ctx, dateStr)
		if err == nil {
			if err := t.saveMargin(dateStr, balance, shortBalance, changePct, shortChangePct); err != nil {
				logging.Warn("twse_margin_provider", "save_margin_warning", logging.Err(err))
			}
			ts := time.Now().Unix()
			snap := MacroDataSnapshot{
				RetailMarginBalance: MacroDataPoint{
					Symbol:    "TAIWAN_MARGIN_BALANCE",
					Value:     balance,
					ChangePct: changePct,
					Timestamp: ts,
				},
				RetailShortBalance: MacroDataPoint{
					Symbol:    "TAIWAN_SHORT_BALANCE",
					Value:     shortBalance,
					ChangePct: shortChangePct,
					Timestamp: ts,
				},
				RecordedAt: ts,
			}
			// Fetch maintenance ratio (best-effort: TWSE endpoint MI_MARGN does not
			// return the aggregate maintenance ratio; this field is expected to be
			// empty until a suitable data source is found).
			if ratio, rerr := t.fetchMaintenanceRatio(ctx, dateStr); rerr == nil {
				snap.MarginMaintenanceRatio = MacroDataPoint{
					Symbol:    "TSE_MARGIN_MAINT",
					Value:     ratio,
					Timestamp: ts,
				}
			} else {
				logging.Warn("twse_margin_provider", "maintenance_ratio_fetch_failed",
					logging.Err(rerr), logging.FStr("date", dateStr))
			}
			return snap, nil
		}
	}
	return MacroDataSnapshot{}, fmt.Errorf("%w: no TWSE margin balance data available in the last 7 days", ErrNoData)
}

func (t *TWSEMarginBalanceProvider) fetchDateExpanded(ctx context.Context, dateStr string) (float64, float64, float64, float64, error) {
	if err := t.rateLimiter.Wait(ctx); err != nil {
		return 0, 0, 0, 0, fmt.Errorf("rate limit wait: %w", err)
	}

	url := fmt.Sprintf("%s/zh/exchangeReport/MI_MARGN?response=json&date=%s&selectType=MS", t.baseURL, dateStr)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, 0, 0, 0, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")

	resp, err := t.client.Do(req)
	if err != nil {
		return 0, 0, 0, 0, fmt.Errorf("http request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, 0, 0, 0, fmt.Errorf("read body: %w", err)
	}

	var apiResp twseMarginResponse
	if err := DecodeJSON(bytes.NewReader(body), resp.Header.Get("Content-Type"), &apiResp); err != nil {
		return 0, 0, 0, 0, fmt.Errorf("decode response: %w", err)
	}

	if apiResp.Stat != "OK" || len(apiResp.Tables) == 0 {
		return 0, 0, 0, 0, fmt.Errorf("TWSE API returned no data: stat=%s tables=%d", apiResp.Stat, len(apiResp.Tables))
	}

	// The MI_MARGN API now returns a single table (Table 0) with data rows for
	// 融資(交易單位), 融券(交易單位), 融資金額(仟元).
	// Fields: [項目, 買進, 賣出, 現金償還, 前日餘額, 今日餘額]
	// Index:     0      1     2      3          4         5
	// data[0] = 融資(交易單位), data[1] = 融券(交易單位), data[2] = 融資金額(仟元)
	marginRow, shortRow := -1, -1
	table := apiResp.Tables[0]
	for i, row := range table.Data {
		if len(row) < 6 {
			continue
		}
		label := row[0]
		switch {
		case strings.Contains(label, "融資金額"):
			marginRow = i
		case strings.Contains(label, "融券"):
			shortRow = i
		}
	}

	if marginRow < 0 {
		return 0, 0, 0, 0, fmt.Errorf("TWSE API response missing 融資金額 row")
	}

	// Extract margin: row[marginRow], columns 5 (今日餘額) and 4 (前日餘額 or 昨日餘額)
	marginRaw := table.Data[marginRow][5]
	marginPrevRaw := table.Data[marginRow][4]
	balance := float64(parseTWSEInt(marginRaw)) / 1e5
	prevBalance := float64(parseTWSEInt(marginPrevRaw)) / 1e5
	changePct := percentChange(balance, prevBalance)

	shortBalance, shortPrevBalance := 0.0, 0.0
	shortChangePct := 0.0
	if shortRow >= 0 && len(table.Data[shortRow]) >= 6 {
		shortRaw := table.Data[shortRow][5]
		shortPrevRaw := table.Data[shortRow][4]
		shortBalance = float64(parseTWSEInt(shortRaw)) / 1e5
		shortPrevBalance = float64(parseTWSEInt(shortPrevRaw)) / 1e5
		shortChangePct = percentChange(shortBalance, shortPrevBalance)
	}

	return balance, shortBalance, changePct, shortChangePct, nil
}

// fetchMaintenanceRatio fetches the aggregate margin maintenance ratio from TWSE.
//
// Endpoint: TWSE MI_MARGN?selectType=ALL.
// NOTE: As of 2026-07-28 investigation (B4c), TWSE MI_MARGN does NOT return a
// maintenance ratio table — the actual response only contains margin balance
// and per-stock detail tables. The aggregate maintenance ratio is not available
// from this endpoint or the TWSE OpenAPI. This field is expected to remain
// empty until a suitable data source is identified.
// Returns the aggregate maintenance ratio (%) for the given date.
func (t *TWSEMarginBalanceProvider) fetchMaintenanceRatio(ctx context.Context, dateStr string) (float64, error) {
	if err := t.rateLimiter.Wait(ctx); err != nil {
		return 0, fmt.Errorf("rate limit wait: %w", err)
	}

	url := fmt.Sprintf("%s/zh/exchangeReport/MI_MARGN?response=json&date=%s&selectType=ALL", t.baseURL, dateStr)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")

	resp, err := t.client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("http request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, fmt.Errorf("read body: %w", err)
	}

	var apiResp twseMarginResponse
	if err := DecodeJSON(bytes.NewReader(body), resp.Header.Get("Content-Type"), &apiResp); err != nil {
		return 0, fmt.Errorf("decode response: %w", err)
	}

	if apiResp.Stat != "OK" || len(apiResp.Tables) < 2 {
		return 0, fmt.Errorf("TWSE maintenance ratio unavailable: stat=%s tables=%d", apiResp.Stat, len(apiResp.Tables))
	}

	// Table 1 is the maintenance ratio table.
	// Columns: [日期, 維持率(%)]
	table := apiResp.Tables[1]
	if len(table.Data) == 0 || len(table.Data[0]) < 2 {
		return 0, fmt.Errorf("TWSE maintenance ratio table empty")
	}

	ratioStr := table.Data[0][1]
	ratio, err := strconv.ParseFloat(strings.TrimSpace(ratioStr), 64)
	if err != nil {
		return 0, fmt.Errorf("parse maintenance ratio %q: %w", ratioStr, err)
	}

	return ratio, nil
}

func (t *TWSEMarginBalanceProvider) saveMargin(dateStr string, balance, shortBalance, changePct, shortChangePct float64) error {
	if t.storageDir == "" {
		return nil
	}
	if err := os.MkdirAll(t.storageDir, 0o755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}
	data := map[string]any{
		"date":             dateStr,
		"margin_balance":   balance,
		"short_balance":    shortBalance,
		"change_pct":       changePct,
		"short_change_pct": shortChangePct,
	}
	out, _ := json.MarshalIndent(data, "", "  ")
	return os.WriteFile(filepath.Join(t.storageDir, dateStr+"_margin.json"), out, 0o644)
}

func percentChange(current, previous float64) float64 {
	if previous == 0 {
		return 0
	}
	return (current - previous) / previous * 100
}

type twseMarginResponse struct {
	Stat   string            `json:"stat"`
	Date   string            `json:"date"`
	Tables []twseMarginTable `json:"tables"`
}

type twseMarginTable struct {
	Title  string     `json:"title"`
	Fields []string   `json:"fields"`
	Data   [][]string `json:"data"`
	Notes  []string   `json:"notes"`
	Total  int        `json:"total"`
}
