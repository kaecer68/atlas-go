package service

import (
	"context"
	"fmt"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/kaecer68/atlas-go/internal/globalmarket"
	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/monitoring/metrics"
)

// cacheTTL controls how long a FetchSnapshot result is reused across
// concurrent API calls (GetStatus + GetUSIndices are both called via
// Promise.all). 5 minutes reduces cache-miss frequency: each miss triggers
// a ~15-20s full CompositeMacroProvider.FetchSnapshot fan-out (28 channels)
// which can exceed the 8s request timeout and 503 without a fresh cache.
// stale-if-error below serves last-known-good data when a miss then fails,
// so this TTL only controls refresh cadence, not hard availability.
const cacheTTL = 5 * time.Minute

// correlationWindow is the rolling window for all cross-market correlation
// estimates. 20 observations corresponds to ~1 trading month at daily
// granularity, matching the original SPX-TWSE pair.
const correlationWindow = 20

// minObservationsForReport is the minimum number of paired observations
// before a correlation is reported as a non-default value. Below this
// threshold the field is reported as NaN (which the JSON encoder emits as
// `null` via the `omitempty` contract on `*float64`).
const minObservationsForReport = 3

// CrossMarketStatus represents the current US-Taiwan cross-market state.
type CrossMarketStatus struct {
	RecordedAt  int64  `json:"recorded_at"`
	GeneratedAt string `json:"generated_at"`

	// US indices
	SPX CrossMarketIndex `json:"spx"`
	NDX CrossMarketIndex `json:"ndx"`
	DJI CrossMarketIndex `json:"dji"`
	SOX CrossMarketIndex `json:"sox"`

	// US tech stocks
	NVDA CrossMarketIndex `json:"nvda"`
	AAPL CrossMarketIndex `json:"aapl"`
	MSFT CrossMarketIndex `json:"msft"`

	// TSM ADR
	TSMADR CrossMarketIndex `json:"tsm_adr"`

	// Cross-market signals
	VIX     CrossMarketIndex `json:"vix"`
	DXY     CrossMarketIndex `json:"dxy"`
	USD_TWD CrossMarketIndex `json:"usd_twd"`
	US10Y   CrossMarketIndex `json:"us10y"`

	// Derived
	CrisisActive       bool    `json:"crisis_active"`
	CorrelationSPXTWSE float64 `json:"correlation_spx_twse"`

	// Expanded cross-market correlations (2026 correlation expansion).
	// Each is the Pearson correlation over a rolling 20-observation window
	// of daily returns. Nil means insufficient observations (encoded as
	// JSON `null`). TAIEX is the canonical TWSE proxy for the new pairs
	// (SPX-TWSE retains SOXIndex to preserve existing behavior).
	CorrelationNDXTWSE  *float64 `json:"correlation_ndx_twse"`
	CorrelationDJITWSE  *float64 `json:"correlation_dji_twse"`
	CorrelationTSMTWSE  *float64 `json:"correlation_tsm_twse"`
	CorrelationNVDATWSE *float64 `json:"correlation_nvda_twse"`
	CorrelationSPXVIX   *float64 `json:"correlation_spx_vix"`

	// Data visibility (Layer 3 of data-visibility safeguard).
	// DataStatus is "ok" when all 10 US index/tech/macro fields have real data,
	// "stale" when at least one channel returned cached data (CB-open / fallback)
	// but no channel fully failed, or "degraded" when at least one channel
	// failed (no data at all). FailedChannels lists the specific channelIDs
	// that returned empty (frontend uses this to render error badges).
	// StaleChannels lists channelIDs that returned cached data — the values
	// are present but may be outdated. Frontend shows an amber warning so
	// users see the CB-open state even when numbers are present.
	DataStatus     string   `json:"data_status"`
	FailedChannels []string `json:"failed_channels,omitempty"`
	StaleChannels  []string `json:"stale_channels,omitempty"`
}

