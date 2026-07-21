// Package tax provides Taiwan tax calculation via TaiwanTaxCalculator
// for post-trade settlement in the capital allocator integration.
//
// TaiwanTaxCalculator implements the tax rules defined in
// docs/specs/taiwan-tax-spec.md (證券交易稅 + 健保補充費 for dividend income).
// It is invoked from internal/portfolio/capital_allocator.go after each
// trade execution to compute net-of-tax returns.
//
// Tax rules (high level):
//   - 證券交易稅: 0.3% for stocks, 0.1% for ETFs
//   - Dividend income: subject to 健保補充費 (2.11% for high-income earners)
//   - Capital gains: Taiwan does not currently tax stock capital gains
//     for individual investors; this may change with regulatory updates
//
// Calculator is pure (no I/O); the capital allocator handles the integration
// with trade records and ledger append.
//
// Maturity: evolving
package tax
