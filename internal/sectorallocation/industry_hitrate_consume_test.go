package sectorallocation_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/industry"
	"github.com/kaecer68/atlas-go/internal/sectorallocation"
)

// ---- fixtures -------------------------------------------------------------

// fakeHitRateProvider is a deterministic test double for the consumption port.
// calls counts provider hits so the gate-off test can prove the provider is
// never consulted.
type fakeHitRateProvider struct {
	rows  []sectorallocation.IndustryHitRateSummary
	err   error
	calls int
}

func (f *fakeHitRateProvider) LoadIndustryWinRate(source, conditionID, rollingWindow string) (sectorallocation.IndustryHitRateReport, error) {
	f.calls++
	if f.err != nil {
		return sectorallocation.IndustryHitRateReport{}, f.err
	}
	return sectorallocation.IndustryHitRateReport{Industries: f.rows}, nil
}

// withHitRateGate loads a parameters config whose
// sector_allocation.industry_hit_rate_consume_enabled value is `enabled`, and
// restores the previous config on cleanup.
func withHitRateGate(t *testing.T, enabled bool) {
	t.Helper()
	prevPath := config.GetParametersConfigPath()
	cfg := config.DefaultParametersConfig()
	cfg.SectorAllocation.IndustryHitRateConsumeEnabled.Value = enabled
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("marshal test parameters: %v", err)
	}
	path := filepath.Join(t.TempDir(), "parameters.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write test parameters: %v", err)
	}
	config.SetParametersConfigPath(path)
	config.ResetParametersConfig()
	t.Cleanup(func() {
		config.SetParametersConfigPath(prevPath)
		config.ResetParametersConfig()
	})
	if got := config.GetIndustryHitRateConsumeEnabled(); got != enabled {
		t.Fatalf("gate did not load: got %v, want %v", got, enabled)
	}
}

func eligibleRow(id, direction string, wilsonLower float64, obs int) sectorallocation.IndustryHitRateSummary {
	return sectorallocation.IndustryHitRateSummary{
		IndustryID:        id,
		Direction:         direction,
		WilsonLower:       wilsonLower,
		WilsonUpper:       wilsonLower + 0.1,
		WinRate:           wilsonLower + 0.05,
		Observations:      obs,
		CalibrationStatus: sectorallocation.IndustryHitRateCalibrationEligible,
	}
}

func calibratingRow(id string, wilsonLower float64, obs int) sectorallocation.IndustryHitRateSummary {
	row := eligibleRow(id, "buy", wilsonLower, obs)
	row.CalibrationStatus = "calibrating"
	return row
}

func findTilt(t *testing.T, tilts []sectorallocation.IndustryHitRateTilt, id string) sectorallocation.IndustryHitRateTilt {
	t.Helper()
	for _, t2 := range tilts {
		if t2.IndustryID == id {
			return t2
		}
	}
	t.Fatalf("no tilt for %s in %+v", id, tilts)
	return sectorallocation.IndustryHitRateTilt{}
}

func almostEqual(a, b float64) bool { return a-b < 1e-12 && b-a < 1e-12 }

// ---- gate-off / fail-closed ----------------------------------------------

func TestEvaluateIndustryHitRateConsume_GateOff_IsDisabledAndNeverReadsProvider(t *testing.T) {
	withHitRateGate(t, false)
	provider := &fakeHitRateProvider{rows: []sectorallocation.IndustryHitRateSummary{eligibleRow("semiconductor", "buy", 0.9, 200)}}

	decision := sectorallocation.EvaluateIndustryHitRateConsume(provider, "src", "cond", "120d")

	if decision.Applied {
		t.Fatalf("gate off must not apply, got %+v", decision)
	}
	if decision.Reason != sectorallocation.IndustryHitRateReasonDisabled {
		t.Fatalf("reason = %q, want %q", decision.Reason, sectorallocation.IndustryHitRateReasonDisabled)
	}
	if provider.calls != 0 {
		t.Fatalf("gate off must not consult the provider, calls = %d", provider.calls)
	}
}

func TestEvaluateIndustryHitRateConsume_NoProvider(t *testing.T) {
	withHitRateGate(t, true)

	decision := sectorallocation.EvaluateIndustryHitRateConsume(nil, "src", "cond", "120d")

	if decision.Applied || decision.Reason != sectorallocation.IndustryHitRateReasonNoProvider {
		t.Fatalf("decision = %+v, want not applied / no_provider", decision)
	}
}

