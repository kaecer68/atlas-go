package apigateway

import "testing"

func TestTSMCRevenueChannelAdapter_Metadata(t *testing.T) {
	a := &TSMCRevenueChannelAdapter{}
	m := a.Metadata()
	if m.ChannelID != "tsmc_revenue" {
		t.Errorf("ChannelID = %q, want tsmc_revenue", m.ChannelID)
	}
	if m.Country != "台灣" {
		t.Errorf("Country = %q, want 台灣", m.Country)
	}
	if m.Platform != "TWSE 台積電月營收" {
		t.Errorf("Platform = %q, want TWSE 台積電月營收", m.Platform)
	}
	if m.APIFormat != "REST JSON / FinMind TWT49U" {
		t.Errorf("APIFormat = %q, want REST JSON / FinMind TWT49U", m.APIFormat)
	}
	if m.Path != "api.finmindtrade.com / www.twse.com.tw" {
		t.Errorf("Path = %q, want api.finmindtrade.com / www.twse.com.tw", m.Path)
	}
	if !m.HasLimiter {
		t.Error("HasLimiter should be true")
	}
}

func TestTSMCRevenueChannelAdapter_RateLimit(t *testing.T) {
	a := NewTSMCRevenueChannelAdapter(nil)
	if a == nil {
		t.Fatal("NewTSMCRevenueChannelAdapter returned nil")
	}
	limiter := a.RateLimit()
	if limiter == nil {
		t.Fatal("RateLimit() returned nil")
	}
}
