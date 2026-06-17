package service

import (
	"context"
	"encoding/json"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// fakeMacroProvider is a stub MacroDataProvider for testing CrossMarketService.
type fakeMacroProvider struct {
	snap marketdata.MacroDataSnapshot
	err  error
}

func (f *fakeMacroProvider) Name() string { return "fake" }
func (f *fakeMacroProvider) FetchSnapshot(_ context.Context) (marketdata.MacroDataSnapshot, error) {
	if f.err != nil {
		return marketdata.MacroDataSnapshot{}, f.err
	}
	return f.snap, nil
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
	status, failed := detectDegradedUSStatus(snap)
	if status != "degraded" {
		t.Errorf("expected status=degraded, got %q", status)
	}
	if len(failed) != 10 {
		t.Errorf("expected 10 failed channels, got %d: %v", len(failed), failed)
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
	status, failed := detectDegradedUSStatus(snap)
	if status != "degraded" {
		t.Errorf("expected status=degraded, got %q", status)
	}
	if len(failed) != 6 {
		t.Errorf("expected 6 failed channels (DJI, SOX, MSFT, TSM, US10Y, VIX), got %d: %v", len(failed), failed)
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
	status, failed := detectDegradedUSStatus(snap)
	if status != "ok" {
		t.Errorf("expected status=ok, got %q", status)
	}
	if failed != nil {
		t.Errorf("expected nil failed list when status=ok, got %v", failed)
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
// defaulting to true, the 8 US channels are registered in production, so a
// healthy snapshot will populate all 10 MacroDataPoint.Symbol fields. The
// Layer-3 safeguard (detectDegradedUSStatus) must then return "ok" with no
// failed channels — proving the safeguard's positive path complements (not
// duplicates) the channel-registration fix in apigateway/register_adapters.go.
//
// See: docs/data_sources.md for the default-flip rationale and
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
		US10Y:    marketdata.MacroDataPoint{Symbol: "^TNX", Value: 4.25, ChangePct: 0.1, Timestamp: time.Now().Unix()},
		// Adjacent fields unaffected by US channel failures
		VIX:     marketdata.MacroDataPoint{Symbol: "^VIX", Value: 18.0, ChangePct: -0.4, Timestamp: time.Now().Unix()},
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
