package marketdata

import (
	"testing"
)

func TestProviderNamesAndConstructors(t *testing.T) {
	tests := []struct {
		name     string
		factory  func() interface{ Name() string }
		wantName string
	}{
		{"SPX", func() interface{ Name() string } { return NewSPXIndexProvider() }, "us_spx"},
		{"NDX", func() interface{ Name() string } { return NewNDXIndexProvider() }, "us_ndx"},
		{"DJI", func() interface{ Name() string } { return NewDJIIndexProvider() }, "us_dji"},
		{"NVDA", func() interface{ Name() string } { return NewNVDAProvider() }, "us_nvda"},
		{"AAPL", func() interface{ Name() string } { return NewAAPLProvider() }, "us_aapl"},
		{"MSFT", func() interface{ Name() string } { return NewMSFTProvider() }, "us_msft"},
		{"TSMADR", func() interface{ Name() string } { return NewTSMADRProvider() }, "tsm_adr"},
		{"DRAM", func() interface{ Name() string } { return NewDRAMSpotPriceProvider() }, "dram_spot_price"},
		{"DayTrading", func() interface{ Name() string } { return NewDayTradingProvider() }, "twse_day_trading"},
		{"ETF", func() interface{ Name() string } { return NewTWSEETFProvider() }, "twse_etf"},
		{"OddLot", func() interface{ Name() string } { return NewTWSEOddLotProvider() }, "twse_oddlot"},
		{"TAIFEX", func() interface{ Name() string } { return NewTAIFEXProvider() }, "taifex"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := tc.factory()
			if p == nil {
				t.Fatal("constructor returned nil")
			}
			if got := p.Name(); got != tc.wantName {
				t.Errorf("Name() = %q, want %q", got, tc.wantName)
			}
		})
	}
}

func TestTAIFEX_ParseHelpers(t *testing.T) {
	if got := parseInt64("42"); got != 42 {
		t.Errorf("parseInt64(42) = %d, want 42", got)
	}
	if got := parseInt64(""); got != 0 {
		t.Errorf("parseInt64('') = %d, want 0", got)
	}

	if got := parseFloat64("3.14"); got != 3.14 {
		t.Errorf("parseFloat64(3.14) = %f, want 3.14", got)
	}
	if got := parseFloat64(""); got != 0 {
		t.Errorf("parseFloat64('') = %f, want 0", got)
	}

	if got := safePercent(500, 1000); got != 50.0 {
		t.Errorf("safePercent(500, 1000) = %f, want 50.0", got)
	}
	if got := safePercent(500, 0); got != 0 {
		t.Errorf("safePercent(500, 0) = %f, want 0", got)
	}
}

func TestTWSEOddLot_Parse(t *testing.T) {
	if got := parseTWSEFloat("1,940.50"); got != 1940.50 {
		t.Errorf("parseTWSEFloat(1,940.50) = %f, want 1940.50", got)
	}
	if got := parseTWSEFloat(""); got != 0 {
		t.Errorf("parseTWSEFloat('') = %f, want 0", got)
	}
}
