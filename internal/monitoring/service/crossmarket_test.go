package service

import (
	"context"
	"encoding/json"
	"math"
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

func TestUpdateAllCorrelations_NilSafe(t *testing.T) {
	// Nil receiver must not panic. This protects the BTM task from
	// startup-race crashes when the service hasn't been wired yet.
	var svc *CrossMarketService
	svc.UpdateAllCorrelations(makeSnapshot())
}

func TestGetStatus_PopulatesAllFiveNewCorrelationsAfterMinObs(t *testing.T) {
	prov := &fakeMacroProvider{snap: makeSnapshot()}
	svc := NewCrossMarketService(prov)

	// Push minObservationsForReport observations to make the new
	// correlation fields reportable (not null).
	for i := 0; i < minObservationsForReport; i++ {
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
		if !containsJSONField(jsonStr, `"`+field+`":null`) {
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
	if status.CorrelationSPXTWSE < 0.5 || status.CorrelationSPXTWSE > 1.0 {
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

func containsJSONField(s, substr string) bool {
	return len(s) >= len(substr) && stringIndex(s, substr) >= 0
}

func stringIndex(s, substr string) int {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
