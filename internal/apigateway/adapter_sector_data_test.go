package apigateway

import (
	"context"
	"os"
	"path/filepath"
	"testing"

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
	if m.Path != "data/state/sector_data" {
		t.Errorf("Path = %q, want data/state/sector_data", m.Path)
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

func TestSectorDataChannelAdapter_HealthCheck(t *testing.T) {
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

	status, err := a.HealthCheck(context.Background())
	if err != nil {
		t.Fatalf("HealthCheck() error = %v", err)
	}
	if status.Status != "ok" {
		t.Errorf("Status = %q, want ok", status.Status)
	}
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
