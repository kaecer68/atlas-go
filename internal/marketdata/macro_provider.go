package marketdata

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/kaecer68/atlas-go/internal/logging"
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
	DRAMSpotPrice       MacroDataPoint `json:"dram_spot_price"`
	TaiwanSemiIndex     MacroDataPoint `json:"taiwan_semi_index"`
	CoWoSUtilization    MacroDataPoint `json:"cowos_utilization"`
	CapexGrowth         MacroDataPoint `json:"capex_growth"`
	CPIYoY              MacroDataPoint `json:"cpi_yoy"`
	Bdi                 MacroDataPoint `json:"bdi"`
	Silver              MacroDataPoint `json:"silver"`
	Copper              MacroDataPoint `json:"copper"`
	TSMADR              MacroDataPoint `json:"tsm_adr"`
	SPXIndex            MacroDataPoint `json:"spx_index"`
	NDXIndex            MacroDataPoint `json:"ndx_index"`
	DJIIndex            MacroDataPoint `json:"dji_index"`
	NVDA                MacroDataPoint `json:"nvda"`
	AAPL                MacroDataPoint `json:"aapl"`
	MSFT                MacroDataPoint `json:"msft"`
	TAIEX               MacroDataPoint `json:"taiex"`
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
// Each provider is given a 10-second timeout via a goroutine wrapper to prevent
// any single provider from hanging the entire bootstrap sequence.
func (c *CompositeMacroProvider) FetchSnapshot(ctx context.Context) (MacroDataSnapshot, error) {
	var merged MacroDataSnapshot
	var errs []error
	providerTimeout := 10 * time.Second

	for _, p := range c.providers {
		type res struct {
			snap MacroDataSnapshot
			err  error
		}
		ch := make(chan res, 1)
		go func(prov MacroDataProvider) {
			s, e := prov.FetchSnapshot(ctx)
			ch <- res{s, e}
		}(p)

		select {
		case <-ctx.Done():
			errs = append(errs, ctx.Err())
			continue
		case <-time.After(providerTimeout):
			errs = append(errs, fmt.Errorf("%s: timed out after %v", p.Name(), providerTimeout))
			logging.Warn("marketdata", "provider_timeout",
				"provider", p.Name(),
				"timeout", providerTimeout.String(),
			)
			continue
		case r := <-ch:
			if r.err != nil {
				errs = append(errs, r.err)
				continue
			}
			mergeSnapshot(&merged, r.snap)
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

// mergeSnapshot copies non-zero fields from src into dst (last-write-wins).
func mergeSnapshot(dst *MacroDataSnapshot, src MacroDataSnapshot) {
	if src.US10Y.Symbol != "" {
		dst.US10Y = src.US10Y
	}
	if src.DXY.Symbol != "" {
		dst.DXY = src.DXY
	}
	if src.VIX.Symbol != "" {
		dst.VIX = src.VIX
	}
	if src.USD_TWD.Symbol != "" {
		dst.USD_TWD = src.USD_TWD
	}
	if src.Oil.Symbol != "" {
		dst.Oil = src.Oil
	}
	if src.Gold.Symbol != "" {
		dst.Gold = src.Gold
	}
	if src.JPY.Symbol != "" {
		dst.JPY = src.JPY
	}
	if src.ForeignInvestorNet.Symbol != "" {
		dst.ForeignInvestorNet = src.ForeignInvestorNet
	}
	if src.DomesticFundNet.Symbol != "" {
		dst.DomesticFundNet = src.DomesticFundNet
	}
	if src.DealerNet.Symbol != "" {
		dst.DealerNet = src.DealerNet
	}
	if src.ExportElectronics.Symbol != "" {
		dst.ExportElectronics = src.ExportElectronics
	}
	if src.RetailMarginBalance.Symbol != "" {
		dst.RetailMarginBalance = src.RetailMarginBalance
	}
	if src.RetailShortBalance.Symbol != "" {
		dst.RetailShortBalance = src.RetailShortBalance
	}
	if src.TSMCRevenue.Symbol != "" {
		dst.TSMCRevenue = src.TSMCRevenue
	}
	if src.SOXIndex.Symbol != "" {
		dst.SOXIndex = src.SOXIndex
	}
	if src.DRAMSpotPrice.Symbol != "" {
		dst.DRAMSpotPrice = src.DRAMSpotPrice
	}
	if src.CoWoSUtilization.Symbol != "" {
		dst.CoWoSUtilization = src.CoWoSUtilization
	}
	if src.CapexGrowth.Symbol != "" {
		dst.CapexGrowth = src.CapexGrowth
	}
	if src.CPIYoY.Symbol != "" {
		dst.CPIYoY = src.CPIYoY
	}
	if src.Bdi.Symbol != "" {
		dst.Bdi = src.Bdi
	}
	if src.Silver.Symbol != "" {
		dst.Silver = src.Silver
	}
	if src.Copper.Symbol != "" {
		dst.Copper = src.Copper
	}
	if src.TSMADR.Symbol != "" {
		dst.TSMADR = src.TSMADR
	}
	if src.SPXIndex.Symbol != "" {
		dst.SPXIndex = src.SPXIndex
	}
	if src.NDXIndex.Symbol != "" {
		dst.NDXIndex = src.NDXIndex
	}
	if src.DJIIndex.Symbol != "" {
		dst.DJIIndex = src.DJIIndex
	}
	if src.NVDA.Symbol != "" {
		dst.NVDA = src.NVDA
	}
	if src.AAPL.Symbol != "" {
		dst.AAPL = src.AAPL
	}
	if src.MSFT.Symbol != "" {
		dst.MSFT = src.MSFT
	}
	if src.TAIEX.Symbol != "" {
		dst.TAIEX = src.TAIEX
	}
	if src.RecordedAt > dst.RecordedAt {
		dst.RecordedAt = src.RecordedAt
	}
}
