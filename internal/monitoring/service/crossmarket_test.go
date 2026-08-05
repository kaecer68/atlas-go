package service

import (
	"context"
	"encoding/json"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/monitoring/metrics"
)

// fakeMacroProvider is a stub MacroDataProvider for testing CrossMarketService.
type fakeMacroProvider struct {
	snap          marketdata.MacroDataSnapshot
	err           error
	channelErrors map[string]string
}

func (f *fakeMacroProvider) Name() string { return "fake" }
func (f *fakeMacroProvider) FetchSnapshot(_ context.Context) (marketdata.MacroDataSnapshot, error) {
	if f.err != nil {
		return marketdata.MacroDataSnapshot{}, f.err
	}
	return f.snap, nil
}

// ChannelErrors implements channelErrorsProvider so tests can simulate the
// L2 gateway adapter's per-channel error map (incl. "stale:" prefixes).
func (f *fakeMacroProvider) ChannelErrors() map[string]string {
	return f.channelErrors
}

func makeSnapshot() marketdata.MacroDataSnapshot {
	return marketdata.MacroDataSnapshot{
		SPXIndex:   marketdata.MacroDataPoint{Symbol: "SPX", Value: 5000.0, ChangePct: 0.5, Timestamp: time.Now().Unix()},
		NDXIndex:   marketdata.MacroDataPoint{Symbol: "NDX", Value: 18000.0, ChangePct: 0.7, Timestamp: time.Now().Unix()},
		DJIIndex:   marketdata.MacroDataPoint{Symbol: "DJI", Value: 40000.0, ChangePct: 0.3, Timestamp: time.Now().Unix()},
		SOXIndex:   marketdata.MacroDataPoint{Symbol: "SOX", Value: 5000.0, ChangePct: 1.0, Timestamp: time.Now().Unix()},
		TSMADR:     marketdata.MacroDataPoint{Symbol: "TSM", Value: 180.0, ChangePct: 0.8, Timestamp: time.Now().Unix()},
		NVDA:       marketdata.MacroDataPoint{Symbol: "NVDA", Value: 900.0, ChangePct: 1.2, Timestamp: time.Now().Unix()},
		VIX:        marketdata.MacroDataPoint{Symbol: "VIX", Value: 18.0, ChangePct: -0.4, Timestamp: time.Now().Unix()},
		TAIEX:      marketdata.MacroDataPoint{Symbol: "TAIEX", Value: 20000.0, ChangePct: 0.6, Timestamp: time.Now().Unix()},
		RecordedAt: time.Now().Unix(),
	}
}

func TestNewCrossMarketService_InitializesAllSixEngines(t *testing.T) {
	svc := NewCrossMarketService(&fakeMacroProvider{})

	// All six engines must be non-nil so subsequent Update calls dispatch
	// into them rather than nil-pointer-panicking.
	if svc.rollingSPXTWSE == nil {
		t.Error("rollingSPXTWSE not initialized")
	}
	if svc.rollingNDXTWSE == nil {
		t.Error("rollingNDXTWSE not initialized")
	}
	if svc.rollingDJITWSE == nil {
		t.Error("rollingDJITWSE not initialized")
	}
	if svc.rollingTSMTWSE == nil {
		t.Error("rollingTSMTWSE not initialized (the key leading indicator!)")
	}
	if svc.rollingNVDATWSE == nil {
		t.Error("rollingNVDATWSE not initialized")
	}
	if svc.rollingSPXVIX == nil {
		t.Error("rollingSPXVIX not initialized")
	}
}

func TestUpdateAllCorrelations_PushesAllPairs(t *testing.T) {
	svc := NewCrossMarketService(&fakeMacroProvider{})
	snap := makeSnapshot()

	svc.UpdateAllCorrelations(snap)

	// After one update, each engine should have 1 observation.
	engines := map[string]struct {
		observations int
	}{
		"SPX-TWSE":  {svc.rollingSPXTWSE.Observations()},
		"NDX-TWSE":  {svc.rollingNDXTWSE.Observations()},
		"DJI-TWSE":  {svc.rollingDJITWSE.Observations()},
		"TSM-TWSE":  {svc.rollingTSMTWSE.Observations()},
		"NVDA-TWSE": {svc.rollingNVDATWSE.Observations()},
		"SPX-VIX":   {svc.rollingSPXVIX.Observations()},
	}
	for name, e := range engines {
		if e.observations != 1 {
			t.Errorf("%s engine: expected 1 observation after Update, got %d", name, e.observations)
		}
	}
}

func TestGetStatus_PopulatesAllFiveNewCorrelationsAfterMinObs(t *testing.T) {
	prov := &fakeMacroProvider{snap: makeSnapshot()}
	svc := NewCrossMarketService(prov)

	// Push minObservationsForReport observations to make the new
	// correlation fields reportable (not null).
	for range minObservationsForReport {
		svc.UpdateAllCorrelations(prov.snap)
	}

	status, err := svc.GetStatus(context.Background())
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}

	if status.CorrelationNDXTWSE == nil {
		t.Error("CorrelationNDXTWSE should be populated after 3 obs, got nil")
	}
	if status.CorrelationDJITWSE == nil {
		t.Error("CorrelationDJITWSE should be populated after 3 obs, got nil")
	}
	if status.CorrelationTSMTWSE == nil {
		t.Error("CorrelationTSMTWSE should be populated after 3 obs, got nil (key leading indicator!)")
	}
	if status.CorrelationNVDATWSE == nil {
		t.Error("CorrelationNVDATWSE should be populated after 3 obs, got nil")
	}
	if status.CorrelationSPXVIX == nil {
		t.Error("CorrelationSPXVIX should be populated after 3 obs, got nil")
	}
}

func TestGetStatus_NullWhenInsufficientObservations(t *testing.T) {
	prov := &fakeMacroProvider{snap: makeSnapshot()}
	svc := NewCrossMarketService(prov)

	// No updates yet — all new correlations must be null.
	status, err := svc.GetStatus(context.Background())
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}

	if status.CorrelationNDXTWSE != nil {
		t.Error("CorrelationNDXTWSE should be nil before any update, got", *status.CorrelationNDXTWSE)
	}
	if status.CorrelationDJITWSE != nil {
		t.Error("CorrelationDJITWSE should be nil before any update")
	}
	if status.CorrelationTSMTWSE != nil {
		t.Error("CorrelationTSMTWSE should be nil before any update (key leading indicator!)")
	}
	if status.CorrelationNVDATWSE != nil {
		t.Error("CorrelationNVDATWSE should be nil before any update")
	}
	if status.CorrelationSPXVIX != nil {
		t.Error("CorrelationSPXVIX should be nil before any update")
	}
	// Legacy field is float64 (always populated); before any update its
	// zero value is 0.0. The engine's 0.5 fallback is only set inside
	// Update() when the engine has 0-2 observations.
	if status.CorrelationSPXTWSE != 0.0 {
		t.Errorf("CorrelationSPXTWSE legacy default should be 0.0 (zero value) before any update, got %v", status.CorrelationSPXTWSE)
	}
}

