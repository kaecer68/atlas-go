package service

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/buildinfo"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/industry"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/marketdata"
)

func TestCheckTSMCRevenueHealth(t *testing.T) {
	tmpDir := t.TempDir()
	now := time.Date(2026, 5, 12, 0, 0, 0, 0, time.FixedZone("CST", 8*60*60))

	// Test 1: empty directory → error
	status, updated := checkTSMCRevenueHealth(tmpDir, now)
	if status != "error" || updated != "無資料" {
		t.Errorf("empty dir: expected error/無資料, got %s/%s", status, updated)
	}

	// Test 2: recent data (ROC 11504 = 2026-04) → ok (within 45 days)
	recentFile := filepath.Join(tmpDir, "11504_revenue.json")
	os.WriteFile(recentFile, []byte(`{"date":"11504","revenue":500}`), 0o644)
	status, updated = checkTSMCRevenueHealth(tmpDir, now)
	if status != "ok" {
		t.Errorf("recent data: expected ok, got %s (updated=%s)", status, updated)
	}

	// Test 3: old data (ROC 11502 = 2026-02) → error (> 90 days from 2026-05-12)
	os.Remove(recentFile)
	oldFile := filepath.Join(tmpDir, "11502_revenue.json")
	os.WriteFile(oldFile, []byte(`{"date":"11502","revenue":400}`), 0o644)
	status, updated = checkTSMCRevenueHealth(tmpDir, now)
	if status != "error" {
		t.Errorf("old data: expected error, got %s (updated=%s)", status, updated)
	}

	// Test 4: warn data (ROC 11503 = 2026-03, ~42 days from 2026-05-12)
	os.Remove(oldFile)
	warnFile := filepath.Join(tmpDir, "11503_revenue.json")
	os.WriteFile(warnFile, []byte(`{"date":"11503","revenue":450}`), 0o644)
	status, updated = checkTSMCRevenueHealth(tmpDir, now)
	if status != "warn" {
		t.Errorf("warn data: expected warn, got %s (updated=%s)", status, updated)
	}
}

func TestParseROCYearMonth(t *testing.T) {
	tests := []struct {
		input    string
		expected time.Time
		valid    bool
	}{
		{"11503", time.Date(2026, 3, 1, 0, 0, 0, 0, time.FixedZone("CST", 8*60*60)), true},
		{"11412", time.Date(2025, 12, 1, 0, 0, 0, 0, time.FixedZone("CST", 8*60*60)), true},
		{"11501", time.Date(2026, 1, 1, 0, 0, 0, 0, time.FixedZone("CST", 8*60*60)), true},
		{"invalid", time.Time{}, false},
		{"11513", time.Time{}, false}, // month 13 invalid
		{"1150", time.Time{}, false},  // too short
	}

	for _, tt := range tests {
		result := parseROCYearMonth(tt.input)
		if tt.valid {
			if result.IsZero() || !result.Equal(tt.expected) {
				t.Errorf("parseROCYearMonth(%s) = %v, expected %v", tt.input, result, tt.expected)
			}
		} else {
			if !result.IsZero() {
				t.Errorf("parseROCYearMonth(%s) = %v, expected zero", tt.input, result)
			}
		}
	}
}

