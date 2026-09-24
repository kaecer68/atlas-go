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

// SBL data sources (provenance labels surfaced by LastSource()).
const (
	// SBLSourceFinMind marks the FinMind fallback path
	// (TaiwanDailyShortSaleBalances), which consumes the shared daily quota
	// (finmindDailyLimit).
	SBLSourceFinMind = "finmind-fallback"
	// SBLSourceFirstParty marks the official exchange path
	// (TWSE TWT93U + TPEx margin/sbl); it consumes no FinMind quota.
	SBLSourceFirstParty = "first-party"
)

// ErrSBLNoSource is returned when neither source is available: the first-party
// source is disabled AND no FinMind client was injected.
var ErrSBLNoSource = fmt.Errorf("twse_sbl: no data source available")

// TWSESBLProvider fetches securities borrowing & lending data.
//
// Sources (fix/20260924-finmind-quota):
//
//  1. First-party (default ON): TWSE 「融券借券賣出餘額」TWT93U (上市) +
//     TPEx margin/sbl (上櫃). Zero FinMind quota. Verified row-for-row
//     equivalent to the FinMind dataset — see twse_sbl_firstparty.go.
//  2. FinMind fallback (only when explicitly wired): dataset
//     TaiwanDailyShortSaleBalances via the shared FinMindClient, which spends
//     the shared daily quota. Used when the first-party source is disabled or
//     errors for a day, so a TWSE/TPEx outage degrades instead of breaking.
//
// Pre-2026-09-24 the provider was FinMind-only, so a FinMind quota exhaustion
// caused by OTHER channels turned twse_sbl into a warn channel
// ("finmind: daily quota exhausted (used=14400, remaining=0)") even though the
// data was available from the exchanges for free.
type TWSESBLProvider struct {
	client     *http.Client
	baseURL    string
	tpexURL    string
	limiter    *rate.Limiter
	finmind    *FinMindClient
	storageDir string

	// firstPartyEnabled defaults to true (set in the constructor). Tests that
	// exercise the FinMind fallback path disable it.
	firstPartyEnabled bool

	lastMu        sync.Mutex
	lastSuccessAt time.Time
	lastErr       string
	lastSource    string
}

// LastFetchState reports the outcome of the most recent FetchSBLSummary for
// lightweight health checks (the full-market fetch is too heavy to re-run
// on every probe).
func (p *TWSESBLProvider) LastFetchState() (successAt time.Time, lastErr string) {
	p.lastMu.Lock()
	defer p.lastMu.Unlock()
	return p.lastSuccessAt, p.lastErr
}

// LastSource reports the provenance of the most recent successful fetch
// (SBLSourceFirstParty / SBLSourceFinMind / "TWSE:TWT93U+TPEx:margin/sbl").
// Empty before the first success. Observability only — health decisions must
// not depend on it.
func (p *TWSESBLProvider) LastSource() string {
	p.lastMu.Lock()
	defer p.lastMu.Unlock()
	return p.lastSource
}

func (p *TWSESBLProvider) recordFetchSuccess(source string) {
	p.lastMu.Lock()
	p.lastSuccessAt = time.Now()
	p.lastErr = ""
	if source != "" {
		p.lastSource = source
	}
	p.lastMu.Unlock()
}

func (p *TWSESBLProvider) recordFetchFailure(err error) {
	p.lastMu.Lock()
	p.lastErr = err.Error()
	p.lastMu.Unlock()
}

// NewTWSESBLProvider creates a TWSE SBL data provider.
// ratePerSec is accepted for API compatibility but the provider now
// shares the single TWSE token bucket (P1-13): 11 independent limiters
// against the same host could collectively exceed the documented policy.
func NewTWSESBLProvider(ratePerSec float64) *TWSESBLProvider {
	_ = ratePerSec
	return &TWSESBLProvider{
		client:            httpclient.NewFactory().NewClient(20 * time.Second),
		baseURL:           constants.TWSEBaseURL,
		tpexURL:           tpexSBLBaseURL,
		limiter:           getTWSESharedLimiter(),
		firstPartyEnabled: true,
	}
}

// tpexLimiter returns the shared TPEx bucket (separate host from TWSE).
func (p *TWSESBLProvider) tpexLimiter() *rate.Limiter { return getTPExSharedLimiter() }

// SetHTTPClient overrides the HTTP client (for testing).
func (p *TWSESBLProvider) SetHTTPClient(c *http.Client) { p.client = c }

// SetFinMindClient injects the shared FinMind client. It is now only the
// FALLBACK source (see the type doc): when the first-party source is disabled
// or errors, the provider falls back to FinMind's
// TaiwanDailyShortSaleBalances dataset.
func (p *TWSESBLProvider) SetFinMindClient(f *FinMindClient) { p.finmind = f }

// SetFirstPartyEnabled toggles the official TWSE/TPEx source. Default true.
// Disabling it restores the pre-2026-09-24 FinMind-only behavior (tests and
// emergency rollback use this).
func (p *TWSESBLProvider) SetFirstPartyEnabled(enabled bool) { p.firstPartyEnabled = enabled }