func TestGetStatus_NullEncodedAsJSONNull(t *testing.T) {
	prov := &fakeMacroProvider{snap: makeSnapshot()}
	svc := NewCrossMarketService(prov)

	status, err := svc.GetStatus(context.Background())
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}

	// Marshal to JSON and confirm the new fields are emitted as `null`
	// (not 0, not omitted) — this is the user's "缺資料時回傳 null" contract.
	data, err := json.Marshal(status)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	jsonStr := string(data)
	for _, field := range []string{
		"correlation_ndx_twse",
		"correlation_dji_twse",
		"correlation_tsm_twse",
		"correlation_nvda_twse",
		"correlation_spx_vix",
	} {
		// Look for the field with a null value (not the default-fallback 0.5).
		// Acceptable forms:  "field":null  or  "field":0.5  (latter is wrong
		// because we marked it omitempty on *float64).
		if !strings.Contains(jsonStr, `"`+field+`":null`) {
			t.Errorf("expected %q to be JSON null when insufficient data; full response: %s", field, jsonStr)
		}
	}
}

func TestGetStatus_LegacyCorrelationStillPopulated(t *testing.T) {
	prov := &fakeMacroProvider{snap: makeSnapshot()}
	svc := NewCrossMarketService(prov)

	// Push some data into the legacy engine via the legacy API to confirm
	// it still works for the /api/cross-market/correlation endpoint.
	svc.UpdateCorrelation(1.0, 0.5)
	svc.UpdateCorrelation(1.1, 0.6)
	svc.UpdateCorrelation(0.9, 0.4)

	status, err := svc.GetStatus(context.Background())
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}

	if status.CorrelationSPXTWSE == 0.0 {
		t.Error("CorrelationSPXTWSE should have a value after 3 updates, got 0")
	}
	const epsilon = 1e-9
	if status.CorrelationSPXTWSE < 0.5 || status.CorrelationSPXTWSE > 1.0+epsilon {
		t.Errorf("CorrelationSPXTWSE out of expected range, got %v", status.CorrelationSPXTWSE)
	}
}

func TestReportableCorrelation_HandlesNilReceiver(t *testing.T) {
	// Internal guard: reportableCorrelation must not panic on nil.
	got := reportableCorrelation(nil)
	if got != nil {
		t.Errorf("expected nil for nil engine, got %v", *got)
	}
}

func TestReportableCorrelation_BelowThresholdReturnsNil(t *testing.T) {
	// 2 obs < minObservationsForReport=3 → nil.
	svc := NewCrossMarketService(&fakeMacroProvider{})
	svc.rollingNDXTWSE.Update(0.1, 0.2)
	svc.rollingNDXTWSE.Update(0.15, 0.25)
	if got := reportableCorrelation(svc.rollingNDXTWSE); got != nil {
		t.Errorf("expected nil with 2 obs, got %v", *got)
	}
}

func TestRollingWindowSize(t *testing.T) {
	// Sanity: the constant matches the legacy behaviour.
	if correlationWindow != 20 {
		t.Errorf("correlationWindow = %d, expected 20 to match legacy SPX-TWSE", correlationWindow)
	}
}

func TestUpdateAllCorrelations_RecomputesTSMTWSEKey(t *testing.T) {
	// The TSM-TWSE pair is the key leading indicator. Push perfectly
	// correlated (proportional) returns and confirm the engine reports ~+1.0.
	prov := &fakeMacroProvider{}
	svc := NewCrossMarketService(prov)

	// Vary the snapshot each iteration so the rolling engine has
	// non-zero variance. (Constant returns have undefined correlation —
	// denX=denY=0 → engine returns fallback 0.5, not 1.0.)
	pairs := []struct{ tsm, taiex float64 }{
		{0.5, 0.30},
		{0.7, 0.42},
		{0.6, 0.36},
		{0.8, 0.48},
		{0.4, 0.24},
		{0.9, 0.54},
		{0.3, 0.18},
		{0.5, 0.30},
		{0.7, 0.42},
		{0.6, 0.36},
	}
	for _, p := range pairs {
		snap := makeSnapshot()
		snap.TSMADR.ChangePct = p.tsm
		snap.TAIEX.ChangePct = p.taiex
		svc.UpdateAllCorrelations(snap)
	}

	status, err := svc.GetStatus(context.Background())
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}

	if status.CorrelationTSMTWSE == nil {
		t.Fatal("CorrelationTSMTWSE nil after 10 obs (key indicator!)")
	}
	rho := *status.CorrelationTSMTWSE
	// The 10 pairs are perfectly proportional (taiex = 0.6 * tsm),
	// so Pearson correlation should be ~+1.0.
	if rho < 0.95 {
		t.Errorf("CorrelationTSMTWSE = %v, expected ~1.0 for proportional returns", rho)
	}
}

func TestSPXVIXInverselyCorrelated(t *testing.T) {
	// Push anti-correlated SPX/VIX returns; expect negative Pearson.
	prov := &fakeMacroProvider{}
	svc := NewCrossMarketService(prov)

	// VIX down when SPX up: classic "risk-on" day. Vary both for non-zero variance.
	pairs := []struct{ spx, vix float64 }{
		{0.8, -0.6},
		{0.5, -0.4},
		{1.0, -0.7},
		{0.3, -0.2},
		{0.6, -0.5},
		{0.9, -0.8},
		{0.4, -0.3},
		{0.7, -0.5},
		{0.5, -0.4},
		{0.8, -0.6},
	}
	for _, p := range pairs {
		snap := makeSnapshot()
		snap.SPXIndex.ChangePct = p.spx
		snap.VIX.ChangePct = p.vix
		svc.UpdateAllCorrelations(snap)
	}

	status, err := svc.GetStatus(context.Background())
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}
	if status.CorrelationSPXVIX == nil {
		t.Fatal("CorrelationSPXVIX nil after 10 obs")
	}
	rho := *status.CorrelationSPXVIX
	// Should be strongly negative (anti-correlated).
	if rho > -0.5 {
		t.Errorf("CorrelationSPXVIX = %v, expected strongly negative for inverse SPX-VIX", rho)
	}
	if math.IsNaN(rho) || math.IsInf(rho, 0) {
		t.Errorf("CorrelationSPXVIX = %v, expected finite", rho)
	}
}

