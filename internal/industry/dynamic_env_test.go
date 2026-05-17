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
	for i := 0; i < 30; i++ {
		dem.RecordSnapshot(marketdata.MacroDataSnapshot{
			Oil: marketdata.MacroDataPoint{Value: float64(70 + i)}, // 70-99
			DXY: marketdata.MacroDataPoint{Value: float64(100 + i)},
			BDI: marketdata.MacroDataPoint{Value: float64(1400 + i*10)},
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
	for i := 0; i < 20; i++ {
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
	for i := 0; i < 15; i++ {
		dem.RecordSnapshot(marketdata.MacroDataSnapshot{
			Oil: marketdata.MacroDataPoint{Value: float64(i)},
		})
	}

	if len(dem.history) > 10 {
		t.Fatalf("expected history capped at 10, got %d", len(dem.history))
	}
}
