package marketdata

import (
	"context"
	"encoding/json"
	"fmt"
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

// SBLStats holds securities borrowing & lending statistics for a single stock.
type SBLStats struct {
	Date             string `json:"date"`
	Symbol           string `json:"symbol"`
	SBLShortBalance  int64  `json:"sbl_short_balance"`  // 借券賣出餘額（股）
	SBLShortVolume   int64  `json:"sbl_short_volume"`   // 當日借券賣出股數
	SBLReturnVolume  int64  `json:"sbl_return_volume"`  // 當日還券股數
	SBLBorrowBalance int64  `json:"sbl_borrow_balance"` // 借券餘額（股）
}

// TWSESBLProvider fetches securities borrowing & lending data from TWSE.
// Data source: TWSE 借券賣出餘額表（TWT93U）。
//
// The TWSE endpoint for SBL data uses a different URL pattern than the
// standard OpenAPI. Currently the provider records a placeholder until
// the exact endpoint is confirmed. When the TWSE SBL API is live:
//   - Set endpoint via SetBaseURL
//   - Implement FetchSBL() with actual HTTP call + JSON parsing
type TWSESBLProvider struct {
	client     *http.Client
	baseURL    string
	limiter    *rate.Limiter
	finmind    *FinMindClient
	storageDir string

	lastMu        sync.Mutex
	lastSuccessAt time.Time
	lastErr       string
}

// LastFetchState reports the outcome of the most recent FetchSBLSummary for
// lightweight health checks (the full-market fetch is too heavy to re-run
// on every probe).
func (p *TWSESBLProvider) LastFetchState() (successAt time.Time, lastErr string) {
	p.lastMu.Lock()
	defer p.lastMu.Unlock()
	return p.lastSuccessAt, p.lastErr
}

func (p *TWSESBLProvider) recordFetchSuccess() {
	p.lastMu.Lock()
	p.lastSuccessAt = time.Now()
	p.lastErr = ""
	p.lastMu.Unlock()
}

func (p *TWSESBLProvider) recordFetchFailure(err error) {
	p.lastMu.Lock()
	p.lastErr = err.Error()
	p.lastMu.Unlock()
}

// NewTWSESBLProvider creates a TWSE SBL data provider.
// ratePerSec defaults to 0.5 (1 request per 2 seconds) per TWSE rate-limiting.
func NewTWSESBLProvider(ratePerSec float64) *TWSESBLProvider {
	// ratePerSec is accepted for API compatibility but the provider now
	// shares the single TWSE token bucket (P1-13): 11 independent limiters
	// against the same host could collectively exceed the documented policy.
	_ = ratePerSec
	return &TWSESBLProvider{
		client:  httpclient.NewFactory().NewClient(20 * time.Second),
		baseURL: constants.TWSEBaseURL,
		limiter: getTWSESharedLimiter(),
	}
}

// SetHTTPClient overrides the HTTP client (for testing).
func (p *TWSESBLProvider) SetHTTPClient(c *http.Client) { p.client = c }

// SetFinMindClient injects the shared FinMind client (G02 wiring). Without
// it FetchSBLSummary keeps returning the "not wired" error.
func (p *TWSESBLProvider) SetFinMindClient(f *FinMindClient) { p.finmind = f }

// SetStorageDir enables per-day file persistence: a successful fetch writes
// data/state/sbl/YYYYMMDD_sbl.json (one file per report date), following the
// margin/capital_flow channel convention. Empty (default) = no persistence.
func (p *TWSESBLProvider) SetStorageDir(dir string) { p.storageDir = dir }

// persistDay writes one report-date file if it does not already exist.
// Returns true when a new file was written.
func (p *TWSESBLProvider) persistDay(dateStr string, stats []SBLStats) (bool, error) {
	if p.storageDir == "" || dateStr == "" || len(stats) == 0 {
		return false, nil
	}
	if err := os.MkdirAll(p.storageDir, 0o755); err != nil {
		return false, fmt.Errorf("twse_sbl: mkdir: %w", err)
	}
	fileName := strings.ReplaceAll(dateStr, "-", "") + "_sbl.json"
	path := filepath.Join(p.storageDir, fileName)
	if _, err := os.Stat(path); err == nil {
		return false, nil // already backfilled/fetched
	}
	payload, err := json.MarshalIndent(stats, "", "  ")
	if err != nil {
		return false, fmt.Errorf("twse_sbl: marshal %s: %w", dateStr, err)
	}
	if err := os.WriteFile(path, payload, 0o644); err != nil {
		return false, fmt.Errorf("twse_sbl: write %s: %w", fileName, err)
	}
	return true, nil
}

// fetchWindow returns raw FinMind rows for [start, end].
func (p *TWSESBLProvider) fetchWindow(ctx context.Context, start, end time.Time) ([]map[string]any, error) {
	return p.finmind.FetchDatasetRaw(ctx, "TaiwanDailyShortSaleBalances", "",
		start.Format("2006-01-02"), end.Format("2006-01-02"))
}

// FetchSBLHistory backfills per-day report files for [start, end].
// EMPIRICAL (2026-09-01): TaiwanDailyShortSaleBalances full-market query
// returns only the START date's rows regardless of the window — so each
// trading day is one dedicated call (weekends skipped; holidays return
// empty and are skipped silently). ~22 calls per month. Idempotent:
// existing files are skipped.
func (p *TWSESBLProvider) FetchSBLHistory(ctx context.Context, start, end time.Time) (int, error) {
	if p.finmind == nil {
		return 0, fmt.Errorf("twse_sbl: FinMind client not wired; see G02 implementation notes in internal/marketdata/twse_sbl_provider.go")
	}
	if p.storageDir == "" {
		return 0, fmt.Errorf("twse_sbl: storage dir not set (SetStorageDir) — history fetch would discard results")
	}
	written := 0
	for day := start; !day.After(end); day = day.AddDate(0, 0, 1) {
		if day.Weekday() == time.Saturday || day.Weekday() == time.Sunday {
			continue
		}
		rows, err := p.fetchWindow(ctx, day, day)
		if err != nil {
			p.recordFetchFailure(err)
			return written, fmt.Errorf("twse_sbl: history day %s: %w", day.Format("2006-01-02"), err)
		}
		stats := make([]SBLStats, 0, len(rows))
		for _, row := range rows {
			sym := strField(row, "stock_id")
			if sym == "" {
				continue
			}
			stats = append(stats, SBLStats{
				Date:            strField(row, "date"),
				Symbol:          sym,
				SBLShortBalance: int64(floatField(row, "SBLShortSalesCurrentDayBalance")),
				SBLShortVolume:  int64(floatField(row, "SBLShortSalesShortSales")),
				SBLReturnVolume: int64(floatField(row, "SBLShortSalesReturns")),
			})
		}
		dateStr := day.Format("2006-01-02")
		isNew, err := p.persistDay(dateStr, stats)
		if err != nil {
			return written, err
		}
		if isNew {
			written++
		}
	}
	p.recordFetchSuccess()
	return written, nil
}

// SetBaseURL overrides the base URL (for testing).
func (p *TWSESBLProvider) SetBaseURL(u string) { p.baseURL = u }

// Name identifies this provider.
func (p *TWSESBLProvider) Name() string { return "twse_sbl" }

// SetRateLimiter overrides the rate limiter (tests only; P1-13 shared-bucket
// tests use SetTWSESharedLimiterForTest instead).
func (p *TWSESBLProvider) SetRateLimiter(l *rate.Limiter) {
	if l != nil {
		p.limiter = l
	}
}

// RateLimiter returns the per-provider rate limiter.
func (p *TWSESBLProvider) RateLimiter() *rate.Limiter { return p.limiter }

// FetchSBLSummary fetches the daily SBL summary data (full market).
//
// Data source: FinMind dataset "TaiwanDailyShortSaleBalances" — the TWSE
// 借券賣出餘額每日報表 (full-market, one call; ~2.2k rows verified
// 2026-09-01). Field mapping into SBLStats:
//
//	SBLShortBalance  ← SBLShortSalesCurrentDayBalance（借券賣出今日餘額）
//	SBLShortVolume   ← SBLShortSalesShortSales（當日借券賣出）
//	SBLReturnVolume  ← SBLShortSalesReturns（當日還券）
//	SBLBorrowBalance ← 未提供（dataset 無借券餘額欄位，保持 0）
//
// date is the target trading day (YYYYMMDD).
func (p *TWSESBLProvider) FetchSBLSummary(ctx context.Context, date string) ([]SBLStats, error) {
	if p.finmind == nil {
		return nil, fmt.Errorf("twse_sbl: FinMind client not wired; see G02 implementation notes in internal/marketdata/twse_sbl_provider.go")
	}
	day, err := time.Parse("20060102", date)
	if err != nil {
		return nil, fmt.Errorf("twse_sbl: parse date %q: %w", date, err)
	}
	// EMPIRICAL: single-day windows only. Probe day, then walk back up to
	// 4 days (weekend/holiday gap) until rows come back.
	var latest map[string]SBLStats
	probed := day
	for i := 0; i < 5; i++ {
		rows, err := p.finmind.FetchDatasetRaw(ctx, "TaiwanDailyShortSaleBalances", "",
			probed.Format("2006-01-02"), probed.Format("2006-01-02"))
		if err != nil {
			p.recordFetchFailure(err)
			return nil, fmt.Errorf("twse_sbl: finmind fetch %s: %w", probed.Format("2006-01-02"), err)
		}
		latest = make(map[string]SBLStats, len(rows))
		for _, row := range rows {
			rowDate := strField(row, "date")
			sym := strField(row, "stock_id")
			if rowDate == "" || sym == "" {
				continue
			}
			latest[sym] = SBLStats{
				Date:            rowDate,
				Symbol:          sym,
				SBLShortBalance: int64(floatField(row, "SBLShortSalesCurrentDayBalance")),
				SBLShortVolume:  int64(floatField(row, "SBLShortSalesShortSales")),
				SBLReturnVolume: int64(floatField(row, "SBLShortSalesReturns")),
			}
		}
		if len(latest) > 0 {
			break
		}
		probed = probed.AddDate(0, 0, -1)
	}
	if len(latest) == 0 {
		err := fmt.Errorf("twse_sbl: no SBL balance data for %s (probed back to %s)", date, probed.Format("2006-01-02"))
		p.recordFetchFailure(err)
		return nil, err
	}
	stats := make([]SBLStats, 0, len(latest))
	for _, s := range latest {
		stats = append(stats, s)
	}
	// Persist the newest report date's file (daily accumulation). Older
	// dates in the window are backfilled via FetchSBLHistory.
	if newest := stats[0].Date; newest != "" {
		if _, err := p.persistDay(newest, stats); err != nil {
			logging.Warn("twse_sbl_provider", "save_sbl_warning", logging.Err(err))
		}
	}
	p.recordFetchSuccess()
	return stats, nil
}
