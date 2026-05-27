package apigateway

import "testing"

func TestExportStatisticsChannelAdapter_Metadata(t *testing.T) {
	a := &ExportStatisticsChannelAdapter{}
	m := a.Metadata()
	if m.ChannelID != "export_statistics" {
		t.Errorf("ChannelID = %q, want export_statistics", m.ChannelID)
	}
	if m.Country != "台灣" {
		t.Errorf("Country = %q, want 台灣", m.Country)
	}
	if m.Platform != "關務署" {
		t.Errorf("Platform = %q, want 關務署", m.Platform)
	}
	if m.APIFormat != "csv" {
		t.Errorf("APIFormat = %q, want csv", m.APIFormat)
	}
	if m.Path != "opendata.customs.gov.tw" {
		t.Errorf("Path = %q, want opendata.customs.gov.tw", m.Path)
	}
	if !m.HasLimiter {
		t.Error("HasLimiter should be true")
	}
}

func TestExportStatisticsChannelAdapter_RateLimit(t *testing.T) {
	a := NewExportStatisticsChannelAdapter(nil)
	if a == nil {
		t.Fatal("NewExportStatisticsChannelAdapter returned nil")
	}
	limiter := a.RateLimit()
	if limiter == nil {
		t.Fatal("RateLimit() returned nil")
	}
}