// CrossMarketIndex bundles an index/stock value with its metadata.
type CrossMarketIndex struct {
	Symbol    string  `json:"symbol"`
	Value     float64 `json:"value"`
	ChangePct float64 `json:"change_pct"`
	Timestamp int64   `json:"timestamp"`
}

// CorrelationResponse carries the dynamic correlation estimate.
type CorrelationResponse struct {
	Correlation    float64 `json:"correlation"`
	WindowSize     int     `json:"window_size"`
	Observations   int     `json:"observations"`
	ComputedAt     string  `json:"computed_at"`
	IsFallback     bool    `json:"is_fallback"`
	FallbackReason string  `json:"fallback_reason,omitempty"`
}

// USIndicesResponse returns snapshot of US market indices.
type USIndicesResponse struct {
	RecordedAt     int64              `json:"recorded_at"`
	GeneratedAt    string             `json:"generated_at"`
	Indices        []CrossMarketIndex `json:"indices"`
	TechStocks     []CrossMarketIndex `json:"tech_stocks"`
	DataStatus     string             `json:"data_status,omitempty"`
	FailedChannels []string           `json:"failed_channels,omitempty"`
	StaleChannels  []string           `json:"stale_channels,omitempty"`
}

// CrossMarketService provides cross-market data from the composite macro provider.
//
// It owns six independent RollingCorrelation engines:
//   - rollingSPXTWSE  : SPX vs SOX (legacy, retained for back-compat)
//   - rollingNDXTWSE  : NDX vs TAIEX
//   - rollingDJITWSE  : DJI vs TAIEX
//   - rollingTSMTWSE  : TSM ADR vs TAIEX  (the strongest leading indicator)
//   - rollingNVDATWSE : NVDA vs TAIEX
//   - rollingSPXVIX   : SPX vs VIX (inverse, panic gauge)
type CrossMarketService struct {
	provider marketdata.MacroDataProvider

	// degradedCallback is called when detectDegradedUSStatus returns
	// "degraded". In production this is wired in main.go to call both
	// gateway.Health().Record() (per-channel) and monitor.Warning()
	// (user-visible alert) — Option B full alerting.
	// callbackMu protects degradedCallback reads/writes and last-degraded state.
	callbackMu         sync.Mutex
	degradedCallback   func(string, []string)
	lastDegradedStatus string   // previous status string ("ok"/"stale"/"degraded")
	lastFailedChannels []string // previous failed channel list for dedup
	lastStaleChannels  []string // previous stale channel list for dedup

	// cacheMu + cachedSnapshot + cacheTime implement a TTL cache for
	// FetchSnapshot, preventing the 2x redundant ~15-20s HTTP cascade
	// when GetStatus and GetUSIndices are called concurrently.
	cacheMu          sync.Mutex
	cachedSnapshot   *marketdata.MacroDataSnapshot
	cachedStatusMeta *snapshotStatusMeta // co-cached with cachedSnapshot under cacheMu
	cacheTime        time.Time

	// rollingSPXTWSE is the legacy SPX-SOX engine. Retained for
	// back-compat with existing consumers and the /api/cross-market/correlation
	// endpoint contract.
	rollingSPXTWSE *globalmarket.RollingCorrelation

	degradedMetrics *metrics.DegradedMetrics

	// rollingNDXTWSE / DJI / TSM / NVDA use TAIEX as the TWSE proxy.
	// TAIEX is the Taiwan Stock Exchange Weighted Index — the canonical
	// broad-market benchmark for Taiwan.
	rollingNDXTWSE  *globalmarket.RollingCorrelation
	rollingDJITWSE  *globalmarket.RollingCorrelation
	rollingTSMTWSE  *globalmarket.RollingCorrelation
	rollingNVDATWSE *globalmarket.RollingCorrelation

	// rollingSPXVIX tracks the SPX-VIX inverse relationship.
	rollingSPXVIX *globalmarket.RollingCorrelation
}

