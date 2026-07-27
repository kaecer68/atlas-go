package monitoring

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/narrative/geopolitical"
)

// Helper: creates a fresh adapter and snapshot for each test.
func newApplyFixture() (*macroDataGatewayAdapter, *marketdata.MacroDataSnapshot) {
	return &macroDataGatewayAdapter{}, &marketdata.MacroDataSnapshot{}
}

func TestApplyUSSPX(t *testing.T) {
	t.Run("populates_field", func(t *testing.T) {
		a, snap := newApplyFixture()
		data := []byte(`{"spx_index":{"symbol":"SPX","value":5432.10,"change_pct":0.5,"timestamp":1700000000}}`)
		a.applyUSSPX(snap, data)
		if snap.SPXIndex.Symbol != "SPX" {
			t.Fatalf("expected Symbol=SPX, got %q", snap.SPXIndex.Symbol)
		}
		if snap.SPXIndex.Value != 5432.10 {
			t.Fatalf("expected Value=5432.10, got %f", snap.SPXIndex.Value)
		}
		if snap.SPXIndex.ChangePct != 0.5 {
			t.Fatalf("expected ChangePct=0.5, got %f", snap.SPXIndex.ChangePct)
		}
	})

	t.Run("empty_data_no_crash", func(t *testing.T) {
		a, snap := newApplyFixture()
		// nil data
		a.applyUSSPX(snap, nil)
		if snap.SPXIndex.Symbol != "" {
			t.Fatalf("expected empty Symbol for nil, got %q", snap.SPXIndex.Symbol)
		}
		// empty byte slice
		a.applyUSSPX(snap, []byte{})
		if snap.SPXIndex.Symbol != "" {
			t.Fatalf("expected empty Symbol for []byte{}, got %q", snap.SPXIndex.Symbol)
		}
		// empty JSON object (no spx_index field)
		a.applyUSSPX(snap, []byte(`{}`))
		if snap.SPXIndex.Symbol != "" {
			t.Fatalf("expected empty Symbol for {}, got %q", snap.SPXIndex.Symbol)
		}
	})

	t.Run("symbol_only_no_value", func(t *testing.T) {
		a, snap := newApplyFixture()
		data := []byte(`{"spx_index":{"symbol":"SPX","value":0,"change_pct":0}}`)
		a.applyUSSPX(snap, data)
		if snap.SPXIndex.Symbol != "SPX" {
			t.Fatalf("expected Symbol=SPX when value is 0, got %q", snap.SPXIndex.Symbol)
		}
		if snap.SPXIndex.Value != 0 {
			t.Fatalf("expected Value=0, got %f", snap.SPXIndex.Value)
		}
	})
}

func TestApplyUSNDX(t *testing.T) {
	t.Run("populates_field", func(t *testing.T) {
		a, snap := newApplyFixture()
		data := []byte(`{"ndx_index":{"symbol":"NDX","value":19876.50,"change_pct":-0.30,"timestamp":1700000000}}`)
		a.applyUSNDX(snap, data)
		if snap.NDXIndex.Symbol != "NDX" {
			t.Fatalf("expected Symbol=NDX, got %q", snap.NDXIndex.Symbol)
		}
		if snap.NDXIndex.Value != 19876.50 {
			t.Fatalf("expected Value=19876.50, got %f", snap.NDXIndex.Value)
		}
		if snap.NDXIndex.ChangePct != -0.30 {
			t.Fatalf("expected ChangePct=-0.30, got %f", snap.NDXIndex.ChangePct)
		}
	})

	t.Run("empty_data_no_crash", func(t *testing.T) {
		a, snap := newApplyFixture()
		a.applyUSNDX(snap, nil)
		a.applyUSNDX(snap, []byte{})
		a.applyUSNDX(snap, []byte(`{}`))
		if snap.NDXIndex.Symbol != "" {
			t.Fatalf("expected empty Symbol for empty data, got %q", snap.NDXIndex.Symbol)
		}
	})

	t.Run("symbol_only_no_value", func(t *testing.T) {
		a, snap := newApplyFixture()
		data := []byte(`{"ndx_index":{"symbol":"NDX","value":0,"change_pct":0}}`)
		a.applyUSNDX(snap, data)
		if snap.NDXIndex.Symbol != "NDX" {
			t.Fatalf("expected Symbol=NDX when value is 0, got %q", snap.NDXIndex.Symbol)
		}
		if snap.NDXIndex.Value != 0 {
			t.Fatalf("expected Value=0, got %f", snap.NDXIndex.Value)
		}
	})
}

