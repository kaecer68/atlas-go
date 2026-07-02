package forecast

import (
	"fmt"
	"time"
)

// ForecastEngine produces per-symbol directional forecasts.
type ForecastEngine struct {
	nowFn func() time.Time
}

// NewForecastEngine creates a forecast engine.
func NewForecastEngine() *ForecastEngine {
	return &ForecastEngine{
		nowFn: time.Now,
	}
}

// Predict returns a directional forecast for symbol over the given horizon.
func (e *ForecastEngine) Predict(symbol string, horizon time.Duration) (ForecastResult, error) {
	if symbol == "" {
		return ForecastResult{}, fmt.Errorf("forecast: symbol is required")
	}
	if horizon <= 0 {
		return ForecastResult{}, fmt.Errorf("forecast: horizon must be positive")
	}

	days := int(horizon.Hours() / 24)
	if days <= 0 {
		days = 1
	}

	return ForecastResult{
		Symbol:      symbol,
		Conviction:  50,
		Direction:   DirectionHold,
		Horizon:     fmt.Sprintf("%dd", days),
		GeneratedAt: e.nowFn(),
	}, nil
}
