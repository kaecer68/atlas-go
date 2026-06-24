package portfolio

import (
	"context"
	"math"
	"time"

	"github.com/kaecer68/atlas-go/internal/logging"
)

// isFinite returns true if f is neither Inf nor NaN.
func isFinite(f float64) bool {
	return !math.IsInf(f, 0) && !math.IsNaN(f)
}

// preciousMetalsRegistry maps known precious-metal-related symbols to their subtype.
// Gold ETFs: 00635U, 00693U, 00708L (Taiwan); GLD, IAU, SGOL, BAR (international)
// Silver ETFs: SLV, SIVR, PSLV (international); no TW silver ETF as of 2026
var preciousMetalsRegistry = map[string]string{
	"00635U": "gold",
	"00693U": "gold",
	"00708L": "gold",
	"GLD":    "gold",
	"IAU":    "gold",
	"SGOL":   "gold",
	"BAR":    "gold",
	"SLV":    "silver",
	"SIVR":   "silver",
	"PSLV":   "silver",
}

// isPreciousMetal checks if a symbol is a known precious metals instrument.
// Returns (isPreciousMetal, subtype) where subtype is "gold" or "silver".
func isPreciousMetal(symbol string) (bool, string) {
	subtype, ok := preciousMetalsRegistry[symbol]
	return ok, subtype
}

// IsPreciousMetal is the public version for external callers.
func (fe *FactorEngine) IsPreciousMetal(symbol string) bool {
	isPM, _ := isPreciousMetal(symbol)
	return isPM
}

// ensureAdjusted fetches corporate actions for the symbol and applies price adjustments
// if not already adjusted within the adjustmentTTL window. If no corporate action provider
// is configured, returns nil without error. The latest adjustment time is tracked internally
// via adjustedSymbols and is not part of the public signature.
//
// Idempotency: a symbol whose last adjustment was within adjustmentTTL is skipped
// without re-fetching corporate actions. If the provider returns an error, the symbol
// is NOT marked adjusted — the next cycle will retry.
func (fe *FactorEngine) ensureAdjusted(ctx context.Context, symbol string) error {
	if fe.corpActions == nil {
		return nil
	}

	fe.adjustedMu.Lock()
	lastAdj, exists := fe.adjustedSymbols[symbol]
	fe.adjustedMu.Unlock()

	if exists && time.Since(lastAdj) < fe.adjustmentTTL {
		return nil
	}

	end := time.Now()
	start := end.Add(-365 * 24 * time.Hour)
	actions, err := fe.corpActions.GetCorporateActions(ctx, symbol, start, end)
	if err != nil {
		return err
	}

	if len(actions) > 0 && fe.history != nil {
		if adjErr := fe.history.AdjustForCorporateActions(actions); adjErr != nil {
			logging.Warn("factor_engine", "adjust_for_actions_failed", "symbol", symbol, logging.Err(adjErr))
		}
	}

	now := time.Now()
	fe.adjustedMu.Lock()
	fe.adjustedSymbols[symbol] = now
	fe.adjustedMu.Unlock()

	return nil
}

// classifyIndustry maps a Taiwan-listed symbol to an industry ID used by
// SetCycleTracker / SetCycleCardBuilder to look up CycleTracker positions.
// Returns "" for symbols not in the known registry.
func classifyIndustry(symbol string) string {
	switch symbol {
	case "2330.TW", "2454.TW", "2303.TW":
		return "semiconductor"
	case "2317.TW", "2382.TW":
		return "electronics"
	case "2881.TW", "2882.TW", "2891.TW":
		return "financials"
	case "2603.TW", "2609.TW", "2615.TW":
		return "shipping"
	default:
		return ""
	}
}
