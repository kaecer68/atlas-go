package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestParseCalibrationWriteback(t *testing.T) {
	cases := []struct {
		in      string
		want    CalibrationWriteback
		wantErr bool
	}{
		{in: "", want: WritebackSSOT},
		{in: "ssot", want: WritebackSSOT},
		{in: "SSOT", want: WritebackSSOT},
		{in: " overlay ", want: WritebackOverlay},
		{in: "ssot.json", wantErr: true},
		{in: "configs", wantErr: true},
	}
	for _, c := range cases {
		got, err := ParseCalibrationWriteback(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("ParseCalibrationWriteback(%q) = %v, want error", c.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseCalibrationWriteback(%q) error: %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("ParseCalibrationWriteback(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestCalibrationWriteback_RegisterForWorkDir(t *testing.T) {
	previous := GetCalibratedOverlayPath()
	defer SetCalibratedOverlayPath(previous)

	SetCalibratedOverlayPath("sentinel")
	if err := WritebackSSOT.RegisterForWorkDir("/app"); err != nil {
		t.Fatalf("ssot RegisterForWorkDir: %v", err)
	}
	if GetCalibratedOverlayPath() != "sentinel" {
		t.Errorf("ssot mode changed the overlay path to %q", GetCalibratedOverlayPath())
	}

	if err := WritebackOverlay.RegisterForWorkDir("/app"); err != nil {
		t.Fatalf("overlay RegisterForWorkDir: %v", err)
	}
	if got, want := GetCalibratedOverlayPath(), "/app/data/state/parameters.calibrated.json"; got != want {
		t.Errorf("overlay path = %q, want %q", got, want)
	}
}

func TestDiffParametersDocuments(t *testing.T) {
	base := map[string]any{
		"risk": map[string]any{
			"max_position_size": map[string]any{"value": 0.15},
			"unchanged":         map[string]any{"value": 1.0},
		},
		"factor_weight": map[string]any{
			"base_weights": map[string]any{"value": map[string]any{"momentum": 0.25}},
		},
		"industry": map[string]any{
			"seasonal_patterns": map[string]any{
				"value": []any{map[string]any{"id": "p1", "adjustment_factor": 1.0}},
			},
		},
	}
	updated := map[string]any{
		"risk": map[string]any{
			"max_position_size": map[string]any{"value": 0.13},
			"unchanged":         map[string]any{"value": 1.0},
		},
		"factor_weight": map[string]any{
			"base_weights": map[string]any{"value": map[string]any{"momentum": 0.30}},
		},
		"industry": map[string]any{
			"seasonal_patterns": map[string]any{
				"value": []any{map[string]any{"id": "p1", "adjustment_factor": 1.4}},
			},
		},
	}

	changed, skipped := DiffParametersDocuments(base, updated)
	if len(skipped) != 0 {
		t.Errorf("skipped = %v, want none", skipped)
	}
	want := map[string]any{
		"risk.max_position_size.value":              0.13,
		"factor_weight.base_weights.value.momentum": 0.30,
		"industry.seasonal_patterns.value":          updated["industry"].(map[string]any)["seasonal_patterns"].(map[string]any)["value"],
	}
	if len(changed) != len(want) {
		t.Fatalf("changed = %#v, want %d entries (%v)", changed, len(want), want)
	}
	for path, wantVal := range want {
		got, ok := changed[path]
		if !ok {
			t.Errorf("changed missing %s (got %#v)", path, changed)
			continue
		}
		if !sameJSONValue(got, wantVal) {
			t.Errorf("changed[%s] = %#v, want %#v", path, got, wantVal)
		}
	}
	if _, ok := changed["risk.unchanged.value"]; ok {
		t.Error("unchanged leaf was reported as changed")
	}

	// Identical documents: no changes.
	changed, _ = DiffParametersDocuments(base, base)
	if len(changed) != 0 {
		t.Errorf("changed = %#v, want none for identical documents", changed)
	}
}

func TestDiffParametersDocuments_DotInKeyIsReported(t *testing.T) {
	base := map[string]any{"stock": map[string]any{"2330.TW": 1.0}}
	updated := map[string]any{"stock": map[string]any{"2330.TW": 2.0}}

	changed, skipped := DiffParametersDocuments(base, updated)
	if len(changed) != 0 {
		t.Errorf("changed = %#v, want none (a dotted key cannot be addressed)", changed)
	}
	if len(skipped) != 1 || skipped[0] != "stock.2330.TW" {
		t.Errorf("skipped = %v, want [stock.2330.TW]", skipped)
	}
}

func TestWriteDocumentOverlay_WritesEntriesAndLeavesSSOTUntouched(t *testing.T) {
	dir := t.TempDir()
	ssotPath := filepath.Join(dir, "configs", "parameters.json")
	overlayPath := filepath.Join(dir, "data", "state", "parameters.calibrated.json")

	base := map[string]any{"industry": map[string]any{"cycle_thresholds": map[string]any{
		"value": map[string]any{"consumer": map[string]any{"expansion_revenue_pct": 0.08}},
	}}}
	if err := os.MkdirAll(filepath.Dir(ssotPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeJSONFile(ssotPath, base); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(ssotPath)
	if err != nil {
		t.Fatal(err)
	}

	updated := map[string]any{"industry": map[string]any{"cycle_thresholds": map[string]any{
		"value": map[string]any{"consumer": map[string]any{"expansion_revenue_pct": 0.11}},
	}}}

	previous := GetCalibratedOverlayPath()
	SetCalibratedOverlayPath(overlayPath)
	defer SetCalibratedOverlayPath(previous)

	at := time.Date(2026, 9, 26, 3, 8, 44, 0, time.UTC)
	n, err := WriteDocumentOverlay("calibrate_thresholds", base, updated, at)
	if err != nil {
		t.Fatalf("WriteDocumentOverlay: %v", err)
	}
	if n != 1 {
		t.Errorf("changed = %d, want 1", n)
	}

	ov, err := LoadCalibrationOverlay(overlayPath)
	if err != nil {
		t.Fatalf("LoadCalibrationOverlay: %v", err)
	}
	if ov == nil || ov.Source != "calibrate_thresholds" {
		t.Fatalf("overlay = %+v, want source calibrate_thresholds", ov)
	}
	entry, ok := ov.Entries["industry.cycle_thresholds.value.consumer.expansion_revenue_pct"]
	if !ok {
		t.Fatalf("missing entry: %+v", ov.Entries)
	}
	if got := numericOf(t, entry.Value); got != 0.11 {
		t.Errorf("entry value = %v, want 0.11", got)
	}
	if entry.SSOT == nil || !entry.SSOT.Present || entry.SSOT.Value != 0.08 {
		t.Errorf("entry baseline = %+v, want the SSOT value 0.08", entry.SSOT)
	}
	if !entry.CalibratedAt.Equal(at) {
		t.Errorf("calibrated_at = %v, want %v", entry.CalibratedAt, at)
	}

	after, err := os.ReadFile(ssotPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Errorf("SSOT document changed:\nbefore=%s\nafter=%s", before, after)
	}
}

func TestWriteDocumentOverlay_NoOverlayPathIsError(t *testing.T) {
	previous := GetCalibratedOverlayPath()
	SetCalibratedOverlayPath("")
	defer SetCalibratedOverlayPath(previous)

	if _, err := WriteDocumentOverlay("x", map[string]any{"a": 1.0}, map[string]any{"a": 2.0}, time.Now()); err == nil {
		t.Fatal("WriteDocumentOverlay without an overlay path = nil error, want error")
	}
}

func TestWriteConfigOverlay(t *testing.T) {
	dir := t.TempDir()
	overlayPath := filepath.Join(dir, "parameters.calibrated.json")

	base := DefaultParametersConfig()
	base.RSITw.A1Weight.Value = 0.25
	updated := DefaultParametersConfig()
	updated.RSITw.A1Weight.Value = 0.31

	previous := GetCalibratedOverlayPath()
	SetCalibratedOverlayPath(overlayPath)
	defer SetCalibratedOverlayPath(previous)

	n, err := WriteConfigOverlay("calibrate_rsi_tw", base, updated, time.Now())
	if err != nil {
		t.Fatalf("WriteConfigOverlay: %v", err)
	}
	if n != 1 {
		t.Errorf("changed = %d, want 1", n)
	}
	ov, err := LoadCalibrationOverlay(overlayPath)
	if err != nil {
		t.Fatalf("LoadCalibrationOverlay: %v", err)
	}
	entry, ok := ov.Entries["rsi_tw.a1_weight.value"]
	if !ok {
		t.Fatalf("missing entry: %+v", ov.Entries)
	}
	if got := numericOf(t, entry.Value); got != 0.31 {
		t.Errorf("entry value = %v, want 0.31", got)
	}
	if got := numericOf(t, entry.Before); got != 0.25 {
		t.Errorf("entry before = %v, want 0.25", got)
	}
}

func TestWriteConfigOverlay_NilConfigsIsError(t *testing.T) {
	previous := GetCalibratedOverlayPath()
	SetCalibratedOverlayPath(filepath.Join(t.TempDir(), "ov.json"))
	defer SetCalibratedOverlayPath(previous)

	if _, err := WriteConfigOverlay("x", nil, DefaultParametersConfig(), time.Now()); err == nil {
		t.Fatal("WriteConfigOverlay(nil base) = nil error, want error")
	}
	if _, err := WriteConfigOverlay("x", DefaultParametersConfig(), nil, time.Now()); err == nil {
		t.Fatal("WriteConfigOverlay(nil updated) = nil error, want error")
	}
}