// SetDegradedCallback injects a handler called when detectDegradedUSStatus
// finds degraded data. Production wiring in main.go routes this to both
// gateway.Health().Record() (per-channel) and monitor.Warning() (user-visible alert).
func (s *CrossMarketService) SetDegradedCallback(cb func(string, []string)) {
	s.callbackMu.Lock()
	defer s.callbackMu.Unlock()
	s.degradedCallback = cb
}

func (s *CrossMarketService) SetDegradedMetrics(m *metrics.DegradedMetrics) {
	s.degradedMetrics = m
}

func (s *CrossMarketService) GetDegradedMetrics() *metrics.DegradedMetrics {
	return s.degradedMetrics
}

func NewCrossMarketService(provider marketdata.MacroDataProvider) *CrossMarketService {
	return &CrossMarketService{
		provider:        provider,
		rollingSPXTWSE:  globalmarket.NewRollingCorrelation(correlationWindow),
		rollingNDXTWSE:  globalmarket.NewRollingCorrelation(correlationWindow),
		rollingDJITWSE:  globalmarket.NewRollingCorrelation(correlationWindow),
		rollingTSMTWSE:  globalmarket.NewRollingCorrelation(correlationWindow),
		rollingNVDATWSE: globalmarket.NewRollingCorrelation(correlationWindow),
		rollingSPXVIX:   globalmarket.NewRollingCorrelation(correlationWindow),
	}
}

// USMacroFields is the canonical list of macro-snapshot channel IDs that
// detectDegradedUSStatus monitors. Exported so cmd/atlas/main.go can
// iterate the same set when wiring the recovery callback into the
// UnifiedHealthStore (PR-F, kaecer 2026-08-05). Keeping a single source
// of truth prevents the recovery loop from drifting out of sync with
// the detector (a previous code review found the detector added a
// channel but the main.go recovery loop was never updated, leaving the
// new channel stuck on "degraded" forever).
//
// Order matches the field order in MacroDataSnapshot and the check
// order in detectDegradedUSStatus.
var USMacroFields = []string{
	"us_spx",
	"us_ndx",
	"us_dji",
	"sox_index",
	"us_nvda",
	"us_aapl",
	"us_msft",
	"tsm_adr",
	"us10y",
	"vix",
}

// UpdateCorrelation pushes a new daily return pair (SPX, TWSE proxy = SOX)
// into the legacy SPX-TWSE rolling correlation engine. Preserved for
// WarmupFromHistory feeds historical snapshots into all rolling correlation
// engines so the API returns meaningful data immediately instead of
// waiting for the window to fill (fix manifest #E05). Each snapshot
// contributes one observation per engine via UpdateAllCorrelations.
func (s *CrossMarketService) WarmupFromHistory(snapshots []marketdata.MacroDataSnapshot) {
	for _, snap := range snapshots {
		s.UpdateAllCorrelations(snap)
	}
}

// back-compat; new callers should use UpdateAllCorrelations.
func (s *CrossMarketService) UpdateCorrelation(spxReturn, soxReturn float64) {
	s.rollingSPXTWSE.Update(spxReturn, soxReturn)
}

// SeedSpXTWSE pre-fills the legacy SPX→SOX correlation engine with historical
// daily return pairs. Call before any UpdateCorrelation to eliminate the
// cold-start fallback period (normally 20 trading days).
func (s *CrossMarketService) SeedSpXTWSE(spxReturns, soxReturns []float64) {
	if s == nil || s.rollingSPXTWSE == nil {
		return
	}
	s.rollingSPXTWSE.SeedWith(spxReturns, soxReturns)
}

