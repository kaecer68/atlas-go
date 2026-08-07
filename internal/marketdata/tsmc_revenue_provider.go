package marketdata

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/internal/logging"
)

type TSMCRevenueProvider struct {
	client     *FinMindClient
	storageDir string
	OnDegraded func(channelID, reason string)
}

func NewTSMCRevenueProvider(apiKey string) *TSMCRevenueProvider {
	return &TSMCRevenueProvider{
		client: GetSharedFinMindClient(apiKey),
	}
}

func NewTSMCRevenueProviderWithStorage(apiKey, storageDir string) *TSMCRevenueProvider {
	return &TSMCRevenueProvider{
		client:     GetSharedFinMindClient(apiKey),
		storageDir: storageDir,
	}
}

// NewTSMCRevenueProviderWithClient returns a provider backed by an explicit
// FinMindClient. Test-only convenience — production callers should use
// NewTSMCRevenueProvider which shares the singleton client and its
// QuotaRegistry-tracked DailyQuotaTracker.
func NewTSMCRevenueProviderWithClient(client *FinMindClient) *TSMCRevenueProvider {
	return &TSMCRevenueProvider{client: client}
}

func (p *TSMCRevenueProvider) Name() string {
	return "tsmc_revenue"
}

func (p *TSMCRevenueProvider) FetchSnapshot(ctx context.Context) (MacroDataSnapshot, error) {
	// Default symbol = "2330" (TSMC). Preserved for backward compatibility
	// with the macro channel consumer silicon_cycle.go:408 which reads
	// TSMCRevenue.ChangePct regardless of symbol. New per-symbol callers
	// should use FetchSnapshotForSymbol directly.
	//
	// Cache layout for the macro path is preserved: {storageDir}/*_revenue.json
	// (root-level flat layout). The per-symbol subdirectory layout is
	// reserved for FetchSnapshotForSymbol to avoid colliding with the
	// existing TSMC cache files.
	now := time.Now()
	year, month := now.Year(), int(now.Month())

	current, prior, err := p.fetchWithFallback(ctx, "2330", year, month)
	if err != nil {
		logging.Info("tsmc_revenue_provider", "fetch_failed_falling_back_to_cache", logging.Err(err))
		snap, loadErr := p.loadLatestSnapshot()
		if loadErr == nil && snap.TSMCRevenue.Symbol != "" && p.OnDegraded != nil {
			p.OnDegraded("tsmc_revenue", "cache_fallback")
		}
		return snap, loadErr
	}

	var yoyPct float64
	if prior != 0 {
		yoyPct = (current - prior) / prior * 100
	}

	snap := MacroDataSnapshot{
		TSMCRevenue: MacroDataPoint{
			Symbol:    "2330.TW",
			Value:     current,
			ChangePct: yoyPct,
			Timestamp: now.Unix(),
		},
	}

	if p.storageDir != "" {
		rocYear := year - 1911
		rocMonth := fmt.Sprintf("%03d%02d", rocYear, month)
		if err := p.saveRevenue(rocMonth, current, yoyPct); err != nil {
			logging.Warn("tsmc_revenue_provider", "save_failed", logging.Err(err))
		}
	}

	return snap, nil
}

