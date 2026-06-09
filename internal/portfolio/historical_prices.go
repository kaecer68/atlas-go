// Package-internal note: prices in HistoricalPrices are RAW and UNADJUSTED for
// corporate actions (cash dividends, stock splits, capital reductions).
//
// Implications for downstream consumers:
//   - MomentumReturn (and any other simple price-difference return) UNDERSTATES
//     true total return across ex-dividend dates by the dividend amount.
//   - Volatility computed from unadjusted daily returns is INFLATED around
//     ex-dividend / split events.
//   - Sharpe and other risk-adjusted metrics that depend on mean/stddev of
//     returns inherit this bias.
//
// The full fix — corporate-action adjustment via
// AdjustForCorporateActions(actions []CorporateAction) error — is tracked
// across two follow-up iterations:
//   - P1-2-α (this branch, wt/p1-2a-domain-provider): the `CorporateAction`
//     canonical type is now landed at `internal/domain/corporate_action.go`
//     and the upstream provider integration is at
//     `internal/marketdata.AggregatedCorporateActionProvider`.
//   - P1-2-β (branch wt/p1-2b-adjust-algorithm): implements the
//     `AdjustForCorporateActions` algorithm consuming the type above.
//
// Until β lands, callers MUST treat these returns as PRICE-ONLY, not
// total-return.
package portfolio

import (
	"bufio"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sort"
	"time"
)

// HistoricalPrices stores closing prices by symbol and date.
type HistoricalPrices struct {
	prices map[string][]pricePoint // symbol -> sorted by date
}

type pricePoint struct {
	Date  time.Time
	Close float64
}

// NewHistoricalPrices creates an empty repository.
func NewHistoricalPrices() *HistoricalPrices {
	return &HistoricalPrices{
		prices: make(map[string][]pricePoint),
	}
}

// LoadFromExtendedJSONL reads prices from the extended 90-day JSONL format.
func (hp *HistoricalPrices) LoadFromExtendedJSONL(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open historical prices: %w", err)
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var rec struct {
			Date   string  `json:"Date"`
			Symbol string  `json:"Symbol"`
			Close  float64 `json:"Close"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &rec); err != nil {
			continue // skip malformed lines
		}
		t, err := time.Parse(time.RFC3339, rec.Date)
		if err != nil {
			continue
		}
		hp.prices[rec.Symbol] = append(hp.prices[rec.Symbol], pricePoint{Date: t, Close: rec.Close})
	}
	if err := scanner.Err(); err != nil {
		return err
	}

	// Sort each symbol's series by date
	for sym := range hp.prices {
		sort.Slice(hp.prices[sym], func(i, j int) bool {
			return hp.prices[sym][i].Date.Before(hp.prices[sym][j].Date)
		})
	}
	return nil
}

// GetCloseSeries returns the full sorted close-price series for a symbol.
func (hp *HistoricalPrices) GetCloseSeries(symbol string) []float64 {
	pts, ok := hp.prices[symbol]
	if !ok {
		return nil
	}
	out := make([]float64, len(pts))
	for i, p := range pts {
		out[i] = p.Close
	}
	return out
}

// SMA computes the simple moving average over the last N days.
func (hp *HistoricalPrices) SMA(symbol string, days int) float64 {
	series := hp.GetCloseSeries(symbol)
	if len(series) < days {
		return 0
	}
	sum := 0.0
	for i := len(series) - days; i < len(series); i++ {
		sum += series[i]
	}
	return sum / float64(days)
}

// MomentumReturn computes (latest / price N days ago - 1).
//
// CAUTION: computed from UNADJUSTED prices — see package doc for bias.
// This UNDERSTATES true total return across ex-dividend dates.
func (hp *HistoricalPrices) MomentumReturn(symbol string, days int) float64 {
	series := hp.GetCloseSeries(symbol)
	if len(series) < days+1 {
		return 0
	}
	latest := series[len(series)-1]
	past := series[len(series)-1-days]
	if past == 0 {
		return 0
	}
	return latest/past - 1
}

// Volatility computes the standard deviation of daily returns over the last N days.
//
// CAUTION: inherits the same unadjusted-price bias as MomentumReturn; do not
// interpret as ex-corporate-action volatility.
func (hp *HistoricalPrices) Volatility(symbol string, days int) float64 {
	series := hp.GetCloseSeries(symbol)
	if len(series) < days+1 {
		return 0
	}
	returns := make([]float64, 0, days)
	start := max(len(series)-days-1, 0)
	for i := start + 1; i < len(series); i++ {
		if series[i-1] == 0 {
			continue
		}
		returns = append(returns, series[i]/series[i-1]-1)
	}
	if len(returns) == 0 {
		return 0
	}
	mean := 0.0
	for _, r := range returns {
		mean += r
	}
	mean /= float64(len(returns))
	variance := 0.0
	for _, r := range returns {
		d := r - mean
		variance += d * d
	}
	variance /= float64(len(returns))
	return math.Sqrt(variance)
}
