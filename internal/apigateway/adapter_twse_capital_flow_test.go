package apigateway

import "testing"

func TestTWSECapitalFlowChannelAdapter_Metadata(t *testing.T) {
	a := &TWSECapitalFlowChannelAdapter{}
	m := a.Metadata()
	if m.ChannelID != "twse_capital_flow" {
		t.Errorf("ChannelID = %q, want twse_capital_flow", m.ChannelID)
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
	if m.Path != "www.twse.com.tw/rwd/zh/fund/T86" {
		t.Errorf("Path = %q, want www.twse.com.tw/rwd/zh/fund/T86", m.Path)
	}
	if !m.HasLimiter {
		t.Error("HasLimiter should be true")
	}
}

func TestTWSECapitalFlowChannelAdapter_RateLimit(t *testing.T) {
	a := NewTWSECapitalFlowChannelAdapter(nil)
	if a == nil {
		t.Fatal("NewTWSECapitalFlowChannelAdapter returned nil")
	}
	limiter := a.RateLimit()
	if limiter == nil {
		t.Fatal("RateLimit() returned nil")
	}
}