func TestCheckCapitalFlowHealth(t *testing.T) {
	cst := time.FixedZone("CST", 8*60*60)
	now := time.Date(2026, 5, 13, 12, 0, 0, 0, cst)

	t.Run("empty directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		status, updated := checkCapitalFlowHealth(tmpDir, now)
		if status != "error" || updated != "無資料" {
			t.Errorf("expected error/無資料, got %s/%s", status, updated)
		}
	})

	t.Run("file modified within 24h returns ok", func(t *testing.T) {
		tmpDir := t.TempDir()
		p := filepath.Join(tmpDir, "20260512.json")
		os.WriteFile(p, []byte(`{"date":"20260512","total_net":0}`), 0o644)
		os.Chtimes(p, time.Now(), time.Date(2026, 5, 13, 10, 0, 0, 0, cst))
		status, updated := checkCapitalFlowHealth(tmpDir, now)
		if status != "ok" {
			t.Errorf("expected ok, got %s (updated=%s)", status, updated)
		}
	})

	t.Run("file modified within 7d but > 24h returns warn", func(t *testing.T) {
		tmpDir := t.TempDir()
		p := filepath.Join(tmpDir, "20260510.json")
		os.WriteFile(p, []byte(`{"date":"20260510","total_net":0}`), 0o644)
		os.Chtimes(p, time.Now(), time.Date(2026, 5, 10, 10, 0, 0, 0, cst))
		status, updated := checkCapitalFlowHealth(tmpDir, now)
		if status != "warn" {
			t.Errorf("expected warn, got %s (updated=%s)", status, updated)
		}
	})

	t.Run("file modified > 7d returns error", func(t *testing.T) {
		tmpDir := t.TempDir()
		p := filepath.Join(tmpDir, "20260501.json")
		os.WriteFile(p, []byte(`{"date":"20260501","total_net":0}`), 0o644)
		os.Chtimes(p, time.Now(), time.Date(2026, 5, 1, 10, 0, 0, 0, cst))
		status, updated := checkCapitalFlowHealth(tmpDir, now)
		if status != "error" {
			t.Errorf("expected error, got %s (updated=%s)", status, updated)
		}
	})

	t.Run("file mod time takes priority over old data date", func(t *testing.T) {
		tmpDir := t.TempDir()
		p := filepath.Join(tmpDir, "20260501.json")
		os.WriteFile(p, []byte(`{"date":"20260501","total_net":0}`), 0o644)
		os.Chtimes(p, time.Now(), time.Date(2026, 5, 13, 10, 0, 0, 0, cst))
		status, updated := checkCapitalFlowHealth(tmpDir, now)
		if status != "ok" {
			t.Errorf("expected ok (mod time overrides), got %s (updated=%s)", status, updated)
		}
	})
}

func TestCheckExportHealth(t *testing.T) {
	cst := time.FixedZone("CST", 8*60*60)
	now := time.Date(2026, 5, 13, 12, 0, 0, 0, cst)

	t.Run("empty directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		status, updated := checkExportHealth(tmpDir, now)
		if status != "error" || updated != "無資料" {
			t.Errorf("expected error/無資料, got %s/%s", status, updated)
		}
	})

	t.Run("file modified within 24h returns ok", func(t *testing.T) {
		tmpDir := t.TempDir()
		p := filepath.Join(tmpDir, "11502_export.json")
		os.WriteFile(p, []byte(`{"year":115,"month":2,"export_total":100}`), 0o644)
		os.Chtimes(p, time.Now(), time.Date(2026, 5, 13, 10, 0, 0, 0, cst))
		status, updated := checkExportHealth(tmpDir, now)
		if status != "ok" {
			t.Errorf("expected ok (recent mod time), got %s (updated=%s)", status, updated)
		}
	})

	t.Run("file modified > 7d returns error", func(t *testing.T) {
		tmpDir := t.TempDir()
		p := filepath.Join(tmpDir, "11502_export.json")
		os.WriteFile(p, []byte(`{"year":115,"month":2,"export_total":100}`), 0o644)
		os.Chtimes(p, time.Now(), time.Date(2026, 5, 1, 10, 0, 0, 0, cst))
		status, updated := checkExportHealth(tmpDir, now)
		if status != "error" {
			t.Errorf("expected error (old mod time), got %s (updated=%s)", status, updated)
		}
	})

	t.Run("mod time takes priority over old data date", func(t *testing.T) {
		tmpDir := t.TempDir()
		p := filepath.Join(tmpDir, "11502_export.json")
		os.WriteFile(p, []byte(`{"year":115,"month":2,"export_total":100}`), 0o644)
		os.Chtimes(p, time.Now(), time.Date(2026, 5, 13, 10, 0, 0, 0, cst))
		status, updated := checkExportHealth(tmpDir, now)
		if status != "ok" {
			t.Errorf("expected ok (mod time overrides old data), got %s (updated=%s)", status, updated)
		}
	})

	t.Run("no _export.json suffix files are ignored", func(t *testing.T) {
		tmpDir := t.TempDir()
		os.WriteFile(filepath.Join(tmpDir, "11502.json"), []byte(`{"year":115,"month":2}`), 0o644)
		status, updated := checkExportHealth(tmpDir, now)
		if status != "error" {
			t.Errorf("expected error (no _export.json files), got %s (updated=%s)", status, updated)
		}
	})
}