// getCachedSnapshot returns the cached snapshot + its degraded-status
// metadata. Concurrent callers that arrive during a stale cache each
// fetch independently (no coalescing), but subsequent requests within
// the TTL window hit the cache instantly. This eliminates the ~15-20s
// redundant FetchSnapshot cascade when GetStatus and GetUSIndices are
// both called.
//
// The cached meta ensures that if a fetch came back degraded, every
// cache hit also reports degraded — preventing a "fixed itself" illusion.
func (s *CrossMarketService) getCachedSnapshot(ctx context.Context) (marketdata.MacroDataSnapshot, *snapshotStatusMeta, error) {
	s.cacheMu.Lock()
	if s.cachedSnapshot != nil && time.Since(s.cacheTime) < cacheTTL {
		snap := *s.cachedSnapshot
		var meta *snapshotStatusMeta
		if s.cachedStatusMeta != nil {
			cp := *s.cachedStatusMeta
			meta = &cp
		}
		s.cacheMu.Unlock()
		return snap, meta, nil
	}
	s.cacheMu.Unlock()

	snap, err := s.provider.FetchSnapshot(ctx)
	if err != nil {
		if s.degradedMetrics != nil {
			s.degradedMetrics.ProviderErrors.WithLabelValues("crossmarket", classifyErrorSeverity(err.Error())).Inc()
		}
		// stale-if-error (R1): the TTL cache has expired, but we still hold the
		// last-known-good snapshot. Serve it marked degraded instead of
		// propagating the error — otherwise every cache miss that also fails to
		// fetch returns a hard 503 to the middleware (which then doesn't write
		// the cache), producing an unrecoverable 503 death-spiral until restart.
		// Only when the cache is completely empty (no prior successful fetch)
		// do we return the error. degradedMetrics.ProviderErrors above still
		// records the provider timeout / failure.
		s.cacheMu.Lock()
		hasStale := s.cachedSnapshot != nil
		var staleSnap marketdata.MacroDataSnapshot
		var staleMeta *snapshotStatusMeta
		if hasStale {
			staleSnap = *s.cachedSnapshot
			if s.cachedStatusMeta != nil {
				cp := *s.cachedStatusMeta
				staleMeta = &cp
			}
		}
		s.cacheMu.Unlock()
		if !hasStale {
			return snap, nil, err
		}
		if staleMeta == nil {
			staleMeta = &snapshotStatusMeta{}
		}
		staleMeta.DataStatus = "degraded"
		return staleSnap, staleMeta, nil
	}

	status, failed, stale := detectDegradedUSStatus(snap, channelErrorsFromProvider(s.provider))
	meta := &snapshotStatusMeta{DataStatus: status, FailedChannels: failed, StaleChannels: stale}

	// Debounce: only fire callback on state transition
	// (ok↔degraded) or when the failed-channel list changes.
	// Without this, the callback fires on EVERY cache refresh
	// (~30s) while the snapshot is degraded — producing duplicate
	// health.Record() calls and alert spam.
	//
	// Recovery path (2026-08-04): when status flips back to "ok" from a
	// previously non-ok state, fire the callback so the dashboard can
	// clear the per-channel "degraded" health records (us10y / vix were
	// stuck on "degraded" for 9 days because no recovery path existed —
	// the snapshot became healthy again but the channel-health page
	// never knew). The callback in cmd/atlas/main.go:1106 handles
	// recovery by recording status="ok" on the same channels that were
	// previously flagged.
	s.callbackMu.Lock()
	cb := s.degradedCallback
	prevStatus := s.lastDegradedStatus
	prevFailed := s.lastFailedChannels
	prevStale := s.lastStaleChannels
	changed := status != prevStatus || !stringSlicesEqual(failed, prevFailed) || !stringSlicesEqual(stale, prevStale)
	// Fire on transition INTO degraded (covers both "stale" and "degraded")
	// AND on transition back TO ok from a non-ok state (recovery path).
	//
	// PR-F (kaecer 2026-08-05): also fire on the very first observation
	// when the snapshot is "ok" and prevStatus is "" (process just
	// started). Without this, the recovery callback never runs for a
	// fresh process, so any "degraded" health records left over from
	// the previous process (e.g. us10y / vix stuck for 9+ days) are
	// never cleared — the channel_health.json file persists across
	// restarts, but the in-memory lastDegradedStatus resets to "".
	// The callback in main.go now ignores the empty "failed" list and
	// iterates USMacroFields directly to clear every monitored channel.
	shouldFire := cb != nil && changed && (status != prevStatus) && ((status != "ok") || (prevStatus != "" && prevStatus != "ok") || (status == "ok" && prevStatus == ""))
	if changed || prevStatus == "" {
		s.lastDegradedStatus = status
		s.lastFailedChannels = failed
		s.lastStaleChannels = stale
	}
	s.callbackMu.Unlock()
	if shouldFire {
		cb(status, failed)
		if s.degradedMetrics != nil {
			s.degradedMetrics.DegradedCallbackCount.WithLabelValues("crossmarket", "missing_us_index_data", status).Inc()
		}
	}

	if status != "ok" && s.degradedMetrics != nil {
		s.degradedMetrics.DegradedActivations.WithLabelValues("crossmarket", "missing_us_index_data").Inc()
	}

	s.cacheMu.Lock()
	s.cachedSnapshot = &snap
	s.cachedStatusMeta = meta
	s.cacheTime = time.Now()
	s.cacheMu.Unlock()

	return snap, meta, nil
}

