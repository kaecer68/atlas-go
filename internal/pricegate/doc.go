// Package pricegate classifies price-derived returns against the gross-error
// bound of FU-20260930-03 / k3 decision 3-1.
//
// Core symbol:
//
//	Check(component, symbol string, prev, curr float64, pos, horizonDays int) bool
//	  — may the consumer use the return between prev and curr? false means
//	  "missing observation": either close is unusable (FU-20260930-04) or the
//	  return exceeds shared.MaxPlausibleReturn(horizonDays) outside the IPO
//	  window (k3 decision 3-1). Rejections are logged as WARN, exemptions as
//	  INFO, and the per-component WARN volume is capped.
//
// Cautions:
//   - Check does NOT return the return value: each consumer keeps its own
//     arithmetic form, so a normal observation scores exactly what it scored
//     before this gate existed
//   - The bound is a gross-error gate (50% per day, compounding for longer
//     horizons), not a volatility filter: limit-up days and corporate-action
//     artifacts below the bound still pass
//   - The first IPOPriceLimitExemptDays observations of a symbol are exempt
//     because TWSE/TPEX apply no daily price limit right after listing
//
// Maturity: stable
package pricegate
