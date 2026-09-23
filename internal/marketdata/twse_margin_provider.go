package marketdata

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
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

	// finmind is optional. When set, it fills MacroDataSnapshot
	// .MarginMaintenanceRatio from FinMind's whole-market
	// TaiwanTotalExchangeMarginMaintenance series — TWSE MI_MARGN publishes no
	// aggregate maintenance ratio (#1924). nil = field stays empty.
	finmind *FinMindClient

	// ratioFillMu guards lastRatioFillDay, the 1-call/day budget marker for
	// the FinMind ratio fill (FetchSnapshot* is called concurrently by the
	// gateway fan-out and the ingest loop).
	ratioFillMu      sync.Mutex
	lastRatioFillDay string
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

// SetBaseURL overrides the TWSE base URL (tests only).
func (t *TWSEMarginBalanceProvider) SetBaseURL(u string) {
	if u != "" {
		t.baseURL = u
	}
}

// SetRateLimiter sets a custom rate limiter for tests.
func (t *TWSEMarginBalanceProvider) SetRateLimiter(l *rate.Limiter) {
	if l != nil {
		t.rateLimiter = l
	}
}

// SetFinMindClient wires the FinMind client used to fill
// MarginMaintenanceRatio (whole-market series). Pass nil to disable the fill,
// in which case the field is left empty — same behavior as before #1924.
func (t *TWSEMarginBalanceProvider) SetFinMindClient(c *FinMindClient) {
	t.finmind = c
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
			// Maintenance ratio: no TWSE source exists. MI_MARGN never carries
			// an aggregate ratio — its selectType=ALL second table is a
			// per-stock 融資融券彙總 whose column 1 is the security NAME, so the
			// old "read Tables[1].Data[0][1]" probe could only ever fail (#1924).
			// Fill it from FinMind instead (best-effort; see
			// fillMaintenanceRatio). d is the trading day that answered above.
			t.fillMaintenanceRatio(ctx, d, &snap)
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

// fillMaintenanceRatio fills snap.MarginMaintenanceRatio from FinMind's
// whole-market TaiwanTotalExchangeMarginMaintenance series (value in %).
//
// Why not TWSE: MI_MARGN exposes only 融資/融券 balance tables; its
// selectType=ALL response adds a per-stock 融資融券彙總 table whose second
// column is the security name, not a ratio. The previous implementation read
// Tables[1].Data[0][1] blindly and therefore always failed with a ParseFloat
// error — which is why MarginMaintenanceRatio stayed empty and the
// period_detector downturn rule (融資維持率 < 門檻) could never fire (#1924).
//
// Best-effort by design: a missing row (weekend, holiday, or before FinMind's
// evening release), an exhausted quota, or upstream throttling are all
// "wait" conditions, not outages — they are logged at debug so a normal tick
// never emits a WARN (the gateway maps ErrNoData to RecordWaiting). Only a
// genuine upstream failure is logged as WARN.
//
// Cost: one FinMind call per successful fill per trading day (the series is
// market-wide and needs no data_id); lastRatioFillDay dedups the gateway
// fan-out, which re-fetches this channel many times per day.
func (t *TWSEMarginBalanceProvider) fillMaintenanceRatio(ctx context.Context, tradeDate time.Time, snap *MacroDataSnapshot) {
	if t.finmind == nil {
		return
	}
	day := tradeDate.Format("2006-01-02")

	t.ratioFillMu.Lock()
	filled := t.lastRatioFillDay == day
	t.ratioFillMu.Unlock()
	if filled {
		return
	}

	rowDate, ratio, err := t.finmind.GetMarginMaintenanceLatest(ctx, day)
	if err != nil {
		switch {
		case errors.Is(err, ErrNoData):
			// Not published yet (or no trading day inside the lookback):
			// expected, retry on the next fetch without noise.
			logging.Debug("twse_margin_provider", "maintenance_ratio_not_published",
				logging.Err(err), logging.FStr("end_date", day))
		case errors.Is(err, ErrQuotaExhausted), errors.Is(err, ErrRateLimited),
			errors.Is(err, ErrIPBanned), errors.Is(err, ErrFinMindBreakerOpen):
			// Budget/throttling conditions (P1-7): not outages, self-heal.
			logging.Debug("twse_margin_provider", "maintenance_ratio_source_waiting",
				logging.Err(err), logging.FStr("end_date", day))
		default:
			logging.Warn("twse_margin_provider", "maintenance_ratio_source_failed",
				logging.Err(err), logging.FStr("end_date", day))
		}
		return
	}

	t.ratioFillMu.Lock()
	t.lastRatioFillDay = day
	t.ratioFillMu.Unlock()

	snap.MarginMaintenanceRatio = MacroDataPoint{
		Symbol:    "TSE_MARGIN_MAINT",
		Value:     ratio,
		Timestamp: time.Now().Unix(),
	}
	logging.Info("twse_margin_provider", "maintenance_ratio_filled",
		logging.FStr("source", "finmind:TaiwanTotalExchangeMarginMaintenance"),
		logging.FStr("row_date", rowDate), logging.FFloat64("value", ratio))
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
