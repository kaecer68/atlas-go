package apigateway

import "testing"

func TestFrankfurterFXChannelAdapter_Metadata(t *testing.T) {
	a := &FrankfurterFXChannelAdapter{}
	m := a.Metadata()
	if m.ChannelID != "frankfurter_fx" {
		t.Errorf("ChannelID = %q, want frankfurter_fx", m.ChannelID)
	}
	if m.Country != "日本" {
		t.Errorf("Country = %q, want 日本", m.Country)
	}
	if m.Platform != "Frankfurter (USD/JPY)" {
		t.Errorf("Platform = %q, want Frankfurter (USD/JPY)", m.Platform)
	}
	if m.APIFormat != "REST JSON" {
		t.Errorf("APIFormat = %q, want REST JSON", m.APIFormat)
	}
	if m.Path != "api.frankfurter.app/latest?from=USD&to=JPY" {
		t.Errorf("Path = %q, want api.frankfurter.app/latest?from=USD&to=JPY", m.Path)
	}
	if !m.HasLimiter {
		t.Error("HasLimiter should be true")
	}
}

func TestFrankfurterFXChannelAdapter_RateLimit(t *testing.T) {
	a := NewFrankfurterFXChannelAdapter(nil)
	if a == nil {
		t.Fatal("NewFrankfurterFXChannelAdapter returned nil")
	}
	limiter := a.RateLimit()
	if limiter == nil {
		t.Fatal("RateLimit() returned nil")
	}
}