func TestApplyUSDJI(t *testing.T) {
	t.Run("populates_field", func(t *testing.T) {
		a, snap := newApplyFixture()
		data := []byte(`{"dji_index":{"symbol":"DJI","value":38900.00,"change_pct":0.20,"timestamp":1700000000}}`)
		a.applyUSDJI(snap, data)
		if snap.DJIIndex.Symbol != "DJI" {
			t.Fatalf("expected Symbol=DJI, got %q", snap.DJIIndex.Symbol)
		}
		if snap.DJIIndex.Value != 38900.00 {
			t.Fatalf("expected Value=38900.00, got %f", snap.DJIIndex.Value)
		}
		if snap.DJIIndex.ChangePct != 0.20 {
			t.Fatalf("expected ChangePct=0.20, got %f", snap.DJIIndex.ChangePct)
		}
	})

	t.Run("empty_data_no_crash", func(t *testing.T) {
		a, snap := newApplyFixture()
		a.applyUSDJI(snap, nil)
		a.applyUSDJI(snap, []byte{})
		a.applyUSDJI(snap, []byte(`{}`))
		if snap.DJIIndex.Symbol != "" {
			t.Fatalf("expected empty Symbol for empty data, got %q", snap.DJIIndex.Symbol)
		}
	})

	t.Run("symbol_only_no_value", func(t *testing.T) {
		a, snap := newApplyFixture()
		data := []byte(`{"dji_index":{"symbol":"DJI","value":0,"change_pct":0}}`)
		a.applyUSDJI(snap, data)
		if snap.DJIIndex.Symbol != "DJI" {
			t.Fatalf("expected Symbol=DJI when value is 0, got %q", snap.DJIIndex.Symbol)
		}
		if snap.DJIIndex.Value != 0 {
			t.Fatalf("expected Value=0, got %f", snap.DJIIndex.Value)
		}
	})
}

func TestApplyUSNVDA(t *testing.T) {
	t.Run("populates_field", func(t *testing.T) {
		a, snap := newApplyFixture()
		data := []byte(`{"nvda":{"symbol":"NVDA","value":875.30,"change_pct":1.20,"timestamp":1700000000}}`)
		a.applyUSNVDA(snap, data)
		if snap.NVDA.Symbol != "NVDA" {
			t.Fatalf("expected Symbol=NVDA, got %q", snap.NVDA.Symbol)
		}
		if snap.NVDA.Value != 875.30 {
			t.Fatalf("expected Value=875.30, got %f", snap.NVDA.Value)
		}
		if snap.NVDA.ChangePct != 1.20 {
			t.Fatalf("expected ChangePct=1.20, got %f", snap.NVDA.ChangePct)
		}
	})

	t.Run("empty_data_no_crash", func(t *testing.T) {
		a, snap := newApplyFixture()
		a.applyUSNVDA(snap, nil)
		a.applyUSNVDA(snap, []byte{})
		a.applyUSNVDA(snap, []byte(`{}`))
		if snap.NVDA.Symbol != "" {
			t.Fatalf("expected empty Symbol for empty data, got %q", snap.NVDA.Symbol)
		}
	})

	t.Run("symbol_only_no_value", func(t *testing.T) {
		a, snap := newApplyFixture()
		data := []byte(`{"nvda":{"symbol":"NVDA","value":0,"change_pct":0}}`)
		a.applyUSNVDA(snap, data)
		if snap.NVDA.Symbol != "NVDA" {
			t.Fatalf("expected Symbol=NVDA when value is 0, got %q", snap.NVDA.Symbol)
		}
		if snap.NVDA.Value != 0 {
			t.Fatalf("expected Value=0, got %f", snap.NVDA.Value)
		}
	})
}

func TestApplyUSAAPL(t *testing.T) {
	t.Run("populates_field", func(t *testing.T) {
		a, snap := newApplyFixture()
		data := []byte(`{"aapl":{"symbol":"AAPL","value":178.50,"change_pct":0.80,"timestamp":1700000000}}`)
		a.applyUSAAPL(snap, data)
		if snap.AAPL.Symbol != "AAPL" {
			t.Fatalf("expected Symbol=AAPL, got %q", snap.AAPL.Symbol)
		}
		if snap.AAPL.Value != 178.50 {
			t.Fatalf("expected Value=178.50, got %f", snap.AAPL.Value)
		}
		if snap.AAPL.ChangePct != 0.80 {
			t.Fatalf("expected ChangePct=0.80, got %f", snap.AAPL.ChangePct)
		}
	})

	t.Run("empty_data_no_crash", func(t *testing.T) {
		a, snap := newApplyFixture()
		a.applyUSAAPL(snap, nil)
		a.applyUSAAPL(snap, []byte{})
		a.applyUSAAPL(snap, []byte(`{}`))
		if snap.AAPL.Symbol != "" {
			t.Fatalf("expected empty Symbol for empty data, got %q", snap.AAPL.Symbol)
		}
	})

	t.Run("symbol_only_no_value", func(t *testing.T) {
		a, snap := newApplyFixture()
		data := []byte(`{"aapl":{"symbol":"AAPL","value":0,"change_pct":0}}`)
		a.applyUSAAPL(snap, data)
		if snap.AAPL.Symbol != "AAPL" {
			t.Fatalf("expected Symbol=AAPL when value is 0, got %q", snap.AAPL.Symbol)
		}
		if snap.AAPL.Value != 0 {
			t.Fatalf("expected Value=0, got %f", snap.AAPL.Value)
		}
	})
}

