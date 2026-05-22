package monitoring

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/kaecer68/atlas-go/internal/marketdata"
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
		{channelID: "jpy_yahoo", apply: a.applyJPYYahoo},
		{channelID: "exchange_rate", apply: a.applyExchangeRate},
		{channelID: "sox_index", apply: a.applySOXIndex},
		{channelID: "twse_capital_flow", apply: a.applyCapitalFlow},
		{channelID: "twse_margin", apply: a.applyMargin},
		{channelID: "export_statistics", apply: a.applyExport},
		{channelID: "tsmc_revenue", apply: a.applyTSMCRevenue},
		{channelID: "sector_data", apply: a.applySectorData},
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
	if s.RecordedAt > snap.RecordedAt {
		snap.RecordedAt = s.RecordedAt
	}
}

func (a *macroDataGatewayAdapter) applyJPYYahoo(snap *marketdata.MacroDataSnapshot, data []byte) {
	var s marketdata.MacroDataSnapshot
	if err := json.Unmarshal(data, &s); err != nil {
		return
	}
	if s.JPY.Symbol != "" {
		snap.JPY = s.JPY
	}
	if s.BDI.Symbol != "" {
		snap.BDI = s.BDI
	}
}

func (a *macroDataGatewayAdapter) applyExchangeRate(snap *marketdata.MacroDataSnapshot, data []byte) {
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
	var s marketdata.MacroDataSnapshot
	if err := json.Unmarshal(data, &s); err != nil {
		return
	}
	if s.TSMCRevenue.Symbol != "" {
		snap.TSMCRevenue = s.TSMCRevenue
	}
	if s.CoWoSUtilization.Symbol != "" {
		snap.CoWoSUtilization = s.CoWoSUtilization
	}
	if s.CapexGrowth.Symbol != "" {
		snap.CapexGrowth = s.CapexGrowth
	}
}

func (a *macroDataGatewayAdapter) applySectorData(snap *marketdata.MacroDataSnapshot, data []byte) {
	// Sector data doesn't directly map to MacroDataSnapshot fields.
	// It's used separately by the industry service.
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
	data, err := a.fetcher(ctx, "geopolitical")
	if err != nil {
		return narrative.GeopoliticalRiskScore{}, err
	}
	var score narrative.GeopoliticalRiskScore
	if err := json.Unmarshal(data, &score); err != nil {
		return narrative.GeopoliticalRiskScore{}, fmt.Errorf("geo unmarshal: %w", err)
	}
	return score, nil
}

// Ensure adapters implement their interfaces at compile time.
var _ marketdata.MacroDataProvider = (*macroDataGatewayAdapter)(nil)
var _ narrative.GeopoliticalRiskProvider = (*geopoliticalGatewayAdapter)(nil)