func TestCheckMarginHealth(t *testing.T) {
	cst := time.FixedZone("CST", 8*60*60)
	now := time.Date(2026, 5, 12, 10, 0, 0, 0, cst)

	t.Run("empty directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		status, updated := checkMarginHealth(tmpDir, now)
		if status != "error" || updated != "無資料" {
			t.Errorf("expected error/無資料, got %s/%s", status, updated)
		}
	})

	t.Run("recent data within 3 days", func(t *testing.T) {
		tmpDir := t.TempDir()
		os.WriteFile(filepath.Join(tmpDir, "20260511_margin.json"),
			[]byte(`{"date":"20260511","margin_balance":0.01}`), 0o644)
		status, updated := checkMarginHealth(tmpDir, now)
		if status != "ok" {
			t.Errorf("expected ok, got %s (updated=%s)", status, updated)
		}
		if updated != "20260511" {
			t.Errorf("expected updated=20260511, got %s", updated)
		}
	})

	t.Run("data 5 days old returns warn", func(t *testing.T) {
		tmpDir := t.TempDir()
		os.WriteFile(filepath.Join(tmpDir, "20260507_margin.json"),
			[]byte(`{"date":"20260507","margin_balance":0.01}`), 0o644)
		status, _ := checkMarginHealth(tmpDir, now)
		if status != "warn" {
			t.Errorf("expected warn, got %s", status)
		}
	})

	t.Run("data 8 days old returns error", func(t *testing.T) {
		tmpDir := t.TempDir()
		os.WriteFile(filepath.Join(tmpDir, "20260504_margin.json"),
			[]byte(`{"date":"20260504","margin_balance":0.01}`), 0o644)
		status, _ := checkMarginHealth(tmpDir, now)
		if status != "error" {
			t.Errorf("expected error, got %s", status)
		}
	})

	t.Run("date from JSON content takes priority", func(t *testing.T) {
		tmpDir := t.TempDir()
		os.WriteFile(filepath.Join(tmpDir, "20260504_margin.json"),
			[]byte(`{"date":"20260511","margin_balance":0.01}`), 0o644)
		status, _ := checkMarginHealth(tmpDir, now)
		if status != "ok" {
			t.Errorf("expected ok (JSON date overrides filename), got %s", status)
		}
	})

	t.Run("non-margin files are ignored", func(t *testing.T) {
		tmpDir := t.TempDir()
		os.WriteFile(filepath.Join(tmpDir, "20260511.json"),
			[]byte(`{"date":"20260511"}`), 0o644)
		status, _ := checkMarginHealth(tmpDir, now)
		if status != "error" {
			t.Errorf("expected error (no _margin.json files), got %s", status)
		}
	})
}

// =============================================================================
// NewSystemService
// =============================================================================

func TestNewSystemService(t *testing.T) {
	svc := NewSystemService("/workdir", "/ledger", "/baseline", nil, nil, nil)
	if svc == nil {
		t.Fatal("expected non-nil service")
	}
	if svc.WorkDir != "/workdir" {
		t.Errorf("expected WorkDir /workdir, got %q", svc.WorkDir)
	}
	if svc.LedgerDir != "/ledger" {
		t.Errorf("expected LedgerDir /ledger, got %q", svc.LedgerDir)
	}
	if svc.BaselinePath != "/baseline" {
		t.Errorf("expected BaselinePath /baseline, got %q", svc.BaselinePath)
	}
}

// =============================================================================
// LoadPhase3Status
// =============================================================================

// =============================================================================
// LoadSystemHealth
// =============================================================================

