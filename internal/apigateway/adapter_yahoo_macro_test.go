package apigateway

import (
	"context"
	"testing"

	"github.com/kaecer68/atlas-go/internal/marketdata"
)

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
	a := NewYahooMacroChannelAdapter(marketdata.NewYahooFinanceMacroProvider())
	if a == nil {
		t.Fatal("NewYahooMacroChannelAdapter returned nil")
	}
	limiter := a.RateLimit()
	if limiter == nil {
		t.Fatal("RateLimit() returned nil")
	}
}

func TestYahooMacroChannelAdapter_Fetch(t *testing.T) {
	writeParametersJSON(t, nil)
	setupYahooMockServer(t)

	a := NewYahooMacroChannelAdapter(marketdata.NewYahooFinanceMacroProvider())
	res, err := a.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if res == nil || len(res.Data) == 0 {
		t.Fatal("Fetch() returned empty data")
	}
	if res.Meta.ChannelID != "us_yahoo" {
		t.Errorf("ChannelID = %q, want us_yahoo", res.Meta.ChannelID)
	}
}

func TestYahooMacroChannelAdapter_HealthCheck(t *testing.T) {
	writeParametersJSON(t, nil)
	setupYahooMockServer(t)

	a := NewYahooMacroChannelAdapter(marketdata.NewYahooFinanceMacroProvider())
	status, err := a.HealthCheck(context.Background())
	if err != nil {
		t.Fatalf("HealthCheck() error = %v", err)
	}
	if status.Status != "ok" {
		t.Errorf("Status = %q, want ok", status.Status)
	}
}

func TestYahooMacroChannelAdapter_HealthCheck_Partial(t *testing.T) {
	writeParametersJSON(t, nil)
	setupYahooMockServer(t)

	a := NewYahooMacroChannelAdapter(marketdata.NewYahooFinanceMacroProvider())
	status, err := a.HealthCheck(context.Background())
	if err != nil {
		t.Fatalf("HealthCheck() error = %v", err)
	}
	if status.Status != "ok" && status.Status != "warn" {
		t.Errorf("Status = %q, want ok or warn", status.Status)
	}
}