func TestGetCorrelation_LegacyEndpoint(t *testing.T) {
	prov := &fakeMacroProvider{snap: makeSnapshot()}
	svc := NewCrossMarketService(prov)

	// Push correlated returns into the legacy engine.
	svc.UpdateCorrelation(1.0, 0.8)
	svc.UpdateCorrelation(1.1, 0.9)
	svc.UpdateCorrelation(0.9, 0.7)

	resp, err := svc.GetCorrelation()
	if err != nil {
		t.Fatalf("GetCorrelation: %v", err)
	}

	if resp.Correlation == 0.0 {
		t.Error("expected non-zero correlation after 3 updates")
	}
	if resp.WindowSize != correlationWindow {
		t.Errorf("WindowSize = %d, expected %d", resp.WindowSize, correlationWindow)
	}
	if resp.Observations != 3 {
		t.Errorf("Observations = %d, expected 3", resp.Observations)
	}
	if resp.ComputedAt == "" {
		t.Error("expected non-empty ComputedAt timestamp")
	}
	// With 3 correlated observations, IsFallback should be false.
	if resp.IsFallback {
		t.Error("IsFallback should be false with 3 observations")
	}
}

// counterProvider wraps a MacroDataProvider and counts FetchSnapshot calls
// so cache tests can assert the provider is bypassed within the TTL window.
type counterProvider struct {
	snap   marketdata.MacroDataSnapshot
	err    error
	calls  int
	lastAt time.Time
}

func (c *counterProvider) Name() string { return "counter" }
func (c *counterProvider) FetchSnapshot(_ context.Context) (marketdata.MacroDataSnapshot, error) {
	c.calls++
	c.lastAt = time.Now()
	if c.err != nil {
		return marketdata.MacroDataSnapshot{}, c.err
	}
	return c.snap, nil
}

func TestGetCachedSnapshot_CacheHit_AvoidsProviderCall(t *testing.T) {
	prov := &counterProvider{snap: makeSnapshot()}
	svc := NewCrossMarketService(prov)

	if _, err := svc.GetStatus(context.Background()); err != nil {
		t.Fatalf("GetStatus (warm): %v", err)
	}
	for i := 0; i < 5; i++ {
		if _, err := svc.GetUSIndices(context.Background()); err != nil {
			t.Fatalf("GetUSIndices[%d]: %v", i, err)
		}
	}
	if prov.calls != 1 {
		t.Errorf("expected 1 provider call (cache hit on subsequent), got %d", prov.calls)
	}
}

func TestGetCachedSnapshot_StaleAfterTTL_Refetches(t *testing.T) {
	prov := &counterProvider{snap: makeSnapshot()}
	svc := NewCrossMarketService(prov)

	if _, err := svc.GetStatus(context.Background()); err != nil {
		t.Fatalf("GetStatus (seed): %v", err)
	}
	if prov.calls != 1 {
		t.Fatalf("expected 1 provider call after seed, got %d", prov.calls)
	}

	// Rewind cacheTime past the TTL rather than sleeping 30s.
	svc.cacheMu.Lock()
	svc.cacheTime = time.Now().Add(-cacheTTL - time.Second)
	svc.cacheMu.Unlock()

	if _, err := svc.GetStatus(context.Background()); err != nil {
		t.Fatalf("GetStatus (after expiry): %v", err)
	}
	if prov.calls != 2 {
		t.Errorf("expected 2 provider calls (1 seed + 1 after stale), got %d", prov.calls)
	}
}

func TestDetectDegradedUSStatus_AllFailed(t *testing.T) {
	snap := marketdata.MacroDataSnapshot{}
	status, failed, stale := detectDegradedUSStatus(snap, nil)
	if status != "degraded" {
		t.Errorf("expected status=degraded, got %q", status)
	}
	if len(failed) != 10 {
		t.Errorf("expected 10 failed channels, got %d: %v", len(failed), failed)
	}
	if stale != nil {
		t.Errorf("expected nil stale when no channelErrors map, got %v", stale)
	}
	expected := map[string]bool{
		"us_spx": false, "us_ndx": false, "us_dji": false, "sox_index": false,
		"us_nvda": false, "us_aapl": false, "us_msft": false, "tsm_adr": false,
		"us10y": false, "vix": false,
	}
	for _, f := range failed {
		if _, ok := expected[f]; !ok {
			t.Errorf("unexpected failed channel: %s", f)
		}
		expected[f] = true
	}
	for ch, seen := range expected {
		if !seen {
			t.Errorf("expected %s in failed list", ch)
		}
	}
}

func TestDetectDegradedUSStatus_PartialFailure(t *testing.T) {
	snap := marketdata.MacroDataSnapshot{
		SPXIndex: marketdata.MacroDataPoint{Symbol: "^GSPC", Value: 5234.5},
		NDXIndex: marketdata.MacroDataPoint{Symbol: "^IXIC", Value: 18432.1},
		// DJIIndex empty
		// SOXIndex empty
		NVDA: marketdata.MacroDataPoint{Symbol: "NVDA", Value: 950.0},
		AAPL: marketdata.MacroDataPoint{Symbol: "AAPL", Value: 220.0},
		// MSFT empty
		// TSMADR empty
	}
	status, failed, stale := detectDegradedUSStatus(snap, nil)
	if status != "degraded" {
		t.Errorf("expected status=degraded, got %q", status)
	}
	if len(failed) != 6 {
		t.Errorf("expected 6 failed channels (DJI, SOX, MSFT, TSM, US10Y, VIX), got %d: %v", len(failed), failed)
	}
	if stale != nil {
		t.Errorf("expected nil stale when no channelErrors, got %v", stale)
	}
	expectedFailed := map[string]bool{"us_dji": false, "sox_index": false, "us_msft": false, "tsm_adr": false, "us10y": false, "vix": false}
	for _, f := range failed {
		if _, ok := expectedFailed[f]; !ok {
			t.Errorf("unexpected failed channel: %s", f)
		}
		expectedFailed[f] = true
	}
}

func TestDetectDegradedUSStatus_AllOK(t *testing.T) {
	snap := marketdata.MacroDataSnapshot{
		SPXIndex: marketdata.MacroDataPoint{Symbol: "^GSPC", Value: 1.0},
		NDXIndex: marketdata.MacroDataPoint{Symbol: "^IXIC", Value: 1.0},
		DJIIndex: marketdata.MacroDataPoint{Symbol: "^DJI", Value: 1.0},
		SOXIndex: marketdata.MacroDataPoint{Symbol: "^SOX", Value: 1.0},
		NVDA:     marketdata.MacroDataPoint{Symbol: "NVDA", Value: 1.0},
		AAPL:     marketdata.MacroDataPoint{Symbol: "AAPL", Value: 1.0},
		MSFT:     marketdata.MacroDataPoint{Symbol: "MSFT", Value: 1.0},
		TSMADR:   marketdata.MacroDataPoint{Symbol: "TSM", Value: 1.0},
		US10Y:    marketdata.MacroDataPoint{Symbol: "^TNX", Value: 4.25},
		VIX:      marketdata.MacroDataPoint{Symbol: "^VIX", Value: 18.0},
	}
	status, failed, stale := detectDegradedUSStatus(snap, nil)
	if status != "ok" {
		t.Errorf("expected status=ok, got %q", status)
	}
	if failed != nil {
		t.Errorf("expected nil failed list when status=ok, got %v", failed)
	}
	if stale != nil {
		t.Errorf("expected nil stale list when no channelErrors, got %v", stale)
	}
}

