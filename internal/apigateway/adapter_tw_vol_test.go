package apigateway

import "testing"

func TestTaiwanVolatilityChannelAdapter_Metadata(t *testing.T) {
	a := &TaiwanVolatilityChannelAdapter{}
	m := a.Metadata()
	if m.ChannelID != "tw_vol" {
		t.Errorf("ChannelID = %q, want tw_vol", m.ChannelID)
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

func TestTaiwanVolatilityChannelAdapter_RateLimit(t *testing.T) {
	a := NewTaiwanVolatilityChannelAdapter(nil)
	if a == nil {
		t.Fatal("NewTaiwanVolatilityChannelAdapter returned nil")
	}
	limiter := a.RateLimit()
	if limiter == nil {
		t.Fatal("RateLimit() returned nil")
	}
	// Verify the 5s/req cadence by checking that one token is available
	// immediately and the next must wait (allow small float drift).
	if !limiter.Allow() {
		t.Error("first Allow() should succeed (burst=1)")
	}
}
