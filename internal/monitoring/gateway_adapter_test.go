package monitoring

import (
	"testing"

	"github.com/kaecer68/atlas-go/internal/marketdata"
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
