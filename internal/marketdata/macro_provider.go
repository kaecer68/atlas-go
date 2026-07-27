package marketdata

import (
	"context"
	"encoding/json"
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
	US10Y                  MacroDataPoint `json:"us10y"`
	DXY                    MacroDataPoint `json:"dxy"`
	VIX                    MacroDataPoint `json:"vix"`
	USD_TWD                MacroDataPoint `json:"usd_twd"`
	Oil                    MacroDataPoint `json:"oil"`
	Gold                   MacroDataPoint `json:"gold"`
	JPY                    MacroDataPoint `json:"jpy"`
	ForeignInvestorNet     MacroDataPoint `json:"foreign_investor_net"`
	DomesticFundNet        MacroDataPoint `json:"domestic_fund_net"`
	DealerNet              MacroDataPoint `json:"dealer_net"`
	ForeignFuturesOINet    MacroDataPoint `json:"foreign_futures_oi_net"`
	GovernmentNet          MacroDataPoint `json:"government_net"`
	InsuranceNet           MacroDataPoint `json:"insurance_net"`
	InsiderNet             MacroDataPoint `json:"insider_net"`
	ExportElectronics      MacroDataPoint `json:"export_electronics"`
	RetailMarginBalance    MacroDataPoint `json:"retail_margin_balance"`
	RetailShortBalance     MacroDataPoint `json:"retail_short_balance"`
	MarginMaintenanceRatio MacroDataPoint `json:"margin_maintenance_ratio"`
	TSMCRevenue            MacroDataPoint `json:"tsmc_revenue"`
	SOXIndex               MacroDataPoint `json:"sox_index"`
	DRAMSpotPrice          MacroDataPoint `json:"dram_spot_price"`
	TaiwanSemiIndex        MacroDataPoint `json:"taiwan_semi_index"`
	CoWoSUtilization       MacroDataPoint `json:"cowos_utilization"`
	CapexGrowth            MacroDataPoint `json:"capex_growth"`
	CPIYoY                 MacroDataPoint `json:"cpi_yoy"`
	Bdi                    MacroDataPoint `json:"bdi"`
	Silver                 MacroDataPoint `json:"silver"`
	Copper                 MacroDataPoint `json:"copper"`
	TSMADR                 MacroDataPoint `json:"tsm_adr"`
	SPXIndex               MacroDataPoint `json:"spx_index"`
	NDXIndex               MacroDataPoint `json:"ndx_index"`
	DJIIndex               MacroDataPoint `json:"dji_index"`
	NVDA                   MacroDataPoint `json:"nvda"`
	AAPL                   MacroDataPoint `json:"aapl"`
	MSFT                   MacroDataPoint `json:"msft"`
	TAIEX                  MacroDataPoint `json:"taiex"`
	HistoricalVolatility   MacroDataPoint `json:"historical_volatility"`
	// P1 B3: 補漏憲章指標
	MarketVolume         MacroDataPoint `json:"market_volume"`          // 集中市場成交量（億）
	DayTradeRatio        MacroDataPoint `json:"day_trade_ratio"`        // 當沖交易佔比（%）
	FedRateExpectations  MacroDataPoint `json:"fed_rate_expectations"`  // Fed 利率預期（CME FedWatch 機率）
	SemiEquipmentImports MacroDataPoint `json:"semi_equipment_imports"` // 半導體設備進口（億美元）
	DataStatus           string         `json:"data_status,omitempty"`  // "ok" | "degraded" | "stale"
	FailedChannels       []string       `json:"failed_channels,omitempty"`
	StaleChannels        []string       `json:"stale_channels,omitempty"`
	RecordedAt           int64          `json:"recorded_at"`

	// VIXBaseline is the 252-day rolling VIX median used by
	// janus.Engine.synthesizeCompositeScore as the panic threshold.
	// Zero means "use fixed 20 fallback" (legacy behavior).
	// Per marketdata/AGENTS.md, Yahoo forbids range:"1y" so this must be
	// populated by a separate historical-data source, not yahoo_macro_provider.
	VIXBaseline float64 `json:"vix_baseline,omitempty"`
}

