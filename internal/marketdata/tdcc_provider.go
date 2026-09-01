package marketdata

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"github.com/kaecer68/atlas-go/internal/apigateway/httpclient"
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
	client  *http.Client
	baseURL string
	limiter *rate.Limiter
	finmind *FinMindClient

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
	// Query a 7-day window ending at date — the weekly snapshot is dated
	// Friday, so any target day inside that week resolves to it.
	start := end.AddDate(0, 0, -7)

	rows, err := p.finmind.FetchDatasetRaw(ctx, "TaiwanStockHoldingSharesPer", "", start.Format("2006-01-02"), end.Format("2006-01-02"))
	if err != nil {
		p.recordFetchFailure(err)
		return nil, fmt.Errorf("tdcc: finmind fetch: %w", err)
	}

	records := make([]EquityDispersionRecord, 0, len(rows))
	for _, row := range rows {
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