// TestDetectDegradedUSStatus_StaleOnly_NoFailure locks the new "stale"
// taxonomy: when at least one channel returned cached data (L2 marked it
// with a "stale:" prefix) but no channel fully failed, the status must be
// "stale" with the failed-list empty and the stale-list populated. This
// is the user-visible fix: CB-open serving cached values must surface as
// a warning, not as silently-ok.
func TestDetectDegradedUSStatus_StaleOnly_NoFailure(t *testing.T) {
	snap := marketdata.MacroDataSnapshot{
		SPXIndex: marketdata.MacroDataPoint{Symbol: "^GSPC", Value: 5234.5},
		NDXIndex: marketdata.MacroDataPoint{Symbol: "^IXIC", Value: 18432.1},
		DJIIndex: marketdata.MacroDataPoint{Symbol: "^DJI", Value: 39850.0},
		SOXIndex: marketdata.MacroDataPoint{Symbol: "^SOX", Value: 4890.0},
		NVDA:     marketdata.MacroDataPoint{Symbol: "NVDA", Value: 950.0},
		AAPL:     marketdata.MacroDataPoint{Symbol: "AAPL", Value: 220.0},
		MSFT:     marketdata.MacroDataPoint{Symbol: "MSFT", Value: 415.0},
		TSMADR:   marketdata.MacroDataPoint{Symbol: "TSM", Value: 180.0},
		US10Y:    marketdata.MacroDataPoint{Symbol: "^TNX", Value: 4.25},
		VIX:      marketdata.MacroDataPoint{Symbol: "^VIX", Value: 18.0},
	}
	channelErrors := map[string]string{
		"us_spx": "stale: gateway returned cached data (CB-open or fallback)",
		"vix":    "stale: upstream 503",
		// Non-stale error on a non-checked channel — should be ignored here.
		"some_other_channel": "real error",
	}
	status, failed, stale := detectDegradedUSStatus(snap, channelErrors)
	if status != "stale" {
		t.Errorf("expected status=stale (CB-open serving cache, no real failures), got %q", status)
	}
	if failed != nil {
		t.Errorf("expected nil failed list on stale-only path, got %v", failed)
	}
	if len(stale) != 2 {
		t.Fatalf("expected 2 stale channels (us_spx, vix), got %d: %v", len(stale), stale)
	}
	// extractStaleChannels sorts lexicographically — verify order is stable.
	if stale[0] != "us_spx" || stale[1] != "vix" {
		t.Errorf("expected lexicographic order [us_spx, vix], got %v", stale)
	}
}

// TestDetectDegradedUSStatus_Mixed_StaleAndFailed locks the precedence
// rule: any failure dominates — even if some channels are merely stale,
// the overall status must be "degraded" so the existing alert path fires.
// The stale list is still returned alongside, since users benefit from
// knowing which subset is fresh vs cached vs missing.
func TestDetectDegradedUSStatus_Mixed_StaleAndFailed(t *testing.T) {
	snap := marketdata.MacroDataSnapshot{
		SPXIndex: marketdata.MacroDataPoint{Symbol: "^GSPC", Value: 5234.5},
		NDXIndex: marketdata.MacroDataPoint{Symbol: "^IXIC", Value: 18432.1},
		// DJIIndex empty (failed)
		SOXIndex: marketdata.MacroDataPoint{Symbol: "^SOX", Value: 4890.0},
		NVDA:     marketdata.MacroDataPoint{Symbol: "NVDA", Value: 950.0},
		AAPL:     marketdata.MacroDataPoint{Symbol: "AAPL", Value: 220.0},
		MSFT:     marketdata.MacroDataPoint{Symbol: "MSFT", Value: 415.0},
		TSMADR:   marketdata.MacroDataPoint{Symbol: "TSM", Value: 180.0},
		US10Y:    marketdata.MacroDataPoint{Symbol: "^TNX", Value: 4.25},
		VIX:      marketdata.MacroDataPoint{Symbol: "^VIX", Value: 18.0},
	}
	channelErrors := map[string]string{
		"us_ndx": "stale: gateway returned cached data",
	}
	status, failed, stale := detectDegradedUSStatus(snap, channelErrors)
	if status != "degraded" {
		t.Errorf("expected status=degraded (DJI failed, NDX stale — failure dominates), got %q", status)
	}
	if len(failed) != 1 || failed[0] != "us_dji" {
		t.Errorf("expected failed=[us_dji], got %v", failed)
	}
	if len(stale) != 1 || stale[0] != "us_ndx" {
		t.Errorf("expected stale=[us_ndx] (returned alongside failure), got %v", stale)
	}
}

// TestDetectDegradedUSStatus_FailedOnly_NoStale ensures the negative case:
// when no channel returned "stale:" but some failed, stale list is nil
// (omitempty contract preserved).
func TestDetectDegradedUSStatus_FailedOnly_NoStale(t *testing.T) {
	snap := marketdata.MacroDataSnapshot{
		SPXIndex: marketdata.MacroDataPoint{Symbol: "^GSPC", Value: 5234.5},
		NDXIndex: marketdata.MacroDataPoint{Symbol: "^IXIC", Value: 18432.1},
		// DJIIndex empty
		SOXIndex: marketdata.MacroDataPoint{Symbol: "^SOX", Value: 4890.0},
		NVDA:     marketdata.MacroDataPoint{Symbol: "NVDA", Value: 950.0},
		AAPL:     marketdata.MacroDataPoint{Symbol: "AAPL", Value: 220.0},
		MSFT:     marketdata.MacroDataPoint{Symbol: "MSFT", Value: 415.0},
		TSMADR:   marketdata.MacroDataPoint{Symbol: "TSM", Value: 180.0},
		US10Y:    marketdata.MacroDataPoint{Symbol: "^TNX", Value: 4.25},
		VIX:      marketdata.MacroDataPoint{Symbol: "^VIX", Value: 18.0},
	}
	channelErrors := map[string]string{
		"us_dji": "timeout: real error, not stale",
	}
	status, failed, stale := detectDegradedUSStatus(snap, channelErrors)
	if status != "degraded" {
		t.Errorf("expected status=degraded, got %q", status)
	}
	if len(failed) != 1 || failed[0] != "us_dji" {
		t.Errorf("expected failed=[us_dji], got %v", failed)
	}
	if stale != nil {
		t.Errorf("expected nil stale list (channelErrors had no stale: prefix), got %v", stale)
	}
}