func TestApplyUSMSFT(t *testing.T) {
	t.Run("populates_field", func(t *testing.T) {
		a, snap := newApplyFixture()
		data := []byte(`{"msft":{"symbol":"MSFT","value":420.70,"change_pct":-0.10,"timestamp":1700000000}}`)
		a.applyUSMSFT(snap, data)
		if snap.MSFT.Symbol != "MSFT" {
			t.Fatalf("expected Symbol=MSFT, got %q", snap.MSFT.Symbol)
		}
		if snap.MSFT.Value != 420.70 {
			t.Fatalf("expected Value=420.70, got %f", snap.MSFT.Value)
		}
		if snap.MSFT.ChangePct != -0.10 {
			t.Fatalf("expected ChangePct=-0.10, got %f", snap.MSFT.ChangePct)
		}
	})

	t.Run("empty_data_no_crash", func(t *testing.T) {
		a, snap := newApplyFixture()
		a.applyUSMSFT(snap, nil)
		a.applyUSMSFT(snap, []byte{})
		a.applyUSMSFT(snap, []byte(`{}`))
		if snap.MSFT.Symbol != "" {
			t.Fatalf("expected empty Symbol for empty data, got %q", snap.MSFT.Symbol)
		}
	})

	t.Run("symbol_only_no_value", func(t *testing.T) {
		a, snap := newApplyFixture()
		data := []byte(`{"msft":{"symbol":"MSFT","value":0,"change_pct":0}}`)
		a.applyUSMSFT(snap, data)
		if snap.MSFT.Symbol != "MSFT" {
			t.Fatalf("expected Symbol=MSFT when value is 0, got %q", snap.MSFT.Symbol)
		}
		if snap.MSFT.Value != 0 {
			t.Fatalf("expected Value=0, got %f", snap.MSFT.Value)
		}
	})
}

func TestApplyTSMADR(t *testing.T) {
	t.Run("populates_field", func(t *testing.T) {
		a, snap := newApplyFixture()
		data := []byte(`{"tsm_adr":{"symbol":"TSM","value":145.20,"change_pct":1.50,"timestamp":1700000000}}`)
		a.applyTSMADR(snap, data)
		if snap.TSMADR.Symbol != "TSM" {
			t.Fatalf("expected Symbol=TSM, got %q", snap.TSMADR.Symbol)
		}
		if snap.TSMADR.Value != 145.20 {
			t.Fatalf("expected Value=145.20, got %f", snap.TSMADR.Value)
		}
		if snap.TSMADR.ChangePct != 1.50 {
			t.Fatalf("expected ChangePct=1.50, got %f", snap.TSMADR.ChangePct)
		}
	})

	t.Run("empty_data_no_crash", func(t *testing.T) {
		a, snap := newApplyFixture()
		a.applyTSMADR(snap, nil)
		a.applyTSMADR(snap, []byte{})
		a.applyTSMADR(snap, []byte(`{}`))
		if snap.TSMADR.Symbol != "" {
			t.Fatalf("expected empty Symbol for empty data, got %q", snap.TSMADR.Symbol)
		}
	})

	t.Run("symbol_only_no_value", func(t *testing.T) {
		a, snap := newApplyFixture()
		data := []byte(`{"tsm_adr":{"symbol":"TSM","value":0,"change_pct":0}}`)
		a.applyTSMADR(snap, data)
		if snap.TSMADR.Symbol != "TSM" {
			t.Fatalf("expected Symbol=TSM when value is 0, got %q", snap.TSMADR.Symbol)
		}
		if snap.TSMADR.Value != 0 {
			t.Fatalf("expected Value=0, got %f", snap.TSMADR.Value)
		}
	})
}

func TestApplyTAIEX(t *testing.T) {
	t.Run("populates_field", func(t *testing.T) {
		a, snap := newApplyFixture()
		data := []byte(`{"taiex":{"symbol":"^TWII","value":23100.50,"change_pct":0.35,"timestamp":1700000000}}`)
		a.applyTAIEX(snap, data)
		if snap.TAIEX.Symbol != "^TWII" {
			t.Fatalf("expected Symbol=^TWII, got %q", snap.TAIEX.Symbol)
		}
		if snap.TAIEX.Value != 23100.50 {
			t.Fatalf("expected Value=23100.50, got %f", snap.TAIEX.Value)
		}
		if snap.TAIEX.ChangePct != 0.35 {
			t.Fatalf("expected ChangePct=0.35, got %f", snap.TAIEX.ChangePct)
		}
	})

	t.Run("empty_data_no_crash", func(t *testing.T) {
		a, snap := newApplyFixture()
		a.applyTAIEX(snap, nil)
		a.applyTAIEX(snap, []byte{})
		a.applyTAIEX(snap, []byte(`{}`))
		if snap.TAIEX.Symbol != "" {
			t.Fatalf("expected empty Symbol for empty data, got %q", snap.TAIEX.Symbol)
		}
	})

	t.Run("symbol_only_no_value", func(t *testing.T) {
		a, snap := newApplyFixture()
		data := []byte(`{"taiex":{"symbol":"^TWII","value":0,"change_pct":0}}`)
		a.applyTAIEX(snap, data)
		if snap.TAIEX.Symbol != "^TWII" {
			t.Fatalf("expected Symbol=^TWII when value is 0, got %q", snap.TAIEX.Symbol)
		}
		if snap.TAIEX.Value != 0 {
			t.Fatalf("expected Value=0, got %f", snap.TAIEX.Value)
		}
	})
}

func TestMacroDataGatewayAdapter_ChannelErrors_AllSuccess(t *testing.T) {
	fetcher := func(ctx context.Context, channelID string) ([]byte, FetchMeta, error) {
		snap := marketdata.MacroDataSnapshot{US10Y: marketdata.MacroDataPoint{Symbol: "^TNX"}}
		b, _ := json.Marshal(snap)
		return b, FetchMeta{}, nil
	}
	gw := NewMacroDataGatewayAdapter(fetcher).(*macroDataGatewayAdapter)
	_, err := gw.FetchSnapshot(context.Background())
	if err != nil {
		t.Fatalf("FetchSnapshot failed: %v", err)
	}
	errs := gw.ChannelErrors()
	if len(errs) != 0 {
		t.Errorf("expected empty errors when all succeed, got: %v", errs)
	}
}