func TestEvaluateIndustryHitRateConsume_ProviderError(t *testing.T) {
	withHitRateGate(t, true)
	wantErr := errors.New("ledger unavailable")
	provider := &fakeHitRateProvider{err: wantErr}

	decision := sectorallocation.EvaluateIndustryHitRateConsume(provider, "src", "cond", "120d")

	if decision.Applied || decision.Reason != sectorallocation.IndustryHitRateReasonProviderError {
		t.Fatalf("decision = %+v, want not applied / provider_error", decision)
	}
	if !errors.Is(decision.Err, wantErr) {
		t.Fatalf("err = %v, want %v", decision.Err, wantErr)
	}
}

func TestEvaluateIndustryHitRateConsume_NoRows(t *testing.T) {
	withHitRateGate(t, true)

	decision := sectorallocation.EvaluateIndustryHitRateConsume(&fakeHitRateProvider{}, "src", "cond", "120d")

	if decision.Applied || decision.Reason != sectorallocation.IndustryHitRateReasonNoRows {
		t.Fatalf("decision = %+v, want not applied / no_rows", decision)
	}
}

// TestEvaluateIndustryHitRateConsume_InsufficientCalibration is the
// fail-closed requirement: rows below the canonical min_samples gate must not
// produce a tilt (no 0-substitution, no guessing).
func TestEvaluateIndustryHitRateConsume_InsufficientCalibration(t *testing.T) {
	withHitRateGate(t, true)
	provider := &fakeHitRateProvider{rows: []sectorallocation.IndustryHitRateSummary{
		calibratingRow("semiconductor", 0.95, 4),
		calibratingRow("financials", 0.10, 12),
	}}

	decision := sectorallocation.EvaluateIndustryHitRateConsume(provider, "src", "cond", "120d")

	if decision.Applied {
		t.Fatalf("uncalibrated rows must not be applied, got %+v", decision)
	}
	if decision.Reason != sectorallocation.IndustryHitRateReasonInsufficient {
		t.Fatalf("reason = %q, want %q", decision.Reason, sectorallocation.IndustryHitRateReasonInsufficient)
	}
	if decision.RowsTotal != 2 || decision.RowsCalibrated != 0 {
		t.Fatalf("rows_total/rows_calibrated = %d/%d, want 2/0", decision.RowsTotal, decision.RowsCalibrated)
	}
	if len(decision.Tilts) != 0 {
		t.Fatalf("tilts = %+v, want none", decision.Tilts)
	}
}

// ---- on path --------------------------------------------------------------

func TestEvaluateIndustryHitRateConsume_Applied(t *testing.T) {
	withHitRateGate(t, true)
	provider := &fakeHitRateProvider{rows: []sectorallocation.IndustryHitRateSummary{
		eligibleRow("semiconductor", "buy", 0.7, 120),     // +0.04
		eligibleRow("financials", "avoid", 0.7, 90),       // -0.04 (inverted)
		eligibleRow("steel", "buy", 0.5, 60),              // 0.0 (no edge)
		calibratingRow("shipping", 0.99, 3),               // dropped: uncalibrated
		eligibleRow("not_a_canonical_l1", "buy", 0.9, 50), // dropped: non-L1
	}}

	decision := sectorallocation.EvaluateIndustryHitRateConsume(provider, "src", "cond", "120d")

	if !decision.Applied || decision.Reason != sectorallocation.IndustryHitRateReasonApplied {
		t.Fatalf("decision = %+v, want applied", decision)
	}
	// rows_total counts every row the provider returned; rows_calibrated counts
	// the rows the chain may consume (canonical L1 AND calibration eligible).
	if decision.RowsTotal != 5 || decision.RowsCalibrated != 3 {
		t.Fatalf("rows_total/rows_calibrated = %d/%d, want 5/3", decision.RowsTotal, decision.RowsCalibrated)
	}
	if len(decision.Tilts) != 3 {
		t.Fatalf("tilts = %+v, want 3 (uncalibrated + non-L1 dropped)", decision.Tilts)
	}
	if got := findTilt(t, decision.Tilts, "semiconductor").TiltMagnitude; !almostEqual(got, 0.04) {
		t.Errorf("semiconductor tilt = %v, want 0.04", got)
	}
	if got := findTilt(t, decision.Tilts, "financials").TiltMagnitude; !almostEqual(got, -0.04) {
		t.Errorf("financials (avoid) tilt = %v, want -0.04", got)
	}
	if got := findTilt(t, decision.Tilts, "steel").TiltMagnitude; !almostEqual(got, 0) {
		t.Errorf("steel tilt = %v, want 0 (no edge at WilsonLower 0.5)", got)
	}
}