// MarshalJSON omits MacroDataPoint fields that have no symbol, preventing
// zero-value sentinel objects (e.g. {"symbol":"","value":0,"change_pct":0})
// from being serialized as valid readings. Backend consumers continue to use
// value-type fields; this only affects the API wire format.
func (s MacroDataSnapshot) MarshalJSON() ([]byte, error) {
	type Alias MacroDataSnapshot
	aux := &struct {
		*Alias
		US10Y                *MacroDataPoint `json:"us10y,omitempty"`
		DXY                  *MacroDataPoint `json:"dxy,omitempty"`
		VIX                  *MacroDataPoint `json:"vix,omitempty"`
		USD_TWD              *MacroDataPoint `json:"usd_twd,omitempty"`
		Oil                  *MacroDataPoint `json:"oil,omitempty"`
		Gold                 *MacroDataPoint `json:"gold,omitempty"`
		JPY                  *MacroDataPoint `json:"jpy,omitempty"`
		ForeignInvestorNet   *MacroDataPoint `json:"foreign_investor_net,omitempty"`
		DomesticFundNet      *MacroDataPoint `json:"domestic_fund_net,omitempty"`
		DealerNet            *MacroDataPoint `json:"dealer_net,omitempty"`
		ForeignFuturesOINet  *MacroDataPoint `json:"foreign_futures_oi_net,omitempty"`
		GovernmentNet        *MacroDataPoint `json:"government_net,omitempty"`
		ExportElectronics    *MacroDataPoint `json:"export_electronics,omitempty"`
		RetailMarginBalance  *MacroDataPoint `json:"retail_margin_balance,omitempty"`
		RetailShortBalance   *MacroDataPoint `json:"retail_short_balance,omitempty"`
		TSMCRevenue          *MacroDataPoint `json:"tsmc_revenue,omitempty"`
		SOXIndex             *MacroDataPoint `json:"sox_index,omitempty"`
		DRAMSpotPrice        *MacroDataPoint `json:"dram_spot_price,omitempty"`
		TaiwanSemiIndex      *MacroDataPoint `json:"taiwan_semi_index,omitempty"`
		CoWoSUtilization     *MacroDataPoint `json:"cowos_utilization,omitempty"`
		CapexGrowth          *MacroDataPoint `json:"capex_growth,omitempty"`
		CPIYoY               *MacroDataPoint `json:"cpi_yoy,omitempty"`
		Bdi                  *MacroDataPoint `json:"bdi,omitempty"`
		Silver               *MacroDataPoint `json:"silver,omitempty"`
		Copper               *MacroDataPoint `json:"copper,omitempty"`
		TSMADR               *MacroDataPoint `json:"tsm_adr,omitempty"`
		SPXIndex             *MacroDataPoint `json:"spx_index,omitempty"`
		NDXIndex             *MacroDataPoint `json:"ndx_index,omitempty"`
		DJIIndex             *MacroDataPoint `json:"dji_index,omitempty"`
		NVDA                 *MacroDataPoint `json:"nvda,omitempty"`
		AAPL                 *MacroDataPoint `json:"aapl,omitempty"`
		MSFT                 *MacroDataPoint `json:"msft,omitempty"`
		TAIEX                *MacroDataPoint `json:"taiex,omitempty"`
		HistoricalVolatility *MacroDataPoint `json:"historical_volatility,omitempty"`
		MarketVolume         *MacroDataPoint `json:"market_volume,omitempty"`
		DayTradeRatio        *MacroDataPoint `json:"day_trade_ratio,omitempty"`
		FedRateExpectations  *MacroDataPoint `json:"fed_rate_expectations,omitempty"`
		SemiEquipmentImports *MacroDataPoint `json:"semi_equipment_imports,omitempty"`
	}{
		Alias: (*Alias)(&s),
	}

	if s.US10Y.Symbol != "" {
		aux.US10Y = &s.US10Y
	}
	if s.DXY.Symbol != "" {
		aux.DXY = &s.DXY
	}
	if s.VIX.Symbol != "" {
		aux.VIX = &s.VIX
	}
	if s.USD_TWD.Symbol != "" {
		aux.USD_TWD = &s.USD_TWD
	}
	if s.Oil.Symbol != "" {
		aux.Oil = &s.Oil
	}
	if s.Gold.Symbol != "" {
		aux.Gold = &s.Gold
	}
	if s.JPY.Symbol != "" {
		aux.JPY = &s.JPY
	}
	if s.ForeignInvestorNet.Symbol != "" {
		aux.ForeignInvestorNet = &s.ForeignInvestorNet
	}
	if s.DomesticFundNet.Symbol != "" {
		aux.DomesticFundNet = &s.DomesticFundNet
	}
	if s.DealerNet.Symbol != "" {
		aux.DealerNet = &s.DealerNet
	}
	if s.ForeignFuturesOINet.Symbol != "" {
		aux.ForeignFuturesOINet = &s.ForeignFuturesOINet
	}
	if s.GovernmentNet.Symbol != "" {
		aux.GovernmentNet = &s.GovernmentNet
	}
	if s.ExportElectronics.Symbol != "" {
		aux.ExportElectronics = &s.ExportElectronics
	}
	if s.RetailMarginBalance.Symbol != "" {
		aux.RetailMarginBalance = &s.RetailMarginBalance
	}
	if s.RetailShortBalance.Symbol != "" {
		aux.RetailShortBalance = &s.RetailShortBalance
	}
	if s.TSMCRevenue.Symbol != "" {
		aux.TSMCRevenue = &s.TSMCRevenue
	}
	if s.SOXIndex.Symbol != "" {
		aux.SOXIndex = &s.SOXIndex
	}
	if s.DRAMSpotPrice.Symbol != "" {
		aux.DRAMSpotPrice = &s.DRAMSpotPrice
	}
	if s.TaiwanSemiIndex.Symbol != "" {
		aux.TaiwanSemiIndex = &s.TaiwanSemiIndex
	}
	if s.CoWoSUtilization.Symbol != "" {
		aux.CoWoSUtilization = &s.CoWoSUtilization
	}
	if s.CapexGrowth.Symbol != "" {
		aux.CapexGrowth = &s.CapexGrowth
	}
	if s.CPIYoY.Symbol != "" {
		aux.CPIYoY = &s.CPIYoY
	}
	if s.Bdi.Symbol != "" {
		aux.Bdi = &s.Bdi
	}
	if s.Silver.Symbol != "" {
		aux.Silver = &s.Silver
	}
	if s.Copper.Symbol != "" {
		aux.Copper = &s.Copper
	}
	if s.TSMADR.Symbol != "" {
		aux.TSMADR = &s.TSMADR
	}
	if s.SPXIndex.Symbol != "" {
		aux.SPXIndex = &s.SPXIndex
	}
	if s.NDXIndex.Symbol != "" {
		aux.NDXIndex = &s.NDXIndex
	}
	if s.DJIIndex.Symbol != "" {
		aux.DJIIndex = &s.DJIIndex
	}
	if s.NVDA.Symbol != "" {
		aux.NVDA = &s.NVDA
	}
	if s.AAPL.Symbol != "" {
		aux.AAPL = &s.AAPL
	}
	if s.MSFT.Symbol != "" {
		aux.MSFT = &s.MSFT
	}
	if s.TAIEX.Symbol != "" {
		aux.TAIEX = &s.TAIEX
	}
	if s.HistoricalVolatility.Symbol != "" {
		aux.HistoricalVolatility = &s.HistoricalVolatility
	}
	if s.MarketVolume.Symbol != "" {
		aux.MarketVolume = &s.MarketVolume
	}
	if s.DayTradeRatio.Symbol != "" {
		aux.DayTradeRatio = &s.DayTradeRatio
	}
	if s.FedRateExpectations.Symbol != "" {
		aux.FedRateExpectations = &s.FedRateExpectations
	}
	if s.SemiEquipmentImports.Symbol != "" {
		aux.SemiEquipmentImports = &s.SemiEquipmentImports
	}

	return json.Marshal(aux)
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
	var failedChannels []string
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
			failedChannels = append(failedChannels, p.Name())
			continue
		case <-time.After(providerTimeout):
			errs = append(errs, fmt.Errorf("%s: timed out after %v", p.Name(), providerTimeout))
			failedChannels = append(failedChannels, p.Name())
			logging.Warn(
				"marketdata", "provider_timeout",
				"provider", p.Name(),
				"timeout", providerTimeout.String(),
			)
			continue
		case r := <-ch:
			if r.err != nil {
				errs = append(errs, r.err)
				failedChannels = append(failedChannels, p.Name())
				logging.Warn(
					"marketdata", "provider_fetch_failed",
					"provider", p.Name(),
					"err", r.err.Error(),
				)
				continue
			}
			mergeSnapshot(&merged, r.snap)
			merged.StaleChannels = append(merged.StaleChannels, r.snap.StaleChannels...)
		}
	}
	if merged.RecordedAt == 0 {
		merged.RecordedAt = time.Now().Unix()
	}

	merged.FailedChannels = failedChannels
	merged.DataStatus = computeMacroDataStatus(len(c.providers), len(failedChannels))
	merged.StaleChannels = uniqueStrings(merged.StaleChannels)

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
	if src.ForeignFuturesOINet.Symbol != "" {
		dst.ForeignFuturesOINet = src.ForeignFuturesOINet
	}
	if src.GovernmentNet.Symbol != "" {
		dst.GovernmentNet = src.GovernmentNet
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
	if src.HistoricalVolatility.Symbol != "" {
		dst.HistoricalVolatility = src.HistoricalVolatility
	}
	if src.RecordedAt > dst.RecordedAt {
		dst.RecordedAt = src.RecordedAt
	}
}

func computeMacroDataStatus(totalProviders, failedCount int) string {
	if failedCount == 0 {
		return "ok"
	}
	if totalProviders > 0 && failedCount == totalProviders {
		return "stale"
	}
	return "degraded"
}

func uniqueStrings(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}