func TestMacroDataGatewayAdapter_ChannelErrors_MixedFailure(t *testing.T) {
	failing := map[string]string{
		"us_spx": "yahoo finance timeout",
		"us_ndx": "rate limited",
	}
	fetcher := func(ctx context.Context, channelID string) ([]byte, FetchMeta, error) {
		if msg, ok := failing[channelID]; ok {
			return nil, FetchMeta{}, errors.New(msg)
		}
		snap := marketdata.MacroDataSnapshot{US10Y: marketdata.MacroDataPoint{Symbol: "^TNX"}}
		b, _ := json.Marshal(snap)
		return b, FetchMeta{}, nil
	}
	gw := NewMacroDataGatewayAdapter(fetcher).(*macroDataGatewayAdapter)
	snap, err := gw.FetchSnapshot(context.Background())
	if err != nil {
		t.Fatalf("FetchSnapshot should not fail when at least one channel succeeds, got: %v", err)
	}
	errs := gw.ChannelErrors()
	if len(errs) == 0 {
		t.Fatal("expected non-empty errors when some channels fail")
	}
	if len(errs) != len(failing) {
		t.Errorf("expected %d failed channels, got %d: %v", len(failing), len(errs), errs)
	}
	for ch, msg := range failing {
		if errs[ch] != msg {
			t.Errorf("expected error for %s to be %q, got %q", ch, msg, errs[ch])
		}
	}
	if snap.RecordedAt == 0 {
		t.Error("expected RecordedAt to be set even with partial failure")
	}
}

func TestMacroDataGatewayAdapter_ChannelErrors_ReturnsCopy(t *testing.T) {
	fetcher := func(ctx context.Context, channelID string) ([]byte, FetchMeta, error) {
		return nil, FetchMeta{}, errors.New("fetch failed")
	}
	gw := NewMacroDataGatewayAdapter(fetcher).(*macroDataGatewayAdapter)
	_, _ = gw.FetchSnapshot(context.Background())

	errs1 := gw.ChannelErrors()
	if len(errs1) == 0 {
		t.Fatal("expected non-empty errors")
	}
	for k := range errs1 {
		delete(errs1, k)
	}
	errs2 := gw.ChannelErrors()
	if len(errs2) == 0 {
		t.Error("expected ChannelErrors to return a copy; original map should not be affected by external mutation")
	}
}

func TestApplyUSYahoo(t *testing.T) {
	a, snap := newApplyFixture()
	data := []byte(`{
		"us10y":{"symbol":"^TNX","value":4.5,"change_pct":1.2},
		"dxy":{"symbol":"DXY","value":103.0},
		"vix":{"symbol":"VIX","value":18.5},
		"oil":{"symbol":"WTI","value":75.0},
		"gold":{"symbol":"GOLD","value":2000.0},
		"jpy":{"symbol":"JPY","value":145.0},
		"usd_twd":{"symbol":"USDTWD","value":31.5},
		"bdi":{"symbol":"BDI","value":1500.0},
		"silver":{"symbol":"SILVER","value":24.0},
		"copper":{"symbol":"COPPER","value":4.0},
		"recorded_at":1700000000
	}`)
	a.applyUSYahoo(snap, data)

	if snap.US10Y.Symbol != "^TNX" {
		t.Errorf("US10Y.Symbol = %q, want ^TNX", snap.US10Y.Symbol)
	}
	if snap.DXY.Symbol != "DXY" {
		t.Errorf("DXY.Symbol = %q, want DXY", snap.DXY.Symbol)
	}
	if snap.VIX.Symbol != "VIX" {
		t.Errorf("VIX.Symbol = %q, want VIX", snap.VIX.Symbol)
	}
	if snap.Oil.Symbol != "WTI" {
		t.Errorf("Oil.Symbol = %q, want WTI", snap.Oil.Symbol)
	}
	if snap.Gold.Symbol != "GOLD" {
		t.Errorf("Gold.Symbol = %q, want GOLD", snap.Gold.Symbol)
	}
	if snap.JPY.Symbol != "JPY" {
		t.Errorf("JPY.Symbol = %q, want JPY", snap.JPY.Symbol)
	}
	if snap.USD_TWD.Symbol != "USDTWD" {
		t.Errorf("USD_TWD.Symbol = %q, want USDTWD", snap.USD_TWD.Symbol)
	}
	if snap.Bdi.Symbol != "BDI" {
		t.Errorf("Bdi.Symbol = %q, want BDI", snap.Bdi.Symbol)
	}
	if snap.Silver.Symbol != "SILVER" {
		t.Errorf("Silver.Symbol = %q, want SILVER", snap.Silver.Symbol)
	}
	if snap.Copper.Symbol != "COPPER" {
		t.Errorf("Copper.Symbol = %q, want COPPER", snap.Copper.Symbol)
	}
	if snap.RecordedAt != 1700000000 {
		t.Errorf("RecordedAt = %d, want 1700000000", snap.RecordedAt)
	}
}