// TestDetectDegradedUSStatus_StaleIgnoredWhenAllFresh ensures the
// "stale" taxonomy fires even when all 10 fields are populated — the L2
// "stale:" marker is the authoritative signal that the data served was
// cached, not freshly fetched. We surface this as a warning rather than
// silently reporting "ok" so users see the CB-open state.
func TestDetectDegradedUSStatus_StaleIgnoredWhenAllFresh(t *testing.T) {
	snap := marketdata.MacroDataSnapshot{
		SPXIndex: marketdata.MacroDataPoint{Symbol: "^GSPC", Value: 5234.5},
		NDXIndex: marketdata.MacroDataPoint{Symbol: "^IXIC", Value: 18432.1},
		DJIIndex: marketdata.MacroDataPoint{Symbol: "^DJI", Value: 39850.0},
		SOXIndex: marketdata.MacroDataPoint{Symbol: "^SOX", Value: 4890.0},
		NVDA:     marketdata.MacroDataPoint{Symbol: "NVDA", Value: 950.0},
		AAPL:     marketdata.MacroDataPoint{Symbol: "AAPL", Value: 220.0},
		MSFT:     marketdata.MacroDataPoint{Symbol: "MSFT", Value: 415.0},
		TSMADR:   marketdata.MacroDataPoint{Symbol: "TSM", Value: 180.0},
		US10Y:    marketdata.MacroDataPoint{Symbol: "^TNX", Value: 4.25},
		VIX:      marketdata.MacroDataPoint{Symbol: "^VIX", Value: 18.0},
	}
	channelErrors := map[string]string{
		"us_spx": "stale: gateway returned cached data",
	}
	status, failed, stale := detectDegradedUSStatus(snap, channelErrors)
	if status != "stale" {
		t.Errorf("expected status=stale when channelErrors has stale: prefix, got %q", status)
	}
	if failed != nil {
		t.Errorf("expected nil failed list, got %v", failed)
	}
	if len(stale) != 1 || stale[0] != "us_spx" {
		t.Errorf("expected stale=[us_spx], got %v", stale)
	}
}

