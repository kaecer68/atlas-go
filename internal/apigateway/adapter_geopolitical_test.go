package apigateway

import (
	"testing"
	"time"

	"golang.org/x/time/rate"
)

func TestGeopoliticalChannelAdapter_Metadata(t *testing.T) {
	a := &GeopoliticalChannelAdapter{}
	m := a.Metadata()
	if m.ChannelID != "geopolitical" {
		t.Errorf("ChannelID = %q, want geopolitical", m.ChannelID)
	}
	if m.Country != "全球" {
		t.Errorf("Country = %q, want 全球", m.Country)
	}
	if m.Platform != "RSS + GDELT" {
		t.Errorf("Platform = %q, want RSS + GDELT", m.Platform)
	}
	if m.APIFormat != "Composite" {
		t.Errorf("APIFormat = %q, want Composite", m.APIFormat)
	}
	if m.Path != "geopolitical" {
		t.Errorf("Path = %q, want geopolitical", m.Path)
	}
	if !m.HasLimiter {
		t.Error("HasLimiter should be true")
	}
}

func TestGeopoliticalChannelAdapter_RateLimit(t *testing.T) {
	a := &GeopoliticalChannelAdapter{
		limiter: rate.NewLimiter(rate.Every(time.Minute), 1),
	}
	limiter := a.RateLimit()
	if limiter == nil {
		t.Fatal("RateLimit() returned nil")
	}
}
