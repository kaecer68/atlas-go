package apigateway

import "testing"

func TestUSCPIChannelAdapter_Metadata(t *testing.T) {
	a := &USCPIChannelAdapter{}
	m := a.Metadata()
	if m.ChannelID != "us_cpi" {
		t.Errorf("ChannelID = %q, want us_cpi", m.ChannelID)
	}
	if m.Country != "美國" {
		t.Errorf("Country = %q, want 美國", m.Country)
	}
	if m.Platform != "BLS" {
		t.Errorf("Platform = %q, want BLS", m.Platform)
	}
	if m.APIFormat != "REST JSON" {
		t.Errorf("APIFormat = %q, want REST JSON", m.APIFormat)
	}
	if m.Path != "api.bls.gov" {
		t.Errorf("Path = %q, want api.bls.gov", m.Path)
	}
	if !m.HasLimiter {
		t.Error("HasLimiter should be true")
	}
}

func TestUSCPIChannelAdapter_RateLimit(t *testing.T) {
	a := NewUSCPIChannelAdapter(nil)
	if a == nil {
		t.Fatal("NewUSCPIChannelAdapter returned nil")
	}
	if a.RateLimit() == nil {
		t.Fatal("RateLimit() returned nil")
	}
}
