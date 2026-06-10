package shared

import "time"

// AdjustType identifies the kind of corporate action effect. The numeric
// mapping is NOT part of the API contract; only the constant names matter.
type AdjustType int

const (
	// AdjustCashDividend is a cash dividend ex-date. Adjustment factor is
	// CashDividend / PreExClose (or computed from ReferencePrice).
	AdjustCashDividend AdjustType = iota
	// AdjustStockDividend is a stock dividend / rights ex-date. Adjustment
	// scales pre-ExDate prices so post-ExDate prices are comparable.
	AdjustStockDividend
	// AdjustCapitalReduction is a capital reduction ex-date. Adjustment
	// scales pre-ExDate prices to reflect the per-share cash return.
	AdjustCapitalReduction
)

// String returns a stable snake_case identifier for serialization and logging.
func (t AdjustType) String() string {
	switch t {
	case AdjustCashDividend:
		return "cash_dividend"
	case AdjustStockDividend:
		return "stock_dividend"
	case AdjustCapitalReduction:
		return "capital_reduction"
	default:
		return "unknown"
	}
}

// ActionEffect describes the price-impact descriptor of one CorporateAction.
// The portfolio adjustment algorithm (P1-2-β) consumes ActionEffect values to
// multiply pre-ExDate prices so that historical series become ex-corporate-
// action comparable.
//
// Adjustment semantics: adjusted = raw / (1 + Adjustment)
//
// Higher-level normalization (Ledoit-Wolf shrinkage across multiple effects,
// reference-price anchoring) is the algorithm's concern, not this type.
type ActionEffect struct {
	Type       AdjustType
	ExDate     time.Time
	Adjustment float64
}
