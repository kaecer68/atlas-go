package apigateway

import "testing"

func TestFinMindChannelAdapter_Metadata(t *testing.T) {
	a := &FinMindChannelAdapter{}
	m := a.Metadata()
	if m.ChannelID != "finmind" {
		t.Errorf("ChannelID = %q, want finmind", m.ChannelID)
	}
	if m.Country != "台灣" {
		t.Errorf("Country = %q, want 台灣", m.Country)
	}
	if m.Platform != "FinMind" {
		t.Errorf("Platform = %q, want FinMind", m.Platform)
	}
	if m.APIFormat != "json" {
		t.Errorf("APIFormat = %q, want json", m.APIFormat)
	}
	if m.Path != "api.finmindtrade.com" {
		t.Errorf("Path = %q, want api.finmindtrade.com", m.Path)
	}
	if !m.HasLimiter {
		t.Error("HasLimiter should be true")
	}
}
