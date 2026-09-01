package marketdata

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"github.com/kaecer68/atlas-go/internal/apigateway/httpclient"
	"github.com/kaecer68/atlas-go/internal/logging"
)

// EquityDispersionRecord holds a single row from the TDCC equity dispersion table.
// Each row represents one shareholding tier for one stock.
type EquityDispersionRecord struct {
	Date       string  `json:"date"`
	Symbol     string  `json:"symbol"`
	Tier       string  `json:"tier"`        // e.g. "1-999", "1000-5000", ">400000"
	Holders    int     `json:"holders"`     // number of shareholders in this tier
	SharesHeld int64   `json:"shares_held"` // total shares held by this tier
	PctHeld    float64 `json:"pct_held"`    // percentage of total outstanding shares
}

// TDCClient provides TDCC (臺灣集中保管結算所) equity dispersion data (G01).
//
// Data source: FinMind dataset "TaiwanStockHoldingSharesPer" — the weekly
// 集保戶股權分散表 (shareholding-tier distribution), mirrored from TDCC.
// Full-market query in a single call (~68k rows: ~2k symbols × 16 tiers);
// the table updates once a week (data dated Friday, available early the
// following week).
//
// The weekly cadence matters for consumers: do not treat "same data as
// yesterday" as staleness — compare against the most recent Friday.
type TDCClient struct {
	client     *http.Client
	baseURL    string
	limiter    *rate.Limiter
	finmind    *FinMindClient
	storageDir string

	lastMu        sync.Mutex
	lastSuccessAt time.Time
	lastErr       string
}

// LastFetchState reports the outcome of the most recent FetchDispersion for
// lightweight health checks (the full-market fetch is too heavy to re-run
// on every probe).
func (p *TDCClient) LastFetchState() (successAt time.Time, lastErr string) {
	p.lastMu.Lock()
	defer p.lastMu.Unlock()
	return p.lastSuccessAt, p.lastErr
}

func (p *TDCClient) recordFetchSuccess() {
	p.lastMu.Lock()
	p.lastSuccessAt = time.Now()
	p.lastErr = ""
	p.lastMu.Unlock()
}

func (p *TDCClient) recordFetchFailure(err error) {
	p.lastMu.Lock()
	p.lastErr = err.Error()
	p.lastMu.Unlock()
}

// NewTDCClient creates a TDCC equity dispersion provider.
func NewTDCClient() *TDCClient {
	return &TDCClient{
		client:  httpclient.NewFactory().NewClient(20 * time.Second),
		baseURL: "https://openapi.tdcc.com.tw",
		limiter: rate.NewLimiter(rate.Limit(0.2), 1), // conservative: 1 req / 5s
	}
}

// Name identifies this provider.
func (p *TDCClient) Name() string { return "tdcc_equity_dispersion" }

// RateLimiter returns the per-provider rate limiter.
func (p *TDCClient) RateLimiter() *rate.Limiter { return p.limiter }

// SetFinMindClient injects the shared FinMind client (G01 wiring). Without
// it FetchDispersion keeps returning the "not configured" error so tests
// and cold-start paths degrade explicitly instead of panicking.
func (p *TDCClient) SetFinMindClient(f *FinMindClient) { p.finmind = f }

// SetStorageDir enables persistence: FetchDispersion writes
// data/state/tdcc_dispersion/YYYYMMDD_tdcc_dispersion.json (one file per
// weekly snapshot date) plus latest.json (channel freshness anchor).
func (p *TDCClient) SetStorageDir(dir string) { p.storageDir = dir }

// StorageDir returns the configured persistence directory ("" = off).
func (p *TDCClient) StorageDir() string { return p.storageDir }

// persistSnapshot writes one weekly snapshot file if missing.
func (p *TDCClient) persistSnapshot(dateStr string, records []EquityDispersionRecord) (bool, error) {
	if p.storageDir == "" || dateStr == "" || len(records) == 0 {
		return false, nil
	}
	if err := os.MkdirAll(p.storageDir, 0o755); err != nil {
		return false, fmt.Errorf("tdcc: mkdir: %w", err)
	}
	fileName := strings.ReplaceAll(dateStr, "-", "") + "_tdcc_dispersion.json"
	path := filepath.Join(p.storageDir, fileName)
	if _, err := os.Stat(path); err == nil {
		return false, nil
	}
	payload, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		return false, fmt.Errorf("tdcc: marshal %s: %w", dateStr, err)
	}
	if err := os.WriteFile(path, payload, 0o644); err != nil {
		return false, fmt.Errorf("tdcc: write %s: %w", fileName, err)
	}
	return true, nil
}