func TestLoadSystemHealth(t *testing.T) {
	svc := NewSystemService("/tmp/nonexistent", "/tmp/nonexistent", "/tmp/nonexistent", nil, nil, nil)
	health, err := svc.LoadSystemHealth()
	if err != nil {
		t.Fatalf("LoadSystemHealth: %v", err)
	}
	if health.ReplayDataPathOK {
		t.Error("expected ReplayDataPathOK false for nonexistent path")
	}
}

// =============================================================================
// degradedFrom — degraded 語意: 只收 warn/error/partial,
// inactive(未啟用) 與 expected_delay(正常延遲) 不計入降級
// =============================================================================

func TestDegradedFrom_OnlyWarnErrorPartial(t *testing.T) {
	channels := []DataChannelInfo{
		{ChannelID: "ok_ch", Status: "ok"},
		{ChannelID: "warn_ch", Status: "warn"},
		{ChannelID: "error_ch", Status: "error"},
		{ChannelID: "partial_ch", Status: "partial"},
		{ChannelID: "inactive_ch", Status: "inactive"},
		{ChannelID: "delay_ch", Status: "expected_delay"},
	}
	got := degradedFrom(channels)
	want := []string{"warn_ch", "error_ch", "partial_ch"}
	if len(got) != len(want) {
		t.Fatalf("expected %d degraded channels %v, got %d: %v", len(want), want, len(got), got)
	}
	for i, id := range want {
		if got[i] != id {
			t.Errorf("expected degraded[%d]=%q, got %q", i, id, got[i])
		}
	}
}

func TestDegradedFrom_AllOkOrInactive_Empty(t *testing.T) {
	channels := []DataChannelInfo{
		{ChannelID: "a", Status: "ok"},
		{ChannelID: "b", Status: "inactive"},
		{ChannelID: "c", Status: "expected_delay"},
	}
	if got := degradedFrom(channels); len(got) != 0 {
		t.Errorf("expected no degraded channels, got %v", got)
	}
}

// =============================================================================
// LoadClampingEvents
// =============================================================================

func TestLoadClampingEvents_FileNotFound(t *testing.T) {
	svc := NewSystemService("/tmp/nonexistent", "/tmp/nonexistent", "", nil, nil, nil)
	events, err := svc.LoadClampingEvents(10)
	if err != nil {
		t.Fatalf("expected no error when file not found, got %v", err)
	}
	if events == nil {
		t.Error("expected non-nil events slice")
	}
	if len(events) != 0 {
		t.Errorf("expected 0 events, got %d", len(events))
	}
}

// =============================================================================
// LoadConvictionClampingEvents
// =============================================================================

func TestLoadConvictionClampingEvents_FileNotFound(t *testing.T) {
	svc := NewSystemService("/tmp/nonexistent", "/tmp/nonexistent", "", nil, nil, nil)
	events, err := svc.LoadConvictionClampingEvents(10)
	if err != nil {
		t.Fatalf("expected no error when file not found, got %v", err)
	}
	if events == nil {
		t.Error("expected non-nil events slice")
	}
	if len(events) != 0 {
		t.Errorf("expected 0 events, got %d", len(events))
	}
}

// =============================================================================
// SetCycleTracker
// =============================================================================

func TestSetCycleTracker(t *testing.T) {
	svc := NewSystemService("/workdir", "/ledger", "", nil, nil, nil)
	ct := industry.NewCycleTracker()
	svc.SetCycleTracker(ct)
	if svc.CycleTracker == nil {
		t.Error("expected CycleTracker to be set")
	}
}

// =============================================================================
// checkCycleStale
// =============================================================================

func TestCheckCycleStale_NilTracker(t *testing.T) {
	svc := NewSystemService("/workdir", "/ledger", "", nil, nil, nil)
	if svc.checkCycleStale() {
		t.Error("expected false when CycleTracker is nil")
	}
}

func TestIsWeekendGap_Within24Hours(t *testing.T) {
	now := time.Date(2026, 6, 15, 10, 0, 0, 0, time.UTC)
	dataTime := time.Date(2026, 6, 15, 8, 0, 0, 0, time.UTC)
	if isWeekendGap(dataTime, now, 72) {
		t.Error("data within 24h should not be weekend gap")
	}
}