// SetStorageDir enables per-day file persistence: a successful fetch writes
// data/state/sbl/YYYYMMDD_sbl.json (one file per report date), following the
// margin/capital_flow channel convention. Empty (default) = no persistence.
func (p *TWSESBLProvider) SetStorageDir(dir string) { p.storageDir = dir }

// dayFilePath returns the per-day file path for a report date ("" when no
// storage dir is configured).
func (p *TWSESBLProvider) dayFilePath(dateStr string) string {
	if p.storageDir == "" || dateStr == "" {
		return ""
	}
	return filepath.Join(p.storageDir, strings.ReplaceAll(dateStr, "-", "")+"_sbl.json")
}

// dayFileExists reports whether the report day is already persisted. The
// history walk uses it to skip whole days WITHOUT spending HTTP calls
// (fix/20260924-finmind-quota: the backfill is retried hourly while an
// unrelated segment is deferred, and first-party calls are not free either —
// they share the official exchange rate budget).
func (p *TWSESBLProvider) dayFileExists(dateStr string) bool {
	path := p.dayFilePath(dateStr)
	if path == "" {
		return false
	}
	_, err := os.Stat(path)
	return err == nil
}

// persistDay writes one report-date file if it does not already exist.
// Returns true when a new file was written.
func (p *TWSESBLProvider) persistDay(dateStr string, stats []SBLStats) (bool, error) {
	if p.storageDir == "" || dateStr == "" || len(stats) == 0 {
		return false, nil
	}
	if err := os.MkdirAll(p.storageDir, 0o755); err != nil {
		return false, fmt.Errorf("twse_sbl: mkdir: %w", err)
	}
	path := p.dayFilePath(dateStr)
	if _, err := os.Stat(path); err == nil {
		return false, nil // already backfilled/fetched
	}
	payload, err := json.MarshalIndent(stats, "", "  ")
	if err != nil {
		return false, fmt.Errorf("twse_sbl: marshal %s: %w", dateStr, err)
	}
	if err := os.WriteFile(path, payload, 0o644); err != nil {
		return false, fmt.Errorf("twse_sbl: write %s: %w", path, err)
	}
	return true, nil
}

// fetchWindow returns raw FinMind rows for [start, end] (fallback source only).
func (p *TWSESBLProvider) fetchWindow(ctx context.Context, start, end time.Time) ([]map[string]any, error) {
	return p.finmind.FetchDatasetRaw(ctx, "TaiwanDailyShortSaleBalances", "",
		start.Format("2006-01-02"), end.Format("2006-01-02"))
}

// fetchSBLDayFinMind is the FinMind fallback for ONE report day.
//
// EMPIRICAL (2026-09-01): TaiwanDailyShortSaleBalances full-market query
// returns only the START date's rows regardless of the window — so each
// trading day is one dedicated call. Holidays return an empty row set.
func (p *TWSESBLProvider) fetchSBLDayFinMind(ctx context.Context, day time.Time) ([]SBLStats, error) {
	rows, err := p.fetchWindow(ctx, day, day)
	if err != nil {
		return nil, err
	}
	latest := make(map[string]SBLStats, len(rows))
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
	if len(latest) == 0 {
		return nil, nil
	}
	stats := make([]SBLStats, 0, len(latest))
	for _, s := range latest {
		stats = append(stats, s)
	}
	return sortSBLStats(stats), nil
}

// sblDayStats fetches ONE report day from the best available source and
// reports provenance.
//
//	(nil, "", nil) — no data published for that day (weekend / holiday /
//	                 report not yet published). Not an error.
//	(nil, "", err) — both sources failed for a day that is expected to have
//	                 data. Errors from the FinMind fallback keep their
//	                 sentinel identity (e.g. ErrQuotaExhausted), so callers
//	                 (gateway health, scheduled tasks) can classify them.
func (p *TWSESBLProvider) sblDayStats(ctx context.Context, day time.Time) ([]SBLStats, string, error) {
	var firstPartyErr error
	if p.firstPartyEnabled {
		stats, srcs, err := p.fetchSBLDayFirstParty(ctx, day)
		switch {
		case err != nil:
			firstPartyErr = err
		case len(stats) > 0:
			return stats, strings.Join(srcs, "+"), nil
		default:
			// Official sources answered "no data" — do NOT burn FinMind
			// quota trying to prove the same thing.
			return nil, "", nil
		}
	}
	if p.finmind == nil {
		if firstPartyErr != nil {
			return nil, "", firstPartyErr
		}
		return nil, "", fmt.Errorf("twse_sbl: %w (first-party disabled and FinMind client not wired; see G02 implementation notes in internal/marketdata/twse_sbl_provider.go)", ErrSBLNoSource)
	}
	stats, err := p.fetchSBLDayFinMind(ctx, day)
	if err != nil {
		if firstPartyErr != nil {
			return nil, "", fmt.Errorf("twse_sbl: %s first-party failed (%v); finmind fallback failed: %w",
				day.Format("2006-01-02"), firstPartyErr, err)
		}
		return nil, "", err
	}
	if len(stats) == 0 {
		return nil, "", nil
	}
	return stats, SBLSourceFinMind, nil
}

