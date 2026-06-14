package apigateway

import "testing"

func TestTWSESectorIndexChannelAdapter_Metadata(t *testing.T) {
	a := &TWSESectorIndexChannelAdapter{}
	m := a.Metadata()
	if m.ChannelID != "twse_sector_index" {
		t.Errorf("ChannelID = %q, want twse_sector_index", m.ChannelID)
	}
	if m.Country != "台灣" {
		t.Errorf("Country = %q, want 台灣", m.Country)
	}
	if m.Platform != "TWSE OpenAPI v1" {
		t.Errorf("Platform = %q, want TWSE OpenAPI v1", m.Platform)
	}
	if m.APIFormat != "REST JSON" {
		t.Errorf("APIFormat = %q, want REST JSON", m.APIFormat)
	}
	if m.Path != "openapi.twse.com.tw" {
		t.Errorf("Path = %q, want openapi.twse.com.tw", m.Path)
	}
	if !m.HasLimiter {
		t.Error("HasLimiter should be true")
	}
}

func TestTWSESectorIndexChannelAdapter_RateLimit(t *testing.T) {
	a := NewTWSESectorIndexChannelAdapter(nil)
	if a == nil {
		t.Fatal("NewTWSESectorIndexChannelAdapter returned nil")
	}
	limiter := a.RateLimit()
	if limiter == nil {
		t.Fatal("RateLimit() returned nil")
	}
}
