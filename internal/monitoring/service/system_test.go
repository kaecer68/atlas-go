package service

import (
	"os"
	"path/filepath"
	"testing"
	"time"
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