// are all covered by FinMind TaiwanStockMonthRevenue, unlike the
// stocktools Fundamentals-based coverage guard which is TWSE-only per
// PR #1477).
//
// On upstream failure the provider falls back to the on-disk cache under
// p.storageDir. The cache layout is per-symbol subdirectory (created
// lazily on first write), so the per-symbol load does not collide with
// the TSMC cache used by FetchSnapshot.
func (p *TSMCRevenueProvider) FetchSnapshotForSymbol(ctx context.Context, symbol string) (MacroDataSnapshot, error) {
	now := time.Now()
	year, month := now.Year(), int(now.Month())

	current, prior, err := p.fetchWithFallback(ctx, symbol, year, month)
	if err != nil {
		logging.Info("tsmc_revenue_provider", "fetch_failed_falling_back_to_cache",
			"symbol", symbol, logging.Err(err))
		snap, loadErr := p.loadLatestSnapshotForSymbol(symbol)
		if loadErr == nil && snap.TSMCRevenue.Symbol != "" && p.OnDegraded != nil {
			p.OnDegraded("tsmc_revenue", "cache_fallback")
		}
		return snap, loadErr
	}

	var yoyPct float64
	if prior != 0 {
		yoyPct = (current - prior) / prior * 100
	}

	snap := MacroDataSnapshot{
		TSMCRevenue: MacroDataPoint{
			Symbol:    symbol + ".TW",
			Value:     current,
			ChangePct: yoyPct,
			Timestamp: now.Unix(),
		},
	}

	if p.storageDir != "" {
		rocYear := year - 1911
		rocMonth := fmt.Sprintf("%03d%02d", rocYear, month)
		if err := p.saveRevenueForSymbol(symbol, rocMonth, current, yoyPct); err != nil {
			logging.Warn("tsmc_revenue_provider", "save_failed",
				"symbol", symbol, logging.Err(err))
		}
	}

	return snap, nil
}

// QuotaRemaining returns the number of FinMind API calls still available
// today. Surfaced via the stocktools /api/stock/monthly_revenue handler
// so the handler can fail-soft (503) before exhausting the daily budget
// on a 3-call per-symbol lookup.
func (p *TSMCRevenueProvider) QuotaRemaining() int {
	if p.client == nil {
		return 0
	}
	return p.client.QuotaRemaining()
}

// FetchSnapshotForSymbolAt is like FetchSnapshotForSymbol but the caller
// supplies an explicit reporting (year, month) instead of "now - 1 month".
// Used by the stocktools handler which parses the year/month query
// params and needs to avoid the provider recomputing them from
// time.Now() (the handler may have a different "last closed month"
// policy than the macro channel).
func (p *TSMCRevenueProvider) FetchSnapshotForSymbolAt(ctx context.Context, symbol string, year, month int) (MacroDataSnapshot, error) {
	current, prior, err := p.fetchWithFallback(ctx, symbol, year, month)
	if err != nil {
		logging.Info("tsmc_revenue_provider", "fetch_failed_falling_back_to_cache",
			"symbol", symbol, "year", year, "month", month, logging.Err(err))
		snap, loadErr := p.loadLatestSnapshotForSymbol(symbol)
		if loadErr == nil && snap.TSMCRevenue.Symbol != "" && p.OnDegraded != nil {
			p.OnDegraded("tsmc_revenue", "cache_fallback")
		}
		return snap, loadErr
	}

	var yoyPct float64
	if prior != 0 {
		yoyPct = (current - prior) / prior * 100
	}

	snap := MacroDataSnapshot{
		TSMCRevenue: MacroDataPoint{
			Symbol:    symbol + ".TW",
			Value:     current,
			ChangePct: yoyPct,
			Timestamp: time.Now().Unix(),
		},
	}

	if p.storageDir != "" {
		rocYear := year - 1911
		rocMonth := fmt.Sprintf("%03d%02d", rocYear, month)
		if err := p.saveRevenueForSymbol(symbol, rocMonth, current, yoyPct); err != nil {
			logging.Warn("tsmc_revenue_provider", "save_failed",
				"symbol", symbol, logging.Err(err))
		}
	}
	return snap, nil
}

func (p *TSMCRevenueProvider) fetchWithFallback(ctx context.Context, symbol string, year, month int) (current, prior float64, err error) {
	current, err = p.client.GetMonthRevenue(ctx, symbol, year, month)
	if err != nil {
		return 0, 0, fmt.Errorf("current month: %w", err)
	}

	prior, err = p.client.GetMonthRevenue(ctx, symbol, year-1, month)
	if err != nil {
		return 0, 0, fmt.Errorf("prior year: %w", err)
	}

	return current, prior, nil
}

