package marketdata

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/kaecer68/atlas-go/internal/apigateway/httpclient"
	"github.com/kaecer68/atlas-go/internal/logging"
)

const exchangeRateEndpoint = "https://open.er-api.com/v6/latest/USD"

// dailyRateSnapshot is the on-disk format of the cross-day rate cache.
// Stored once per UTC day so ChangePct survives container restarts and
// the 5-min macro_ingest window (where forex typically doesn't move enough
// for a 5-min cache diff to be useful).
type dailyRateSnapshot struct {
	// Date is the UTC date (YYYY-MM-DD) on which USDTWD/USDJPY were observed.
	Date string `json:"date"`
	// USDTWD is the rate observed on that date (used as next-day baseline).
	USDTWD float64 `json:"usd_twd"`
	// USDJPY is the rate observed on that date (used as next-day baseline).
	USDJPY float64 `json:"usd_jpy"`
}

// ExchangeRateProvider fetches exchange rates from the free ExchangeRate-API.
// Supports TWD (not available in ECB/Frankfurter dataset) and JPY.
// No API key required, rate-limited to ~1 request/minute on free tier.
//
// ChangePct derivation (Layer 2b, see decision
// 2026-07-13-usd-twd-routing-recurring-bug-root-cause.md):
//   - If the on-disk daily cache is from a previous UTC day, use it as
//     baseline → ChangePct = today's rate vs yesterday's close.
//   - Otherwise (same-day or no cache), fall back to the in-memory 5-min
//     cache → intra-day diff.
//   - The daily cache file survives container restarts (it's a real file
//     under ATLAS_DATA_DIR), so the first fetch of a new UTC day has a
//     non-zero ChangePct as soon as the clock crosses midnight UTC.
type ExchangeRateProvider struct {
	client    *http.Client
	latestURL string
	cachePath string

	mu sync.RWMutex
	// Intra-day cache (5-min, in-memory only).
	lastUSDTWD float64
	lastUSDJPY float64
	// Cross-day cache (persisted to disk).
	daily dailyRateSnapshot
}

func NewExchangeRateProvider() *ExchangeRateProvider {
	cachePath := defaultExchangeRateCachePath()
	p := &ExchangeRateProvider{
		client:    httpclient.NewFactory().NewClient(10 * time.Second),
		latestURL: exchangeRateEndpoint,
		cachePath: cachePath,
	}
	p.loadDailyCache()
	return p
}

func defaultExchangeRateCachePath() string {
	if override := os.Getenv("ATLAS_EXCHANGE_RATE_CACHE"); override != "" {
		return override
	}
	dataDir := os.Getenv("ATLAS_DATA_DIR")
	if dataDir == "" {
		dataDir = "/app/data"
	}
	return filepath.Join(dataDir, "state", "exchange_rate_daily.json")
}

// SetHTTPClient sets a custom HTTP client for tests.
func (e *ExchangeRateProvider) SetHTTPClient(client *http.Client) {
	if client != nil {
		e.client = client
	}
}

// SetCachePath overrides the default cache file path (for tests).
func (e *ExchangeRateProvider) SetCachePath(path string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.cachePath = path
	e.daily = dailyRateSnapshot{}
	e.loadDailyCache()
}

func (e *ExchangeRateProvider) Name() string {
	return "exchange_rate_api"
}

