package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// writeTestSSOT writes a minimal but representative parameters document and
// returns its path. It carries one scalar tunable (risk.max_position_size), a
// nested block (rsi_tw.a1_weight), and a map-shaped block
// (factor_weight.base_weights.value) so both overlay entry kinds can be tested.
func writeTestSSOT(t *testing.T, maxPosition, maxDailyLoss float64) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "parameters.json")
	body := map[string]any{
		"version": "test",
		"risk": map[string]any{
			"max_position_size":  map[string]any{"value": maxPosition},
			"max_daily_loss_pct": map[string]any{"value": maxDailyLoss},
		},
		"rsi_tw": map[string]any{
			"a1_weight": map[string]any{"value": 0.25},
		},
		"industry": map[string]any{
			"cycle_thresholds": map[string]any{
				"value": map[string]any{
					"consumer": map[string]any{
						"expansion_revenue_pct": 0.08, "expansion_profit_pct": 0.1,
						"recovery_revenue_pct": 0.03, "recovery_profit_pct": 0.05,
						"mature_revenue_pct": 0.01, "mature_profit_pct": 0.02,
					},
				},
			},
		},
		"factor_weight": map[string]any{
			"base_weights": map[string]any{
				"value": map[string]any{
					"momentum": 0.25, "value": 0.2, "quality": 0.2, "agent": 0.05,
					"inst_sent": 0.1, "liquidity": 0.05, "narrative": 0.05, "industry_cycle": 0.0,
				},
			},
		},
	}
	return writeTestSSOTDocument(t, path, body)
}