func TestIsWeekendGap_FridayToMonday(t *testing.T) {
	now := time.Date(2026, 6, 15, 10, 0, 0, 0, time.UTC)
	dataTime := time.Date(2026, 6, 12, 18, 0, 0, 0, time.UTC)
	if !isWeekendGap(dataTime, now, 72) {
		t.Error("friday 18:00 to monday 10:00 should be weekend gap")
	}
}

func TestIsWeekendGap_FridayToTuesday(t *testing.T) {
	now := time.Date(2026, 6, 16, 10, 0, 0, 0, time.UTC)
	dataTime := time.Date(2026, 6, 12, 18, 0, 0, 0, time.UTC)
	if isWeekendGap(dataTime, now, 72) {
		t.Error("tuesday is not in weekend gap window")
	}
}

func TestIsWeekendGap_Outside72Hours(t *testing.T) {
	now := time.Date(2026, 6, 17, 10, 0, 0, 0, time.UTC)
	dataTime := time.Date(2026, 6, 12, 18, 0, 0, 0, time.UTC)
	if isWeekendGap(dataTime, now, 72) {
		t.Error("beyond 72h should not be weekend gap")
	}
}

func TestIsWeekendGap_DefaultMaxWeekendHours(t *testing.T) {
	now := time.Date(2026, 6, 15, 10, 0, 0, 0, time.UTC)
	dataTime := time.Date(2026, 6, 12, 18, 0, 0, 0, time.UTC)
	if got := isWeekendGap(dataTime, now, 0); !got {
		t.Error("zero maxWeekendHours should use default 72h")
	}
}

func TestCheckMacroPointHealth(t *testing.T) {
	cst := time.FixedZone("CST", 8*60*60)
	now := time.Date(2026, 7, 5, 10, 0, 0, 0, cst)
	recentTs := now.Add(-2 * time.Hour).Unix()
	warnTs := now.Add(-3 * 24 * time.Hour).Unix()
	errorTs := now.Add(-8 * 24 * time.Hour).Unix()

	t.Run("missing file returns error", func(t *testing.T) {
		status, updated := checkMacroPointHealth("/nonexistent/latest.json", now, "S&P 500", func(s marketdata.MacroDataSnapshot) marketdata.MacroDataPoint { return s.SPXIndex })
		if status != "error" {
			t.Errorf("expected error, got %s", status)
		}
		if !containsAny(updated, "檔案不存在") {
			t.Errorf("expected file-not-found message, got %s", updated)
		}
	})

	t.Run("missing timestamp returns error", func(t *testing.T) {
		tmpDir := t.TempDir()
		p := filepath.Join(tmpDir, "latest.json")
		os.WriteFile(p, []byte(`{"spx_index":{"symbol":"^GSPC","value":0,"change_pct":0,"timestamp":0}}`), 0o644)
		status, updated := checkMacroPointHealth(p, now, "S&P 500", func(s marketdata.MacroDataSnapshot) marketdata.MacroDataPoint { return s.SPXIndex })
		if status != "error" {
			t.Errorf("expected error, got %s", status)
		}
		if !containsAny(updated, "無 S&P 500 資料") {
			t.Errorf("expected missing-data message, got %s", updated)
		}
	})

	t.Run("fresh data returns ok", func(t *testing.T) {
		tmpDir := t.TempDir()
		p := filepath.Join(tmpDir, "latest.json")
		os.WriteFile(p, []byte(`{"spx_index":{"symbol":"^GSPC","value":5400,"change_pct":1.2,"timestamp":`+strconv.FormatInt(recentTs, 10)+`}}`), 0o644)
		status, updated := checkMacroPointHealth(p, now, "S&P 500", func(s marketdata.MacroDataSnapshot) marketdata.MacroDataPoint { return s.SPXIndex })
		if status != "ok" {
			t.Errorf("expected ok, got %s (updated=%s)", status, updated)
		}
	})

	t.Run("stale data returns warn", func(t *testing.T) {
		tmpDir := t.TempDir()
		p := filepath.Join(tmpDir, "latest.json")
		os.WriteFile(p, []byte(`{"spx_index":{"symbol":"^GSPC","value":5400,"change_pct":1.2,"timestamp":`+strconv.FormatInt(warnTs, 10)+`}}`), 0o644)
		status, updated := checkMacroPointHealth(p, now, "S&P 500", func(s marketdata.MacroDataSnapshot) marketdata.MacroDataPoint { return s.SPXIndex })
		if status != "warn" {
			t.Errorf("expected warn, got %s (updated=%s)", status, updated)
		}
		if !containsAny(updated, "天前") {
			t.Errorf("expected age message, got %s", updated)
		}
	})

	t.Run("very stale data returns error", func(t *testing.T) {
		tmpDir := t.TempDir()
		p := filepath.Join(tmpDir, "latest.json")
		os.WriteFile(p, []byte(`{"spx_index":{"symbol":"^GSPC","value":5400,"change_pct":1.2,"timestamp":`+strconv.FormatInt(errorTs, 10)+`}}`), 0o644)
		status, updated := checkMacroPointHealth(p, now, "S&P 500", func(s marketdata.MacroDataSnapshot) marketdata.MacroDataPoint { return s.SPXIndex })
		if status != "error" {
			t.Errorf("expected error, got %s (updated=%s)", status, updated)
		}
		if !containsAny(updated, "已超過 7 天閾值") {
			t.Errorf("expected threshold message, got %s", updated)
		}
	})
}

