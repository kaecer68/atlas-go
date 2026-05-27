package apigateway

import "testing"

func TestFugleChannelAdapter_Metadata(t *testing.T) {
	a := &FugleChannelAdapter{}
	m := a.Metadata()
	if m.ChannelID != "fugle" {
		t.Errorf("ChannelID = %q, want fugle", m.ChannelID)
	}
	if m.Country != "台灣" {
		t.Errorf("Country = %q, want 台灣", m.Country)
	}
	if m.Platform != "Fugle 富果" {
		t.Errorf("Platform = %q, want Fugle 富果", m.Platform)
	}
	if m.APIFormat != "json" {
		t.Errorf("APIFormat = %q, want json", m.APIFormat)
	}
	if m.Path != "api.fugle.tw" {
		t.Errorf("Path = %q, want api.fugle.tw", m.Path)
	}
	if !m.HasLimiter {
		t.Error("HasLimiter should be true")
	}
}