// TestExtractStaleChannels_UnitCases locks the helper in isolation,
// including the empty-map and no-stale-prefix cases.
func TestExtractStaleChannels_UnitCases(t *testing.T) {
	cases := []struct {
		name string
		in   map[string]string
		want []string
	}{
		{"nil map", nil, nil},
		{"empty map", map[string]string{}, nil},
		{"no stale prefix", map[string]string{"ch1": "real error", "ch2": "timeout"}, nil},
		{"single stale", map[string]string{"ch1": "stale: cached"}, []string{"ch1"}},
		{"mixed", map[string]string{
			"ch1": "stale: cached",
			"ch2": "real error",
			"ch3": "stale: fallback",
		}, []string{"ch1", "ch3"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := extractStaleChannels(tc.in)
			if !stringSlicesEqual(got, tc.want) {
				t.Errorf("extractStaleChannels(%v) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestGetStatus_DataStatusJSONMarshaling(t *testing.T) {
	// Build a minimal provider that returns a snapshot with all-zero US fields
	// (simulating the production bug where channels fail).
	provider := &fakeMacroProvider{
		snap: marketdata.MacroDataSnapshot{
			// All 8 original US fields empty (zero-value); VIX is populated.
			// US10Y is also empty → 9 failed total.
			VIX:     marketdata.MacroDataPoint{Symbol: "^VIX", Value: 20.5},
			DXY:     marketdata.MacroDataPoint{Symbol: "DX-Y.NYB", Value: 99.5},
			USD_TWD: marketdata.MacroDataPoint{Symbol: "USDTWD=X", Value: 31.5},
		},
	}
	svc := NewCrossMarketService(provider)
	status, err := svc.GetStatus(context.Background())
	if err != nil {
		t.Fatalf("GetStatus failed: %v", err)
	}

	// Verify new fields populated
	if status.DataStatus != "degraded" {
		t.Errorf("expected DataStatus=degraded, got %q", status.DataStatus)
	}
	if len(status.FailedChannels) != 9 {
		t.Errorf("expected 9 failed channels (8 orig + US10Y; VIX is populated), got %d: %v", len(status.FailedChannels), status.FailedChannels)
	}

	// Verify JSON marshaling includes the new fields
	data, err := json.Marshal(status)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}
	jsonStr := string(data)
	if !strings.Contains(jsonStr, `"data_status":"degraded"`) {
		t.Errorf("expected data_status in JSON, got: %s", jsonStr)
	}
	if !strings.Contains(jsonStr, `"failed_channels":["us_spx"`) {
		t.Errorf("expected failed_channels array in JSON, got: %s", jsonStr)
	}
	// Verify adjacent fields (VIX/DXY/USD_TWD) still appear with values
	if !strings.Contains(jsonStr, `"symbol":"^VIX"`) {
		t.Error("expected VIX symbol in JSON (adjacent regression check)")
	}
	if !strings.Contains(jsonStr, `"value":20.5`) {
		t.Error("expected VIX value in JSON (adjacent regression check)")
	}
}

func TestGetCachedSnapshot_ProviderError_DoesNotPoisonCache(t *testing.T) {
	// Provider fails from the start. The cache must stay empty so a later
	// successful call can populate it (a poisoned cache would freeze the
	// service on the first error).
	prov := &counterProvider{err: context.DeadlineExceeded}
	svc := NewCrossMarketService(prov)

	_, err := svc.GetStatus(context.Background())
	if err == nil {
		t.Fatal("expected error from failing provider, got nil")
	}
	if prov.calls != 1 {
		t.Errorf("expected 1 provider call, got %d", prov.calls)
	}
	svc.cacheMu.Lock()
	cached := svc.cachedSnapshot
	svc.cacheMu.Unlock()
	if cached != nil {
		t.Error("cache must remain empty after provider error (would poison next caller)")
	}

	// Recover the provider and verify the next call populates the cache
	// instead of returning the stale error.
	prov.err = nil
	if _, err := svc.GetStatus(context.Background()); err != nil {
		t.Fatalf("GetStatus after recovery: %v", err)
	}
	if prov.calls != 2 {
		t.Errorf("expected 2 provider calls after recovery, got %d", prov.calls)
	}
}

// TestGetStatus_DataStatusOKWhenAllChannelsPopulated is the post-default-flip
// green path for the PR #484 data-visibility safeguard. With cfg.YahooEnabled
// defaulting to true, the 10 US channels are registered in production, so a
// healthy snapshot will populate all 10 MacroDataPoint.Symbol fields. The
// Layer-3 safeguard (detectDegradedUSStatus) must then return "ok" with no
// failed channels — proving the safeguard's positive path complements (not
// duplicates) the channel-registration fix in apigateway/register_adapters.go.
//
// See: docs/data-sources.md for the default-flip rationale and
// .claude/skills/atlas-data-visibility/SKILL.md for the 4-layer model.
func TestGetStatus_DataStatusOKWhenAllChannelsPopulated(t *testing.T) {
	healthy := marketdata.MacroDataSnapshot{
		SPXIndex: marketdata.MacroDataPoint{Symbol: "^GSPC", Value: 5234.5, ChangePct: 0.4, Timestamp: time.Now().Unix()},
		NDXIndex: marketdata.MacroDataPoint{Symbol: "^IXIC", Value: 18432.1, ChangePct: 0.6, Timestamp: time.Now().Unix()},
		DJIIndex: marketdata.MacroDataPoint{Symbol: "^DJI", Value: 39850.0, ChangePct: 0.2, Timestamp: time.Now().Unix()},
		SOXIndex: marketdata.MacroDataPoint{Symbol: "^SOX", Value: 4890.0, ChangePct: 1.1, Timestamp: time.Now().Unix()},
		NVDA:     marketdata.MacroDataPoint{Symbol: "NVDA", Value: 950.0, ChangePct: 1.2, Timestamp: time.Now().Unix()},
		AAPL:     marketdata.MacroDataPoint{Symbol: "AAPL", Value: 220.0, ChangePct: 0.3, Timestamp: time.Now().Unix()},
		MSFT:     marketdata.MacroDataPoint{Symbol: "MSFT", Value: 415.0, ChangePct: 0.5, Timestamp: time.Now().Unix()},
		TSMADR:   marketdata.MacroDataPoint{Symbol: "TSM", Value: 180.0, ChangePct: 0.8, Timestamp: time.Now().Unix()},
		// US10Y and VIX are now checked by detectDegradedUSStatus (Fix 3)
		US10Y: marketdata.MacroDataPoint{Symbol: "^TNX", Value: 4.25, ChangePct: 0.1, Timestamp: time.Now().Unix()},
		VIX:   marketdata.MacroDataPoint{Symbol: "^VIX", Value: 18.0, ChangePct: -0.4, Timestamp: time.Now().Unix()},
		// Adjacent fields unaffected by US channel failures
		DXY:     marketdata.MacroDataPoint{Symbol: "DX-Y.NYB", Value: 99.5, Timestamp: time.Now().Unix()},
		USD_TWD: marketdata.MacroDataPoint{Symbol: "USDTWD=X", Value: 31.5, Timestamp: time.Now().Unix()},
	}
	prov := &fakeMacroProvider{snap: healthy}
	svc := NewCrossMarketService(prov)

	status, err := svc.GetStatus(context.Background())
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}

	if status.DataStatus != "ok" {
		t.Errorf("expected DataStatus=ok (all 10 US channels populated), got %q", status.DataStatus)
	}
	if len(status.FailedChannels) != 0 {
		t.Errorf("expected no failed channels when DataStatus=ok, got %v", status.FailedChannels)
	}

	if status.VIX.Value != 18.0 {
		t.Errorf("VIX.Value = %v, expected 18.0 (sibling regression check)", status.VIX.Value)
	}
	if status.DXY.Symbol != "DX-Y.NYB" {
		t.Errorf("DXY.Symbol = %q, expected DX-Y.NYB (sibling regression check)", status.DXY.Symbol)
	}

	data, err := json.Marshal(status)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	jsonStr := string(data)
	if !strings.Contains(jsonStr, `"data_status":"ok"`) {
		t.Errorf("expected data_status=ok in JSON, got: %s", jsonStr)
	}
	if strings.Contains(jsonStr, `"failed_channels":`) {
		t.Errorf("failed_channels must be omitted when DataStatus=ok, got: %s", jsonStr)
	}
}

// TestDetectDegradedUSStatus_ZeroValueNonNullSymbol isolates the Value<=0
// branch from the Symbol=="" branch. In the production bug, VIX had
// Symbol="^VIX" but Value=0, which must be detected as failed.
func TestDetectDegradedUSStatus_ZeroValueNonNullSymbol(t *testing.T) {
	snap := marketdata.MacroDataSnapshot{
		SPXIndex: marketdata.MacroDataPoint{Symbol: "^GSPC", Value: 5234.5},
		NDXIndex: marketdata.MacroDataPoint{Symbol: "^IXIC", Value: 18432.1},
		DJIIndex: marketdata.MacroDataPoint{Symbol: "^DJI", Value: 39850.0},
		SOXIndex: marketdata.MacroDataPoint{Symbol: "^SOX", Value: 0}, // Zero value — should fail
		NVDA:     marketdata.MacroDataPoint{Symbol: "NVDA", Value: 950.0},
		AAPL:     marketdata.MacroDataPoint{Symbol: "AAPL", Value: 220.0},
		MSFT:     marketdata.MacroDataPoint{Symbol: "MSFT", Value: 415.0},
		TSMADR:   marketdata.MacroDataPoint{Symbol: "TSM", Value: 180.0},
		US10Y:    marketdata.MacroDataPoint{Symbol: "^TNX", Value: 4.25},
		VIX:      marketdata.MacroDataPoint{Symbol: "^VIX", Value: 0}, // Zero value — should fail
	}
	status, failed, stale := detectDegradedUSStatus(snap, nil)
	if status != "degraded" {
		t.Errorf("expected status=degraded (SOX and VIX are Value=0), got %q", status)
	}
	if len(failed) != 2 {
		t.Errorf("expected 2 failed channels (sox_index, vix), got %d: %v", len(failed), failed)
	}
	if stale != nil {
		t.Errorf("expected nil stale when no channelErrors, got %v", stale)
	}
}

// TestSetDegradedCallback_InvokedOnDegradation verifies that the degradedCallback
// is called when detectDegradedUSStatus returns "degraded".
func TestSetDegradedCallback_InvokedOnDegradation(t *testing.T) {
	// All fields empty → all 10 degraded
	prov := &fakeMacroProvider{snap: marketdata.MacroDataSnapshot{}}
	svc := NewCrossMarketService(prov)

	var calledStatus string
	var calledFailed []string
	svc.SetDegradedCallback(func(s string, f []string) {
		calledStatus = s
		calledFailed = f
	})

	_, err := svc.GetStatus(context.Background())
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}

	if calledStatus != "degraded" {
		t.Errorf("expected callback status='degraded', got %q", calledStatus)
	}
	if len(calledFailed) != 10 {
		t.Errorf("expected 10 failed channels in callback, got %d: %v", len(calledFailed), calledFailed)
	}
}

// TestSetDegradedCallback_NilCallbackNoPanic verifies that no panic occurs
// when degradedCallback is nil and status is "degraded".
func TestSetDegradedCallback_NilCallbackNoPanic(t *testing.T) {
	// Empty snapshot → degraded; no callback set → must not panic
	svc := NewCrossMarketService(&fakeMacroProvider{snap: marketdata.MacroDataSnapshot{}})
	_, err := svc.GetStatus(context.Background())
	if err != nil {
		t.Fatalf("GetStatus with nil callback: %v", err)
	}
	// If we reach here without panic, the test passes.
}

// TestSetDegradedCallback_NotInvokedWhenOKAfterFirst verifies that the
// first-OK recovery callback (PR-F) does NOT re-fire on every cache hit.
//
// The first observation of "ok" fires the callback (so the gateway can
// clear stale degraded records from a previous process). After that,
// subsequent calls within the 30s cache window hit the cache at L233
// and never re-enter L253, so the callback is not re-invoked. This
// protects against alert spam.
func TestSetDegradedCallback_NotInvokedWhenOKAfterFirst(t *testing.T) {
	prov := &fakeMacroProvider{snap: marketdata.MacroDataSnapshot{
		SPXIndex: marketdata.MacroDataPoint{Symbol: "^GSPC", Value: 5234.5},
		NDXIndex: marketdata.MacroDataPoint{Symbol: "^IXIC", Value: 18432.1},
		DJIIndex: marketdata.MacroDataPoint{Symbol: "^DJI", Value: 39850.0},
		SOXIndex: marketdata.MacroDataPoint{Symbol: "^SOX", Value: 4890.0},
		NVDA:     marketdata.MacroDataPoint{Symbol: "NVDA", Value: 950.0},
		AAPL:     marketdata.MacroDataPoint{Symbol: "AAPL", Value: 220.0},
		MSFT:     marketdata.MacroDataPoint{Symbol: "MSFT", Value: 415.0},
		TSMADR:   marketdata.MacroDataPoint{Symbol: "TSM", Value: 180.0},
		US10Y:    marketdata.MacroDataPoint{Symbol: "^TNX", Value: 4.25},
		VIX:      marketdata.MacroDataPoint{Symbol: "^VIX", Value: 18.0},
	}}
	svc := NewCrossMarketService(prov)

	callCount := 0
	svc.SetDegradedCallback(func(string, []string) { callCount++ })

	// First call: should fire the recovery callback (first-OK path, PR-F).
	if _, err := svc.GetStatus(context.Background()); err != nil {
		t.Fatalf("first GetStatus: %v", err)
	}
	if callCount != 1 {
		t.Errorf("first-OK should fire callback exactly once, got %d", callCount)
	}

	// Second call within cache window: should NOT re-fire.
	if _, err := svc.GetStatus(context.Background()); err != nil {
		t.Fatalf("second GetStatus: %v", err)
	}
	if callCount != 1 {
		t.Errorf("cached OK snapshot should NOT re-fire callback, got %d calls", callCount)
	}
}

// TestSetDegradedCallback_RecoveryFiresOnOK verifies the recovery path added
// 2026-08-04: when the snapshot transitions from degraded/stale back to
// ok, the callback must fire so the dashboard can clear per-channel
// "degraded" health records (regression: us10y / vix were stuck on
// "degraded" for 9 days because no recovery path existed).
func TestSetDegradedCallback_RecoveryFiresOnOK(t *testing.T) {
	// Phase 1: degraded snapshot → callback fires once.
	degradedProv := &fakeMacroProvider{snap: marketdata.MacroDataSnapshot{}}
	svc := NewCrossMarketService(degradedProv)
	var fired []string
	svc.SetDegradedCallback(func(status string, failed []string) {
		fired = append(fired, status)
	})
	if _, err := svc.GetStatus(context.Background()); err != nil {
		t.Fatalf("phase 1 GetStatus: %v", err)
	}
	if len(fired) != 1 || fired[0] == "ok" {
		t.Fatalf("phase 1: expected degraded callback fire, got %v", fired)
	}

	// Phase 2: healthy snapshot → recovery callback fires (status="ok").
	// Force the cache to expire so the new provider is actually queried.
	healthyProv := &fakeMacroProvider{snap: marketdata.MacroDataSnapshot{
		SPXIndex: marketdata.MacroDataPoint{Symbol: "^GSPC", Value: 5234.5},
		NDXIndex: marketdata.MacroDataPoint{Symbol: "^IXIC", Value: 18432.1},
		DJIIndex: marketdata.MacroDataPoint{Symbol: "^DJI", Value: 39850.0},
		SOXIndex: marketdata.MacroDataPoint{Symbol: "^SOX", Value: 4890.0},
		NVDA:     marketdata.MacroDataPoint{Symbol: "NVDA", Value: 950.0},
		AAPL:     marketdata.MacroDataPoint{Symbol: "AAPL", Value: 220.0},
		MSFT:     marketdata.MacroDataPoint{Symbol: "MSFT", Value: 415.0},
		TSMADR:   marketdata.MacroDataPoint{Symbol: "TSM", Value: 180.0},
		US10Y:    marketdata.MacroDataPoint{Symbol: "^TNX", Value: 4.25},
		VIX:      marketdata.MacroDataPoint{Symbol: "^VIX", Value: 18.0},
	}}
	svc.provider = healthyProv // swap provider — same service, new snapshot
	svc.cachedSnapshot = nil   // bypass the 30s cache so the new snapshot is fetched
	svc.cachedStatusMeta = nil
	if _, err := svc.GetStatus(context.Background()); err != nil {
		t.Fatalf("phase 2 GetStatus: %v", err)
	}
	if len(fired) != 2 || fired[1] != "ok" {
		t.Fatalf("phase 2: expected recovery callback fire with status='ok', got %v", fired)
	}
}

// TestSetDegradedCallback_FirstSnapshotOK_StillAllowsRecovery reproduces
// the bug that kept us10y / vix stuck on "degraded" for 9+ days
// (PR-F kaecer 2026-08-05):
//
//  1. Container restart → process loads channel_health.json from disk
//     → still has stale "degraded" records for us10y / vix from the
//     previous process.
//  2. The very first snapshot the new process fetches is already
//     healthy (us10y val=4.25, vix val=18.0 are non-zero).
//  3. shouldFire condition (crossmarket.go L278) requires
//     prevStatus != "" for the recovery path, so the first call
//     with prevStatus="" never fires the recovery callback.
//  4. Subsequent calls hit the 30s cache (L233-242) so L278 is
//     never re-evaluated; the stale "degraded" records persist
//     indefinitely.
//
// The fix is to fire the callback on first-OK observation so the
// gateway can clear any stale "degraded" health records. PR-F
// does NOT widen the union-list (nothing to union with on first
// call) — the gateway-side main.go:1133 fix is responsible for
// knowing which channels to clear on a first-OK observation.
func TestSetDegradedCallback_FirstSnapshotOK_StillAllowsRecovery(t *testing.T) {
	prov := &fakeMacroProvider{snap: marketdata.MacroDataSnapshot{
		SPXIndex: marketdata.MacroDataPoint{Symbol: "^GSPC", Value: 5234.5},
		NDXIndex: marketdata.MacroDataPoint{Symbol: "^IXIC", Value: 18432.1},
		DJIIndex: marketdata.MacroDataPoint{Symbol: "^DJI", Value: 39850.0},
		SOXIndex: marketdata.MacroDataPoint{Symbol: "^SOX", Value: 4890.0},
		NVDA:     marketdata.MacroDataPoint{Symbol: "NVDA", Value: 950.0},
		AAPL:     marketdata.MacroDataPoint{Symbol: "AAPL", Value: 220.0},
		MSFT:     marketdata.MacroDataPoint{Symbol: "MSFT", Value: 415.0},
		TSMADR:   marketdata.MacroDataPoint{Symbol: "TSM", Value: 180.0},
		US10Y:    marketdata.MacroDataPoint{Symbol: "^TNX", Value: 4.25},
		VIX:      marketdata.MacroDataPoint{Symbol: "^VIX", Value: 18.0},
	}}
	svc := NewCrossMarketService(prov)

	var firedStatuses []string
	svc.SetDegradedCallback(func(status string, failed []string) {
		firedStatuses = append(firedStatuses, status)
	})

	if _, err := svc.GetStatus(context.Background()); err != nil {
		t.Fatalf("GetStatus: %v", err)
	}

	// PR-F: first-OK observation must fire the callback so the gateway
	// can clear any stale "degraded" health records from a previous
	// process. Without this, us10y / vix stay flagged degraded forever
	// after the upstream data recovers.
	if len(firedStatuses) == 0 {
		t.Fatalf("first-snapshot-OK did not fire callback — gateway cannot clear stale degraded records from previous process (regression: us10y/vix stuck 9+ days)")
	}
	if firedStatuses[0] != "ok" {
		t.Errorf("expected first callback status=\"ok\", got %q", firedStatuses[0])
	}
}

// TestDegradedCallbackCount_InvokedOnDegradation verifies that the
// DegradedCallbackCount counter increments with the expected labels when the
// degraded callback fires.
func TestDegradedCallbackCount_InvokedOnDegradation(t *testing.T) {
	prov := &fakeMacroProvider{snap: marketdata.MacroDataSnapshot{}}
	svc := NewCrossMarketService(prov)

	m := metrics.NewDegradedMetrics()
	svc.SetDegradedMetrics(m)
	svc.SetDegradedCallback(func(string, []string) {})

	_, err := svc.GetStatus(context.Background())
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}

	got := m.DegradedCallbackCount.WithLabelValues("crossmarket", "missing_us_index_data", "degraded").Value()
	if got != 1 {
		t.Errorf("DegradedCallbackCount = %v, want 1", got)
	}
}

// TestDegradedCallbackCount_NotInvokedWhenOK verifies that the
// DegradedCallbackCount counter is NOT incremented when all channels are
// healthy and the callback does not fire.
func TestDegradedCallbackCount_NotInvokedWhenOK(t *testing.T) {
	prov := &fakeMacroProvider{snap: marketdata.MacroDataSnapshot{
		SPXIndex: marketdata.MacroDataPoint{Symbol: "^GSPC", Value: 5234.5},
		NDXIndex: marketdata.MacroDataPoint{Symbol: "^IXIC", Value: 18432.1},
		DJIIndex: marketdata.MacroDataPoint{Symbol: "^DJI", Value: 39850.0},
		SOXIndex: marketdata.MacroDataPoint{Symbol: "^SOX", Value: 4890.0},
		NVDA:     marketdata.MacroDataPoint{Symbol: "NVDA", Value: 950.0},
		AAPL:     marketdata.MacroDataPoint{Symbol: "AAPL", Value: 220.0},
		MSFT:     marketdata.MacroDataPoint{Symbol: "MSFT", Value: 415.0},
		TSMADR:   marketdata.MacroDataPoint{Symbol: "TSM", Value: 180.0},
		US10Y:    marketdata.MacroDataPoint{Symbol: "^TNX", Value: 4.25},
		VIX:      marketdata.MacroDataPoint{Symbol: "^VIX", Value: 18.0},
	}}
	svc := NewCrossMarketService(prov)

	m := metrics.NewDegradedMetrics()
	svc.SetDegradedMetrics(m)
	svc.SetDegradedCallback(func(string, []string) {})

	_, err := svc.GetStatus(context.Background())
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}

	got := m.DegradedCallbackCount.WithLabelValues("crossmarket", "missing_us_index_data", "degraded").Value()
	if got != 0 {
		t.Errorf("DegradedCallbackCount = %v, want 0", got)
	}
}