func writeTestSSOTDocument(t *testing.T, path string, body map[string]any) string {
	t.Helper()
	data, err := json.MarshalIndent(body, "", "  ")
	if err != nil {
		t.Fatalf("marshal ssot: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write ssot: %v", err)
	}
	return path
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

// loadEffective loads path with the overlay registered at overlayPath.
func loadEffective(t *testing.T, ssotPath, overlayPath string) *ParametersConfig {
	t.Helper()
	previous := GetCalibratedOverlayPath()
	SetCalibratedOverlayPath(overlayPath)
	defer SetCalibratedOverlayPath(previous)

	cfg, err := LoadEffectiveParametersConfig(ssotPath)
	if err != nil {
		t.Fatalf("LoadEffectiveParametersConfig: %v", err)
	}
	return cfg
}

// applyOverlay applies the overlay to a freshly loaded SSOT config, returning the
// effective config and the report.
func applyOverlay(t *testing.T, ssotPath, overlayPath string) (*ParametersConfig, CalibrationOverlayReport) {
	t.Helper()
	previous := GetCalibratedOverlayPath()
	SetCalibratedOverlayPath(overlayPath)
	defer SetCalibratedOverlayPath(previous)

	src, err := LoadParametersSource(ssotPath)
	if err != nil {
		t.Fatalf("LoadParametersSource: %v", err)
	}
	effective, report := ApplyCalibratedOverlayLayer(src.Config, src.Raw)
	return effective, report
}

func mustUpdateOverlay(t *testing.T, overlayPath, source string, entries map[string]CalibrationOverlayEntry) {
	t.Helper()
	if _, err := UpdateCalibrationOverlay(overlayPath, source, entries); err != nil {
		t.Fatalf("UpdateCalibrationOverlay: %v", err)
	}
}

func baselineOfFloat(v float64) *OverlaySSOTBaseline {
	return &OverlaySSOTBaseline{Present: true, Value: v}
}

// TestCalibrationOverlayPath pins the overlay location: it must live under the
// bind-mounted data/ tree (FU-20260926-07), never under configs/.
func TestCalibrationOverlayPath(t *testing.T) {
	got := CalibrationOverlayPath("/app")
	want := "/app/data/state/parameters.calibrated.json"
	if got != want {
		t.Fatalf("CalibrationOverlayPath(/app) = %q, want %q", got, want)
	}
}

// TestApplyCalibratedOverlayLayer_AppliesNamedTunableOnTopOfSSOT is the core
// contract for parameter-table tunables: the SSOT file keeps its reviewed value
// while the effective value is the overlay's.
func TestApplyCalibratedOverlayLayer_AppliesNamedTunableOnTopOfSSOT(t *testing.T) {
	ssotPath := writeTestSSOT(t, 0.15, 0.03)
	overlayPath := filepath.Join(t.TempDir(), "parameters.calibrated.json")
	mustUpdateOverlay(t, overlayPath, "risk_gate_calibrate", map[string]CalibrationOverlayEntry{
		"risk_max_position_size": {
			Value: 0.13, Before: 0.15, SSOT: baselineOfFloat(0.15),
			CalibratedAt: time.Now(), Method: "bayesian_optimization",
		},
	})

	cfg := loadEffective(t, ssotPath, overlayPath)
	if got := cfg.Risk.MaxPositionSize.Value; got != 0.13 {
		t.Errorf("effective risk_max_position_size = %v, want 0.13 (overlay value)", got)
	}
	if got := cfg.Risk.MaxDailyLossPct.Value; got != 0.03 {
		t.Errorf("effective risk_max_daily_loss_pct = %v, want 0.03 (ssot value)", got)
	}
	if got := cfg.RSITw.A1Weight.Value; got != 0.25 {
		t.Errorf("effective rsi_tw.a1_weight = %v, want 0.25 (untouched)", got)
	}

	// The SSOT file itself must be untouched: it is what a human reviews.
	base, err := LoadParametersConfig(ssotPath)
	if err != nil {
		t.Fatalf("LoadParametersConfig: %v", err)
	}
	if got := base.Risk.MaxPositionSize.Value; got != 0.15 {
		t.Errorf("ssot risk_max_position_size = %v, want 0.15 (overlay must not rewrite the SSOT)", got)
	}
}

// TestApplyCalibratedOverlayLayer_AppliesPathEntriesOnTopOfSSOT covers nested and
// map-shaped adaptations: a scalar field, a whole map, and a new leaf inside an
// existing section.
func TestApplyCalibratedOverlayLayer_AppliesPathEntriesOnTopOfSSOT(t *testing.T) {
	ssotPath := writeTestSSOT(t, 0.15, 0.03)
	ssotBefore := readFile(t, ssotPath)
	overlayPath := filepath.Join(t.TempDir(), "parameters.calibrated.json")
	mustUpdateOverlay(t, overlayPath, "rsi_tw_calibrate", map[string]CalibrationOverlayEntry{
		"rsi_tw.a1_weight.value": {
			Path: "rsi_tw.a1_weight.value", Value: 0.31, Before: 0.25,
			SSOT:         &OverlaySSOTBaseline{Present: true, Value: 0.25},
			CalibratedAt: time.Now(), Method: "grid_search",
		},
		"factor_weight.base_weights.value": {
			Path: "factor_weight.base_weights.value",
			// map[string]float64 values survive both a direct call and a JSON
			// round-trip, so this is also the shape the writers produce.
			Value: map[string]any{
				"momentum": 0.3, "value": 0.15, "quality": 0.15, "agent": 0.05,
				"inst_sent": 0.1, "liquidity": 0.05, "narrative": 0.05, "industry_cycle": 0.0,
			},
			SSOT: &OverlaySSOTBaseline{Present: true, Value: map[string]any{
				"momentum": 0.25, "value": 0.2, "quality": 0.2, "agent": 0.05,
				"inst_sent": 0.1, "liquidity": 0.05, "narrative": 0.05, "industry_cycle": 0.0,
			}},
			CalibratedAt: time.Now(), Method: "bayesian_search",
		},
		"industry.cycle_thresholds.source": {
			Path: "industry.cycle_thresholds.source", Value: "percentile_based",
			SSOT:         &OverlaySSOTBaseline{Present: false},
			CalibratedAt: time.Now(), Method: "percentile_based",
		},
	})

	cfg, report := applyOverlay(t, ssotPath, overlayPath)
	if len(report.Applied) != 3 {
		t.Fatalf("applied %d entries, want 3 (report: %+v)", len(report.Applied), report)
	}
	if got := cfg.RSITw.A1Weight.Value; got != 0.31 {
		t.Errorf("effective rsi_tw.a1_weight = %v, want 0.31", got)
	}
	if got := cfg.FactorWeight.BaseWeights.Value["momentum"]; got != 0.3 {
		t.Errorf("effective base_weights.momentum = %v, want 0.3", got)
	}
	if got := cfg.FactorWeight.BaseWeights.Value["quality"]; got != 0.15 {
		t.Errorf("effective base_weights.quality = %v, want 0.15", got)
	}
	if got := cfg.Industry.CycleThresholds.Source; got != "percentile_based" {
		t.Errorf("effective industry.cycle_thresholds.source = %q, want percentile_based (new leaf inside an existing section)", got)
	}

	// The SSOT document must be byte-identical after the load: the overlay is the
	// only thing that changed.
	if after := readFile(t, ssotPath); after != ssotBefore {
		t.Errorf("SSOT document changed during overlay application:\nbefore=%s\nafter=%s", ssotBefore, after)
	}
}

// TestApplyCalibratedOverlayLayer_DisabledByDefault is the negative control for
// the whole overlay: with no path registered, the loader must behave exactly
// like a plain SSOT read even when an overlay file exists on disk.
func TestApplyCalibratedOverlayLayer_DisabledByDefault(t *testing.T) {
	ssotPath := writeTestSSOT(t, 0.15, 0.03)
	overlayPath := filepath.Join(t.TempDir(), "parameters.calibrated.json")
	mustUpdateOverlay(t, overlayPath, "risk_gate_calibrate", map[string]CalibrationOverlayEntry{
		"risk_max_position_size": {
			Value: 0.13, SSOT: baselineOfFloat(0.15), CalibratedAt: time.Now(),
		},
		"rsi_tw.a1_weight.value": {
			Path: "rsi_tw.a1_weight.value", Value: 0.31,
			SSOT: &OverlaySSOTBaseline{Present: true, Value: 0.25}, CalibratedAt: time.Now(),
		},
	})

	cfg := loadEffective(t, ssotPath, "") // default: overlay off
	if got := cfg.Risk.MaxPositionSize.Value; got != 0.15 {
		t.Errorf("effective risk_max_position_size = %v, want 0.15 (overlay disabled ⇒ SSOT only)", got)
	}
	if got := cfg.RSITw.A1Weight.Value; got != 0.25 {
		t.Errorf("effective rsi_tw.a1_weight = %v, want 0.25 (overlay disabled ⇒ SSOT only)", got)
	}
}

// TestApplyCalibratedOverlayLayer_InvalidatesWhenSSOTMoved covers the guard that
// keeps a reviewed charter edit authoritative, for both entry kinds.
func TestApplyCalibratedOverlayLayer_InvalidatesWhenSSOTMoved(t *testing.T) {
	ssotPath := writeTestSSOT(t, 0.20, 0.03)
	overlayPath := filepath.Join(t.TempDir(), "parameters.calibrated.json")
	mustUpdateOverlay(t, overlayPath, "risk_gate_calibrate", map[string]CalibrationOverlayEntry{
		"risk_max_position_size": {
			Value: 0.13, SSOT: baselineOfFloat(0.15), CalibratedAt: time.Now(),
		},
		"rsi_tw.a1_weight.value": {
			Path: "rsi_tw.a1_weight.value", Value: 0.31,
			SSOT: &OverlaySSOTBaseline{Present: true, Value: 0.10}, CalibratedAt: time.Now(),
		},
	})

	cfg, report := applyOverlay(t, ssotPath, overlayPath)
	if got := cfg.Risk.MaxPositionSize.Value; got != 0.20 {
		t.Errorf("effective risk_max_position_size = %v, want 0.20 (stale overlay must not override a moved SSOT)", got)
	}
	if got := cfg.RSITw.A1Weight.Value; got != 0.25 {
		t.Errorf("effective rsi_tw.a1_weight = %v, want 0.25 (stale path entry must not override a moved SSOT)", got)
	}
	if len(report.Invalidated) != 2 {
		t.Errorf("invalidated = %v, want both entries", report.Invalidated)
	}

	ov, err := LoadCalibrationOverlay(overlayPath)
	if err != nil {
		t.Fatalf("LoadCalibrationOverlay: %v", err)
	}
	if len(ov.Entries) != 0 {
		t.Errorf("stale entries survived reconciliation: %+v", ov.Entries)
	}
}

// TestApplyCalibratedOverlayLayer_DropsUnknownEntries covers typos: an unknown
// parameter name and a path whose top-level section does not exist.
func TestApplyCalibratedOverlayLayer_DropsUnknownEntries(t *testing.T) {
	ssotPath := writeTestSSOT(t, 0.15, 0.03)
	overlayPath := filepath.Join(t.TempDir(), "parameters.calibrated.json")
	mustUpdateOverlay(t, overlayPath, "risk_gate_calibrate", map[string]CalibrationOverlayEntry{
		"risk_max_position_sixe": {Value: 0.13, CalibratedAt: time.Now()},
		"not_a_section.a.value": {
			Path: "not_a_section.a.value", Value: 0.5, CalibratedAt: time.Now(),
		},
	})

	cfg, report := applyOverlay(t, ssotPath, overlayPath)
	if got := cfg.Risk.MaxPositionSize.Value; got != 0.15 {
		t.Errorf("effective risk_max_position_size = %v, want 0.15 (SSOT value: nothing applicable was overlaid)", got)
	}
	if len(report.Unknown) != 2 {
		t.Fatalf("Unknown = %v, want both entries", report.Unknown)
	}
	if len(report.Applied) != 0 {
		t.Errorf("Applied = %v, want none", report.AppliedKeys())
	}
	ov, err := LoadCalibrationOverlay(overlayPath)
	if err != nil {
		t.Fatalf("LoadCalibrationOverlay: %v", err)
	}
	if len(ov.Entries) != 0 {
		t.Errorf("unknown entries survived reconciliation: %+v", ov.Entries)
	}
}

// TestApplyCalibratedOverlayLayer_PathEntryWithoutSSOTDocumentIsDropped covers a
// missing SSOT file: there is no document to validate a path against, so the
// entry must be dropped loudly instead of applied blind.
func TestApplyCalibratedOverlayLayer_PathEntryWithoutSSOTDocumentIsDropped(t *testing.T) {
	overlayPath := filepath.Join(t.TempDir(), "parameters.calibrated.json")
	mustUpdateOverlay(t, overlayPath, "factor_weight_calibrate", map[string]CalibrationOverlayEntry{
		"factor_weight.base_weights.value": {
			Path: "factor_weight.base_weights.value", Value: map[string]any{"momentum": 0.3},
			CalibratedAt: time.Now(),
		},
	})

	previous := GetCalibratedOverlayPath()
	SetCalibratedOverlayPath(overlayPath)
	defer SetCalibratedOverlayPath(previous)

	effective, report := ApplyCalibratedOverlayLayer(DefaultParametersConfig(), nil)
	if effective == nil {
		t.Fatal("ApplyCalibratedOverlayLayer returned nil config")
	}
	if len(report.Unknown) != 1 {
		t.Errorf("Unknown = %v, want the path entry", report.Unknown)
	}
}

// TestApplyCalibratedOverlayLayer_ReconcilesBaseline pins the bootstrap: an entry
// written with no reconciliation baseline gets one, so a later SSOT edit can
// invalidate it.
func TestApplyCalibratedOverlayLayer_ReconcilesBaseline(t *testing.T) {
	ssotPath := writeTestSSOT(t, 0.15, 0.03)
	overlayPath := filepath.Join(t.TempDir(), "parameters.calibrated.json")
	mustUpdateOverlay(t, overlayPath, "risk_gate_calibrate", map[string]CalibrationOverlayEntry{
		"risk_max_position_size": {Value: 0.13, CalibratedAt: time.Now()}, // never reconciled
		"rsi_tw.a1_weight.value": {
			Path: "rsi_tw.a1_weight.value", Value: 0.31, CalibratedAt: time.Now(), // never reconciled
		},
	})

	cfg, report := applyOverlay(t, ssotPath, overlayPath)
	if got := cfg.Risk.MaxPositionSize.Value; got != 0.13 {
		t.Errorf("effective risk_max_position_size = %v, want 0.13", got)
	}
	if got := cfg.RSITw.A1Weight.Value; got != 0.31 {
		t.Errorf("effective rsi_tw.a1_weight = %v, want 0.31", got)
	}
	if !report.Reconciled {
		t.Errorf("Reconciled = false, want true (baselines had to be recorded)")
	}
	ov, err := LoadCalibrationOverlay(overlayPath)
	if err != nil {
		t.Fatalf("LoadCalibrationOverlay: %v", err)
	}
	if b := ov.Entries["risk_max_position_size"].SSOT; b == nil || !b.Present || b.Value != 0.15 {
		t.Errorf("named entry baseline = %+v, want present 0.15", ov.Entries["risk_max_position_size"].SSOT)
	}
	if b := ov.Entries["rsi_tw.a1_weight.value"].SSOT; b == nil || !b.Present || b.Value != 0.25 {
		t.Errorf("path entry baseline = %+v, want present 0.25", ov.Entries["rsi_tw.a1_weight.value"].SSOT)
	}
}

// TestUpdateCalibrationOverlay_MergesEntries guards the merge: a later round
// that only reports one tunable must not drop the others, and the reconciliation
// baseline must survive an update.
func TestUpdateCalibrationOverlay_MergesEntries(t *testing.T) {
	path := filepath.Join(t.TempDir(), "parameters.calibrated.json")
	mustUpdateOverlay(t, path, "risk_gate_calibrate", map[string]CalibrationOverlayEntry{
		"risk_max_position_size":  {Value: 0.13, SSOT: baselineOfFloat(0.15), CalibratedAt: time.Now()},
		"risk_max_daily_loss_pct": {Value: 0.031, SSOT: baselineOfFloat(0.03), CalibratedAt: time.Now()},
	})
	mustUpdateOverlay(t, path, "risk_gate_calibrate", map[string]CalibrationOverlayEntry{
		"risk_max_position_size": {Value: 0.14, CalibratedAt: time.Now()},
	})

	ov, err := LoadCalibrationOverlay(path)
	if err != nil {
		t.Fatalf("LoadCalibrationOverlay: %v", err)
	}
	if len(ov.Entries) != 2 {
		t.Fatalf("entries = %d, want 2 (merge must keep unmentioned entries): %+v", len(ov.Entries), ov.Entries)
	}
	if got := numericOf(t, ov.Entries["risk_max_daily_loss_pct"].Value); got != 0.031 {
		t.Errorf("daily loss entry = %v, want 0.031 (must survive)", got)
	}
	if got := numericOf(t, ov.Entries["risk_max_position_size"].Value); got != 0.14 {
		t.Errorf("position entry = %v, want 0.14 (updated)", got)
	}
	if b := ov.Entries["risk_max_position_size"].SSOT; b == nil || b.Value != 0.15 {
		t.Errorf("baseline = %+v, want preserved 0.15", b)
	}
}

func numericOf(t *testing.T, v any) float64 {
	t.Helper()
	f, ok := numericValue(v)
	if !ok {
		t.Fatalf("value %#v is not numeric", v)
	}
	return f
}

// TestUpdateCalibrationOverlay_EmptyPathIsError pins the fail-loud path: writing
// without a registered overlay path must not silently succeed.
func TestUpdateCalibrationOverlay_EmptyPathIsError(t *testing.T) {
	if _, err := UpdateCalibrationOverlay("", "risk_gate_calibrate", nil); err == nil {
		t.Fatal(`UpdateCalibrationOverlay("", ...) = nil error, want error`)
	}
}

// TestLoadCalibrationOverlay_MissingFileIsNotAnError pins the fail-safe read.
func TestLoadCalibrationOverlay_MissingFileIsNotAnError(t *testing.T) {
	ov, err := LoadCalibrationOverlay(filepath.Join(t.TempDir(), "nope.json"))
	if err != nil {
		t.Fatalf("LoadCalibrationOverlay(missing) = %v, want nil error", err)
	}
	if ov != nil {
		t.Fatalf("LoadCalibrationOverlay(missing) = %+v, want nil overlay", ov)
	}
}

// TestSSOTBaselines_IgnoreOverlay is the reason the writers can record an exact
// reconciliation baseline: the SSOT reads must not return overlaid values.
func TestSSOTBaselines_IgnoreOverlay(t *testing.T) {
	ssotPath := writeTestSSOT(t, 0.15, 0.03)
	overlayPath := filepath.Join(t.TempDir(), "parameters.calibrated.json")
	mustUpdateOverlay(t, overlayPath, "risk_gate_calibrate", map[string]CalibrationOverlayEntry{
		"risk_max_position_size": {Value: 0.13, SSOT: baselineOfFloat(0.15), CalibratedAt: time.Now()},
		"rsi_tw.a1_weight.value": {
			Path: "rsi_tw.a1_weight.value", Value: 0.31,
			SSOT: &OverlaySSOTBaseline{Present: true, Value: 0.25}, CalibratedAt: time.Now(),
		},
	})

	previousPath := GetParametersConfigPath()
	previousOverlay := GetCalibratedOverlayPath()
	SetParametersConfigPath(ssotPath)
	SetCalibratedOverlayPath(overlayPath)
	defer func() {
		SetParametersConfigPath(previousPath)
		SetCalibratedOverlayPath(previousOverlay)
	}()

	named := SSOTParameterBaseline("risk_max_position_size")
	if named == nil || !named.Present || named.Value != 0.15 {
		t.Errorf("SSOTParameterBaseline = %+v, want present 0.15 (must ignore the overlay value 0.13)", named)
	}
	if SSOTParameterBaseline("not_a_parameter") != nil {
		t.Error("SSOTParameterBaseline(unknown) != nil, want nil")
	}

	nested := SSOTPathBaseline("rsi_tw.a1_weight.value")
	if nested == nil || !nested.Present || nested.Value != 0.25 {
		t.Errorf("SSOTPathBaseline(rsi_tw.a1_weight.value) = %+v, want present 0.25", nested)
	}
	absent := SSOTPathBaseline("factor_weight.base_weights.last_calibrated")
	if absent == nil || absent.Present {
		t.Errorf("SSOTPathBaseline(absent leaf) = %+v, want present=false", absent)
	}
}

// TestCalibratedEntryHelpers_ResolveBaselinesFromSSOT pins the helpers the
// writers use: they must record what the SSOT actually says.
func TestCalibratedEntryHelpers_ResolveBaselinesFromSSOT(t *testing.T) {
	ssotPath := writeTestSSOT(t, 0.15, 0.03)
	previousPath := GetParametersConfigPath()
	SetParametersConfigPath(ssotPath)
	defer SetParametersConfigPath(previousPath)

	at := time.Date(2026, 9, 26, 3, 8, 44, 0, time.UTC)
	named := CalibratedEntryForParameter("risk_max_position_size", 0.13, 0.15, "bayesian_optimization", "why", at)
	if named.Path != "" {
		t.Errorf("named entry path = %q, want empty", named.Path)
	}
	if named.SSOT == nil || named.SSOT.Value != 0.15 {
		t.Errorf("named entry baseline = %+v, want present 0.15", named.SSOT)
	}
	if !named.CalibratedAt.Equal(at) || named.Method != "bayesian_optimization" || named.Rationale != "why" {
		t.Errorf("named entry metadata = %+v", named)
	}

	pathEntry := CalibratedEntryForPath("rsi_tw.a1_weight.value", 0.31, 0.25, "grid_search", "", at)
	if pathEntry.Path != "rsi_tw.a1_weight.value" {
		t.Errorf("path entry path = %q", pathEntry.Path)
	}
	if pathEntry.SSOT == nil || !pathEntry.SSOT.Present || pathEntry.SSOT.Value != 0.25 {
		t.Errorf("path entry baseline = %+v, want present 0.25", pathEntry.SSOT)
	}
}
