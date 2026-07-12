package apigateway

import "testing"

func TestTAIEXIndexChannelAdapter_Metadata(t *testing.T) {
	a := &TAIEXIndexChannelAdapter{}
	m := a.Metadata()
	if m.ChannelID != "taiex_index" {
		t.Errorf("ChannelID = %q, want taiex_index", m.ChannelID)
	}
	if m.Country != "台灣" {
		t.Errorf("Country = %q, want 台灣", m.Country)
	}
	if m.Platform != "Yahoo Finance" {
		t.Errorf("Platform = %q, want Yahoo Finance", m.Platform)
	}
	if m.APIFormat != "REST JSON" {
		t.Errorf("APIFormat = %q, want REST JSON", m.APIFormat)
	}
	if m.Path != "query1.finance.yahoo.com" {
		t.Errorf("Path = %q, want query1.finance.yahoo.com", m.Path)
	}
	if !m.HasLimiter {
		t.Error("HasLimiter should be true")
	}
}

func TestTAIEXIndexChannelAdapter_RateLimit(t *testing.T) {
	a := NewTAIEXIndexChannelAdapter(nil)
	if a == nil {
		t.Fatal("NewTAIEXIndexChannelAdapter returned nil")
	}
	limiter := a.RateLimit()
	if limiter == nil {
		t.Fatal("RateLimit() returned nil")
	}
}
