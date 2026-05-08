package marketdata

import (
	"context"
	"time"

	"github.com/kaecer68/atlas-go/internal/logging"
)

type TSMCRevenueProvider struct {
	client *FinMindClient
}

func NewTSMCRevenueProvider(apiKey string) *TSMCRevenueProvider {
	return &TSMCRevenueProvider{
		client: NewFinMindClient(apiKey),
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

	return MacroDataSnapshot{
		TSMCRevenue: MacroDataPoint{
			Symbol:    "2330.TW",
			Value:     current,
			ChangePct: yoyPct,
			Timestamp: now.Unix(),
		},
	}, nil
}
