package apigateway

import "testing"

func TestTaifexChannelAdapter_Metadata(t *testing.T) {
	a := NewTaifexChannelAdapter()
	m := a.Metadata()
	if m.ChannelID != "taifex_daily" {
		t.Errorf("ChannelID = %q, want taifex_daily", m.ChannelID)
	}
	if m.Country != "台灣" {
		t.Errorf("Country = %q, want 台灣", m.Country)
	}
	if m.Platform != "TAIFEX 期交所" {
		t.Errorf("Platform = %q, want TAIFEX 期交所", m.Platform)
	}
	if m.APIFormat != "REST JSON" {
		t.Errorf("APIFormat = %q, want REST JSON", m.APIFormat)
	}
	if m.Path != "openapi.taifex.com.tw/v1/PutCallRatio" {
		t.Errorf("Path = %q, want openapi.taifex.com.tw/v1/PutCallRatio", m.Path)
	}
	if !m.HasLimiter {
		t.Error("HasLimiter should be true")
	}
}

func TestTaifexChannelAdapter_RateLimit(t *testing.T) {
	a := NewTaifexChannelAdapter()
	if a == nil {
		t.Fatal("NewTaifexChannelAdapter returned nil")
	}
	limiter := a.RateLimit()
	if limiter == nil {
		t.Fatal("RateLimit() returned nil")
	}
}