// TestEvaluateIndustryHitRateConsume_TiltCap pins the clamp: one row can never
// move a sector by more than +/-0.05, whatever the Wilson bound says.
func TestEvaluateIndustryHitRateConsume_TiltCap(t *testing.T) {
	withHitRateGate(t, true)
	provider := &fakeHitRateProvider{rows: []sectorallocation.IndustryHitRateSummary{
		eligibleRow("semiconductor", "buy", 0.0, 100), // raw -0.10 -> clamped
		eligibleRow("financials", "buy", 1.0, 100),    // raw +0.10 -> clamped
	}}

	decision := sectorallocation.EvaluateIndustryHitRateConsume(provider, "src", "cond", "120d")

	if got := findTilt(t, decision.Tilts, "semiconductor").TiltMagnitude; !almostEqual(got, -0.05) {
		t.Errorf("semiconductor tilt = %v, want -0.05 (clamped)", got)
	}
	if got := findTilt(t, decision.Tilts, "financials").TiltMagnitude; !almostEqual(got, 0.05) {
		t.Errorf("financials tilt = %v, want 0.05 (clamped)", got)
	}
}

func TestBuildIndustryHitRateTilt_GateOff_ReturnsNil(t *testing.T) {
	withHitRateGate(t, false)
	provider := &fakeHitRateProvider{rows: []sectorallocation.IndustryHitRateSummary{eligibleRow("semiconductor", "buy", 0.9, 200)}}

	tilts, err := sectorallocation.BuildIndustryHitRateTilt(provider, "src", "cond", "120d")

	if err != nil {
		t.Fatalf("gate off must not error: %v", err)
	}
	if tilts != nil {
		t.Fatalf("gate off must return nil tilts, got %+v", tilts)
	}
}

func TestBuildIndustryHitRateTilt_FailClosed_ReturnsNil(t *testing.T) {
	withHitRateGate(t, true)
	provider := &fakeHitRateProvider{rows: []sectorallocation.IndustryHitRateSummary{calibratingRow("semiconductor", 0.9, 5)}}

	tilts, err := sectorallocation.BuildIndustryHitRateTilt(provider, "src", "cond", "120d")

	if err != nil {
		t.Fatalf("fail-closed must not error: %v", err)
	}
	if tilts != nil {
		t.Fatalf("fail-closed must return nil tilts, got %+v", tilts)
	}
}

func TestTiltToDriverMap_EmptyReturnsNil(t *testing.T) {
	if m := sectorallocation.TiltToDriverMap(nil); m != nil {
		t.Fatalf("TiltToDriverMap(nil) = %v, want nil", m)
	}
	if m := sectorallocation.TiltToDriverMap([]sectorallocation.IndustryHitRateTilt{}); m != nil {
		t.Fatalf("TiltToDriverMap(empty) = %v, want nil", m)
	}
}

func TestTiltToDriverMap_RejectsNonL1Keys(t *testing.T) {
	tilts := []sectorallocation.IndustryHitRateTilt{
		{IndustryID: "semiconductor", TiltMagnitude: 0.05},
		{IndustryID: "bogus_subindustry", TiltMagnitude: 0.99},
		{IndustryID: "financials", TiltMagnitude: -0.03},
	}

	m := sectorallocation.TiltToDriverMap(tilts)

	if len(m) != 2 {
		t.Fatalf("map = %v, want only the two canonical L1 keys", m)
	}
	if _, ok := m[industry.SectorSemiconductor]; !ok {
		t.Fatalf("semiconductor missing from %v", m)
	}
	if _, ok := m["bogus_subindustry"]; ok {
		t.Fatalf("non-L1 key leaked into %v", m)
	}
	if m[industry.SectorFinancials] != -0.03 {
		t.Fatalf("financials tilt = %v, want -0.03", m[industry.SectorFinancials])
	}
}

