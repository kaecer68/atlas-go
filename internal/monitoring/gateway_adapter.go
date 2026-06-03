package monitoring

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/kaecer68/atlas-go/internal/marketdata"
	apisystem "github.com/kaecer68/atlas-go/internal/monitoring/api/system"
	"github.com/kaecer68/atlas-go/internal/narrative"
)

// macroDataGatewayAdapter implements marketdata.MacroDataProvider using DataFetcher.
// Replaces the CompositeMacroProvider + 9 individual provider pattern in NewDashboardAPI.
type macroDataGatewayAdapter struct {
	fetcher DataFetcher
}

// NewMacroDataGatewayAdapter creates a MacroDataProvider backed by the Gateway.
func NewMacroDataGatewayAdapter(fetcher DataFetcher) marketdata.MacroDataProvider {
	return &macroDataGatewayAdapter{fetcher: fetcher}
}

func (a *macroDataGatewayAdapter) Name() string {
	return "gateway_macro"
}

// FetchSnapshot fetches from all macro-related Gateway channels and merges results.
// Maps each channel to the corresponding MacroDataSnapshot field.
func (a *macroDataGatewayAdapter) FetchSnapshot(ctx context.Context) (marketdata.MacroDataSnapshot, error) {
	type channelMapping struct {
		channelID string
		apply     func(snap *marketdata.MacroDataSnapshot, data []byte)
	}

	channels := []channelMapping{
		{channelID: "us_yahoo", apply: a.applyUSYahoo},
		{channelID: "jpy_yahoo", apply: a.applyJPYFrankfurter},
		{channelID: "exchange_rate", apply: a.applyExchangeRate},
		{channelID: "sox_index", apply: a.applySOXIndex},
		{channelID: "twse_capital_flow", apply: a.applyCapitalFlow},
		{channelID: "twse_margin", apply: a.applyMargin},
		{channelID: "export_statistics", apply: a.applyExport},
		{channelID: "tsmc_revenue", apply: a.applyTSMCRevenue},
		{channelID: "sector_data", apply: a.applySectorData},
		{channelID: "bdi", apply: a.applyBDI},
		{channelID: "dram_spot_price", apply: a.applyDRAMSpotPrice},
		{channelID: "twse_sector_index", apply: a.applyTWSESectorIndex},
	}

	var merged marketdata.MacroDataSnapshot
	var errs []error

	for _, ch := range channels {
		data, err := a.fetcher(ctx, ch.channelID)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", ch.channelID, err))
			continue
		}
		ch.apply(&merged, data)
	}

	if merged.RecordedAt == 0 {
		merged.RecordedAt = time.Now().Unix()
	}

	// Only return error if ALL channels failed
	if len(errs) > 0 && len(errs) == len(channels) {
		return merged, errors.Join(errs...)
	}
	return merged, nil
}

func (a *macroDataGatewayAdapter) applyUSYahoo(snap *marketdata.MacroDataSnapshot, data []byte) {
	var s marketdata.MacroDataSnapshot
	if err := json.Unmarshal(data, &s); err != nil {
		return
	}
	if s.US10Y.Symbol != "" {
		snap.US10Y = s.US10Y
	}
	if s.DXY.Symbol != "" {
		snap.DXY = s.DXY
	}
	if s.VIX.Symbol != "" {
		snap.VIX = s.VIX
	}
	if s.Oil.Symbol != "" {
		snap.Oil = s.Oil
	}
	if s.Gold.Symbol != "" {
		snap.Gold = s.Gold
	}
	if s.JPY.Symbol != "" {
		snap.JPY = s.JPY
	}
	if s.USD_TWD.Symbol != "" {
		snap.USD_TWD = s.USD_TWD
	}
	if s.Bdi.Symbol != "" {
		snap.Bdi = s.Bdi
	}
	if s.Silver.Symbol != "" {
		snap.Silver = s.Silver
	}
	if s.Copper.Symbol != "" {
		snap.Copper = s.Copper
	}
	if s.RecordedAt > snap.RecordedAt {
		snap.RecordedAt = s.RecordedAt
	}
}