func (p *TSMCRevenueProvider) loadLatestSnapshot() (MacroDataSnapshot, error) {
	if p.storageDir == "" {
		return MacroDataSnapshot{TSMCRevenue: MacroDataPoint{Symbol: ""}}, nil
	}

	entries, err := os.ReadDir(p.storageDir)
	if err != nil {
		return MacroDataSnapshot{TSMCRevenue: MacroDataPoint{Symbol: ""}}, nil
	}

	var latestFile string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), "_revenue.json") {
			continue
		}
		if e.Name() > latestFile {
			latestFile = e.Name()
		}
	}

	if latestFile == "" {
		return MacroDataSnapshot{TSMCRevenue: MacroDataPoint{Symbol: ""}}, nil
	}

	data, err := os.ReadFile(filepath.Join(p.storageDir, latestFile))
	if err != nil {
		return MacroDataSnapshot{TSMCRevenue: MacroDataPoint{Symbol: ""}}, nil
	}

	var record tsmcRevenueRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return MacroDataSnapshot{TSMCRevenue: MacroDataPoint{Symbol: ""}}, nil
	}

	return MacroDataSnapshot{
		TSMCRevenue: MacroDataPoint{
			Symbol:    "2330.TW",
			Value:     record.Revenue,
			ChangePct: record.YoYPct,
			Timestamp: record.Timestamp,
		},
	}, nil
}

type tsmcRevenueRecord struct {
	Date      string  `json:"date"`
	Revenue   float64 `json:"revenue"`
	YoYPct    float64 `json:"yoy_pct"`
	Timestamp int64   `json:"timestamp"`
}

func (p *TSMCRevenueProvider) saveRevenue(dateStr string, revenue, yoy float64) error {
	if err := os.MkdirAll(p.storageDir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(p.storageDir, dateStr+"_revenue.json")
	record := tsmcRevenueRecord{
		Date:      dateStr,
		Revenue:   revenue,
		YoYPct:    yoy,
		Timestamp: time.Now().Unix(),
	}
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// loadLatestSnapshotForSymbol reads the most recent on-disk cache for the
// given symbol under {storageDir}/{symbol}/. Per-symbol subdirectories
// keep the TSMC macro cache (root of storageDir) cleanly separated from
// the per-symbol stocktools caches.
func (p *TSMCRevenueProvider) loadLatestSnapshotForSymbol(symbol string) (MacroDataSnapshot, error) {
	if p.storageDir == "" {
		return MacroDataSnapshot{TSMCRevenue: MacroDataPoint{Symbol: ""}}, nil
	}
	symDir := filepath.Join(p.storageDir, symbol)
	entries, err := os.ReadDir(symDir)
	if err != nil {
		return MacroDataSnapshot{TSMCRevenue: MacroDataPoint{Symbol: ""}}, nil
	}
	var latestFile string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), "_revenue.json") {
			continue
		}
		if e.Name() > latestFile {
			latestFile = e.Name()
		}
	}
	if latestFile == "" {
		return MacroDataSnapshot{TSMCRevenue: MacroDataPoint{Symbol: ""}}, nil
	}
	data, err := os.ReadFile(filepath.Join(symDir, latestFile))
	if err != nil {
		return MacroDataSnapshot{TSMCRevenue: MacroDataPoint{Symbol: ""}}, nil
	}
	var record tsmcRevenueRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return MacroDataSnapshot{TSMCRevenue: MacroDataPoint{Symbol: ""}}, nil
	}
	return MacroDataSnapshot{
		TSMCRevenue: MacroDataPoint{
			Symbol:    symbol + ".TW",
			Value:     record.Revenue,
			ChangePct: record.YoYPct,
			Timestamp: record.Timestamp,
		},
	}, nil
}

// saveRevenueForSymbol writes a per-symbol revenue record under
// {storageDir}/{symbol}/{date}_revenue.json. See FetchSnapshotForSymbol
// for the rationale behind the per-symbol subdirectory layout.
func (p *TSMCRevenueProvider) saveRevenueForSymbol(symbol, dateStr string, revenue, yoy float64) error {
	if p.storageDir == "" {
		return nil
	}
	symDir := filepath.Join(p.storageDir, symbol)
	if err := os.MkdirAll(symDir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(symDir, dateStr+"_revenue.json")
	record := tsmcRevenueRecord{
		Date:      dateStr,
		Revenue:   revenue,
		YoYPct:    yoy,
		Timestamp: time.Now().Unix(),
	}
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
