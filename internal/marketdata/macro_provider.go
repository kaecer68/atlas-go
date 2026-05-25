package marketdata

import (
	"context"
	"errors"
	"time"
)

// MacroDataPoint represents a single macro indicator reading.
type MacroDataPoint struct {
	Symbol    string  `json:"symbol"`
	Value     float64 `json:"value"`
	ChangePct float64 `json:"change_pct"`
	Timestamp int64   `json:"timestamp"`
}

// MacroDataSnapshot holds the latest readings for all tracked indicators.
type MacroDataSnapshot struct {
	US10Y               MacroDataPoint `json:"us10y"`
	DXY                 MacroDataPoint `json:"dxy"`
	VIX                 MacroDataPoint `json:"vix"`
	USD_TWD             MacroDataPoint `json:"usd_twd"`
	Oil                 MacroDataPoint `json:"oil"`
	Gold                MacroDataPoint `json:"gold"`
	JPY                 MacroDataPoint `json:"jpy"`
	ForeignInvestorNet  MacroDataPoint `json:"foreign_investor_net"`
	DomesticFundNet     MacroDataPoint `json:"domestic_fund_net"`
	DealerNet           MacroDataPoint `json:"dealer_net"`
	ExportElectronics   MacroDataPoint `json:"export_electronics"`
	RetailMarginBalance MacroDataPoint `json:"retail_margin_balance"`
	RetailShortBalance  MacroDataPoint `json:"retail_short_balance"`
	TSMCRevenue         MacroDataPoint `json:"tsmc_revenue"`
	SOXIndex            MacroDataPoint `json:"sox_index"`
	CoWoSUtilization    MacroDataPoint `json:"cowos_utilization"`
	CapexGrowth         MacroDataPoint `json:"capex_growth"`
	CPIYoY              MacroDataPoint `json:"cpi_yoy"`
	Bdi                 MacroDataPoint `json:"bdi"`
	RecordedAt          int64          `json:"recorded_at"`
}

// MacroDataProvider fetches macroeconomic indicators.
type MacroDataProvider interface {
	Name() string
	FetchSnapshot(ctx context.Context) (MacroDataSnapshot, error)
}

// CompositeMacroProvider aggregates multiple providers.
type CompositeMacroProvider struct {
	providers []MacroDataProvider
}

// NewCompositeMacroProvider creates a composite from given providers.
func NewCompositeMacroProvider(providers ...MacroDataProvider) *CompositeMacroProvider {
	return &CompositeMacroProvider{providers: providers}
}

// Name returns the provider name.
func (c *CompositeMacroProvider) Name() string {
	return "composite"
}

// FetchSnapshot merges snapshots from all providers (last write wins).
func (c *CompositeMacroProvider) FetchSnapshot(ctx context.Context) (MacroDataSnapshot, error) {
	var merged MacroDataSnapshot
	var errs []error
	for _, p := range c.providers {
		snap, err := p.FetchSnapshot(ctx)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if snap.US10Y.Symbol != "" {
			merged.US10Y = snap.US10Y
		}
		if snap.DXY.Symbol != "" {
			merged.DXY = snap.DXY
		}
		if snap.VIX.Symbol != "" {
			merged.VIX = snap.VIX
		}
		if snap.USD_TWD.Symbol != "" {
			merged.USD_TWD = snap.USD_TWD
		}
		if snap.Oil.Symbol != "" {
			merged.Oil = snap.Oil
		}
		if snap.Gold.Symbol != "" {
			merged.Gold = snap.Gold
		}
		if snap.JPY.Symbol != "" {
			merged.JPY = snap.JPY
		}
		if snap.ForeignInvestorNet.Symbol != "" {
			merged.ForeignInvestorNet = snap.ForeignInvestorNet
		}
		if snap.DomesticFundNet.Symbol != "" {
			merged.DomesticFundNet = snap.DomesticFundNet
		}
		if snap.DealerNet.Symbol != "" {
			merged.DealerNet = snap.DealerNet
		}
		if snap.ExportElectronics.Symbol != "" {
			merged.ExportElectronics = snap.ExportElectronics
		}
		if snap.RetailMarginBalance.Symbol != "" {
			merged.RetailMarginBalance = snap.RetailMarginBalance
		}
		if snap.RetailShortBalance.Symbol != "" {
			merged.RetailShortBalance = snap.RetailShortBalance
		}
		if snap.TSMCRevenue.Symbol != "" {
			merged.TSMCRevenue = snap.TSMCRevenue
		}
		if snap.SOXIndex.Symbol != "" {
			merged.SOXIndex = snap.SOXIndex
		}
		if snap.CoWoSUtilization.Symbol != "" {
			merged.CoWoSUtilization = snap.CoWoSUtilization
		}
		if snap.CapexGrowth.Symbol != "" {
			merged.CapexGrowth = snap.CapexGrowth
		}
		if snap.CPIYoY.Symbol != "" {
			merged.CPIYoY = snap.CPIYoY
		}
		if snap.Bdi.Symbol != "" {
			merged.Bdi = snap.Bdi
		}
		if snap.RecordedAt > merged.RecordedAt {
			merged.RecordedAt = snap.RecordedAt
		}
	}
	if merged.RecordedAt == 0 {
		merged.RecordedAt = time.Now().Unix()
	}
	if len(errs) > 0 && len(errs) == len(c.providers) {
		return merged, errors.Join(errs...)
	}
	return merged, nil
}