// UpdateAllCorrelations ingests a MacroDataSnapshot and pushes the relevant
// return pairs into all six rolling correlation engines. This is the
// canonical entry point for the realtime_feed / macro_ingest BTM task.
//
// Pairs updated:
//   - SPX × SOX   (legacy, retained)
//   - NDX × TAIEX
//   - DJI × TAIEX
//   - TSM ADR × TAIEX
//   - NVDA × TAIEX
//   - SPX × VIX
//
// Engines whose source has a zero `ChangePct` are still updated (the
// RollingCorrelation engine handles zero observations gracefully and
// returns the default 0.5 with insufficient data flag).
func (s *CrossMarketService) UpdateAllCorrelations(snap marketdata.MacroDataSnapshot) {
	if s == nil {
		return
	}
	// Legacy: SPX-SOX (preserved for back-compat)
	s.rollingSPXTWSE.Update(snap.SPXIndex.ChangePct, snap.SOXIndex.ChangePct)

	// TAIEX-anchored Taiwan correlations
	taiex := snap.TAIEX.ChangePct
	s.rollingNDXTWSE.Update(snap.NDXIndex.ChangePct, taiex)
	s.rollingDJITWSE.Update(snap.DJIIndex.ChangePct, taiex)
	s.rollingTSMTWSE.Update(snap.TSMADR.ChangePct, taiex)
	s.rollingNVDATWSE.Update(snap.NVDA.ChangePct, taiex)

	// SPX-VIX inverse (panic gauge)
	s.rollingSPXVIX.Update(snap.SPXIndex.ChangePct, snap.VIX.ChangePct)
}

// GetStatus returns the full cross-market status snapshot.
func (s *CrossMarketService) GetStatus(ctx context.Context) (*CrossMarketStatus, error) {
	snap, meta, err := s.getCachedSnapshot(ctx)
	if err != nil {
		return nil, fmt.Errorf("fetch macro snapshot: %w", err)
	}

	status := &CrossMarketStatus{
		RecordedAt:   snap.RecordedAt,
		GeneratedAt:  time.Now().Format(time.RFC3339),
		CrisisActive: snap.VIX.Value >= 35.0,
	}

	status.SPX = toIndex(snap.SPXIndex)
	status.NDX = toIndex(snap.NDXIndex)
	status.DJI = toIndex(snap.DJIIndex)
	status.SOX = toIndex(snap.SOXIndex)
	status.NVDA = toIndex(snap.NVDA)
	status.AAPL = toIndex(snap.AAPL)
	status.MSFT = toIndex(snap.MSFT)
	status.TSMADR = toIndex(snap.TSMADR)
	status.VIX = toIndex(snap.VIX)
	status.DXY = toIndex(snap.DXY)
	status.USD_TWD = toIndex(snap.USD_TWD)
	status.US10Y = toIndex(snap.US10Y)

	// Legacy: SPX-TWSE (always populated; uses SOX as TWSE proxy).
	status.CorrelationSPXTWSE = s.rollingSPXTWSE.GetCurrent()

	// Expanded pairs: only populate when the engine has at least
	// minObservationsForReport paired observations. Otherwise emit `null`.
	status.CorrelationNDXTWSE = reportableCorrelation(s.rollingNDXTWSE)
	status.CorrelationDJITWSE = reportableCorrelation(s.rollingDJITWSE)
	status.CorrelationTSMTWSE = reportableCorrelation(s.rollingTSMTWSE)
	status.CorrelationNVDATWSE = reportableCorrelation(s.rollingNVDATWSE)
	status.CorrelationSPXVIX = reportableCorrelation(s.rollingSPXVIX)

	// Layer 3: Use cached status meta (co-cached with the snapshot to
	// prevent cache hits from "fixing" a degraded state).
	if meta != nil {
		status.DataStatus = meta.DataStatus
		status.FailedChannels = meta.FailedChannels
		status.StaleChannels = meta.StaleChannels
	}

	return status, nil
}