// TestApplyIndustryHitRateToDrivers_GateOff_DoesNotTouchInput pins the
// byte-identity guarantee at the driver level: with the gate off the returned
// DriverInputs is exactly the input (same JSON), and the caller's map is not
// written to.
func TestApplyIndustryHitRateToDrivers_GateOff_DoesNotTouchInput(t *testing.T) {
	withHitRateGate(t, false)
	provider := &fakeHitRateProvider{rows: []sectorallocation.IndustryHitRateSummary{eligibleRow("semiconductor", "buy", 0.9, 200)}}
	in := sectorallocation.DriverInputs{
		AsOfTradingDate: "2026-07-17",
		CapitalFlow:     map[industry.SectorID]float64{industry.SectorFinancials: 0.05},
	}

	out, err := sectorallocation.ApplyIndustryHitRateToDrivers(in, provider, "src", "cond", "120d")

	if err != nil {
		t.Fatalf("gate off must not error: %v", err)
	}
	before, _ := json.Marshal(in)
	after, _ := json.Marshal(out)
	if string(before) != string(after) {
		t.Fatalf("gate off changed drivers:\n before=%s\n after =%s", before, after)
	}
	if provider.calls != 0 {
		t.Fatalf("gate off must not consult the provider, calls = %d", provider.calls)
	}
	if len(in.CapitalFlow) != 1 || in.CapitalFlow[industry.SectorFinancials] != 0.05 {
		t.Fatalf("gate off mutated the caller map: %v", in.CapitalFlow)
	}
}

func TestApplyIndustryHitRateToDrivers_FailClosed_DoesNotTouchInput(t *testing.T) {
	withHitRateGate(t, true)
	provider := &fakeHitRateProvider{rows: []sectorallocation.IndustryHitRateSummary{calibratingRow("semiconductor", 0.95, 3)}}
	in := sectorallocation.DriverInputs{
		AsOfTradingDate: "2026-07-17",
		CapitalFlow:     map[industry.SectorID]float64{industry.SectorFinancials: 0.05},
	}

	out, err := sectorallocation.ApplyIndustryHitRateToDrivers(in, provider, "src", "cond", "120d")

	if err != nil {
		t.Fatalf("fail-closed must not error: %v", err)
	}
	before, _ := json.Marshal(in)
	after, _ := json.Marshal(out)
	if string(before) != string(after) {
		t.Fatalf("fail-closed changed drivers:\n before=%s\n after =%s", before, after)
	}
}

func TestApplyIndustryHitRateToDrivers_Applied_SumsWithoutMutatingCaller(t *testing.T) {
	withHitRateGate(t, true)
	provider := &fakeHitRateProvider{rows: []sectorallocation.IndustryHitRateSummary{
		eligibleRow("semiconductor", "buy", 0.7, 120), // +0.04
		eligibleRow("financials", "buy", 0.7, 120),    // +0.04
	}}
	in := sectorallocation.DriverInputs{
		AsOfTradingDate: "2026-07-17",
		CapitalFlow:     map[industry.SectorID]float64{industry.SectorFinancials: 0.05},
	}

	out, err := sectorallocation.ApplyIndustryHitRateToDrivers(in, provider, "src", "cond", "120d")

	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if got := out.CapitalFlow[industry.SectorSemiconductor]; !almostEqual(got, 0.04) {
		t.Errorf("semiconductor driver = %v, want 0.04", got)
	}
	if got := out.CapitalFlow[industry.SectorFinancials]; !almostEqual(got, 0.09) {
		t.Errorf("financials driver = %v, want 0.09 (existing 0.05 summed with tilt 0.04)", got)
	}
	if len(in.CapitalFlow) != 1 || in.CapitalFlow[industry.SectorFinancials] != 0.05 {
		t.Errorf("caller map mutated: %v", in.CapitalFlow)
	}
}

func TestGetRegisteredIndustryHitRateProvider_RoundTrip(t *testing.T) {
	sectorallocation.ResetIndustryHitRateProvider()
	t.Cleanup(sectorallocation.ResetIndustryHitRateProvider)
	if got := sectorallocation.GetRegisteredIndustryHitRateProvider(); got != nil {
		t.Fatalf("fresh registry = %v, want nil", got)
	}
	provider := &fakeHitRateProvider{}
	sectorallocation.RegisterIndustryHitRateProvider(provider)
	if got := sectorallocation.GetRegisteredIndustryHitRateProvider(); got != provider {
		t.Fatalf("registry = %v, want the registered provider", got)
	}
	sectorallocation.ResetIndustryHitRateProvider()
	if got := sectorallocation.GetRegisteredIndustryHitRateProvider(); got != nil {
		t.Fatalf("after reset = %v, want nil", got)
	}
}
