package tax

import (
	"github.com/kaecer68/atlas-go/internal/domain"
)

// TaiwanTaxCalculator computes Taiwan equity taxes for simulation and reporting.
//
// Tax rates:
//   - Dividend income tax: 28% (included in comprehensive income tax)
//   - Securities transaction tax (sell side only): 0.3% of sell notional
type TaiwanTaxCalculator struct {
	cfg domain.TaxConfig
}

// NewTaiwanTaxCalculator creates a calculator with the given config.
// Use domain.DefaultTaiwanTaxConfig() for standard Taiwan rates.
func NewTaiwanTaxCalculator(cfg domain.TaxConfig) *TaiwanTaxCalculator {
	return &TaiwanTaxCalculator{cfg: cfg}
}

// Config returns the active tax configuration.
func (c *TaiwanTaxCalculator) Config() domain.TaxConfig {
	return c.cfg
}

// CalculateDividendTax returns the tax owed on a dividend amount.
func (c *TaiwanTaxCalculator) CalculateDividendTax(dividendAmount float64) float64 {
	if dividendAmount <= 0 {
		return 0
	}
	return dividendAmount * c.cfg.DividendTaxRate
}

// CalculateTransactionTax returns the securities transaction tax on a sell notional.
// Taiwan STT applies only to sell-side transactions.
func (c *TaiwanTaxCalculator) CalculateTransactionTax(sellNotional float64) float64 {
	if sellNotional <= 0 {
		return 0
	}
	return sellNotional * c.cfg.TransactionTaxRate
}

// CalculatePositionTax computes the full tax snapshot for a single position
// assuming it is sold at sellPrice with dividendReceived during the holding period.
func (c *TaiwanTaxCalculator) CalculatePositionTax(pos domain.Position, sellPrice float64, dividendReceived float64) domain.TaxSnapshot {
	if pos.Quantity <= 0 || sellPrice <= 0 {
		return domain.TaxSnapshot{
			Symbol:             pos.Symbol,
			DividendTaxRate:    c.cfg.DividendTaxRate,
			TransactionTaxRate: c.cfg.TransactionTaxRate,
		}
	}

	sellNotional := float64(pos.Quantity) * sellPrice
	divTax := c.CalculateDividendTax(dividendReceived)
	txnTax := c.CalculateTransactionTax(sellNotional)
	totalTax := divTax + txnTax

	unrealizedPnL := float64(pos.Quantity) * (sellPrice - pos.AverageCost)
	afterTaxPnL := unrealizedPnL - totalTax

	return domain.TaxSnapshot{
		Symbol:             pos.Symbol,
		DividendTaxRate:    c.cfg.DividendTaxRate,
		TransactionTaxRate: c.cfg.TransactionTaxRate,
		DividendTax:        divTax,
		TransactionTax:     txnTax,
		TotalTax:           totalTax,
		AfterTaxPnL:        afterTaxPnL,
	}
}

// CalculatePortfolioTax computes tax snapshots for an entire portfolio.
// sellPrices maps symbol → assumed sell price; dividends maps symbol → dividend received.
func (c *TaiwanTaxCalculator) CalculatePortfolioTax(
	positions []domain.Position,
	sellPrices map[string]float64,
	dividends map[string]float64,
) []domain.TaxSnapshot {
	snapshots := make([]domain.TaxSnapshot, 0, len(positions))
	for _, pos := range positions {
		sellPrice := sellPrices[pos.Symbol]
		if sellPrice <= 0 {
			sellPrice = pos.CurrentPrice
		}
		div := dividends[pos.Symbol]
		snap := c.CalculatePositionTax(pos, sellPrice, div)
		snapshots = append(snapshots, snap)
	}
	return snapshots
}