func TestApplyUSYahoo_InvalidJSON(t *testing.T) {
	a, snap := newApplyFixture()
	a.applyUSYahoo(snap, []byte(`not json`))
	if snap.RecordedAt != 0 {
		t.Error("expected no changes on invalid JSON")
	}
}

func TestApplyFrankfurterFX(t *testing.T) {
	t.Run("fills empty JPY", func(t *testing.T) {
		a, snap := newApplyFixture()
		data := []byte(`{"symbol":"JPY","value":145.0,"change_pct":0.5}`)
		a.applyFrankfurterFX(snap, data)
		if snap.JPY.Symbol != "JPY" {
			t.Errorf("JPY.Symbol = %q, want JPY", snap.JPY.Symbol)
		}
		if snap.JPY.Value != 145.0 {
			t.Errorf("JPY.Value = %v, want 145.0", snap.JPY.Value)
		}
	})

	t.Run("skips when JPY already present", func(t *testing.T) {
		a, snap := newApplyFixture()
		snap.JPY = marketdata.MacroDataPoint{Symbol: "JPY", Value: 150.0}
		data := []byte(`{"symbol":"JPY","value":145.0,"change_pct":0.5}`)
		a.applyFrankfurterFX(snap, data)
		if snap.JPY.Value != 150.0 {
			t.Errorf("JPY.Value = %v, want 150.0 (unchanged)", snap.JPY.Value)
		}
	})

	t.Run("invalid json no panic", func(t *testing.T) {
		a, snap := newApplyFixture()
		a.applyFrankfurterFX(snap, []byte(`not json`))
		if snap.JPY.Symbol != "" {
			t.Error("expected empty JPY on invalid JSON")
		}
	})
}

func TestApplyExchangeRate(t *testing.T) {
	t.Run("fills empty USD_TWD", func(t *testing.T) {
		a, snap := newApplyFixture()
		data := []byte(`{"usd_twd":{"symbol":"USDTWD","value":31.5}}`)
		a.applyExchangeRate(snap, data)
		if snap.USD_TWD.Symbol != "USDTWD" {
			t.Errorf("USD_TWD.Symbol = %q, want USDTWD", snap.USD_TWD.Symbol)
		}
	})

	t.Run("skips when USD_TWD already present", func(t *testing.T) {
		a, snap := newApplyFixture()
		snap.USD_TWD = marketdata.MacroDataPoint{Symbol: "USDTWD", Value: 32.0}
		data := []byte(`{"usd_twd":{"symbol":"USDTWD","value":31.5}}`)
		a.applyExchangeRate(snap, data)
		if snap.USD_TWD.Value != 32.0 {
			t.Errorf("USD_TWD.Value = %v, want 32.0 (unchanged)", snap.USD_TWD.Value)
		}
	})
}

func TestApplySOXIndex(t *testing.T) {
	a, snap := newApplyFixture()
	data := []byte(`{"sox_index":{"symbol":"SOX","value":4200.0,"change_pct":1.5}}`)
	a.applySOXIndex(snap, data)
	if snap.SOXIndex.Symbol != "SOX" {
		t.Errorf("SOXIndex.Symbol = %q, want SOX", snap.SOXIndex.Symbol)
	}
	if snap.SOXIndex.Value != 4200.0 {
		t.Errorf("SOXIndex.Value = %v, want 4200.0", snap.SOXIndex.Value)
	}
}

func TestApplyCapitalFlow(t *testing.T) {
	a, snap := newApplyFixture()
	data := []byte(`{
		"foreign_investor_net":{"symbol":"FI","value":100.0},
		"domestic_fund_net":{"symbol":"DF","value":50.0},
		"dealer_net":{"symbol":"DE","value":-30.0}
	}`)
	a.applyCapitalFlow(snap, data)
	if snap.ForeignInvestorNet.Symbol != "FI" {
		t.Errorf("ForeignInvestorNet.Symbol = %q, want FI", snap.ForeignInvestorNet.Symbol)
	}
	if snap.DomesticFundNet.Symbol != "DF" {
		t.Errorf("DomesticFundNet.Symbol = %q, want DF", snap.DomesticFundNet.Symbol)
	}
	if snap.DealerNet.Symbol != "DE" {
		t.Errorf("DealerNet.Symbol = %q, want DE", snap.DealerNet.Symbol)
	}
}