// TestUSMacroFields_MatchesDetectorChannels pins the USMacroFields
// slice to the same 10 channel IDs that detectDegradedUSStatus
// monitors. PR-F (kaecer 2026-08-05) introduced this slice so the
// main.go recovery callback can iterate it to clear stale "degraded"
// records when the snapshot is healthy. If a future commit adds a
// channel to detectDegradedUSStatus without updating USMacroFields,
// stale degraded records will accumulate again — this test catches
// that drift.
func TestUSMacroFields_MatchesDetectorChannels(t *testing.T) {
	// Provoke the detector by giving it an all-empty snapshot. All 10
	// fields will be marked failed, so we can compare the set of failed
	// channel IDs against USMacroFields.
	prov := &fakeMacroProvider{snap: marketdata.MacroDataSnapshot{}}
	svc := NewCrossMarketService(prov)

	var failedChannels []string
	svc.SetDegradedCallback(func(status string, failed []string) {
		if status == "degraded" {
			failedChannels = failed
		}
	})
	if _, err := svc.GetStatus(context.Background()); err != nil {
		t.Fatalf("GetStatus: %v", err)
	}
	if len(failedChannels) != len(USMacroFields) {
		t.Fatalf("detector marked %d channels failed, but USMacroFields has %d entries: detector=%v macrofields=%v",
			len(failedChannels), len(USMacroFields), failedChannels, USMacroFields)
	}
	detected := make(map[string]bool, len(failedChannels))
	for _, ch := range failedChannels {
		detected[ch] = true
	}
	for _, ch := range USMacroFields {
		if !detected[ch] {
			t.Errorf("USMacroFields entry %q is not monitored by detectDegradedUSStatus — stale records for this channel will never be cleared by the recovery callback", ch)
		}
	}
	for ch := range detected {
		found := false
		for _, m := range USMacroFields {
			if m == ch {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("detectDegradedUSStatus marks %q as failed but USMacroFields does not list it — main.go recovery loop will not clear this channel", ch)
		}
	}
}