// =============================================================================
// Runtime BuildInfo Block (E08 — system health runtime version reconciliation)
// =============================================================================
//
// Spec: docs/specs/capital-flow-seven-dimension-spec.md §11.4 + Task 9 brief.
// Production must populate SystemHealthResponse.Runtime from the buildinfo
// package so a deployed binary's commit/version can be audited from the health
// endpoint, not by reading git log.

// TestSystemHealth_RuntimeBlockFromBuildinfo verifies that LoadSystemHealth
// populates the new Runtime block from internal/buildinfo, surfacing the
// injected Version and Commit.
//
// RED state today: SystemHealthResponse has no Runtime field → compile fail.
// GREEN state in Task 11: field exists, populated via buildinfo.Current().
// We mutate buildinfo.Version/Commit/BuildTime directly with a t.Cleanup so
// no leakage across tests in this binary.
func TestSystemHealth_RuntimeBlockFromBuildinfo(t *testing.T) {
	originalVer := buildinfo.Version
	originalCommit := buildinfo.Commit
	originalBuildTime := buildinfo.BuildTime
	t.Cleanup(func() {
		buildinfo.Version = originalVer
		buildinfo.Commit = originalCommit
		buildinfo.BuildTime = originalBuildTime
	})

	buildinfo.Version = "v0.0.0.32-test"
	buildinfo.Commit = "deadbeef1234"
	buildinfo.BuildTime = "2026-07-17T00:00:00Z"

	svc := NewSystemService("/tmp/nonexistent", "/tmp/nonexistent", "/tmp/nonexistent", nil, nil, nil)
	health, err := svc.LoadSystemHealth()
	if err != nil {
		t.Fatalf("LoadSystemHealth: %v", err)
	}
	if health.Runtime == nil {
		t.Fatal("expected SystemHealthResponse.Runtime to be populated")
	}
	if health.Runtime.Commit == "" {
		t.Error("expected Runtime.Commit to be non-empty (buildinfo default or injected)")
	}
	if health.Runtime.Version != "v0.0.0.32-test" {
		t.Errorf("Runtime.Version: want %q, got %q", "v0.0.0.32-test", health.Runtime.Version)
	}
	if health.Runtime.GoVersion == "" {
		t.Error("expected Runtime.GoVersion to be non-empty")
	}
}

