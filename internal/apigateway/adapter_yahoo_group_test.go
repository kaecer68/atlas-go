package apigateway

import (
	"context"
	"testing"

	"github.com/kaecer68/atlas-go/internal/marketdata"
)

func TestUSIndexChannelAdapters_HealthCheck(t *testing.T) {
	writeParametersJSON(t, nil)
	setupYahooMockServer(t)

	adapters := []struct {
		name string
		a    DataProvider
	}{
		{"us_spx", NewUSSPXIndexChannelAdapter(marketdata.NewSPXIndexProvider())},
		{"us_ndx", NewUSNDXIndexChannelAdapter(marketdata.NewNDXIndexProvider())},
		{"us_dji", NewUSDJIIndexChannelAdapter(marketdata.NewDJIIndexProvider())},
	}

	for _, tc := range adapters {
		t.Run(tc.name, func(t *testing.T) {
			status, err := tc.a.HealthCheck(context.Background())
			if err != nil {
				t.Fatalf("HealthCheck() error = %v", err)
			}
			if status.Status != "ok" {
				t.Errorf("Status = %q, want ok", status.Status)
			}
		})
	}
}

func TestUSIndexChannelAdapters_Fetch(t *testing.T) {
	writeParametersJSON(t, nil)
	setupYahooMockServer(t)

	adapters := []struct {
		name string
		a    DataProvider
	}{
		{"us_spx", NewUSSPXIndexChannelAdapter(marketdata.NewSPXIndexProvider())},
		{"us_ndx", NewUSNDXIndexChannelAdapter(marketdata.NewNDXIndexProvider())},
		{"us_dji", NewUSDJIIndexChannelAdapter(marketdata.NewDJIIndexProvider())},
	}

	for _, tc := range adapters {
		t.Run(tc.name, func(t *testing.T) {
			res, err := tc.a.Fetch(context.Background())
			if err != nil {
				t.Fatalf("Fetch() error = %v", err)
			}
			if res == nil || len(res.Data) == 0 {
				t.Fatal("Fetch() returned empty data")
			}
		})
	}
}

func TestUSIndexChannelAdapters_RateLimit(t *testing.T) {
	adapters := []struct {
		name string
		a    DataProvider
	}{
		{"us_spx", NewUSSPXIndexChannelAdapter(nil)},
		{"us_ndx", NewUSNDXIndexChannelAdapter(nil)},
		{"us_dji", NewUSDJIIndexChannelAdapter(nil)},
	}

	for _, tc := range adapters {
		t.Run(tc.name, func(t *testing.T) {
			if tc.a.RateLimit() == nil {
				t.Fatal("RateLimit() returned nil")
			}
		})
	}
}

func TestUSIndexChannelAdapters_Metadata(t *testing.T) {
	adapters := []struct {
		name      string
		channelID string
		a         DataProvider
	}{
		{"us_spx", "us_spx", &USSPXIndexChannelAdapter{}},
		{"us_ndx", "us_ndx", &USNDXIndexChannelAdapter{}},
		{"us_dji", "us_dji", &USDJIIndexChannelAdapter{}},
	}

	for _, tc := range adapters {
		t.Run(tc.name, func(t *testing.T) {
			m := tc.a.Metadata()
			if m.ChannelID != tc.channelID {
				t.Errorf("ChannelID = %q, want %q", m.ChannelID, tc.channelID)
			}
		})
	}
}

func TestUSTechChannelAdapters_HealthCheck(t *testing.T) {
	writeParametersJSON(t, nil)
	setupYahooMockServer(t)

	adapters := []struct {
		name string
		a    DataProvider
	}{
		{"us_nvda", NewUSNVDAChannelAdapter(marketdata.NewNVDAProvider())},
		{"us_aapl", NewUSAAPLChannelAdapter(marketdata.NewAAPLProvider())},
		{"us_msft", NewUSMSFTChannelAdapter(marketdata.NewMSFTProvider())},
	}

	for _, tc := range adapters {
		t.Run(tc.name, func(t *testing.T) {
			status, err := tc.a.HealthCheck(context.Background())
			if err != nil {
				t.Fatalf("HealthCheck() error = %v", err)
			}
			if status.Status != "ok" {
				t.Errorf("Status = %q, want ok", status.Status)
			}
		})
	}
}

func TestUSTechChannelAdapters_Fetch(t *testing.T) {
	writeParametersJSON(t, nil)
	setupYahooMockServer(t)

	adapters := []struct {
		name string
		a    DataProvider
	}{
		{"us_nvda", NewUSNVDAChannelAdapter(marketdata.NewNVDAProvider())},
		{"us_aapl", NewUSAAPLChannelAdapter(marketdata.NewAAPLProvider())},
		{"us_msft", NewUSMSFTChannelAdapter(marketdata.NewMSFTProvider())},
	}

	for _, tc := range adapters {
		t.Run(tc.name, func(t *testing.T) {
			res, err := tc.a.Fetch(context.Background())
			if err != nil {
				t.Fatalf("Fetch() error = %v", err)
			}
			if res == nil || len(res.Data) == 0 {
				t.Fatal("Fetch() returned empty data")
			}
		})
	}
}

