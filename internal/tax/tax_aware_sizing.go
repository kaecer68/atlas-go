package tax

import (
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/portfolio"
)

// TaiwanLotSize is the standard trading unit for Taiwan equities.
// 1 張 = 1000 股. Position sizes are rounded down to multiples of this unit.
const TaiwanLotSize = 1000

// TaxAwareSizer wraps a portfolio.Sizer and reduces effective capital
// by the transaction tax rate before computing position size.
type TaxAwareSizer struct {
	base   *portfolio.Sizer
	taxCfg domain.TaxConfig
}

// NewTaxAwareSizer creates a tax-aware sizer wrapping the given base sizer.
func NewTaxAwareSizer(base *portfolio.Sizer, taxCfg domain.TaxConfig) *TaxAwareSizer {
	return &TaxAwareSizer{base: base, taxCfg: taxCfg}
}

// SizePosition returns the number of shares to buy after reducing capital
// by the transaction tax rate. The tax-adjusted capital is:
//
//	effectiveCapital = capital / (1 + transactionTaxRate)
//
// This ensures the total cost (shares × price + transaction tax) stays
// within the original capital budget.
func (s *TaxAwareSizer) SizePosition(symbol string, capital float64, price float64) int {
	if price <= 0 || capital <= 0 {
		return 0
	}

	effectiveCapital := capital / (1 + s.taxCfg.TransactionTaxRate)
	shares := int(effectiveCapital / price)
	shares = (shares / TaiwanLotSize) * TaiwanLotSize
	if shares < 0 {
		return 0
	}
	return shares
}

// BaseSizer returns the underlying portfolio.Sizer for advanced sizing.
func (s *TaxAwareSizer) BaseSizer() *portfolio.Sizer {
	return s.base
}
