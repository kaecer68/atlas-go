package apigateway

import "testing"

func TestFubonChannelAdapter_Metadata(t *testing.T) {
	a := &FubonChannelAdapter{}
	m := a.Metadata()
	if m.ChannelID != "fubon" {
		t.Errorf("ChannelID = %q, want fubon", m.ChannelID)
	}
	if m.Country != "台灣" {
		t.Errorf("Country = %q, want 台灣", m.Country)
	}
	if m.Platform != "富邦證券" {
		t.Errorf("Platform = %q, want 富邦證券", m.Platform)
	}
	if m.APIFormat != "REST JSON" {
		t.Errorf("APIFormat = %q, want REST JSON", m.APIFormat)
	}
	if m.Path != "api.fubon.com.tw (via Python proxy)" {
		t.Errorf("Path = %q, want api.fubon.com.tw (via Python proxy)", m.Path)
	}
	if !m.HasLimiter {
		t.Error("HasLimiter should be true")
	}
}

func TestFubonChannelAdapter_RateLimit(t *testing.T) {
	a := NewFubonChannelAdapter(nil)
	if a == nil {
		t.Fatal("NewFubonChannelAdapter returned nil")
	}
	limiter := a.RateLimit()
	if limiter == nil {
		t.Fatal("RateLimit() returned nil")
	}
}