func (e *ExchangeRateProvider) FetchSnapshot(ctx context.Context) (MacroDataSnapshot, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, e.latestURL, nil)
	if err != nil {
		return MacroDataSnapshot{}, fmt.Errorf("exchangerate request: %w", err)
	}
	req.Header.Set("User-Agent", "atlas-go/1.0")

	resp, err := e.client.Do(req)
	if err != nil {
		return MacroDataSnapshot{}, fmt.Errorf("exchangerate fetch: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return MacroDataSnapshot{}, fmt.Errorf("exchangerate http status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return MacroDataSnapshot{}, fmt.Errorf("exchangerate read body: %w", err)
	}

	var fxResp exchangeRateResponse
	if err := json.Unmarshal(body, &fxResp); err != nil {
		return MacroDataSnapshot{}, fmt.Errorf("exchangerate unmarshal: %w", err)
	}

	if fxResp.Result != "success" {
		return MacroDataSnapshot{}, fmt.Errorf("exchangerate API error: %s", fxResp.Result)
	}

	snap := MacroDataSnapshot{RecordedAt: time.Now().Unix()}

	e.mu.Lock()
	defer e.mu.Unlock()

	today := time.Now().UTC().Format("2006-01-02")

	// Capture daily cache baseline BEFORE any update, so the JPY leg of
	// this FetchSnapshot call still sees yesterday's date and computes
	// daily diff correctly (otherwise TWD's updateDailyCache flips daily.Date
	// to today before JPY runs, breaking its diff).
	dailyDate := e.daily.Date
	dailyUSDTWD := e.daily.USDTWD
	dailyUSDJPY := e.daily.USDJPY

	if twdRate, ok := fxResp.Rates["TWD"]; ok && twdRate > 0 {
		snap.USD_TWD = MacroDataPoint{
			Symbol:    "USD/TWD=X",
			Value:     twdRate,
			ChangePct: e.computeChange(twdRate, e.lastUSDTWD, dailyUSDTWD, today, dailyDate, "TWD"),
			Timestamp: time.Now().Unix(),
		}
	} else {
		logging.Warn("exchangerate_provider", "missing_or_zero_rate", "currency", "TWD")
	}

	if jpyRate, ok := fxResp.Rates["JPY"]; ok && jpyRate > 0 {
		snap.JPY = MacroDataPoint{
			Symbol:    "JPY=X",
			Value:     jpyRate,
			ChangePct: e.computeChange(jpyRate, e.lastUSDJPY, dailyUSDJPY, today, dailyDate, "JPY"),
			Timestamp: time.Now().Unix(),
		}
	} else {
		logging.Info("exchangerate_provider", "jpy_change_pct_unavailable",
			"reason", "free tier lacks historical endpoint",
			"recommendation", "use FrankfurterFXProvider for daily change tracking")
	}

	// Update caches only AFTER both ChangePct computations above have completed.
	if _, ok := fxResp.Rates["TWD"]; ok {
		if twdRate := fxResp.Rates["TWD"]; twdRate > 0 {
			e.lastUSDTWD = twdRate
			e.updateDailyCache(twdRate, 0, today)
		}
	}
	if _, ok := fxResp.Rates["JPY"]; ok {
		if jpyRate := fxResp.Rates["JPY"]; jpyRate > 0 {
			e.lastUSDJPY = jpyRate
			e.updateDailyCache(0, jpyRate, today)
		}
	}

	return snap, nil
}

// computeChange derives ChangePct using (in priority order):
//  1. Yesterday's persisted rate (if cache date < today)  → real daily diff
//  2. Intra-day 5-min cache (if cache date == today)      → intra-day diff
//  3. 0 (cold start — no baseline available yet)
func (e *ExchangeRateProvider) computeChange(current, intraDay, dailyBaseline float64, today, dailyDate, currency string) float64 {
	if dailyDate != "" && dailyDate < today && dailyBaseline > 0 {
		return pctChange(current, dailyBaseline)
	}
	if intraDay > 0 {
		return pctChange(current, intraDay)
	}
	return 0
}

// updateDailyCache persists today's rate to disk if the date changed.
// On intra-day updates, the file is rewritten to capture the latest rate
// (so the 5-min intra-day cache can survive process restarts within a day).
func (e *ExchangeRateProvider) updateDailyCache(twdRate, jpyRate float64, today string) {
	if e.daily.Date != today {
		e.daily.Date = today
	}
	if twdRate > 0 {
		e.daily.USDTWD = twdRate
	}
	if jpyRate > 0 {
		e.daily.USDJPY = jpyRate
	}
	e.saveDailyCache()
}

func (e *ExchangeRateProvider) loadDailyCache() {
	data, err := os.ReadFile(e.cachePath)
	if err != nil {
		return
	}
	var d dailyRateSnapshot
	if err := json.Unmarshal(data, &d); err != nil {
		logging.Warn("exchangerate_provider", "cache_load_failed",
			"path", e.cachePath, "err", err.Error())
		return
	}
	e.daily = d
}

func (e *ExchangeRateProvider) saveDailyCache() {
	if e.cachePath == "" {
		return
	}
	data, err := json.MarshalIndent(e.daily, "", "  ")
	if err != nil {
		return
	}
	if err := os.WriteFile(e.cachePath, data, 0o644); err != nil {
		logging.Warn("exchangerate_provider", "cache_save_failed",
			"path", e.cachePath, "err", err.Error())
	}
}

type exchangeRateResponse struct {
	Result   string             `json:"result"`
	BaseCode string             `json:"base_code"`
	Rates    map[string]float64 `json:"rates"`
}
