package apigateway

import (
	"testing"
	"time"

	"golang.org/x/time/rate"
)

func TestTaiwanGeopoliticalChannelAdapter_Metadata(t *testing.T) {
	a := &TaiwanGeopoliticalChannelAdapter{}
	m := a.Metadata()
	if m.ChannelID != "geopolitical_taiwan" {
		t.Errorf("ChannelID = %q, want geopolitical_taiwan", m.ChannelID)
	}
	if m.Country != "台灣" {
		t.Errorf("Country = %q, want 台灣", m.Country)
	}
	if m.Platform != "CNA / 自由時報 / TVBS RSS" {
		t.Errorf("Platform = %q, want CNA / 自由時報 / TVBS RSS", m.Platform)
	}
	if m.APIFormat != "RSS XML" {
		t.Errorf("APIFormat = %q, want RSS XML", m.APIFormat)
	}
	if m.Path != "www.cna.com.tw / news.ltn.com.tw / news.tvbs.com.tw" {
		t.Errorf("Path = %q, want www.cna.com.tw / news.ltn.com.tw / news.tvbs.com.tw", m.Path)
	}
	if !m.HasLimiter {
		t.Error("HasLimiter should be true")
	}
}

func TestTaiwanGeopoliticalChannelAdapter_RateLimit(t *testing.T) {
	a := &TaiwanGeopoliticalChannelAdapter{
		limiter: rate.NewLimiter(rate.Every(time.Minute), 1),
	}
	limiter := a.RateLimit()
	if limiter == nil {
		t.Fatal("RateLimit() returned nil")
	}
}