func (a *macroDataGatewayAdapter) applyJPYFrankfurter(snap *marketdata.MacroDataSnapshot, data []byte) {
	// Only use Frankfurter as fallback when Yahoo didn't provide JPY data.
	if snap.JPY.Symbol != "" {
		return
	}
	var jpy marketdata.MacroDataPoint
	if err := json.Unmarshal(data, &jpy); err != nil {
		return
	}
	if jpy.Symbol != "" {
		snap.JPY = jpy
	}
}

func (a *macroDataGatewayAdapter) applyExchangeRate(snap *marketdata.MacroDataSnapshot, data []byte) {
	// Only use ExchangeRate-API as fallback when Yahoo didn't provide USD/TWD data.
	if snap.USD_TWD.Symbol != "" {
		return
	}
	var s marketdata.MacroDataSnapshot
	if err := json.Unmarshal(data, &s); err != nil {
		return
	}
	if s.USD_TWD.Symbol != "" {
		snap.USD_TWD = s.USD_TWD
	}
}

func (a *macroDataGatewayAdapter) applySOXIndex(snap *marketdata.MacroDataSnapshot, data []byte) {
	var s marketdata.MacroDataSnapshot
	if err := json.Unmarshal(data, &s); err != nil {
		return
	}
	if s.SOXIndex.Symbol != "" {
		snap.SOXIndex = s.SOXIndex
	}
}

func (a *macroDataGatewayAdapter) applyCapitalFlow(snap *marketdata.MacroDataSnapshot, data []byte) {
	var s marketdata.MacroDataSnapshot
	if err := json.Unmarshal(data, &s); err != nil {
		return
	}
	if s.ForeignInvestorNet.Symbol != "" {
		snap.ForeignInvestorNet = s.ForeignInvestorNet
	}
	if s.DomesticFundNet.Symbol != "" {
		snap.DomesticFundNet = s.DomesticFundNet
	}
	if s.DealerNet.Symbol != "" {
		snap.DealerNet = s.DealerNet
	}
}

func (a *macroDataGatewayAdapter) applyMargin(snap *marketdata.MacroDataSnapshot, data []byte) {
	var s marketdata.MacroDataSnapshot
	if err := json.Unmarshal(data, &s); err != nil {
		return
	}
	if s.RetailMarginBalance.Symbol != "" {
		snap.RetailMarginBalance = s.RetailMarginBalance
	}
	if s.RetailShortBalance.Symbol != "" {
		snap.RetailShortBalance = s.RetailShortBalance
	}
}

func (a *macroDataGatewayAdapter) applyExport(snap *marketdata.MacroDataSnapshot, data []byte) {
	var s marketdata.MacroDataSnapshot
	if err := json.Unmarshal(data, &s); err != nil {
		return
	}
	if s.ExportElectronics.Symbol != "" {
		snap.ExportElectronics = s.ExportElectronics
	}
}

func (a *macroDataGatewayAdapter) applyTSMCRevenue(snap *marketdata.MacroDataSnapshot, data []byte) {
	// The TSMC revenue adapter marshals a single MacroDataPoint (not a full
	// MacroDataSnapshot), so we must unmarshal into MacroDataPoint.
	var point marketdata.MacroDataPoint
	if err := json.Unmarshal(data, &point); err != nil {
		return
	}
	if point.Symbol != "" {
		snap.TSMCRevenue = point
	}
}

func (a *macroDataGatewayAdapter) applySectorData(snap *marketdata.MacroDataSnapshot, data []byte) {
	// Sector data doesn't directly map to MacroDataSnapshot fields.
	// It's used separately by the industry service.
}

func (a *macroDataGatewayAdapter) applyDRAMSpotPrice(snap *marketdata.MacroDataSnapshot, data []byte) {
	var s marketdata.MacroDataSnapshot
	if err := json.Unmarshal(data, &s); err != nil {
		return
	}
	if s.DRAMSpotPrice.Symbol != "" {
		snap.DRAMSpotPrice = s.DRAMSpotPrice
	}
}

