package forecast

import "time"

// Direction classifies the forecasted price move for a symbol.
type Direction string

const (
	DirectionBullish Direction = "bullish"
	DirectionBearish Direction = "bearish"
	DirectionHold    Direction = "hold"
)

// ForecastResult is the output of a forecast engine for a single symbol.
type ForecastResult struct {
	Symbol      string    `json:"symbol"`
	Conviction  int       `json:"conviction"` // 0-100
	Direction   Direction `json:"direction"`
	Horizon     string    `json:"horizon"` // "7d", "30d"
	Scenarios   []string  `json:"scenarios,omitempty"`
	GeneratedAt time.Time `json:"generated_at"`
}

// TradeSignal bridges a forecast result into a trade direction multiplier.
type TradeSignal struct {
	Symbol           string    `json:"symbol"`
	Action           string    `json:"action"`            // "buy", "sell", "hold"
	WeightMultiplier float64   `json:"weight_multiplier"` // 0.0 - 2.0
	Rationale        string    `json:"rationale"`
	SourceForecastID string    `json:"source_forecast_id"`
	GeneratedAt      time.Time `json:"generated_at"`
}
