package apigateway

import "testing"

func TestSOXIndexChannelAdapter_Metadata(t *testing.T) {
	a := &SOXIndexChannelAdapter{}
	m := a.Metadata()
	if m.ChannelID != "sox_index" {
		t.Errorf("ChannelID = %q, want sox_index", m.ChannelID)
	}
	if m.Country != "美國" {
		t.Errorf("Country = %q, want 美國", m.Country)
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

func TestSOXIndexChannelAdapter_RateLimit(t *testing.T) {
	a := NewSOXIndexChannelAdapter(nil)
	if a == nil {
		t.Fatal("NewSOXIndexChannelAdapter returned nil")
	}
	limiter := a.RateLimit()
	if limiter == nil {
		t.Fatal("RateLimit() returned nil")
	}
}