// FetchDispersionHistory backfills weekly snapshot files for [start, end].
// Each month chunk is one FinMind call (~270k rows); returns the number of
// NEW snapshot files written. Idempotent.
func (p *TDCClient) FetchDispersionHistory(ctx context.Context, start, end time.Time) (int, error) {
	if p.finmind == nil {
		return 0, fmt.Errorf("tdcc: API access not yet configured; see G01 implementation notes in internal/marketdata/tdcc_provider.go")
	}
	if p.storageDir == "" {
		return 0, fmt.Errorf("tdcc: storage dir not set (SetStorageDir) — history fetch would discard results")
	}
	written := 0
	for chunkStart := start; !chunkStart.After(end); chunkStart = chunkStart.AddDate(0, 1, 0) {
		chunkEnd := chunkStart.AddDate(0, 1, 0).AddDate(0, 0, -1)
		if chunkEnd.After(end) {
			chunkEnd = end
		}
		rows, err := p.finmind.FetchDatasetRaw(ctx, "TaiwanStockHoldingSharesPer", "",
			chunkStart.Format("2006-01-02"), chunkEnd.Format("2006-01-02"))
		if err != nil {
			p.recordFetchFailure(err)
			return written, fmt.Errorf("tdcc: history chunk %s..%s: %w", chunkStart.Format("2006-01-02"), chunkEnd.Format("2006-01-02"), err)
		}
		byDate := map[string][]EquityDispersionRecord{}
		for _, row := range rows {
			dateStr := strField(row, "date")
			sym := strField(row, "stock_id")
			if dateStr == "" || sym == "" {
				continue
			}
			byDate[dateStr] = append(byDate[dateStr], EquityDispersionRecord{
				Date:       dateStr,
				Symbol:     sym,
				Tier:       strField(row, "HoldingSharesLevel"),
				Holders:    int(floatField(row, "people")),
				SharesHeld: int64(floatField(row, "unit")),
				PctHeld:    floatField(row, "percent"),
			})
		}
		for dateStr, records := range byDate {
			isNew, err := p.persistSnapshot(dateStr, records)
			if err != nil {
				return written, err
			}
			if isNew {
				written++
			}
		}
	}
	p.recordFetchSuccess()
	return written, nil
}

// FetchDispersion fetches the weekly equity dispersion table (full market).
//
// date is the target trading day (YYYYMMDD); FinMind returns the most
// recent weekly snapshot on or before it.
func (p *TDCClient) FetchDispersion(ctx context.Context, date string) ([]EquityDispersionRecord, error) {
	if p.finmind == nil {
		return nil, fmt.Errorf("tdcc: API access not yet configured; see G01 implementation notes in internal/marketdata/tdcc_provider.go")
	}
	end, err := time.Parse("20060102", date)
	if err != nil {
		return nil, fmt.Errorf("tdcc: parse date %q: %w", date, err)
	}
	// Query a 21-day window ending at date. The weekly snapshot is dated
	// Friday, but FinMind's copy can lag publishing by 1-2 weeks (verified
	// 2026-09-01: latest snapshot 2026-08-21 while target 2026-09-01).
	// Only the newest snapshot date in the window is kept (below).
	start := end.AddDate(0, 0, -21)

	rows, err := p.finmind.FetchDatasetRaw(ctx, "TaiwanStockHoldingSharesPer", "", start.Format("2006-01-02"), end.Format("2006-01-02"))
	if err != nil {
		p.recordFetchFailure(err)
		return nil, fmt.Errorf("tdcc: finmind fetch: %w", err)
	}

	// Keep only the newest weekly snapshot inside the window.
	newestDate := ""
	for _, row := range rows {
		if d := strField(row, "date"); d > newestDate {
			newestDate = d
		}
	}
	records := make([]EquityDispersionRecord, 0, len(rows))
	for _, row := range rows {
		if strField(row, "date") != newestDate {
			continue
		}
		rec := EquityDispersionRecord{
			Date:       strField(row, "date"),
			Symbol:     strField(row, "stock_id"),
			Tier:       strField(row, "HoldingSharesLevel"),
			Holders:    int(floatField(row, "people")),
			SharesHeld: int64(floatField(row, "unit")),
			PctHeld:    floatField(row, "percent"),
		}
		records = append(records, rec)
	}
	if len(records) == 0 {
		err := fmt.Errorf("tdcc: no dispersion data for %s (weekly snapshot may not be published yet)", date)
		p.recordFetchFailure(err)
		return nil, err
	}
	// Persist the weekly snapshot + refresh latest.json (channel freshness
	// anchor for the gap detector's CoverageLatestFile check).
	if _, err := p.persistSnapshot(records[0].Date, records); err != nil {
		logging.Warn("tdcc_provider", "save_dispersion_warning", logging.Err(err))
	}
	if p.storageDir != "" {
		latestPath := filepath.Join(p.storageDir, "latest.json")
		if payload, err := json.MarshalIndent(records, "", "  "); err == nil {
			if err := os.WriteFile(latestPath, payload, 0o644); err != nil {
				logging.Warn("tdcc_provider", "save_latest_warning", logging.Err(err))
			}
		}
	}
	p.recordFetchSuccess()
	return records, nil
}

// SetHTTPClient overrides the HTTP client (for testing).
func (p *TDCClient) SetHTTPClient(c *http.Client) { p.client = c }

// strField extracts a string field from a FinMind generic row.
func strField(row map[string]any, key string) string {
	if v, ok := row[key].(string); ok {
		return v
	}
	return ""
}

// floatField extracts a numeric field (FinMind numbers decode as float64).
func floatField(row map[string]any, key string) float64 {
	switch v := row[key].(type) {
	case float64:
		return v
	case string:
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return 0
}
