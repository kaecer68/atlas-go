package portfolio

import "github.com/kaecer68/atlas-go/internal/domain/shared"

// Frequency selects the annualization convention for Sharpe calculation.
type Frequency = shared.Frequency

const (
	// FrequencyPerOutcome treats each entry as a single return observation
	// with no annualization.
	FrequencyPerOutcome = shared.FrequencyPerOutcome
	// FrequencyPerDay treats each entry as one trading day and annualizes
	// by sqrt(252).
	FrequencyPerDay = shared.FrequencyPerDay
	// FrequencyTWSE treats each entry as one Taiwan stock trading day and
	// annualizes by sqrt(243).
	FrequencyTWSE = shared.FrequencyTWSE
)

// SharpeConfig configures ComputeSharpe.
type SharpeConfig = shared.SharpeConfig

// ComputeSharpe delegates to the canonical implementation in domain/shared.
func ComputeSharpe(returns []float64, cfg SharpeConfig) float64 {
	return shared.ComputeSharpe(returns, cfg)
}
