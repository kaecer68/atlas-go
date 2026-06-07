package marketdata

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/kaecer68/atlas-go/internal/logging"
)

// odmSymbols lists the ODM channel stocks tracked by this provider.
// 2317 = Hon Hai (Foxconn), 2382 = Quanta, 6669 = Wiwynn.
var odmSymbols = []string{"2317", "2382", "6669"}

// ODMRevenuePoint represents a single ODM stock's monthly revenue reading.
type ODMRevenuePoint struct {
	Symbol    string  `json:"symbol"`
	Revenue   float64 `json:"revenue"`
	YoYPct    float64 `json:"yoy_pct"`
	Timestamp int64   `json:"timestamp"`
}

// ODMDataProvider exposes ODM channel revenue fetches.
type ODMDataProvider interface {
	FetchODMRevenue(ctx context.Context, symbol string) (ODMRevenuePoint, error)
	FetchAllODMRevenue(ctx context.Context) (map[string]ODMRevenuePoint, error)
}

// ODMRevenueProvider fetches monthly revenue for ODM channel stocks via FinMind.
type ODMRevenueProvider struct {
	client     *FinMindClient
	storageDir string
}

// NewODMRevenueProvider creates a provider that uses the shared FinMind client.
func NewODMRevenueProvider(apiKey string) *ODMRevenueProvider {
	return &ODMRevenueProvider{
		client: GetSharedFinMindClient(apiKey),
	}
}

// NewODMRevenueProviderWithStorage creates a provider that persists fetched
// revenue snapshots to the given directory (one JSON per symbol per month).
func NewODMRevenueProviderWithStorage(apiKey, storageDir string) *ODMRevenueProvider {
	return &ODMRevenueProvider{
		client:     GetSharedFinMindClient(apiKey),
		storageDir: storageDir,
	}
}

// Name returns the provider identifier.
func (p *ODMRevenueProvider) Name() string {
	return "odm_revenue"
}

// FetchODMRevenue fetches the latest month revenue for a single ODM stock and
// computes year-over-year growth against the same month one year prior.
func (p *ODMRevenueProvider) FetchODMRevenue(ctx context.Context, symbol string) (ODMRevenuePoint, error) {
	now := time.Now()
	year, month := now.Year(), int(now.Month())

	current, err := p.client.GetMonthRevenue(ctx, symbol, year, month)
	if err != nil {
		return ODMRevenuePoint{}, fmt.Errorf("odm: current month %s %d-%02d: %w", symbol, year, month, err)
	}

	prior, err := p.client.GetMonthRevenue(ctx, symbol, year-1, month)
	if err != nil {
		return ODMRevenuePoint{}, fmt.Errorf("odm: prior year %s %d-%02d: %w", symbol, year-1, month, err)
	}

	var yoyPct float64
	if prior != 0 {
		yoyPct = (current - prior) / prior * 100
	}

	point := ODMRevenuePoint{
		Symbol:    symbol,
		Revenue:   current,
		YoYPct:    yoyPct,
		Timestamp: now.Unix(),
	}

	if p.storageDir != "" {
		rocYear := year - 1911
		dateStr := fmt.Sprintf("%03d%02d", rocYear, month)
		if err := p.saveRevenue(symbol, dateStr, current, yoyPct); err != nil {
			logging.Warn("odm_revenue_provider", "save_failed", "symbol", symbol, logging.Err(err))
		}
	}

	return point, nil
}

// FetchAllODMRevenue fetches revenue for all tracked ODM stocks. Individual
// failures are logged and skipped; the returned map contains only successes.
// An error is returned only if every fetch failed.
func (p *ODMRevenueProvider) FetchAllODMRevenue(ctx context.Context) (map[string]ODMRevenuePoint, error) {
	result := make(map[string]ODMRevenuePoint, len(odmSymbols))

	for _, symbol := range odmSymbols {
		point, err := p.FetchODMRevenue(ctx, symbol)
		if err != nil {
			logging.Warn("odm_revenue_provider", "fetch_failed", "symbol", symbol, logging.Err(err))
			continue
		}
		result[symbol] = point
	}

	if len(result) == 0 {
		return result, fmt.Errorf("odm_revenue_provider: all %d symbols failed", len(odmSymbols))
	}

	logging.Info("odm_revenue_provider", "fetch_all_success", "count", len(result))
	return result, nil
}

type odmRevenueRecord struct {
	Date      string  `json:"date"`
	Symbol    string  `json:"symbol"`
	Revenue   float64 `json:"revenue"`
	YoYPct    float64 `json:"yoy_pct"`
	Timestamp int64   `json:"timestamp"`
}

func (p *ODMRevenueProvider) saveRevenue(symbol, dateStr string, revenue, yoy float64) error {
	if err := os.MkdirAll(p.storageDir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(p.storageDir, fmt.Sprintf("%s_%s_revenue.json", dateStr, symbol))
	record := odmRevenueRecord{
		Date:      dateStr,
		Symbol:    symbol,
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
