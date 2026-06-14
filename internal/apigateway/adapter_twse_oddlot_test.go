package apigateway

import "testing"

func TestTWSEOddLotChannelAdapter_Metadata(t *testing.T) {
	a := NewTWSEOddLotChannelAdapter()
	m := a.Metadata()
	if m.ChannelID != "twse_oddlot" {
		t.Errorf("ChannelID = %q, want twse_oddlot", m.ChannelID)
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
	if m.Path != "www.twse.com.tw/exchangeReport/BFI84U" {
		t.Errorf("Path = %q, want www.twse.com.tw/exchangeReport/BFI84U", m.Path)
	}
	if !m.HasLimiter {
		t.Error("HasLimiter should be true")
	}
}

func TestTWSEOddLotChannelAdapter_RateLimit(t *testing.T) {
	a := NewTWSEOddLotChannelAdapter()
	if a == nil {
		t.Fatal("NewTWSEOddLotChannelAdapter returned nil")
	}
	limiter := a.RateLimit()
	if limiter == nil {
		t.Fatal("RateLimit() returned nil")
	}
}
