package apigateway

import "testing"

func TestExchangeRateChannelAdapter_Metadata(t *testing.T) {
	a := &ExchangeRateChannelAdapter{}
	m := a.Metadata()
	if m.ChannelID != "exchange_rate" {
		t.Errorf("ChannelID = %q, want exchange_rate", m.ChannelID)
	}
	if m.Country != "全球" {
		t.Errorf("Country = %q, want 全球", m.Country)
	}
	if m.Platform != "Frankfurter/ECB" {
		t.Errorf("Platform = %q, want Frankfurter/ECB", m.Platform)
	}
	if m.APIFormat != "REST JSON" {
		t.Errorf("APIFormat = %q, want REST JSON", m.APIFormat)
	}
	if m.Path != "api.frankfurter.dev" {
		t.Errorf("Path = %q, want api.frankfurter.dev", m.Path)
	}
	if !m.HasLimiter {
		t.Error("HasLimiter should be true")
	}
}

func TestExchangeRateChannelAdapter_RateLimit(t *testing.T) {
	a := NewExchangeRateChannelAdapter(nil)
	if a == nil {
		t.Fatal("NewExchangeRateChannelAdapter returned nil")
	}
	limiter := a.RateLimit()
	if limiter == nil {
		t.Fatal("RateLimit() returned nil")
	}
}
