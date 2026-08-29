package industry

import (
	"math"
	"testing"

	"github.com/kaecer68/atlas-go/internal/marketdata"
)

func TestDynamicEnvModulator_UpdateRollingBaseline(t *testing.T) {
	dem := NewDynamicEnvModulator(
		marketdata.MacroDataSnapshot{Oil: marketdata.MacroDataPoint{Value: 75.0}},
		marketdata.MacroDataSnapshot{Oil: marketdata.MacroDataPoint{Value: 80.0}},
	)

	// Record 30 days of history with known median
	for i := range 30 {
		dem.RecordSnapshot(marketdata.MacroDataSnapshot{
			Oil: marketdata.MacroDataPoint{Value: float64(70 + i)}, // 70-99
			DXY: marketdata.MacroDataPoint{Value: float64(100 + i)},
		})
	}

	dem.UpdateRollingBaseline()

	// Median of 70-99 = 84.5
	if math.Abs(dem.baseline.Oil.Value-84.5) > 0.01 {
		t.Fatalf("expected oil baseline 84.5, got %f", dem.baseline.Oil.Value)
	}
}

func TestDynamicEnvModulator_UpdateRollingBaseline_Window(t *testing.T) {
	dem := NewDynamicEnvModulator(
		marketdata.MacroDataSnapshot{Oil: marketdata.MacroDataPoint{Value: 75.0}},
		marketdata.MacroDataSnapshot{Oil: marketdata.MacroDataPoint{Value: 80.0}},
	)
	dem.windowDays = 10

	// Record 20 days
	for i := range 20 {
		dem.RecordSnapshot(marketdata.MacroDataSnapshot{
			Oil: marketdata.MacroDataPoint{Value: float64(50 + i*5)}, // 50, 55, 60, ..., 145
		})
	}

	dem.UpdateRollingBaseline()

	// Last 10 values: 100, 105, 110, 115, 120, 125, 130, 135, 140, 145
	// Median = (120+125)/2 = 122.5
	if math.Abs(dem.baseline.Oil.Value-122.5) > 0.01 {
		t.Fatalf("expected oil baseline 122.5 with 10-day window, got %f", dem.baseline.Oil.Value)
	}
}

func TestDynamicEnvModulator_RecordSnapshot_Cap(t *testing.T) {
	dem := NewDynamicEnvModulator(
		marketdata.MacroDataSnapshot{Oil: marketdata.MacroDataPoint{Value: 75.0}},
		marketdata.MacroDataSnapshot{Oil: marketdata.MacroDataPoint{Value: 80.0}},
	)
	dem.windowDays = 5

	// Record 15 days (max should be 10 = 2*window)
	for i := range 15 {
		dem.RecordSnapshot(marketdata.MacroDataSnapshot{
			Oil: marketdata.MacroDataPoint{Value: float64(i)},
		})
	}

	if len(dem.history) > 10 {
		t.Fatalf("expected history capped at 10, got %d", len(dem.history))
	}
}

func TestDynamicEnvModulator_UpdateRollingBaseline_BDI(t *testing.T) {
	dem := NewDynamicEnvModulator(
		marketdata.MacroDataSnapshot{
			Oil: marketdata.MacroDataPoint{Value: 75.0},
			Bdi: marketdata.MacroDataPoint{Value: 1500},
		},
		marketdata.MacroDataSnapshot{
			Oil: marketdata.MacroDataPoint{Value: 80.0},
			Bdi: marketdata.MacroDataPoint{Value: 1600},
		},
	)

	for i := range 5 {
		dem.RecordSnapshot(marketdata.MacroDataSnapshot{
			Oil: marketdata.MacroDataPoint{Value: float64(70 + i)},
			Bdi: marketdata.MacroDataPoint{Value: float64(1500 + i*50)},
		})
	}

	dem.UpdateRollingBaseline()

	if math.Abs(dem.baseline.Bdi.Value-1600) > 0.01 {
		t.Fatalf("expected BDI baseline 1600 (median of 1500-1700), got %f", dem.baseline.Bdi.Value)
	}
}

func TestDynamicEnvModulator_BDIDeviation_Zero_Baseline(t *testing.T) {
	dem := NewDynamicEnvModulator(
		marketdata.MacroDataSnapshot{},
		marketdata.MacroDataSnapshot{Bdi: marketdata.MacroDataPoint{Value: 1800}},
	)
	dev := dem.BDIDeviation()
	if dev != 0 {
		t.Fatalf("expected zero deviation when baseline BDI is 0, got %f", dev)
	}
}