func TestApplyMargin(t *testing.T) {
	// Test with maintenance ratio present.
	t.Run("with_maintenance_ratio", func(t *testing.T) {
		a, snap := newApplyFixture()
		data := []byte(`{
			"retail_margin_balance":{"symbol":"MARGIN","value":2000.0},
			"retail_short_balance":{"symbol":"SHORT","value":500.0},
			"margin_maintenance_ratio":{"symbol":"TSE_MARGIN_MAINT","value":165.5}
		}`)
		a.applyMargin(snap, data)
		if snap.RetailMarginBalance.Symbol != "MARGIN" {
			t.Errorf("RetailMarginBalance.Symbol = %q, want MARGIN", snap.RetailMarginBalance.Symbol)
		}
		if snap.RetailShortBalance.Symbol != "SHORT" {
			t.Errorf("RetailShortBalance.Symbol = %q, want SHORT", snap.RetailShortBalance.Symbol)
		}
		if snap.MarginMaintenanceRatio.Symbol != "TSE_MARGIN_MAINT" {
			t.Errorf("MarginMaintenanceRatio.Symbol = %q, want TSE_MARGIN_MAINT", snap.MarginMaintenanceRatio.Symbol)
		}
		if snap.MarginMaintenanceRatio.Value != 165.5 {
			t.Errorf("MarginMaintenanceRatio.Value = %v, want 165.5", snap.MarginMaintenanceRatio.Value)
		}
	})

	// Test without maintenance ratio (backward compat — no regression).
	t.Run("without_maintenance_ratio", func(t *testing.T) {
		a, snap := newApplyFixture()
		data := []byte(`{
			"retail_margin_balance":{"symbol":"MARGIN","value":2000.0},
			"retail_short_balance":{"symbol":"SHORT","value":500.0}
		}`)
		a.applyMargin(snap, data)
		if snap.MarginMaintenanceRatio.Symbol != "" {
			t.Errorf("MarginMaintenanceRatio.Symbol = %q, want empty (no data)", snap.MarginMaintenanceRatio.Symbol)
		}
	})

	// Test with maintenance ratio but empty symbol (should not be mapped).
	t.Run("empty_symbol_maintenance_ratio", func(t *testing.T) {
		a, snap := newApplyFixture()
		data := []byte(`{
			"retail_margin_balance":{"symbol":"MARGIN","value":2000.0},
			"margin_maintenance_ratio":{"symbol":"","value":0}
		}`)
		a.applyMargin(snap, data)
		if snap.MarginMaintenanceRatio.Symbol != "" {
			t.Errorf("MarginMaintenanceRatio.Symbol = %q, want empty (empty symbol filtered)", snap.MarginMaintenanceRatio.Symbol)
		}
	})
}

func TestApplyExport(t *testing.T) {
	a, snap := newApplyFixture()
	data := []byte(`{"export_electronics":{"symbol":"EXPORT","value":3000.0}}`)
	a.applyExport(snap, data)
	if snap.ExportElectronics.Symbol != "EXPORT" {
		t.Errorf("ExportElectronics.Symbol = %q, want EXPORT", snap.ExportElectronics.Symbol)
	}
}

func TestApplyTSMCRevenue(t *testing.T) {
	a, snap := newApplyFixture()
	data := []byte(`{"symbol":"TSMC","value":200.0,"change_pct":5.0}`)
	a.applyTSMCRevenue(snap, data)
	if snap.TSMCRevenue.Symbol != "TSMC" {
		t.Errorf("TSMCRevenue.Symbol = %q, want TSMC", snap.TSMCRevenue.Symbol)
	}
	if snap.TSMCRevenue.Value != 200.0 {
		t.Errorf("TSMCRevenue.Value = %v, want 200.0", snap.TSMCRevenue.Value)
	}
}

func TestApplyDRAMSpotPrice(t *testing.T) {
	a, snap := newApplyFixture()
	data := []byte(`{"dram_spot_price":{"symbol":"DRAM","value":4.5,"change_pct":2.0}}`)
	a.applyDRAMSpotPrice(snap, data)
	if snap.DRAMSpotPrice.Symbol != "DRAM" {
		t.Errorf("DRAMSpotPrice.Symbol = %q, want DRAM", snap.DRAMSpotPrice.Symbol)
	}
}

func TestApplyTWSESectorIndex(t *testing.T) {
	a, snap := newApplyFixture()
	data := []byte(`{"symbol":"TWSEMI","value":500.0,"change_pct":1.0}`)
	a.applyTWSESectorIndex(snap, data)
	if snap.TaiwanSemiIndex.Symbol != "TWSEMI" {
		t.Errorf("TaiwanSemiIndex.Symbol = %q, want TWSEMI", snap.TaiwanSemiIndex.Symbol)
	}
}

func TestApplyBDI(t *testing.T) {
	a, snap := newApplyFixture()
	data := []byte(`{"bdi":{"symbol":"BDI","value":1800.0,"change_pct":3.0}}`)
	a.applyBDI(snap, data)
	if snap.Bdi.Symbol != "BDI" {
		t.Errorf("Bdi.Symbol = %q, want BDI", snap.Bdi.Symbol)
	}
	if snap.Bdi.Value != 1800.0 {
		t.Errorf("Bdi.Value = %v, want 1800.0", snap.Bdi.Value)
	}
}

func TestApplySectorData(t *testing.T) {
	a, snap := newApplyFixture()
	a.applySectorData(snap, []byte(`{"anything":true}`))
	if snap.SPXIndex.Symbol != "" {
		t.Error("expected no field changes from sector data")
	}
}

