package monitoring

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/kaecer68/atlas-go/internal/marketdata"
	apisystem "github.com/kaecer68/atlas-go/internal/monitoring/api/system"
	"github.com/kaecer68/atlas-go/internal/narrative/geopolitical"
)

// MacroSnapshotCacheTTL controls how long FetchSnapshot results are cached
// in memory. Set to 60s to balance freshness (browser auto-refresh every 30s)
// against fan-out cost (~15s for 28 gateway channels). Matches the TTL used
// by the dailyreport layer for consistency.
const MacroSnapshotCacheTTL = 60 * time.Second

// macroDataGatewayAdapter implements marketdata.MacroDataProvider using DataFetcher.
// Replaces the CompositeMacroProvider + 9 individual provider pattern in NewDashboardAPI.
type macroDataGatewayAdapter struct {
	fetcher        DataFetcher
	mu             sync.Mutex
	lastErrors     map[string]string // channelID → last error message (Layer 2 of data-visibility)
	cacheMu        sync.RWMutex
	cachedSnapshot *marketdata.MacroDataSnapshot
	cachedAt       time.Time
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

// FetchSnapshot returns the latest macro data snapshot with TTL caching.
// Cache hit (within MacroSnapshotCacheTTL) returns the cached value without
// fan-out. Cache miss triggers the full 28-channel gateway fan-out; a
// double-checked locking pattern prevents duplicate fan-outs when multiple
// callers arrive simultaneously.
func (a *macroDataGatewayAdapter) FetchSnapshot(ctx context.Context) (marketdata.MacroDataSnapshot, error) {
	// Fast path: cache hit under read lock.
	a.cacheMu.RLock()
	if a.cachedSnapshot != nil && time.Since(a.cachedAt) < MacroSnapshotCacheTTL {
		snap := *a.cachedSnapshot
		a.cacheMu.RUnlock()
		return snap, nil
	}
	a.cacheMu.RUnlock()

	// Double-check under write lock: if another goroutine filled the cache
	// between our RUnlock and Lock, return the warm value.
	a.cacheMu.Lock()
	if a.cachedSnapshot != nil && time.Since(a.cachedAt) < MacroSnapshotCacheTTL {
		snap := *a.cachedSnapshot
		a.cacheMu.Unlock()
		return snap, nil
	}
	a.cacheMu.Unlock()

	// Cache miss: do the actual fan-out. Runs outside the lock so concurrent
	// cache readers (including the first fast-path check) are not blocked by
	// a slow 15s fetch.
	snap, err := a.fetchFresh(ctx)
	if err == nil {
		a.cacheMu.Lock()
		a.cachedSnapshot = &snap
		a.cachedAt = time.Now()
		a.cacheMu.Unlock()
	}
	return snap, err
}

// fetchFresh performs the actual 28-channel gateway fan-out without caching.
// Errors are recorded per-channel in lastErrors; a single non-stale error
// does not fail the entire fetch.
func (a *macroDataGatewayAdapter) fetchFresh(ctx context.Context) (marketdata.MacroDataSnapshot, error) {
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
		{channelID: "twse_margin", apply: a.applyMargin},
		{channelID: "day_trading", apply: a.applyDayTradeRatioFromStats},
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
	var failedChannels, staleChannels []string
	var errMsgs []string
	for ch, e := range a.lastErrors {
		if len(e) >= 6 && e[:6] == "stale:" {
			staleChannels = append(staleChannels, ch)
		} else {
			failedChannels = append(failedChannels, ch)
			errMsgs = append(errMsgs, fmt.Sprintf("%s: %s", ch, e))
		}
	}

	merged.FailedChannels = failedChannels
	merged.StaleChannels = staleChannels
	switch {
	case len(failedChannels) == 0 && len(staleChannels) == 0:
		merged.DataStatus = "ok"
	case len(failedChannels) > 0 && len(failedChannels) == len(channels):
		merged.DataStatus = "stale"
	case len(failedChannels) > 0:
		merged.DataStatus = "degraded"
	default:
		merged.DataStatus = "stale"
	}
	allFailed := len(failedChannels) > 0 && len(failedChannels) == len(channels)
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
	if s.MarginMaintenanceRatio.Symbol != "" {
		snap.MarginMaintenanceRatio = s.MarginMaintenanceRatio
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
//
// The twse_oddlot upstream was removed (BFI84U now serves the 停券預告表
// report, MI_INDEX type=ODDLOT returns empty — see known_issues
// twse_oddlot_upstream_60d). As a short-term redirect, when the odd-lot
// channel yields no usable imbalance we derive a retail contrarian proxy from
// twse_capital_flow's total institutional net instead of surfacing 0.
func NewOddLotFetcher(fetcher DataFetcher) apisystem.OddLotFetcher {
	return func(ctx context.Context) (*marketdata.OddLotStats, error) {
		data, _, err := fetcher(ctx, "twse_oddlot")
		if err == nil && len(data) > 0 {
			var result marketdata.OddLotStats
			if err := json.Unmarshal(data, &result); err == nil {
				return &result, nil
			}
		}
		// twse_oddlot returned no usable bytes (upstream removed) — redirect to
		// the healthy twse_capital_flow channel for a retail imbalance proxy.
		return oddLotFromCapitalFlow(ctx, fetcher)
	}
}

// oddLotFromCapitalFlow derives a bounded retail imbalance proxy from the
// twse_capital_flow channel when twse_oddlot is unavailable.
func oddLotFromCapitalFlow(ctx context.Context, fetcher DataFetcher) (*marketdata.OddLotStats, error) {
	data, _, err := fetcher(ctx, "twse_capital_flow")
	if err != nil {
		return nil, err
	}
	var snap marketdata.MacroDataSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return nil, fmt.Errorf("capital flow unmarshal: %w", err)
	}

	totalNet := snap.ForeignInvestorNet.Value + snap.DomesticFundNet.Value + snap.DealerNet.Value
	if totalNet == 0 {
		return nil, fmt.Errorf("capital flow total net is zero")
	}

	// Contrarian proxy: odd-lot (零股) buyers are retail investors who tend to
	// trade against institutional net flows. -tanh bounds the result to [-1,1],
	// matching the original odd-lot imbalance ratio semantics. The 30億 scale
	// approximates a large daily institutional net flow (values are in 億).
	imbalance := -math.Tanh(totalNet / 30.0)
	return &marketdata.OddLotStats{ImbalanceRatio: imbalance}, nil
}

// NewETFFetcher creates a fetcher for ETF net-subscription data consumed by
// RSI-tw subC3（ETF 申購分數）.
//
// 資料源策略（2026-08-17）：TWSE TWT44U（全市場 ETF 申購贖回淨額彙總報表）
// 已於上游移除（見 known_issues.go twse_etf_upstream_60d），twse_etf channel
// 預設不註冊。因此主要路徑改為富邦投信官網「申購買回清單 (PCF)」頁面
// （免費、免 key、純 HTTP GET；實測取回真實差異數 006208→0、
// 00900→-1,000,000、00692→-500,000，見 marketdata/fubon_etf_provider.go）。
// twse_etf gateway channel 保留為第一優先嘗試（僅在 TWSE_ETF_API_KEY 設定
// 重新啟用時會成功）；channel 失敗或回傳無資料時自動改抓富邦 PCF。
func NewETFFetcher(fetcher DataFetcher, fubon ...marketdata.ETFNetSubFetcher) apisystem.ETFFetcher {
	var fubonProv marketdata.ETFNetSubFetcher
	if len(fubon) > 0 && fubon[0] != nil {
		fubonProv = fubon[0]
	} else {
		fubonProv = marketdata.NewFubonETFProvider()
	}
	return func(ctx context.Context) (*marketdata.ETFStats, error) {
		// 第一優先：twse_etf gateway channel（legacy 全市場彙總；上游已移除，
		// 實務上會失敗或無資料而落回富邦 PCF）。
		if data, _, err := fetcher(ctx, "twse_etf"); err == nil {
			var result marketdata.ETFStats
			if err := json.Unmarshal(data, &result); err == nil && result.Date != "" && result.NetSubscription != 0 {
				return &result, nil
			}
		}
		// 主要路徑：富邦投信 PCF 申購買回清單（8 支主力 ETF TWD 加權淨額）。
		return fubonProv.FetchETFNetSubscription(ctx)
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

// applyDayTradeRatioFromStats extracts 當沖交易佔比 from DayTradingStats.
// Maps DayTradingStats.VolumeRatio → snap.DayTradeRatio.
// Date must be present and parseable as "20060102"; missing/invalid → no fill.
func (a *macroDataGatewayAdapter) applyDayTradeRatioFromStats(snap *marketdata.MacroDataSnapshot, data []byte) {
	var stats marketdata.DayTradingStats
	if err := json.Unmarshal(data, &stats); err != nil {
		return
	}
	if stats.Date == "" {
		return
	}
	ts, err := time.Parse("20060102", stats.Date)
	if err != nil {
		return
	}
	snap.DayTradeRatio = marketdata.MacroDataPoint{
		Symbol:    "TSE_DAYTRADE",
		Value:     stats.VolumeRatio,
		Timestamp: ts.Unix(),
	}
}