// TestSystemHealth_RuntimeBlockStableForUninjectedBuild pins the default-value
// contract when no -ldflags injection happens (i.e. plain `go test`).
// Guards against the "Runtime.Version = \"\"" regression: empty string must
// be replaced with the spec'd "unknown" sentinel.
func TestSystemHealth_RuntimeBlockStableForUninjectedBuild(t *testing.T) {
	// No mutation: relies on the package-default "unknown" sentinels.
	svc := NewSystemService("/tmp/nonexistent", "/tmp/nonexistent", "/tmp/nonexistent", nil, nil, nil)
	health, err := svc.LoadSystemHealth()
	if err != nil {
		t.Fatalf("LoadSystemHealth: %v", err)
	}
	if health.Runtime == nil {
		t.Fatal("expected Runtime to be populated even without injection")
	}
	if health.Runtime.Version == "" {
		t.Error("expected Runtime.Version to never be empty (use 'unknown' sentinel)")
	}
	if health.Runtime.GoVersion == "" {
		t.Error("expected Runtime.GoVersion to be auto-filled, never empty")
	}
}

func TestLoadSystemHealth_YahooChannels(t *testing.T) {
	tmpDir := t.TempDir()
	macroDir := filepath.Join(tmpDir, "data", "state", "macro")
	os.MkdirAll(macroDir, 0o755)
	now := time.Now()
	macroJSON := `{
		"recorded_at": ` + strconv.FormatInt(now.Unix(), 10) + `,
		"spx_index":{"symbol":"^GSPC","value":5400,"change_pct":1.2,"timestamp":` + strconv.FormatInt(now.Unix(), 10) + `},
		"ndx_index":{"symbol":"^NDX","value":19500,"change_pct":1.5,"timestamp":` + strconv.FormatInt(now.Unix(), 10) + `},
		"dji_index":{"symbol":"^DJI","value":39000,"change_pct":0.8,"timestamp":` + strconv.FormatInt(now.Unix(), 10) + `},
		"sox_index":{"symbol":"^SOX","value":4500,"change_pct":2.1,"timestamp":` + strconv.FormatInt(now.Unix(), 10) + `},
		"nvda":{"symbol":"NVDA","value":135,"change_pct":3.0,"timestamp":` + strconv.FormatInt(now.Unix(), 10) + `},
		"aapl":{"symbol":"AAPL","value":220,"change_pct":0.5,"timestamp":` + strconv.FormatInt(now.Unix(), 10) + `},
		"msft":{"symbol":"MSFT","value":440,"change_pct":0.9,"timestamp":` + strconv.FormatInt(now.Unix(), 10) + `},
		"tsm_adr":{"symbol":"TSM","value":180,"change_pct":1.1,"timestamp":` + strconv.FormatInt(now.Unix(), 10) + `}
	}`
	os.WriteFile(filepath.Join(macroDir, "latest.json"), []byte(macroJSON), 0o644)

	svc := NewSystemService(tmpDir, filepath.Join(tmpDir, "ledger"), "", nil, nil, nil)
	health, err := svc.LoadSystemHealth()
	if err != nil {
		t.Fatalf("LoadSystemHealth: %v", err)
	}

	expectedIDs := map[string]bool{
		"us_spx":    true,
		"us_ndx":    true,
		"us_dji":    true,
		"sox_index": true,
		"us_nvda":   true,
		"us_aapl":   true,
		"us_msft":   true,
		"tsm_adr":   true,
	}
	found := make(map[string]bool)
	for _, ch := range health.DataChannels {
		if expectedIDs[ch.ChannelID] {
			found[ch.ChannelID] = true
			if ch.Status != "ok" {
				t.Errorf("channel %s expected ok, got %s (%s)", ch.ChannelID, ch.Status, ch.StatusText)
			}
		}
	}
	for id := range expectedIDs {
		if !found[id] {
			t.Errorf("expected channel %s not found in DataChannels", id)
		}
	}
}

// =============================================================================
// resolveRegime — R120: system-health regime must match /api/regime/history
// =============================================================================

