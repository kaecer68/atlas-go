package apigateway

import "testing"

func TestTEJChannelAdapter_Metadata(t *testing.T) {
	a := &TEJChannelAdapter{}
	m := a.Metadata()
	if m.ChannelID != "tej" {
		t.Errorf("ChannelID = %q, want tej", m.ChannelID)
	}
	if m.Country != "台灣" {
		t.Errorf("Country = %q, want 台灣", m.Country)
	}
	if m.Platform != "TEJ 台灣經濟新報" {
		t.Errorf("Platform = %q, want TEJ 台灣經濟新報", m.Platform)
	}
	if m.APIFormat != "REST JSON" {
		t.Errorf("APIFormat = %q, want REST JSON", m.APIFormat)
	}
	if m.Path != "api.tej.com.tw" {
		t.Errorf("Path = %q, want api.tej.com.tw", m.Path)
	}
	if !m.HasLimiter {
		t.Error("HasLimiter should be true")
	}
}

func TestTEJChannelAdapter_RateLimit(t *testing.T) {
	a := NewTEJChannelAdapter(nil)
	if a == nil {
		t.Fatal("NewTEJChannelAdapter returned nil")
	}
	limiter := a.RateLimit()
	if limiter == nil {
		t.Fatal("RateLimit() returned nil")
	}
}
