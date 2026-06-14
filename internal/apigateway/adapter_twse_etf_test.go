package apigateway

import "testing"

func TestTWSEETFChannelAdapter_Metadata(t *testing.T) {
	a := NewTWSEETFChannelAdapter()
	m := a.Metadata()
	if m.ChannelID != "twse_etf" {
		t.Errorf("ChannelID = %q, want twse_etf", m.ChannelID)
	}
	if m.Country != "台灣" {
		t.Errorf("Country = %q, want 台灣", m.Country)
	}
	if m.Platform != "TWSE" {
		t.Errorf("Platform = %q, want TWSE", m.Platform)
	}
	if m.APIFormat != "REST JSON" {
		t.Errorf("APIFormat = %q, want REST JSON", m.APIFormat)
	}
	if m.Path != "www.twse.com.tw/exchangeReport/TWT44U" {
		t.Errorf("Path = %q, want www.twse.com.tw/exchangeReport/TWT44U", m.Path)
	}
	if !m.HasLimiter {
		t.Error("HasLimiter should be true")
	}
}

func TestTWSEETFChannelAdapter_RateLimit(t *testing.T) {
	a := NewTWSEETFChannelAdapter()
	if a == nil {
		t.Fatal("NewTWSEETFChannelAdapter returned nil")
	}
	limiter := a.RateLimit()
	if limiter == nil {
		t.Fatal("RateLimit() returned nil")
	}
}