func (a *macroDataGatewayAdapter) applyTWSESectorIndex(snap *marketdata.MacroDataSnapshot, data []byte) {
	var point marketdata.MacroDataPoint
	if err := json.Unmarshal(data, &point); err != nil {
		return
	}
	if point.Symbol != "" {
		snap.TaiwanSemiIndex = point
	}
}

func (a *macroDataGatewayAdapter) applyBDI(snap *marketdata.MacroDataSnapshot, data []byte) {
	var s marketdata.MacroDataSnapshot
	if err := json.Unmarshal(data, &s); err != nil {
		return
	}
	if s.Bdi.Symbol != "" {
		snap.Bdi = s.Bdi
	}
}

// ---------------------------------------------------------------------------
// Geopolitical Gateway Adapter
// ---------------------------------------------------------------------------

// geopoliticalGatewayAdapter implements narrative.GeopoliticalRiskProvider using DataFetcher.
type geopoliticalGatewayAdapter struct {
	fetcher DataFetcher
}

// NewGeopoliticalGatewayAdapter creates a GeopoliticalRiskProvider backed by the Gateway.
func NewGeopoliticalGatewayAdapter(fetcher DataFetcher) narrative.GeopoliticalRiskProvider {
	return &geopoliticalGatewayAdapter{fetcher: fetcher}
}

func (a *geopoliticalGatewayAdapter) Name() string {
	return "gateway_geopolitical"
}

// FetchScore fetches geopolitical risk data from the Gateway.
func (a *geopoliticalGatewayAdapter) FetchScore(ctx context.Context) (narrative.GeopoliticalRiskScore, error) {
	// Use a short timeout — store fallback preferred over slow live fetch.
	fastCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	data, err := a.fetcher(fastCtx, "geopolitical")
	if err != nil {
		return narrative.GeopoliticalRiskScore{}, err
	}
	// The Gateway geopolitical channel wraps scores in {global, taiwan} envelope.
	var wrapper struct {
		Global *narrative.GeopoliticalRiskScore `json:"global"`
		Taiwan *narrative.GeopoliticalRiskScore `json:"taiwan"`
	}
	if err := json.Unmarshal(data, &wrapper); err != nil {
		return narrative.GeopoliticalRiskScore{}, fmt.Errorf("geo unmarshal: %w", err)
	}
	if wrapper.Global == nil || wrapper.Global.Intensity == 0 {
		return narrative.GeopoliticalRiskScore{}, fmt.Errorf("geopolitical score unavailable (global=nil or intensity=0)")
	}
	return *wrapper.Global, nil
}

// ---------------------------------------------------------------------------
// Taiwan Geopolitical Gateway Adapter
// ---------------------------------------------------------------------------

// taiwanGeopoliticalGatewayAdapter implements narrative.GeopoliticalRiskProvider
// using DataFetcher for the geopolitical_taiwan channel.
type taiwanGeopoliticalGatewayAdapter struct {
	fetcher DataFetcher
}

// NewTaiwanGeopoliticalGatewayAdapter creates a GeopoliticalRiskProvider backed by the Gateway.
func NewTaiwanGeopoliticalGatewayAdapter(fetcher DataFetcher) narrative.GeopoliticalRiskProvider {
	return &taiwanGeopoliticalGatewayAdapter{fetcher: fetcher}
}

func (a *taiwanGeopoliticalGatewayAdapter) Name() string {
	return "gateway_taiwan_geopolitical"
}

// FetchScore fetches Taiwan-specific geopolitical risk data from the Gateway.
func (a *taiwanGeopoliticalGatewayAdapter) FetchScore(ctx context.Context) (narrative.GeopoliticalRiskScore, error) {
	data, err := a.fetcher(ctx, "geopolitical_taiwan")
	if err != nil {
		return narrative.GeopoliticalRiskScore{}, err
	}
	var score narrative.GeopoliticalRiskScore
	if err := json.Unmarshal(data, &score); err != nil {
		return narrative.GeopoliticalRiskScore{}, fmt.Errorf("taiwan geo unmarshal: %w", err)
	}
	return score, nil
}

