package apigateway

import "testing"

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
