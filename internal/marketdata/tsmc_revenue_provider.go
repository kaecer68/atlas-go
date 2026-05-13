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

	current, prior, err := p.fetchWithFallback(ctx, year, month)
	if err != nil {
		logging.Warn("tsmc_revenue_provider", "fetch_failed", logging.Err(err))
		return p.loadLatestSnapshot()
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

func (p *TSMCRevenueProvider) fetchWithFallback(ctx context.Context, year, month int) (current, prior float64, err error) {
	current, err = p.client.GetMonthRevenue(ctx, "2330", year, month)
	if err != nil {
		return 0, 0, fmt.Errorf("current month: %w", err)
	}

	prior, err = p.client.GetMonthRevenue(ctx, "2330", year-1, month)
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