// TestMacroDataGatewayAdapter_DetectsStaleData locks the bug where L2
// silently swallows FetchResult.Stale=true (CB-open path returns stale
// bytes with nil error at gateway.go:107). Expected to FAIL until L2
// learns to surface staleness via ChannelErrors() with "stale:" prefix.
func TestMacroDataGatewayAdapter_DetectsStaleData(t *testing.T) {
	staleFetcher := func(ctx context.Context, channelID string) ([]byte, FetchMeta, error) {
		snap := marketdata.MacroDataSnapshot{
			SPXIndex: marketdata.MacroDataPoint{Symbol: "SPX", Value: 5432.10, ChangePct: 0.5, Timestamp: 1700000000},
			NDXIndex: marketdata.MacroDataPoint{Symbol: "NDX", Value: 19876.50, ChangePct: -0.3, Timestamp: 1700000000},
			DJIIndex: marketdata.MacroDataPoint{Symbol: "DJI", Value: 38900.00, ChangePct: 0.2, Timestamp: 1700000000},
		}
		b, _ := json.Marshal(snap)
		_ = channelID
		return b, FetchMeta{Stale: true, LastError: "circuit breaker open for us_spx"}, nil
	}

	gw := NewMacroDataGatewayAdapter(staleFetcher).(*macroDataGatewayAdapter)
	snap, err := gw.FetchSnapshot(context.Background())
	if err != nil {
		t.Fatalf("FetchSnapshot returned error (should not): %v", err)
	}
	if snap.SPXIndex.Symbol != "SPX" || snap.SPXIndex.Value != 5432.10 {
		t.Fatalf("snapshot should contain stale data, got SPX=%+v", snap.SPXIndex)
	}

	errs := gw.ChannelErrors()
	staleCount := 0
	for _, msg := range errs {
		if len(msg) >= 6 && msg[:6] == "stale:" {
			staleCount++
		}
	}
	if staleCount == 0 {
		t.Fatalf("BUG: stale data silently passed through, ChannelErrors() "+
			"reported no stale channels. errs=%v", errs)
	}
	if staleCount < 3 {
		t.Errorf("expected at least 3 stale channels (SPX/NDX/DJI), got %d (errs=%v)",
			staleCount, errs)
	}
}