// NewDayTradingFetcher creates a day trading fetcher backed by the Gateway.
// Returns a function that fetches DayTradingStats from the day_trading channel.
func NewDayTradingFetcher(fetcher DataFetcher) func(ctx context.Context) (*marketdata.DayTradingStats, error) {
	return func(ctx context.Context) (*marketdata.DayTradingStats, error) {
		data, err := fetcher(ctx, "day_trading")
		if err != nil {
			return nil, err
		}
		var stats marketdata.DayTradingStats
		if err := json.Unmarshal(data, &stats); err != nil {
			return nil, fmt.Errorf("day trading unmarshal: %w", err)
		}
		return &stats, nil
	}
}

// Ensure adapters implement their interfaces at compile time.
var (
	_ marketdata.MacroDataProvider       = (*macroDataGatewayAdapter)(nil)
	_ narrative.GeopoliticalRiskProvider = (*geopoliticalGatewayAdapter)(nil)
	_ narrative.GeopoliticalRiskProvider = (*taiwanGeopoliticalGatewayAdapter)(nil)
)

// NewTaifexFetcher creates a fetcher for TAIFEX PCR and retail futures OI data.
func NewTaifexFetcher(fetcher DataFetcher) apisystem.TaifexFetcher {
	return func(ctx context.Context) (*marketdata.PCRStats, *marketdata.RetailFuturesOI, error) {
		data, err := fetcher(ctx, "taifex-daily")
		if err != nil {
			return nil, nil, err
		}
		var result struct {
			PCR             *marketdata.PCRStats        `json:"pcr"`
			RetailFuturesOI *marketdata.RetailFuturesOI `json:"retail_futures_oi"`
		}
		if err := json.Unmarshal(data, &result); err != nil {
			return nil, nil, fmt.Errorf("taifex unmarshal: %w", err)
		}
		return result.PCR, result.RetailFuturesOI, nil
	}
}

// NewOddLotFetcher creates a fetcher for TWSE odd-lot trading data.
func NewOddLotFetcher(fetcher DataFetcher) apisystem.OddLotFetcher {
	return func(ctx context.Context) (*marketdata.OddLotStats, error) {
		data, err := fetcher(ctx, "twse_oddlot")
		if err != nil {
			return nil, err
		}
		var result marketdata.OddLotStats
		if err := json.Unmarshal(data, &result); err != nil {
			return nil, fmt.Errorf("oddlot unmarshal: %w", err)
		}
		return &result, nil
	}
}

// NewETFFetcher creates a fetcher for TWSE ETF subscription data.
func NewETFFetcher(fetcher DataFetcher) apisystem.ETFFetcher {
	return func(ctx context.Context) (*marketdata.ETFStats, error) {
		data, err := fetcher(ctx, "twse-etf")
		if err != nil {
			return nil, err
		}
		var result marketdata.ETFStats
		if err := json.Unmarshal(data, &result); err != nil {
			return nil, fmt.Errorf("etf unmarshal: %w", err)
		}
		return &result, nil
	}
}

// newGeopoliticalRiskFetcher creates a GeopoliticalRiskFetcher that normalizes
// the narrative GeopoliticalRiskProvider output to [0, 1].
// Prefers Taiwan-specific provider; falls back to global; returns 0 on any error.
func newGeopoliticalRiskFetcher(global, taiwan narrative.GeopoliticalRiskProvider) apisystem.GeopoliticalRiskFetcher {
	return func(ctx context.Context) float64 {
		if taiwan != nil {
			if score, err := taiwan.FetchScore(ctx); err == nil && score.Intensity > 0 {
				return score.Intensity / 100.0
			}
		}
		if global != nil {
			if score, err := global.FetchScore(ctx); err == nil && score.Intensity > 0 {
				return score.Intensity / 100.0
			}
		}
		return 0
	}
}