func TestUSTechChannelAdapters_RateLimit(t *testing.T) {
	adapters := []struct {
		name string
		a    DataProvider
	}{
		{"us_nvda", NewUSNVDAChannelAdapter(nil)},
		{"us_aapl", NewUSAAPLChannelAdapter(nil)},
		{"us_msft", NewUSMSFTChannelAdapter(nil)},
	}

	for _, tc := range adapters {
		t.Run(tc.name, func(t *testing.T) {
			if tc.a.RateLimit() == nil {
				t.Fatal("RateLimit() returned nil")
			}
		})
	}
}

func TestUSTechChannelAdapters_Metadata(t *testing.T) {
	adapters := []struct {
		name      string
		channelID string
		a         DataProvider
	}{
		{"us_nvda", "us_nvda", &USNVDAChannelAdapter{}},
		{"us_aapl", "us_aapl", &USAAPLChannelAdapter{}},
		{"us_msft", "us_msft", &USMSFTChannelAdapter{}},
	}

	for _, tc := range adapters {
		t.Run(tc.name, func(t *testing.T) {
			m := tc.a.Metadata()
			if m.ChannelID != tc.channelID {
				t.Errorf("ChannelID = %q, want %q", m.ChannelID, tc.channelID)
			}
		})
	}
}

func TestSOXIndexChannelAdapter_Fetch(t *testing.T) {
	writeParametersJSON(t, nil)
	setupYahooMockServer(t)

	a := NewSOXIndexChannelAdapter(marketdata.NewSOXIndexProvider())
	res, err := a.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if res == nil || len(res.Data) == 0 {
		t.Fatal("Fetch() returned empty data")
	}
	if res.Meta.ChannelID != "sox_index" {
		t.Errorf("ChannelID = %q, want sox_index", res.Meta.ChannelID)
	}
}

func TestSOXIndexChannelAdapter_HealthCheck(t *testing.T) {
	writeParametersJSON(t, nil)
	setupYahooMockServer(t)

	a := NewSOXIndexChannelAdapter(marketdata.NewSOXIndexProvider())
	status, err := a.HealthCheck(context.Background())
	if err != nil {
		t.Fatalf("HealthCheck() error = %v", err)
	}
	if status.Status != "ok" {
		t.Errorf("Status = %q, want ok", status.Status)
	}
}

func TestTSMADRChannelAdapter_HealthCheck(t *testing.T) {
	writeParametersJSON(t, nil)
	setupYahooMockServer(t)

	a := NewTSMADRChannelAdapter(marketdata.NewTSMADRProvider())
	status, err := a.HealthCheck(context.Background())
	if err != nil {
		t.Fatalf("HealthCheck() error = %v", err)
	}
	if status.Status != "ok" {
		t.Errorf("Status = %q, want ok", status.Status)
	}
}

func TestTSMADRChannelAdapter_Fetch(t *testing.T) {
	writeParametersJSON(t, nil)
	setupYahooMockServer(t)

	a := NewTSMADRChannelAdapter(marketdata.NewTSMADRProvider())
	res, err := a.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if res == nil || len(res.Data) == 0 {
		t.Fatal("Fetch() returned empty data")
	}
}

func TestTSMADRChannelAdapter_RateLimit(t *testing.T) {
	a := NewTSMADRChannelAdapter(nil)
	if a.RateLimit() == nil {
		t.Fatal("RateLimit() returned nil")
	}
}

func TestTSMADRChannelAdapter_Metadata(t *testing.T) {
	a := &TSMADRChannelAdapter{}
	m := a.Metadata()
	if m.ChannelID != "tsm_adr" {
		t.Errorf("ChannelID = %q, want tsm_adr", m.ChannelID)
	}
}

func TestDRAMSpotPriceChannelAdapter_HealthCheck(t *testing.T) {
	writeParametersJSON(t, nil)
	setupYahooMockServer(t)

	a := NewDRAMSpotPriceChannelAdapter(marketdata.NewDRAMSpotPriceProvider())
	status, err := a.HealthCheck(context.Background())
	if err != nil {
		t.Fatalf("HealthCheck() error = %v", err)
	}
	if status.Status != "ok" {
		t.Errorf("Status = %q, want ok", status.Status)
	}
}

func TestDRAMSpotPriceChannelAdapter_Fetch(t *testing.T) {
	writeParametersJSON(t, nil)
	setupYahooMockServer(t)

	a := NewDRAMSpotPriceChannelAdapter(marketdata.NewDRAMSpotPriceProvider())
	res, err := a.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if res == nil || len(res.Data) == 0 {
		t.Fatal("Fetch() returned empty data")
	}
}

func TestDRAMSpotPriceChannelAdapter_RateLimit(t *testing.T) {
	a := NewDRAMSpotPriceChannelAdapter(nil)
	if a.RateLimit() == nil {
		t.Fatal("RateLimit() returned nil")
	}
}

func TestDRAMSpotPriceChannelAdapter_Metadata(t *testing.T) {
	a := &DRAMSpotPriceChannelAdapter{}
	m := a.Metadata()
	if m.ChannelID != "dram_spot_price" {
		t.Errorf("ChannelID = %q, want dram_spot_price", m.ChannelID)
	}
}