func TestDynamicEnvModulator_BDIDeviation_High(t *testing.T) {
	baseline := marketdata.MacroDataSnapshot{Bdi: marketdata.MacroDataPoint{Value: 1500}}
	current := marketdata.MacroDataSnapshot{Bdi: marketdata.MacroDataPoint{Value: 1800}}
	dem := NewDynamicEnvModulator(baseline, current)

	dev := dem.BDIDeviation()
	expected := (1800.0 - 1500.0) / 1500.0
	if math.Abs(dev-expected) > 0.001 {
		t.Fatalf("expected BDI deviation %.4f, got %.4f", expected, dev)
	}
}

func TestDynamicEnvModulator_BDIDeviation_Low(t *testing.T) {
	baseline := marketdata.MacroDataSnapshot{Bdi: marketdata.MacroDataPoint{Value: 1500}}
	current := marketdata.MacroDataSnapshot{Bdi: marketdata.MacroDataPoint{Value: 1200}}
	dem := NewDynamicEnvModulator(baseline, current)

	dev := dem.BDIDeviation()
	expected := (1200.0 - 1500.0) / 1500.0
	if math.Abs(dev-expected) > 0.001 {
		t.Fatalf("expected BDI deviation %.4f, got %.4f", expected, dev)
	}
}

func TestDynamicEnvModulator_SeasonalModulation_Shipping_BDI_High(t *testing.T) {
	baseline := marketdata.MacroDataSnapshot{
		Oil: marketdata.MacroDataPoint{Value: 75},
		Bdi: marketdata.MacroDataPoint{Value: 1500},
	}
	current := marketdata.MacroDataSnapshot{
		Oil: marketdata.MacroDataPoint{Value: 80},
		Bdi: marketdata.MacroDataPoint{Value: 1800},
		DXY: marketdata.MacroDataPoint{Value: 100},
	}
	dem := NewDynamicEnvModulator(baseline, current)

	mod := dem.SeasonalModulation("shipping")
	// Oil deviation (80-75)/75 = 6.7%, below OilHighThreshold (0.10) → no oil effect.
	// BDI deviation (1800-1500)/1500 = 0.20, above BDIHighThreshold (0.10) → apply BDIShippingBoost (0.30).
	// Expected: 1.0 × 1.30 = 1.30
	if math.Abs(mod-1.30) > 0.01 {
		t.Fatalf("shipping with +20%% BDI: expected mod ~1.30, got %.4f", mod)
	}
}

func TestDynamicEnvModulator_SeasonalModulation_Shipping_BDI_Low(t *testing.T) {
	baseline := marketdata.MacroDataSnapshot{
		Oil: marketdata.MacroDataPoint{Value: 75},
		Bdi: marketdata.MacroDataPoint{Value: 1500},
	}
	current := marketdata.MacroDataSnapshot{
		Oil: marketdata.MacroDataPoint{Value: 78},
		Bdi: marketdata.MacroDataPoint{Value: 1200},
		DXY: marketdata.MacroDataPoint{Value: 100},
	}
	dem := NewDynamicEnvModulator(baseline, current)

	mod := dem.SeasonalModulation("shipping")
	// BDI deviation (1200-1500)/1500 = -0.20, below BDILowThreshold (0.10) → apply BDICostPenalty (0.04).
	// Expected: 1.0 × (1 - 0.04) = 0.96
	if math.Abs(mod-0.96) > 0.01 {
		t.Fatalf("shipping with -20%% BDI: expected mod ~0.96, got %.4f", mod)
	}
}

func TestDynamicEnvModulator_SeasonalModulation_Industrial_NoBDI(t *testing.T) {
	baseline := marketdata.MacroDataSnapshot{
		Oil: marketdata.MacroDataPoint{Value: 75},
		Bdi: marketdata.MacroDataPoint{Value: 1500},
	}
	current := marketdata.MacroDataSnapshot{
		Oil: marketdata.MacroDataPoint{Value: 120},
		Bdi: marketdata.MacroDataPoint{Value: 1800},
		DXY: marketdata.MacroDataPoint{Value: 100},
	}
	dem := NewDynamicEnvModulator(baseline, current)

	mod := dem.SeasonalModulation("industrial")
	// Oil deviation (120-75)/75 = 0.60, above OilHighThreshold (0.10) → apply OilIndustrialPenalty (0.06).
	// BDI is not used for industrial sector.
	// Expected: 1.0 × (1 - 0.06) = 0.94
	if math.Abs(mod-0.94) > 0.01 {
		t.Fatalf("industrial with +60%% oil: expected mod ~0.94, got %.4f", mod)
	}
}