// GetCorrelation returns the current SPX-TWSE correlation estimate (legacy).
func (s *CrossMarketService) GetCorrelation() (*CorrelationResponse, error) {
	rho := s.rollingSPXTWSE.GetCurrent()
	obs := s.rollingSPXTWSE.Observations()
	return &CorrelationResponse{
		Correlation:    rho,
		WindowSize:     correlationWindow,
		Observations:   obs,
		ComputedAt:     time.Now().Format(time.RFC3339),
		IsFallback:     s.rollingSPXTWSE.IsFallback(),
		FallbackReason: s.rollingSPXTWSE.FallbackReason(),
	}, nil
}

// GetUSIndices returns the current US market indices snapshot.
func (s *CrossMarketService) GetUSIndices(ctx context.Context) (*USIndicesResponse, error) {
	snap, meta, err := s.getCachedSnapshot(ctx)
	if err != nil {
		return nil, fmt.Errorf("fetch macro snapshot: %w", err)
	}

	resp := &USIndicesResponse{
		RecordedAt:  snap.RecordedAt,
		GeneratedAt: time.Now().Format(time.RFC3339),
	}

	if meta != nil {
		resp.DataStatus = meta.DataStatus
		resp.FailedChannels = meta.FailedChannels
		resp.StaleChannels = meta.StaleChannels
	}

	resp.Indices = []CrossMarketIndex{
		toIndex(snap.SPXIndex),
		toIndex(snap.NDXIndex),
		toIndex(snap.DJIIndex),
		toIndex(snap.SOXIndex),
	}

	resp.TechStocks = []CrossMarketIndex{
		toIndex(snap.NVDA),
		toIndex(snap.AAPL),
		toIndex(snap.MSFT),
		toIndex(snap.TSMADR),
	}

	return resp, nil
}

// reportableCorrelation returns the current correlation value only when
// the engine has accumulated at least minObservationsForReport observations
// and the result is a finite number. Otherwise returns nil so the JSON
// field is emitted as `null` (per the user's "缺資料時回傳 null" contract).
func reportableCorrelation(rc *globalmarket.RollingCorrelation) *float64 {
	if rc == nil {
		return nil
	}
	if rc.Observations() < minObservationsForReport {
		return nil
	}
	rho := rc.GetCurrent()
	// Defensive: the engine already guards NaN/Inf, but a final check
	// here keeps the API contract explicit.
	if math.IsNaN(rho) {
		return nil
	}
	// Belt-and-suspenders: the Observations() guard above already
	// prevents reporting before minObservationsForReport. The engine
	// also returns a sentinel default (0.5) when data is insufficient.
	// Both checks together ensure the API contract is never violated.
	return &rho
}

// toIndex converts a MacroDataPoint to a CrossMarketIndex.
func toIndex(dp marketdata.MacroDataPoint) CrossMarketIndex {
	return CrossMarketIndex{
		Symbol:    dp.Symbol,
		Value:     dp.Value,
		ChangePct: dp.ChangePct,
		Timestamp: dp.Timestamp,
	}
}

