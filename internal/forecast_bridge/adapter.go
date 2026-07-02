package forecast_bridge

import (
	"fmt"

	"github.com/kaecer68/atlas-go/internal/forecast"
)

// Adapter converts forecast.ForecastResult values into forecast.TradeSignal
// values using conviction thresholds defined in the Phase 3.5 spec.
type Adapter struct{}

// NewAdapter returns a new Adapter ready for use.
func NewAdapter() *Adapter {
	return &Adapter{}
}

// ToTradeSignal maps a ForecastResult to a TradeSignal.
//
// Rules:
//   - Conviction >= 70 and Direction == bullish => buy, 1.2x
//   - Conviction <= 30 and Direction == bearish => sell, 0.8x
//   - Otherwise => hold, 1.0x
func (a *Adapter) ToTradeSignal(result forecast.ForecastResult) (forecast.TradeSignal, error) {
	var action string
	var multiplier float64

	switch {
	case result.Conviction >= 70 && result.Direction == forecast.DirectionBullish:
		action = "buy"
		multiplier = 1.2
	case result.Conviction <= 30 && result.Direction == forecast.DirectionBearish:
		action = "sell"
		multiplier = 0.8
	default:
		action = "hold"
		multiplier = 1.0
	}

	rationale := fmt.Sprintf("%s conviction %d → %s with %.1fx multiplier",
		result.Direction, result.Conviction, action, multiplier)

	return forecast.TradeSignal{
		Symbol:           result.Symbol,
		Action:           action,
		WeightMultiplier: multiplier,
		Rationale:        rationale,
		SourceForecastID: fmt.Sprintf("%s_%d", result.Symbol, result.GeneratedAt.Unix()),
		GeneratedAt:      result.GeneratedAt,
	}, nil
}
