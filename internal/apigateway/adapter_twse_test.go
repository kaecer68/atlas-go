package apigateway

import "testing"

func TestTWSEChannelAdapter_Metadata(t *testing.T) {
	a := &TWSEChannelAdapter{}
	m := a.Metadata()
	if m.ChannelID != "twse_replay" {
		t.Errorf("ChannelID = %q, want twse_replay", m.ChannelID)
	}
	if m.Country != "台灣" {
		t.Errorf("Country = %q, want 台灣", m.Country)
	}
	if m.Platform != "TWSE 證交所" {
		t.Errorf("Platform = %q, want TWSE 證交所", m.Platform)
	}
	if m.APIFormat != "json" {
		t.Errorf("APIFormat = %q, want json", m.APIFormat)
	}
	if m.Path != "www.twse.com.tw" {
		t.Errorf("Path = %q, want www.twse.com.tw", m.Path)
	}
	if !m.HasLimiter {
		t.Error("HasLimiter should be true")
	}
}

func TestTWSEChannelAdapter_RateLimit(t *testing.T) {
	a := NewTWSEChannelAdapter(nil)
	if a == nil {
		t.Fatal("NewTWSEChannelAdapter returned nil")
	}
	limiter := a.RateLimit()
	if limiter == nil {
		t.Fatal("RateLimit() returned nil")
	}
}
