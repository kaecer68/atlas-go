package apigateway

import (
	"testing"
	"time"

	"golang.org/x/time/rate"
)

func TestDayTradingChannelAdapter_Metadata(t *testing.T) {
	a := &DayTradingChannelAdapter{}
	m := a.Metadata()
	if m.ChannelID != "day_trading" {
		t.Errorf("ChannelID = %q, want day_trading", m.ChannelID)
	}
	if m.Country != "台灣" {
		t.Errorf("Country = %q, want 台灣", m.Country)
	}
	if m.Platform != "TWSE" {
		t.Errorf("Platform = %q, want TWSE", m.Platform)
	}
	if m.APIFormat != "REST JSON" {
		t.Errorf("APIFormat = %q, want REST JSON", m.APIFormat)
	}
	if m.Path != "www.twse.com.tw/exchangeReport/TWTB4U" {
		t.Errorf("Path = %q, want www.twse.com.tw/exchangeReport/TWTB4U", m.Path)
	}
	if !m.HasLimiter {
		t.Error("HasLimiter should be true")
	}
}

func TestDayTradingChannelAdapter_RateLimit(t *testing.T) {
	a := &DayTradingChannelAdapter{
		limiter: rate.NewLimiter(rate.Every(5*time.Second), 1),
	}
	limiter := a.RateLimit()
	if limiter == nil {
		t.Fatal("RateLimit() returned nil")
	}
}
