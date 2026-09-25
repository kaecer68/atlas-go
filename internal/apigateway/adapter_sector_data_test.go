package apigateway

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/marketdata"
)

func TestSectorDataChannelAdapter_Metadata(t *testing.T) {
	a := &SectorDataChannelAdapter{}
	m := a.Metadata()
	if m.ChannelID != "sector_data" {
		t.Errorf("ChannelID = %q, want sector_data", m.ChannelID)
	}
	if m.Country != "台灣" {
		t.Errorf("Country = %q, want 台灣", m.Country)
	}
	if m.Platform != "TWSE" {
		t.Errorf("Platform = %q, want TWSE", m.Platform)
	}
	if m.APIFormat != "CSV/JSON" {
		t.Errorf("APIFormat = %q, want CSV/JSON", m.APIFormat)
	}
	// Issue #1944 Batch 2 (Q6 I14): the advertised path is the single-authority
	// location; data/state/sector_data was never written by anything.
	if m.Path != marketdata.SectorDataDirRel {
		t.Errorf("Path = %q, want %q", m.Path, marketdata.SectorDataDirRel)
	}
	if m.HasLimiter {
		t.Error("HasLimiter should be false for file-based channel")
	}
}

func TestSectorDataChannelAdapter_RateLimit(t *testing.T) {
	a := NewSectorDataChannelAdapter(nil)
	if a == nil {
		t.Fatal("NewSectorDataChannelAdapter returned nil")
	}
	limiter := a.RateLimit()
	if limiter == nil {
		t.Fatal("RateLimit() returned nil")
	}
}

func TestSectorDataChannelAdapter_Fetch(t *testing.T) {
	tmpDir := t.TempDir()
	fixture := `{
		"ai_revenue_growth": 0.25,
		"cowos_utilization": 0.85,
		"capex_growth": 0.12,
		"semiconductor_index": 4500.5,
		"updated_at": "2026-06-01T00:00:00+08:00"
	}`
	if err := os.WriteFile(filepath.Join(tmpDir, "sector_data.json"), []byte(fixture), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	provider := marketdata.NewSectorDataProvider(tmpDir)
	a := NewSectorDataChannelAdapter(provider)

	res, err := a.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if res == nil || len(res.Data) == 0 {
		t.Fatal("Fetch() returned empty data")
	}
	if res.Meta.ChannelID != "sector_data" {
		t.Errorf("ChannelID = %q, want sector_data", res.Meta.ChannelID)
	}
}

// TestSectorDataChannelAdapter_HealthCheck pins the truthful verdicts for a
// file-backed channel (issue #1944 Batch 2, Q6 I14): a fresh file is ok, a file
// older than the contract window is degraded, and a missing file is degraded
// instead of the previous unconditional "ok".
func TestSectorDataChannelAdapter_HealthCheck(t *testing.T) {
	write := func(t *testing.T, updatedAt string) string {
		t.Helper()
		tmpDir := t.TempDir()
		fixture := `{"ai_revenue_growth":0.25,"cowos_utilization":0.85,"capex_growth":0.12,"semiconductor_index":4500.5,"updated_at":"` + updatedAt + `"}`
		if err := os.WriteFile(filepath.Join(tmpDir, "sector_data.json"), []byte(fixture), 0o644); err != nil {
			t.Fatalf("write fixture: %v", err)
		}
		return tmpDir
	}

	t.Run("fresh_file_is_ok", func(t *testing.T) {
		dir := write(t, time.Now().UTC().Format(time.RFC3339))
		a := NewSectorDataChannelAdapter(marketdata.NewSectorDataProvider(dir))
		status, err := a.HealthCheck(context.Background())
		if err != nil {
			t.Fatalf("HealthCheck() error = %v", err)
		}
		if status.Status != "ok" {
			t.Errorf("Status = %q want ok (last_error=%q)", status.Status, status.LastError)
		}
	})

	t.Run("stale_file_is_degraded", func(t *testing.T) {
		dir := write(t, "2026-06-01T00:00:00+08:00")
		a := NewSectorDataChannelAdapter(marketdata.NewSectorDataProvider(dir))
		status, err := a.HealthCheck(context.Background())
		if err != nil {
			t.Fatalf("HealthCheck() error = %v", err)
		}
		if status.Status != "degraded" {
			t.Errorf("Status = %q want degraded", status.Status)
		}
		if !strings.Contains(status.LastError, "stale") {
			t.Errorf("LastError = %q, want it to mention stale", status.LastError)
		}
	})

	t.Run("missing_file_is_degraded", func(t *testing.T) {
		a := NewSectorDataChannelAdapter(marketdata.NewSectorDataProvider(t.TempDir()))
		status, err := a.HealthCheck(context.Background())
		if err != nil {
			t.Fatalf("HealthCheck() error = %v", err)
		}
		if status.Status != "degraded" {
			t.Errorf("Status = %q want degraded", status.Status)
		}
		if !strings.Contains(status.LastError, "not loaded") {
			t.Errorf("LastError = %q, want it to mention not loaded", status.LastError)
		}
	})
}

func TestSectorDataChannelAdapter_Fetch_MissingFile(t *testing.T) {
	tmpDir := t.TempDir()
	provider := marketdata.NewSectorDataProvider(tmpDir)
	a := NewSectorDataChannelAdapter(provider)

	res, err := a.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if res == nil || len(res.Data) == 0 {
		t.Fatal("Fetch() returned empty data for missing file")
	}
}
