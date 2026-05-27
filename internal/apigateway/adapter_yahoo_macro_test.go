package apigateway

import "testing"

func TestYahooMacroChannelAdapter_Metadata(t *testing.T) {
	a := &YahooMacroChannelAdapter{}
	m := a.Metadata()
	if m.ChannelID != "us_yahoo" {
		t.Errorf("ChannelID = %q, want us_yahoo", m.ChannelID)
	}
	if m.Country != "美國" {
		t.Errorf("Country = %q, want 美國", m.Country)
	}
	if m.Platform != "Yahoo Finance" {
		t.Errorf("Platform = %q, want Yahoo Finance", m.Platform)
	}
	if m.APIFormat != "json" {
		t.Errorf("APIFormat = %q, want json", m.APIFormat)
	}
	if m.Path != "query1.finance.yahoo.com" {
		t.Errorf("Path = %q, want query1.finance.yahoo.com", m.Path)
	}
	if !m.HasLimiter {
		t.Error("HasLimiter should be true")
	}
}

func TestYahooMacroChannelAdapter_RateLimit(t *testing.T) {
	a := NewYahooMacroChannelAdapter(nil)
	if a == nil {
		t.Fatal("NewYahooMacroChannelAdapter returned nil")
	}
	limiter := a.RateLimit()
	if limiter == nil {
		t.Fatal("RateLimit() returned nil")
	}
}