// FetchSBLHistory backfills per-day report files for [start, end].
//
// Source: first-party TWSE TWT93U (上市) + TPEx margin/sbl (上櫃) — at most 2
// official calls per weekday and ZERO FinMind quota
// (fix/20260924-finmind-quota). The FinMind client is a fallback only.
//
// Non-trading days come back empty from both exchanges and are skipped
// silently. Idempotent: existing files are skipped (~22 days per month).
func (p *TWSESBLProvider) FetchSBLHistory(ctx context.Context, start, end time.Time) (int, error) {
	if p.storageDir == "" {
		return 0, fmt.Errorf("twse_sbl: storage dir not set (SetStorageDir) — history fetch would discard results")
	}
	if !p.firstPartyEnabled && p.finmind == nil {
		return 0, fmt.Errorf("twse_sbl: %w (first-party disabled and FinMind client not wired)", ErrSBLNoSource)
	}
	written := 0
	lastSource := ""
	for day := start; !day.After(end); day = day.AddDate(0, 0, 1) {
		if day.Weekday() == time.Saturday || day.Weekday() == time.Sunday {
			continue
		}
		if p.dayFileExists(day.Format("2006-01-02")) {
			continue // 已回補：不發任何 HTTP（每小時重跑的成本趨近 0）
		}
		stats, source, err := p.sblDayStats(ctx, day)
		if err != nil {
			p.recordFetchFailure(err)
			return written, fmt.Errorf("twse_sbl: history day %s: %w", day.Format("2006-01-02"), err)
		}
		if len(stats) == 0 {
			continue // holiday or report not published yet
		}
		lastSource = source
		isNew, err := p.persistDay(day.Format("2006-01-02"), stats)
		if err != nil {
			p.recordFetchFailure(err)
			return written, err
		}
		if isNew {
			written++
		}
	}
	p.recordFetchSuccess(lastSource)
	return written, nil
}

// SetBaseURL overrides the TWSE base URL (for testing).
func (p *TWSESBLProvider) SetBaseURL(u string) { p.baseURL = u }

// SetTPExBaseURL overrides the TPEx base URL (for testing).
func (p *TWSESBLProvider) SetTPExBaseURL(u string) { p.tpexURL = u }

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
// Data source: TWSE 「融券借券賣出餘額」TWT93U (上市) + TPEx margin/sbl (上櫃).
// Both expose the same 14-column 信用額度總量管制餘額表; the 借券 half maps to:
//
//	SBLShortBalance  ← 當日餘額（借券賣出今日餘額）
//	SBLShortVolume   ← 當日賣出（當日借券賣出）
//	SBLReturnVolume  ← 當日還券
//	SBLBorrowBalance ← 兩張表都沒有此欄位，保持 0
//
// date is the target trading day (YYYYMMDD). The report is published after
// close, so the call probes `date` and walks back up to sblProbeDays-1 days
// (weekend/holiday/not-yet-published) until rows come back — the same
// contract the FinMind path had. When the FinMind client is wired and the
// first-party source fails, FinMind is used as fallback (and may return
// ErrQuotaExhausted — a budget condition, not an outage).
func (p *TWSESBLProvider) FetchSBLSummary(ctx context.Context, date string) ([]SBLStats, error) {
	if !p.firstPartyEnabled && p.finmind == nil {
		return nil, fmt.Errorf("twse_sbl: %w (first-party disabled and FinMind client not wired; see G02 implementation notes in internal/marketdata/twse_sbl_provider.go)", ErrSBLNoSource)
	}
	day, err := time.Parse("20060102", date)
	if err != nil {
		return nil, fmt.Errorf("twse_sbl: parse date %q: %w", date, err)
	}
	probed := day
	for i := 0; i < sblProbeDays; i++ {
		stats, source, err := p.sblDayStats(ctx, probed)
		if err != nil {
			p.recordFetchFailure(err)
			return nil, fmt.Errorf("twse_sbl: fetch %s: %w", probed.Format("2006-01-02"), err)
		}
		if len(stats) > 0 {
			// Persist the newest report date's file (daily accumulation).
			// Older dates in the window are backfilled via FetchSBLHistory.
			if newest := stats[0].Date; newest != "" {
				if _, err := p.persistDay(newest, stats); err != nil {
					logging.Warn("twse_sbl_provider", "save_sbl_warning", logging.Err(err))
				}
			}
			p.recordFetchSuccess(source)
			return stats, nil
		}
		probed = probed.AddDate(0, 0, -1)
	}
	err = fmt.Errorf("twse_sbl: no SBL balance data for %s (probed back to %s)", date, probed.Format("2006-01-02"))
	p.recordFetchFailure(err)
	return nil, err
}
