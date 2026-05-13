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

type TSMCRevenueProvider struct {
	client     *FinMindClient
	storageDir string
}

func NewTSMCRevenueProvider(apiKey string) *TSMCRevenueProvider {
	return &TSMCRevenueProvider{
		client: NewFinMindClient(apiKey),
	}
}

func NewTSMCRevenueProviderWithStorage(apiKey, storageDir string) *TSMCRevenueProvider {
	return &TSMCRevenueProvider{
		client:     NewFinMindClient(apiKey),
		storageDir: storageDir,
	}
}

func (p *TSMCRevenueProvider) Name() string {
	return "tsmc_revenue"
}

func (p *TSMCRevenueProvider) FetchSnapshot(ctx context.Context) (MacroDataSnapshot, error) {
	now := time.Now()
	year, month := now.Year(), int(now.Month())

	current, err := p.client.GetMonthRevenue(ctx, "2330", year, month)
	if err != nil {
		logging.Warn("tsmc_revenue_provider", "fetch_current_month_failed", logging.Err(err))
		return MacroDataSnapshot{TSMCRevenue: MacroDataPoint{Symbol: ""}}, nil
	}

	prior, err := p.client.GetMonthRevenue(ctx, "2330", year-1, month)
	if err != nil {
		logging.Warn("tsmc_revenue_provider", "fetch_prior_year_failed", logging.Err(err))
		return MacroDataSnapshot{TSMCRevenue: MacroDataPoint{Symbol: ""}}, nil
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
