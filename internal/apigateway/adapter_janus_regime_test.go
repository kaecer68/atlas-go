package apigateway

import "testing"

func TestJANUSRegimeChannelAdapter_Metadata(t *testing.T) {
	a := &JANUSRegimeChannelAdapter{}
	m := a.Metadata()
	if m.ChannelID != "janus_regime" {
		t.Errorf("ChannelID = %q, want janus_regime", m.ChannelID)
	}
	if m.Country != "全域" {
		t.Errorf("Country = %q, want 全域", m.Country)
	}
	if m.Platform != "JANUS Engine" {
		t.Errorf("Platform = %q, want JANUS Engine", m.Platform)
	}
	if m.APIFormat != "Internal (computed)" {
		t.Errorf("APIFormat = %q, want Internal (computed)", m.APIFormat)
	}
	if m.Path != "internal/janus" {
		t.Errorf("Path = %q, want internal/janus", m.Path)
	}
	if m.HasLimiter {
		t.Error("HasLimiter should be false for computed channel")
	}
}

func TestJANUSRegimeChannelAdapter_RateLimit(t *testing.T) {
	a := NewJANUSRegimeChannelAdapter(nil)
	if a == nil {
		t.Fatal("NewJANUSRegimeChannelAdapter returned nil")
	}
	limiter := a.RateLimit()
	if limiter == nil {
		t.Fatal("RateLimit() returned nil")
	}
}