// TestMacroDataGatewayAdapter_FreshDataUnaffected guards against false
// positives: healthy (non-stale) fetches must not be reported as stale.
func TestMacroDataGatewayAdapter_FreshDataUnaffected(t *testing.T) {
	freshFetcher := func(ctx context.Context, channelID string) ([]byte, FetchMeta, error) {
		snap := marketdata.MacroDataSnapshot{
			SPXIndex: marketdata.MacroDataPoint{Symbol: "SPX", Value: 5432.10},
		}
		b, _ := json.Marshal(snap)
		return b, FetchMeta{}, nil
	}
	gw := NewMacroDataGatewayAdapter(freshFetcher).(*macroDataGatewayAdapter)
	if _, err := gw.FetchSnapshot(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	errs := gw.ChannelErrors()
	for ch, msg := range errs {
		if len(msg) >= 6 && msg[:6] == "stale:" {
			t.Errorf("fresh fetch should NOT be reported stale; channel=%s msg=%s", ch, msg)
		}
	}
}

func TestNewDayTradingFetcher(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		fetcher := func(ctx context.Context, channelID string) ([]byte, FetchMeta, error) {
			stats := marketdata.DayTradingStats{Date: "2026-01-01", DayTradingVolume: 1000}
			b, _ := json.Marshal(stats)
			return b, FetchMeta{}, nil
		}
		f := NewDayTradingFetcher(fetcher)
		stats, err := f(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if stats.Date != "2026-01-01" {
			t.Errorf("Date = %q, want 2026-01-01", stats.Date)
		}
	})

	t.Run("fetch error", func(t *testing.T) {
		fetcher := func(ctx context.Context, channelID string) ([]byte, FetchMeta, error) {
			return nil, FetchMeta{}, errors.New("down")
		}
		f := NewDayTradingFetcher(fetcher)
		_, err := f(context.Background())
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("unmarshal error", func(t *testing.T) {
		fetcher := func(ctx context.Context, channelID string) ([]byte, FetchMeta, error) {
			return []byte(`not json`), FetchMeta{}, nil
		}
		f := NewDayTradingFetcher(fetcher)
		_, err := f(context.Background())
		if err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestNewTaifexFetcher(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		fetcher := func(ctx context.Context, channelID string) ([]byte, FetchMeta, error) {
			result := struct {
				PCR             *marketdata.PCRStats        `json:"pcr"`
				RetailFuturesOI *marketdata.RetailFuturesOI `json:"retail_futures_oi"`
			}{
				PCR:             &marketdata.PCRStats{Date: "2026-01-01", PutVolume: 100},
				RetailFuturesOI: &marketdata.RetailFuturesOI{Date: "2026-01-01", RetailLongOI: 50},
			}
			b, _ := json.Marshal(result)
			return b, FetchMeta{}, nil
		}
		f := NewTaifexFetcher(fetcher)
		pcr, oi, err := f(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if pcr.PutVolume != 100 {
			t.Errorf("PutVolume = %d, want 100", pcr.PutVolume)
		}
		if oi.RetailLongOI != 50 {
			t.Errorf("RetailLongOI = %d, want 50", oi.RetailLongOI)
		}
	})

	t.Run("error", func(t *testing.T) {
		fetcher := func(ctx context.Context, channelID string) ([]byte, FetchMeta, error) {
			return nil, FetchMeta{}, errors.New("down")
		}
		f := NewTaifexFetcher(fetcher)
		_, _, err := f(context.Background())
		if err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestNewOddLotFetcher(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		fetcher := func(ctx context.Context, channelID string) ([]byte, FetchMeta, error) {
			stats := marketdata.OddLotStats{Date: "2026-01-01", BuyVolume: 100}
			b, _ := json.Marshal(stats)
			return b, FetchMeta{}, nil
		}
		f := NewOddLotFetcher(fetcher)
		stats, err := f(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if stats.BuyVolume != 100 {
			t.Errorf("BuyVolume = %d, want 100", stats.BuyVolume)
		}
	})

	t.Run("error", func(t *testing.T) {
		fetcher := func(ctx context.Context, channelID string) ([]byte, FetchMeta, error) {
			return nil, FetchMeta{}, errors.New("down")
		}
		f := NewOddLotFetcher(fetcher)
		_, err := f(context.Background())
		if err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestNewETFFetcher(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		fetcher := func(ctx context.Context, channelID string) ([]byte, FetchMeta, error) {
			stats := marketdata.ETFStats{Date: "2026-01-01", NetSubscription: 1000}
			b, _ := json.Marshal(stats)
			return b, FetchMeta{}, nil
		}
		f := NewETFFetcher(fetcher)
		stats, err := f(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if stats.NetSubscription != 1000 {
			t.Errorf("NetSubscription = %d, want 1000", stats.NetSubscription)
		}
	})

	t.Run("error", func(t *testing.T) {
		fetcher := func(ctx context.Context, channelID string) ([]byte, FetchMeta, error) {
			return nil, FetchMeta{}, errors.New("down")
		}
		f := NewETFFetcher(fetcher)
		_, err := f(context.Background())
		if err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestNewGeopoliticalRiskFetcher(t *testing.T) {
	t.Run("prefers taiwan", func(t *testing.T) {
		global := &mockGeoProvider{score: geopolitical.GeopoliticalRiskScore{Intensity: 50}}
		taiwan := &mockGeoProvider{score: geopolitical.GeopoliticalRiskScore{Intensity: 70}}
		f := newGeopoliticalRiskFetcher(global, taiwan)
		if got := f(context.Background()); got != 0.7 {
			t.Errorf("intensity = %v, want 0.7", got)
		}
	})

	t.Run("falls back to global", func(t *testing.T) {
		global := &mockGeoProvider{score: geopolitical.GeopoliticalRiskScore{Intensity: 50}}
		f := newGeopoliticalRiskFetcher(global, nil)
		if got := f(context.Background()); got != 0.5 {
			t.Errorf("intensity = %v, want 0.5", got)
		}
	})

	t.Run("returns zero when both fail", func(t *testing.T) {
		global := &mockGeoProvider{err: errors.New("fail")}
		taiwan := &mockGeoProvider{err: errors.New("fail")}
		f := newGeopoliticalRiskFetcher(global, taiwan)
		if got := f(context.Background()); got != 0 {
			t.Errorf("intensity = %v, want 0", got)
		}
	})

	t.Run("returns zero when intensity zero", func(t *testing.T) {
		global := &mockGeoProvider{score: geopolitical.GeopoliticalRiskScore{Intensity: 0}}
		f := newGeopoliticalRiskFetcher(global, nil)
		if got := f(context.Background()); got != 0 {
			t.Errorf("intensity = %v, want 0", got)
		}
	})
}

func TestApplyDayTradeRatioFromStats(t *testing.T) {
	// Normal DayTradingStats → DayTradeRatio populated.
	t.Run("normal_stats", func(t *testing.T) {
		a, snap := newApplyFixture()
		data := []byte(`{"date":"20260728","volume_ratio":35.5,"buy_value_ratio":38.2}`)
		a.applyDayTradeRatioFromStats(snap, data)
		if snap.DayTradeRatio.Symbol != "TSE_DAYTRADE" {
			t.Errorf("DayTradeRatio.Symbol = %q, want TSE_DAYTRADE", snap.DayTradeRatio.Symbol)
		}
		if snap.DayTradeRatio.Value != 35.5 {
			t.Errorf("DayTradeRatio.Value = %v, want 35.5", snap.DayTradeRatio.Value)
		}
		// Verify timestamp was parsed from "20260728".
		if snap.DayTradeRatio.Timestamp == 0 {
			t.Error("DayTradeRatio.Timestamp = 0, want non-zero")
		}
	})

	// Empty Date → no fill.
	t.Run("empty_date", func(t *testing.T) {
		a, snap := newApplyFixture()
		data := []byte(`{"date":"","volume_ratio":35.5}`)
		a.applyDayTradeRatioFromStats(snap, data)
		if snap.DayTradeRatio.Symbol != "" {
			t.Errorf("DayTradeRatio.Symbol = %q, want empty (date is empty)", snap.DayTradeRatio.Symbol)
		}
	})

	// Bad JSON → no panic.
	t.Run("bad_json", func(t *testing.T) {
		a, snap := newApplyFixture()
		a.applyDayTradeRatioFromStats(snap, []byte(`not json`))
		if snap.DayTradeRatio.Symbol != "" {
			t.Errorf("DayTradeRatio.Symbol = %q, want empty (bad JSON)", snap.DayTradeRatio.Symbol)
		}
	})

	// Invalid date format → no fill.
	t.Run("invalid_date", func(t *testing.T) {
		a, snap := newApplyFixture()
		data := []byte(`{"date":"2026-07-28","volume_ratio":35.5}`)
		a.applyDayTradeRatioFromStats(snap, data)
		if snap.DayTradeRatio.Symbol != "" {
			t.Errorf("DayTradeRatio.Symbol = %q, want empty (invalid date format)", snap.DayTradeRatio.Symbol)
		}
	})
}

type mockGeoProvider struct {
	score geopolitical.GeopoliticalRiskScore
	err   error
}

func (m *mockGeoProvider) Name() string { return "mock" }
func (m *mockGeoProvider) FetchScore(ctx context.Context) (geopolitical.GeopoliticalRiskScore, error) {
	return m.score, m.err
}
