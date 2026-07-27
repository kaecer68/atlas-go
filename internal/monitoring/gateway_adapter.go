package monitoring

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/kaecer68/atlas-go/internal/marketdata"
	apisystem "github.com/kaecer68/atlas-go/internal/monitoring/api/system"
	"github.com/kaecer68/atlas-go/internal/narrative/geopolitical"
)

// macroDataGatewayAdapter implements marketdata.MacroDataProvider using DataFetcher.
// Replaces the CompositeMacroProvider + 9 individual provider pattern in NewDashboardAPI.
type macroDataGatewayAdapter struct {
	fetcher    DataFetcher
	mu         sync.Mutex
	lastErrors map[string]string // channelID → last error message (Layer 2 of data-visibility)
}

// NewMacroDataGatewayAdapter creates a MacroDataProvider backed by the Gateway.
func NewMacroDataGatewayAdapter(fetcher DataFetcher) marketdata.MacroDataProvider {
	return &macroDataGatewayAdapter{
		fetcher:    fetcher,
		lastErrors: make(map[string]string),
	}
}

// ChannelErrors returns a snapshot of the per-channel error map populated
// by the most recent FetchSnapshot call. The map is a copy, so callers
// can mutate it freely. Returns nil if no fetch has run yet.
//
// This is Layer 2 of the 4-layer data-visibility safeguard — it lets the
// service layer surface channel failures to the API response instead of
// silently producing all-zero MacroDataSnapshot fields.
func (a *macroDataGatewayAdapter) ChannelErrors() map[string]string {
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.lastErrors) == 0 {
		return nil
	}
	cp := make(map[string]string, len(a.lastErrors))
	for k, v := range a.lastErrors {
		cp[k] = v
	}
	return cp
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
		{channelID: "frankfurter_fx", apply: a.applyFrankfurterFX},
		{channelID: "exchange_rate", apply: a.applyExchangeRate},
		{channelID: "sox_index", apply: a.applySOXIndex},
		{channelID: "twse_capital_flow", apply: a.applyCapitalFlow},
		{channelID: "twse_etf", apply: a.applyETF},
		{channelID: "export_statistics", apply: a.applyExport},
		{channelID: "tsmc_revenue", apply: a.applyTSMCRevenue},
		{channelID: "sector_data", apply: a.applySectorData},
		{channelID: "bdi", apply: a.applyBDI},
		{channelID: "dram_spot_price", apply: a.applyDRAMSpotPrice},
		{channelID: "twse_sector_index", apply: a.applyTWSESectorIndex},
		{channelID: "us_spx", apply: a.applyUSSPX},
		{channelID: "us_ndx", apply: a.applyUSNDX},
		{channelID: "us_dji", apply: a.applyUSDJI},
		{channelID: "us_nvda", apply: a.applyUSNVDA},
		{channelID: "us_aapl", apply: a.applyUSAAPL},
		{channelID: "us_msft", apply: a.applyUSMSFT},
		{channelID: "tsm_adr", apply: a.applyTSMADR},
		{channelID: "taiex_index", apply: a.applyTAIEX},
		{channelID: "tw_vol", apply: a.applyTWVol},
		{channelID: "taifex_institutional", apply: a.applyTaifexInstitutional},
		{channelID: "government_flow", apply: a.applyGovernmentFlow},
		{channelID: "twse_insider", apply: a.applyInsiderTrading},
		{channelID: "market_volume", apply: a.applyMarketVolume},
		{channelID: "day_trade_ratio", apply: a.applyDayTradeRatio},
	}

	var (
		merged marketdata.MacroDataSnapshot
		wg     sync.WaitGroup
	)

	for _, ch := range channels {
		wg.Add(1)
		go func(ch channelMapping) {
			defer wg.Done()
			data, meta, err := a.fetcher(ctx, ch.channelID)
			if err != nil {
				a.mu.Lock()
				a.lastErrors[ch.channelID] = err.Error()
				a.mu.Unlock()
				return
			}
			a.mu.Lock()
			if meta.Stale {
				msg := "stale: gateway returned cached data (CB-open or fallback)"
				if meta.LastError != "" {
					msg = "stale: " + meta.LastError
				}
				a.lastErrors[ch.channelID] = msg
			} else {
				delete(a.lastErrors, ch.channelID)
			}
			ch.apply(&merged, data)
			a.mu.Unlock()
		}(ch)
	}
	wg.Wait()

	if merged.RecordedAt == 0 {
		merged.RecordedAt = time.Now().Unix()
	}

	a.mu.Lock()
	// Count "real" errors only (exclude stale: channels, which still
	// returned usable cached data). Without this filter, a CB-open
	// across all channels would be misclassified as "all failed"
	// even though stale data is still populated.
	realErrCount := 0
	errMsgs := make([]string, 0, len(a.lastErrors))
	for ch, e := range a.lastErrors {
		if len(e) >= 6 && e[:6] == "stale:" {
			continue
		}
		realErrCount++
		errMsgs = append(errMsgs, fmt.Sprintf("%s: %s", ch, e))
	}
	allFailed := realErrCount > 0 && realErrCount == len(channels)
	a.mu.Unlock()

	if allFailed {
		return merged, fmt.Errorf("all %d channels failed: %s", len(channels), strings.Join(errMsgs, "; "))
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

func (a *macroDataGatewayAdapter) applyFrankfurterFX(snap *marketdata.MacroDataSnapshot, data []byte) {
	// Frankfurter is the primary (and now sole) JPY source.
	// us_yahoo no longer fetches JPY=X, so there's no fallback logic needed.
	// The defensive check below handles the edge case where another channel
	// provided JPY data first.
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
	// The gateway channel adapter normally returns a full MacroDataSnapshot.
	var s marketdata.MacroDataSnapshot
	if err := json.Unmarshal(data, &s); err == nil {
		if s.ForeignInvestorNet.Symbol != "" {
			snap.ForeignInvestorNet = s.ForeignInvestorNet
		}
		if s.ForeignDealerNet.Symbol != "" {
			snap.ForeignDealerNet = s.ForeignDealerNet
		}
		if s.DomesticFundNet.Symbol != "" {
			snap.DomesticFundNet = s.DomesticFundNet
		}
		if s.DealerNet.Symbol != "" {
			snap.DealerNet = s.DealerNet
		}
		if s.DealerSelfNet.Symbol != "" {
			snap.DealerSelfNet = s.DealerSelfNet
		}
		if s.DealerHedgingNet.Symbol != "" {
			snap.DealerHedgingNet = s.DealerHedgingNet
		}
		return
	}

	// Fallback: older/persisted files may contain the raw TWSECapitalFlow shape.
	var flow marketdata.TWSECapitalFlow
	if err := json.Unmarshal(data, &flow); err != nil {
		return
	}
	if flow.Date == "" {
		return
	}
	flowTime, _ := time.Parse("20060102", flow.Date)
	ts := flowTime.Unix()
	if flow.ForeignInvestorNet != 0 || flow.DomesticFundNet != 0 || flow.DealerNet != 0 {
		snap.ForeignInvestorNet = marketdata.MacroDataPoint{Symbol: "TAIWAN_FOREIGN", Value: flow.ForeignInvestorNet, Timestamp: ts}
		snap.ForeignDealerNet = marketdata.MacroDataPoint{Symbol: "TAIWAN_FOREIGN_DEALER", Value: flow.ForeignDealerNet, Timestamp: ts}
		snap.DomesticFundNet = marketdata.MacroDataPoint{Symbol: "TAIWAN_DOMESTIC", Value: flow.DomesticFundNet, Timestamp: ts}
		snap.DealerNet = marketdata.MacroDataPoint{Symbol: "TAIWAN_DEALER", Value: flow.DealerNet, Timestamp: ts}
		snap.DealerSelfNet = marketdata.MacroDataPoint{Symbol: "TAIWAN_DEALER_SELF", Value: flow.DealerSelfNet, Timestamp: ts}
		snap.DealerHedgingNet = marketdata.MacroDataPoint{Symbol: "TAIWAN_DEALER_HEDGING", Value: flow.DealerHedgingNet, Timestamp: ts}
	}
}

func (a *macroDataGatewayAdapter) applyETF(snap *marketdata.MacroDataSnapshot, data []byte) {
	var stats marketdata.ETFStats
	if err := json.Unmarshal(data, &stats); err != nil {
		return
	}
	if stats.Date == "" {
		return
	}
	etfTime, _ := time.Parse("20060102", stats.Date)
	ts := etfTime.Unix()
	snap.ETFNetSubscription = marketdata.MacroDataPoint{
		Symbol:    "TAIWAN_ETF",
		Value:     float64(stats.NetSubscription),
		Timestamp: ts,
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

func (a *macroDataGatewayAdapter) applyUSSPX(snap *marketdata.MacroDataSnapshot, data []byte) {
	var s marketdata.MacroDataSnapshot
	if err := json.Unmarshal(data, &s); err != nil {
		return
	}
	if s.SPXIndex.Symbol != "" {
		snap.SPXIndex = s.SPXIndex
	}
}

func (a *macroDataGatewayAdapter) applyUSNDX(snap *marketdata.MacroDataSnapshot, data []byte) {
	var s marketdata.MacroDataSnapshot
	if err := json.Unmarshal(data, &s); err != nil {
		return
	}
	if s.NDXIndex.Symbol != "" {
		snap.NDXIndex = s.NDXIndex
	}
}

func (a *macroDataGatewayAdapter) applyUSDJI(snap *marketdata.MacroDataSnapshot, data []byte) {
	var s marketdata.MacroDataSnapshot
	if err := json.Unmarshal(data, &s); err != nil {
		return
	}
	if s.DJIIndex.Symbol != "" {
		snap.DJIIndex = s.DJIIndex
	}
}

func (a *macroDataGatewayAdapter) applyUSNVDA(snap *marketdata.MacroDataSnapshot, data []byte) {
	var s marketdata.MacroDataSnapshot
	if err := json.Unmarshal(data, &s); err != nil {
		return
	}
	if s.NVDA.Symbol != "" {
		snap.NVDA = s.NVDA
	}
}

func (a *macroDataGatewayAdapter) applyUSAAPL(snap *marketdata.MacroDataSnapshot, data []byte) {
	var s marketdata.MacroDataSnapshot
	if err := json.Unmarshal(data, &s); err != nil {
		return
	}
	if s.AAPL.Symbol != "" {
		snap.AAPL = s.AAPL
	}
}

func (a *macroDataGatewayAdapter) applyUSMSFT(snap *marketdata.MacroDataSnapshot, data []byte) {
	var s marketdata.MacroDataSnapshot
	if err := json.Unmarshal(data, &s); err != nil {
		return
	}
	if s.MSFT.Symbol != "" {
		snap.MSFT = s.MSFT
	}
}

func (a *macroDataGatewayAdapter) applyTSMADR(snap *marketdata.MacroDataSnapshot, data []byte) {
	var s marketdata.MacroDataSnapshot
	if err := json.Unmarshal(data, &s); err != nil {
		return
	}
	if s.TSMADR.Symbol != "" {
		snap.TSMADR = s.TSMADR
	}
}

func (a *macroDataGatewayAdapter) applyTAIEX(snap *marketdata.MacroDataSnapshot, data []byte) {
	var s marketdata.MacroDataSnapshot
	if err := json.Unmarshal(data, &s); err != nil {
		return
	}
	if s.TAIEX.Symbol != "" {
		snap.TAIEX = s.TAIEX
	}
}

func (a *macroDataGatewayAdapter) applyTWVol(snap *marketdata.MacroDataSnapshot, data []byte) {
	var s marketdata.MacroDataSnapshot
	if err := json.Unmarshal(data, &s); err != nil {
		return
	}
	if s.HistoricalVolatility.Symbol != "" {
		snap.HistoricalVolatility = s.HistoricalVolatility
	}
}

func (a *macroDataGatewayAdapter) applyTaifexInstitutional(snap *marketdata.MacroDataSnapshot, data []byte) {
	var inst marketdata.InstitutionalFuturesDaily
	if err := json.Unmarshal(data, &inst); err != nil {
		return
	}
	if inst.Date == "" {
		return
	}
	// Best-effort: foreign OI net is the leading indicator for foreign
	// direction. ChangePct is left zero (we only have one observation; the
	// ForceExtractor rolling window rebuilds Z-scores from daily history
	// persisted in capital_flow state).
	ts, _ := time.Parse("20060102", inst.Date)
	snap.ForeignFuturesOINet = marketdata.MacroDataPoint{
		Symbol:    "TX_FOREIGN_OI_NET",
		Value:     float64(inst.Foreign.OINet),
		Timestamp: ts.Unix(),
	}
}

func (a *macroDataGatewayAdapter) applyGovernmentFlow(snap *marketdata.MacroDataSnapshot, data []byte) {
	var payload struct {
		Available        bool                              `json:"available"`
		Reading          *marketdata.GovernmentFlowReading `json:"reading"`
		InsuranceReading *marketdata.GovernmentFlowReading `json:"insurance_reading"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return
	}
	if !payload.Available || payload.Reading == nil || payload.Reading.Date == "" {
		return
	}
	ts, _ := time.Parse("20060102", payload.Reading.Date)
	v := float64(payload.Reading.TotalNet) / 1e8
	snap.GovernmentNet = marketdata.MacroDataPoint{
		Symbol:    "GOV_FLOW_NET",
		Value:     v,
		Timestamp: ts.Unix(),
	}
	// Best-effort: insurance flow.
	if payload.InsuranceReading != nil && payload.InsuranceReading.Date != "" {
		insTs, _ := time.Parse("20060102", payload.InsuranceReading.Date)
		insV := float64(payload.InsuranceReading.TotalNet) / 1e8
		snap.InsuranceNet = marketdata.MacroDataPoint{
			Symbol:    "INS_FLOW_NET",
			Value:     insV,
			Timestamp: insTs.Unix(),
		}
	}
}

func (a *macroDataGatewayAdapter) applyInsiderTrading(snap *marketdata.MacroDataSnapshot, data []byte) {
	var agg marketdata.InsiderAggregate
	if err := json.Unmarshal(data, &agg); err != nil {
		return
	}
	if agg.Date == "" {
		return
	}
	ts, _ := time.Parse("20060102", agg.Date)
	// TotalDeclared scales total declared transfer shares for Z-score range.
	v := float64(agg.TotalDeclared) / 1e5
	snap.InsiderNet = marketdata.MacroDataPoint{
		Symbol:    "INSIDER_DECLARED",
		Value:     v,
		Timestamp: ts.Unix(),
	}
}

// ---------------------------------------------------------------------------
// Geopolitical Gateway Adapter
// ---------------------------------------------------------------------------

// geopoliticalGatewayAdapter implements geopolitical.GeopoliticalRiskProvider using DataFetcher.
type geopoliticalGatewayAdapter struct {
	fetcher DataFetcher
}

// NewGeopoliticalGatewayAdapter creates a GeopoliticalRiskProvider backed by the Gateway.
func NewGeopoliticalGatewayAdapter(fetcher DataFetcher) geopolitical.GeopoliticalRiskProvider {
	return &geopoliticalGatewayAdapter{fetcher: fetcher}
}

func (a *geopoliticalGatewayAdapter) Name() string {
	return "gateway_geopolitical"
}

// FetchScore fetches geopolitical risk data from the Gateway.
func (a *geopoliticalGatewayAdapter) FetchScore(ctx context.Context) (geopolitical.GeopoliticalRiskScore, error) {
	// Use a short timeout — store fallback preferred over slow live fetch.
	fastCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	data, _, err := a.fetcher(fastCtx, "geopolitical")
	if err != nil {
		return geopolitical.GeopoliticalRiskScore{}, err
	}
	// The Gateway geopolitical channel wraps scores in {global, taiwan} envelope.
	var wrapper struct {
		Global *geopolitical.GeopoliticalRiskScore `json:"global"`
		Taiwan *geopolitical.GeopoliticalRiskScore `json:"taiwan"`
	}
	if err := json.Unmarshal(data, &wrapper); err != nil {
		return geopolitical.GeopoliticalRiskScore{}, fmt.Errorf("geo unmarshal: %w", err)
	}
	if wrapper.Global == nil || wrapper.Global.Intensity == 0 {
		return geopolitical.GeopoliticalRiskScore{}, fmt.Errorf("geopolitical score unavailable (global=nil or intensity=0)")
	}
	return *wrapper.Global, nil
}

// ---------------------------------------------------------------------------
// Taiwan Geopolitical Gateway Adapter
// ---------------------------------------------------------------------------

// taiwanGeopoliticalGatewayAdapter implements geopolitical.GeopoliticalRiskProvider
// using DataFetcher for the geopolitical_taiwan channel.
type taiwanGeopoliticalGatewayAdapter struct {
	fetcher DataFetcher
}

// NewTaiwanGeopoliticalGatewayAdapter creates a GeopoliticalRiskProvider backed by the Gateway.
func NewTaiwanGeopoliticalGatewayAdapter(fetcher DataFetcher) geopolitical.GeopoliticalRiskProvider {
	return &taiwanGeopoliticalGatewayAdapter{fetcher: fetcher}
}

func (a *taiwanGeopoliticalGatewayAdapter) Name() string {
	return "gateway_taiwan_geopolitical"
}

// FetchScore fetches Taiwan-specific geopolitical risk data from the Gateway.
func (a *taiwanGeopoliticalGatewayAdapter) FetchScore(ctx context.Context) (geopolitical.GeopoliticalRiskScore, error) {
	data, _, err := a.fetcher(ctx, "geopolitical_taiwan")
	if err != nil {
		return geopolitical.GeopoliticalRiskScore{}, err
	}
	var score geopolitical.GeopoliticalRiskScore
	if err := json.Unmarshal(data, &score); err != nil {
		return geopolitical.GeopoliticalRiskScore{}, fmt.Errorf("taiwan geo unmarshal: %w", err)
	}
	return score, nil
}

// NewDayTradingFetcher creates a day trading fetcher backed by the Gateway.
// Returns a function that fetches DayTradingStats from the day_trading channel.
func NewDayTradingFetcher(fetcher DataFetcher) func(ctx context.Context) (*marketdata.DayTradingStats, error) {
	return func(ctx context.Context) (*marketdata.DayTradingStats, error) {
		data, _, err := fetcher(ctx, "day_trading")
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
	_ marketdata.MacroDataProvider          = (*macroDataGatewayAdapter)(nil)
	_ geopolitical.GeopoliticalRiskProvider = (*geopoliticalGatewayAdapter)(nil)
	_ geopolitical.GeopoliticalRiskProvider = (*taiwanGeopoliticalGatewayAdapter)(nil)
)

// NewTaifexFetcher creates a fetcher for TAIFEX PCR and retail futures OI data.
func NewTaifexFetcher(fetcher DataFetcher) apisystem.TaifexFetcher {
	return func(ctx context.Context) (*marketdata.PCRStats, *marketdata.RetailFuturesOI, error) {
		data, _, err := fetcher(ctx, "taifex_daily")
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
		data, _, err := fetcher(ctx, "twse_oddlot")
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
		data, _, err := fetcher(ctx, "twse_etf")
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
func newGeopoliticalRiskFetcher(global, taiwan geopolitical.GeopoliticalRiskProvider) apisystem.GeopoliticalRiskFetcher {
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

// ─── P1 B3: 補漏憲章指標 apply 函數 ───

// applyMarketVolume extracts 集中市場成交量 from TWSE market stats data.
// Data source: TWSE OpenAPI (待接入: /opendata/t187ap03_L 大盤統計資訊).
// Field: MarketVolume (億).
func (a *macroDataGatewayAdapter) applyMarketVolume(snap *marketdata.MacroDataSnapshot, data []byte) {
	var payload struct {
		MarketVolume float64 `json:"market_volume"`
		Date         string  `json:"date"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return
	}
	if payload.Date == "" || payload.MarketVolume <= 0 {
		return
	}
	ts, _ := time.Parse("20060102", payload.Date)
	snap.MarketVolume = marketdata.MacroDataPoint{
		Symbol:    "TSE_VOLUME",
		Value:     payload.MarketVolume,
		Timestamp: ts.Unix(),
	}
}

// applyDayTradeRatio extracts 當沖交易佔比 from TWSE day-trading stats.
// Data source: TWSE OpenAPI (待接入: /opendata/t187ap05_L 當日沖銷交易統計).
// Field: DayTradeRatio (%, 0-100).
func (a *macroDataGatewayAdapter) applyDayTradeRatio(snap *marketdata.MacroDataSnapshot, data []byte) {
	var payload struct {
		DayTradeRatio float64 `json:"day_trade_ratio"`
		Date          string  `json:"date"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return
	}
	if payload.Date == "" || payload.DayTradeRatio < 0 {
		return
	}
	ts, _ := time.Parse("20060102", payload.Date)
	snap.DayTradeRatio = marketdata.MacroDataPoint{
		Symbol:    "TSE_DAYTRADE",
		Value:     payload.DayTradeRatio,
		Timestamp: ts.Unix(),
	}
}
