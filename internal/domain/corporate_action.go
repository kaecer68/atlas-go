package domain

import "time"

// CorporateAction is the canonical representation of a single corporate action
// event for a Taiwan-listed symbol. It is the unified shape returned by
// marketdata.CorporateActionProvider implementations and consumed by the
// portfolio adjustment algorithm (AdjustForCorporateActions, P1-2-β).
//
// Maturity: evolving (canonical type introduced by P1-2-α)
//
// Fields:
//   - Symbol:               Stock ticker (e.g., "2330"); matches TWSE / FinMind
//     without ".TW" suffix. Required.
//   - ExDate:               Ex-dividend / ex-right / ex-reduction date, CST
//     timezone. Required.
//   - CashDividend:         Cash dividend per share in TWD; 0 means no cash.
//   - StockDividend:        Stock dividend per share in TWD (face-value basis);
//     0 means no stock dividend.
//   - CapitalReductionRatio: Capital reduction ratio in [0, 1]; 0.10 means 10%
//     capital returned to shareholders; 0 means no reduction.
//   - ReferencePrice:       TWSE-published ex-dividend reference price (除權息
//     參考價), TWD. 0 means not provided by the upstream source. When non-zero,
//     consumers SHOULD prefer this as the post-ExDate anchor instead of
//     recomputing it from raw dividend fields — it avoids floating-point drift
//     across sources.
//   - Source:               Provenance tag — "twse_calendar" | "finmind" |
//     "manual" | "aggregated". Helps downstream audit and dedup decisions.
type CorporateAction struct {
	Symbol                string    `json:"symbol"`
	ExDate                time.Time `json:"ex_date"`
	CashDividend          float64   `json:"cash_dividend"`
	StockDividend         float64   `json:"stock_dividend"`
	CapitalReductionRatio float64   `json:"capital_reduction_ratio"`
	ReferencePrice        float64   `json:"reference_price"`
	Source                string    `json:"source"`
}
