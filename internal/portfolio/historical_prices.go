// Package-internal note: prices in HistoricalPrices are stored RAW and
// UNADJUSTED by default. Call AdjustForCorporateActions to backward-adjust
// historical prices for dividends, stock splits, and capital reductions.
//
// Implications for downstream consumers:
//   - MomentumReturn (and any other simple price-difference return) UNDERSTATES
//     true total return across ex-dividend dates by the dividend amount.
//   - Volatility computed from unadjusted daily returns is INFLATED around
//     ex-dividend / split events.
//   - Sharpe and other risk-adjusted metrics that depend on mean/stddev of
//     returns inherit this bias.
//
// AdjustForCorporateActions (landed β): backward-adjusts pre-event prices
// using TWSE-published ReferencePrice when available, falling back to cash
// dividend, stock dividend, and capital reduction fields. The operation is
// idempotent. Call ActionEffects(symbol) to retrieve the list of applied
// adjustments.
//
// Historical:
//   - P1-2-α: CorporateAction domain type and upstream provider integration.
//   - P1-2-β: AdjustForCorporateActions algorithm (this branch).
package portfolio

import (
	"bufio"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sort"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/domain/shared"
	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// HistoricalPrices stores closing prices by symbol and date.
type HistoricalPrices struct {
	prices  map[string][]pricePoint          // symbol -> sorted by date
	effects map[string][]shared.ActionEffect // symbol -> applied adjustments
}

type pricePoint struct {
	Date  time.Time
	Close float64
}

// NewHistoricalPrices creates an empty repository.
func NewHistoricalPrices() *HistoricalPrices {
	return &HistoricalPrices{
		prices:  make(map[string][]pricePoint),
		effects: make(map[string][]shared.ActionEffect),
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

// CloseSeries returns the full sorted (date, close) series for a symbol.
//
// Unlike GetCloseSeries it preserves the dates, which the composition needs when
// it compares the stored (raw) replay series against an official adjusted
// series. It reuses marketdata.DatedClose so that no extra portfolio type is
// exported.
func (hp *HistoricalPrices) CloseSeries(symbol string) []marketdata.DatedClose {
	pts, ok := hp.prices[symbol]
	if !ok {
		return nil
	}
	out := make([]marketdata.DatedClose, len(pts))
	for i, p := range pts {
		out[i] = marketdata.DatedClose{Date: p.Date, Close: p.Close}
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

// AdjustForCorporateActions rewrites the internal price points so that all
// prices BEFORE each corporate action are backward-adjusted.
//
// The implementation is idempotent (calling twice produces the same result).
// Returns nil if no actions are supplied (no-op).
//
// actions must be sorted by ExDate ascending; caller is responsible.
// actions must be for symbols present in the HistoricalPrices instance;
// unknown symbols are silently ignored.
func (hp *HistoricalPrices) AdjustForCorporateActions(actions []domain.CorporateAction) error {
	if len(actions) == 0 {
		return nil
	}
	for _, action := range actions {
		pts, ok := hp.prices[action.Symbol]
		if !ok {
			continue
		}
		splitIdx := -1
		for i, p := range pts {
			if !p.Date.Before(action.ExDate) {
				splitIdx = i
				break
			}
		}
		if splitIdx <= 0 {
			continue
		}
		postEventPrice := pts[splitIdx].Close
		if postEventPrice <= 0 {
			return fmt.Errorf("adjust corporate actions: symbol %s post-event price at %s is %f (non-positive)",
				action.Symbol, action.ExDate.Format("2006-01-02"), postEventPrice)
		}
		factor, err := computeBackwardAdjustmentFactor(action, postEventPrice)
		if err != nil {
			return fmt.Errorf("adjust corporate actions: symbol %s: %w", action.Symbol, err)
		}
		if factor <= 0 {
			return fmt.Errorf("adjust corporate actions: symbol %s computed adjustment factor %f is non-positive",
				action.Symbol, factor)
		}
		adjustType := adjustTypeFromAction(action)
		if hp.hasAppliedEffect(action.Symbol, action.ExDate, adjustType, factor) {
			continue // idempotent: this exact action was already applied
		}
		for i := 0; i < splitIdx; i++ {
			hp.prices[action.Symbol][i].Close *= factor
		}
		hp.recordEffect(action, factor)
	}
	return nil
}

// computeBackwardAdjustmentFactor returns the multiplier for pre-event prices.
// ReferencePrice > 0: factor = ReferencePrice / postEventRawPrice
// ReferencePrice == 0: factor derived from cash/stock/reduction fields.
func computeBackwardAdjustmentFactor(action domain.CorporateAction, postEventRawPrice float64) (float64, error) {
	if action.ReferencePrice > 0 {
		return action.ReferencePrice / postEventRawPrice, nil
	}
	factor := 1.0
	if action.CashDividend > 0 {
		subFactor := (postEventRawPrice - action.CashDividend) / postEventRawPrice
		if subFactor <= 0 {
			return 0, fmt.Errorf("cash dividend %f exceeds post-event price %f", action.CashDividend, postEventRawPrice)
		}
		factor *= subFactor
	}
	if action.StockDividend > 0 {
		// Stock dividend units: shares per 10 shares held (FinMind
		// StockEarningsDistribution), equivalently S/10 shares per share, so a
		// holder ends up with 1+S/10 shares per pre-event share. Pre-event
		// prices are therefore rewritten by 10/(10+S), i.e. 1/(1+S/10) when S is
		// expressed per share.
		//
		// The earlier implementation used (10.0-S)/10.0, which is the
		// cash-dividend shape; it agrees with 10/(10+S) only for small S and goes
		// non-positive for S >= 10. Real case: 6669.TW's 2026-09-02 stock
		// dividend carries S = 19.827946, for which the old shape yields -0.9828
		// and AdjustForCorporateActions rejects the action as a non-positive
		// factor.
		//
		// Fingerprint (official TaiwanStockPriceAdj / raw replay close for
		// 6669.TW on 2026-09-01): 2614.997/7800 = 0.33526, while
		// 10/(10+19.827946) = 0.33528.
		subFactor := 10.0 / (10.0 + action.StockDividend)
		factor *= subFactor
	}
	if action.CapitalReductionRatio > 0 {
		subFactor := 1.0 - action.CapitalReductionRatio
		factor *= subFactor
	}
	return factor, nil
}

func adjustTypeFromAction(action domain.CorporateAction) shared.AdjustType {
	if action.CashDividend > 0 {
		return shared.AdjustCashDividend
	}
	if action.StockDividend > 0 {
		return shared.AdjustStockDividend
	}
	if action.CapitalReductionRatio > 0 {
		return shared.AdjustCapitalReduction
	}
	return shared.AdjustCashDividend
}

// hasAppliedEffect reports whether the identical action — same ex-date, same
// adjustment type and same factor — was already applied to the symbol.
//
// This is what makes AdjustForCorporateActions idempotent in practice: without
// it, re-applying the same actions shrank pre-event prices again (the previous
// heuristic compared the bar before the event against postEventPrice*factor,
// which is unrelated to the already-adjusted value). A genuinely different
// factor for the same ex-date and type still applies, so a corrected upstream
// action is not masked.
func (hp *HistoricalPrices) hasAppliedEffect(symbol string, exDate time.Time, adjustType shared.AdjustType, factor float64) bool {
	for _, eff := range hp.effects[symbol] {
		if eff.ExDate.Equal(exDate) && eff.Type == adjustType && math.Abs(eff.Adjustment-factor) <= 1e-9 {
			return true
		}
	}
	return false
}

func (hp *HistoricalPrices) recordEffect(action domain.CorporateAction, factor float64) {
	eff := shared.ActionEffect{
		Type:       adjustTypeFromAction(action),
		ExDate:     action.ExDate,
		Adjustment: factor,
	}
	hp.effects[action.Symbol] = append(hp.effects[action.Symbol], eff)
}

// ActionEffects returns the list of corporate actions as ActionEffect records
// for downstream consumers (FactorEngine, reporters).
// The Adjustment field is the cumulative backward adjustment factor for
// pre-event prices (1.0 means no adjustment).
func (hp *HistoricalPrices) ActionEffects(symbol string) []shared.ActionEffect {
	return hp.effects[symbol]
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