// TestLoadSystemHealth_RegimeFromRegimeHistory verifies that when the
// authoritative regime_history store is wired, LoadSystemHealth surfaces the
// latest regime_history row (macro_ingest 收盤權威值) with RegimeSource
// "regime_history", consistent with /api/regime/history.
func TestLoadSystemHealth_RegimeFromRegimeHistory(t *testing.T) {
	svc := NewSystemService("/tmp/nonexistent", "/tmp/nonexistent", "/tmp/nonexistent", nil, nil, nil)
	svc.SetHistoricalStore(&mockHistoricalStore{
		rows: []ledger.RegimeRow{
			{Date: "2026-08-19", Regime: "RISK_ON"},
			{Date: "2026-08-18", Regime: "RISK_OFF"},
		},
	})

	health, err := svc.LoadSystemHealth()
	if err != nil {
		t.Fatalf("LoadSystemHealth: %v", err)
	}
	if health.Regime != domain.RegimeRiskOn {
		t.Errorf("Regime = %q, want RISK_ON", health.Regime)
	}
	if health.RegimeSource != "regime_history" {
		t.Errorf("RegimeSource = %q, want regime_history", health.RegimeSource)
	}
}

// TestLoadSystemHealth_RegimeSource_FallbackSession verifies the fallback path:
// when regime_history is absent (no historical store wired), the regime comes
// from the latest session summary and RegimeSource is "session_summary".
func TestLoadSystemHealth_RegimeSource_FallbackSession(t *testing.T) {
	svc := NewSystemService("/tmp/nonexistent", "/tmp/nonexistent", "/tmp/nonexistent", nil, nil, nil)

	health, err := svc.LoadSystemHealth()
	if err != nil {
		t.Fatalf("LoadSystemHealth: %v", err)
	}
	if health.RegimeSource != "session_summary" {
		t.Errorf("RegimeSource = %q, want session_summary", health.RegimeSource)
	}
	// With no session summaries available the fallback yields Neutral.
	if health.Regime != domain.RegimeNeutral {
		t.Errorf("Regime = %q, want NEUTRAL on empty fallback", health.Regime)
	}
}

// TestResolveRegime_UsesRegimeHistory verifies resolveRegime picks the latest
// regime_history row when the store is available.
func TestResolveRegime_UsesRegimeHistory(t *testing.T) {
	svc := NewSystemService("/tmp/nonexistent", "/tmp/nonexistent", "/tmp/nonexistent", nil, nil, nil)
	svc.SetHistoricalStore(&mockHistoricalStore{
		rows: []ledger.RegimeRow{{Date: "2026-08-19", Regime: "RISK_OFF"}},
	})
	regime, source := svc.resolveRegime()
	if regime != domain.RegimeRiskOff {
		t.Errorf("resolveRegime regime = %q, want RISK_OFF", regime)
	}
	if source != "regime_history" {
		t.Errorf("resolveRegime source = %q, want regime_history", source)
	}
}

// TestResolveRegime_EmptyHistoryFallsBackToSession ensures an empty (or error)
// regime_history falls back to the session-summary source label.
func TestResolveRegime_EmptyHistoryFallsBack(t *testing.T) {
	svc := NewSystemService("/tmp/nonexistent", "/tmp/nonexistent", "/tmp/nonexistent", nil, nil, nil)
	svc.SetHistoricalStore(&mockHistoricalStore{}) // no rows
	regime, source := svc.resolveRegime()
	if source != "session_summary" {
		t.Errorf("resolveRegime source = %q, want session_summary (empty regime_history)", source)
	}
	_ = regime

	svcErr := NewSystemService("/tmp/nonexistent", "/tmp/nonexistent", "/tmp/nonexistent", nil, nil, nil)
	svcErr.SetHistoricalStore(&mockHistoricalStore{err: context.DeadlineExceeded})
	_, sourceErr := svcErr.resolveRegime()
	if sourceErr != "session_summary" {
		t.Errorf("resolveRegime source = %q, want session_summary (store error)", sourceErr)
	}
}

// TestSetHistoricalStore verifies the setter wires the store reference.
func TestSetHistoricalStore(t *testing.T) {
	svc := NewSystemService("/workdir", "/ledger", "/baseline", nil, nil, nil)
	hs := &mockHistoricalStore{}
	svc.SetHistoricalStore(hs)
	if svc.historicalStore != hs {
		t.Error("expected SetHistoricalStore to wire the historical store")
	}
}