// detectDegradedUSStatus returns the data status, failed channels, and stale
// channels for the 10 US index/tech/macro fields in MacroDataSnapshot.
// A field is "failed" when its Symbol is empty (meaning the channel returned
// an error or no data) or its Value is ≤ 0 (meaning the provider returned
// garbage/zero data). "Stale" channels are those the L2 gateway adapter
// marked with a "stale:" prefix (CB-open / fallback serving cached data) —
// they still produced a value, but it may be outdated.
//
// This is Layer 3 of the 4-layer data-visibility safeguard
// (see .claude/skills/atlas-data-visibility/SKILL.md).
//
// Taxonomy:
//   - "ok"        + nil          when all 10 fields fresh and no channel stale
//   - "stale"     + staleList    when no failures but at least one channel stale
//   - "degraded"  + failedList   when at least one field failed (stale list still
//     returned alongside, since users benefit from
//     knowing which subset is fresh vs cached)
func detectDegradedUSStatus(snap marketdata.MacroDataSnapshot, channelErrors map[string]string) (string, []string, []string) {
	failed := []string{}
	checks := []struct {
		channelID string
		point     marketdata.MacroDataPoint
	}{
		{"us_spx", snap.SPXIndex},
		{"us_ndx", snap.NDXIndex},
		{"us_dji", snap.DJIIndex},
		{"sox_index", snap.SOXIndex},
		{"us_nvda", snap.NVDA},
		{"us_aapl", snap.AAPL},
		{"us_msft", snap.MSFT},
		{"tsm_adr", snap.TSMADR},
		{"us10y", snap.US10Y},
		{"vix", snap.VIX},
	}
	for _, c := range checks {
		if c.point.Symbol == "" || c.point.Value <= 0 {
			failed = append(failed, c.channelID)
		}
	}
	stale := extractStaleChannels(channelErrors)

	switch {
	case len(failed) > 0:
		return "degraded", failed, stale
	case len(stale) > 0:
		return "stale", nil, stale
	default:
		return "ok", nil, nil
	}
}

// stringSlicesEqual reports whether two string slices have identical
// elements in the same order. Both nil and empty slices are treated as
// equivalent (len == 0 comparison).
func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// snapshotStatusMeta is the per-fetch degraded-status metadata co-cached
// with the MacroDataSnapshot. Layer 3 of the data-visibility safeguard —
// the cache must not "fix" a degraded snapshot into an "ok" one just
// because it's served from cache instead of refetched.
type snapshotStatusMeta struct {
	DataStatus     string
	FailedChannels []string
	StaleChannels  []string
}

// channelErrorsProvider is the optional L2 capability that exposes the
// gateway adapter's per-channel error map (with "stale:" prefixes for
// CB-open / fallback channels). The production wiring uses
// *macroDataGatewayAdapter which implements this; test fakes may not.
type channelErrorsProvider interface {
	ChannelErrors() map[string]string
}

func channelErrorsFromProvider(p marketdata.MacroDataProvider) map[string]string {
	if p == nil {
		return nil
	}
	if cep, ok := p.(channelErrorsProvider); ok {
		return cep.ChannelErrors()
	}
	return nil
}

// extractStaleChannels filters a channel-errors map for entries whose
// message starts with "stale:" (set by the L2 gateway adapter when the
// CB served cached data). Returns a deterministic, lexicographically
// sorted slice so tests and dedup logic see a stable order.
func extractStaleChannels(channelErrors map[string]string) []string {
	if len(channelErrors) == 0 {
		return nil
	}
	stale := make([]string, 0, len(channelErrors))
	for ch, msg := range channelErrors {
		if len(msg) >= 6 && msg[:6] == "stale:" {
			stale = append(stale, ch)
		}
	}
	if len(stale) == 0 {
		return nil
	}
	sort.Strings(stale)
	return stale
}
