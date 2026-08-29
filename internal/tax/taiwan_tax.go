package tax

import (
	"errors"
	"fmt"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// ErrInvalidPositionTaxInput is returned by strict tax calculation variants
// when position quantity or sell price is non-positive.
var ErrInvalidPositionTaxInput = errors.New("invalid position tax input")

// TaiwanTaxCalculator computes Taiwan equity taxes for simulation and reporting.
//
// Tax rates:
//   - Dividend income tax: 28% (included in comprehensive income tax)
//   - Securities transaction tax (sell side only): 0.3% of sell notional
//
// The default DividendTaxRate (0.28) already bakes in the NHI supplementary
// premium (二代健保補充保費) at NHISurchargeRate (2.11%). Set TaxConfig.IncludeNHI
// to false to compute dividend tax at the NHI-exclusive rate (28% − 2.11% = 25.89%),
// modeling scenarios where the supplementary premium is waived (e.g. non-
// resident accounts, employer-paid dividends, or counterfactual analysis).
type TaiwanTaxCalculator struct {
	cfg domain.TaxConfig
}

// NHISurchargeRate is the NHI supplementary premium portion embedded in
// domain.DefaultTaiwanTaxConfig().DividendTaxRate (0.28). It is subtracted
// from the dividend tax rate when TaxConfig.IncludeNHI is false.
const NHISurchargeRate = 0.0211

// NewTaiwanTaxCalculator creates a calculator with the given config.
// Use domain.DefaultTaiwanTaxConfig() for standard Taiwan rates.
func NewTaiwanTaxCalculator(cfg domain.TaxConfig) *TaiwanTaxCalculator {
	return &TaiwanTaxCalculator{cfg: cfg}
}

// Config returns the active tax configuration.
func (c *TaiwanTaxCalculator) Config() domain.TaxConfig {
	return c.cfg
}

// effectiveDividendTaxRate returns the dividend tax rate actually applied to
// calculations, gating out the NHI surcharge when cfg.IncludeNHI is false.
func (c *TaiwanTaxCalculator) effectiveDividendTaxRate() float64 {
	if c.cfg.IncludeNHI {
		return c.cfg.DividendTaxRate
	}
	// Use the config's NHISurchargeRate if set; fall back to the package
	// constant for backwards compatibility with zero-value NHISurchargeRate.
	surcharge := c.cfg.NHISurchargeRate
	if surcharge == 0 {
		surcharge = NHISurchargeRate
	}
	// Defensive floor: never go below zero even if a caller misconfigures
	// DividendTaxRate below the NHI surcharge.
	rate := c.cfg.DividendTaxRate - surcharge
	if rate < 0 {
		return 0
	}
	return rate
}

// CalculateDividendTax returns the tax owed on a dividend amount.
// When TaxConfig.IncludeNHI is false, the NHI surcharge (NHISurchargeRate)
// is excluded from the effective rate.
func (c *TaiwanTaxCalculator) CalculateDividendTax(dividendAmount float64) float64 {
	if dividendAmount <= 0 {
		return 0
	}
	return dividendAmount * c.effectiveDividendTaxRate()
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
//
// Deprecated: use CalculatePositionTaxStrict for explicit error handling on
// invalid inputs (zero/negative quantity or sell price).
func (c *TaiwanTaxCalculator) CalculatePositionTax(pos domain.Position, sellPrice float64, dividendReceived float64) domain.TaxSnapshot {
	if pos.Quantity <= 0 || sellPrice <= 0 {
		return domain.TaxSnapshot{
			Symbol:             pos.Symbol,
			DividendTaxRate:    c.effectiveDividendTaxRate(),
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
		DividendTaxRate:    c.effectiveDividendTaxRate(),
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

// CalculatePositionTaxStrict computes the full tax snapshot for a single position
// and returns an error when pos.Quantity <= 0 or sellPrice <= 0.
func (c *TaiwanTaxCalculator) CalculatePositionTaxStrict(pos domain.Position, sellPrice float64, dividendReceived float64) (domain.TaxSnapshot, error) {
	if pos.Quantity <= 0 {
		return domain.TaxSnapshot{}, fmt.Errorf("%w: quantity must be positive, got %d", ErrInvalidPositionTaxInput, pos.Quantity)
	}
	if sellPrice <= 0 {
		return domain.TaxSnapshot{}, fmt.Errorf("%w: sell price must be positive, got %v", ErrInvalidPositionTaxInput, sellPrice)
	}

	sellNotional := float64(pos.Quantity) * sellPrice
	divTax := c.CalculateDividendTax(dividendReceived)
	txnTax := c.CalculateTransactionTax(sellNotional)
	totalTax := divTax + txnTax

	unrealizedPnL := float64(pos.Quantity) * (sellPrice - pos.AverageCost)
	afterTaxPnL := unrealizedPnL - totalTax

	return domain.TaxSnapshot{
		Symbol:             pos.Symbol,
		DividendTaxRate:    c.effectiveDividendTaxRate(),
		TransactionTaxRate: c.cfg.TransactionTaxRate,
		DividendTax:        divTax,
		TransactionTax:     txnTax,
		TotalTax:           totalTax,
		AfterTaxPnL:        afterTaxPnL,
	}, nil
}

// CalculatePortfolioTaxStrict computes tax snapshots for an entire portfolio
// and returns an error on the first position with invalid input.
// sellPrices maps symbol → assumed sell price; dividends maps symbol → dividend received.
func (c *TaiwanTaxCalculator) CalculatePortfolioTaxStrict(
	positions []domain.Position,
	sellPrices map[string]float64,
	dividends map[string]float64,
) ([]domain.TaxSnapshot, error) {
	snapshots := make([]domain.TaxSnapshot, 0, len(positions))
	for _, pos := range positions {
		sellPrice := sellPrices[pos.Symbol]
		if sellPrice <= 0 {
			sellPrice = pos.CurrentPrice
		}
		div := dividends[pos.Symbol]
		snap, err := c.CalculatePositionTaxStrict(pos, sellPrice, div)
		if err != nil {
			return nil, err
		}
		snapshots = append(snapshots, snap)
	}
	return snapshots, nil
}

// TaiwanCostModel computes round-trip trading costs for Taiwan equities.
//
// Based on Taiwan empirical data:
//   - AvgTradingCost: 0.00654 (~0.654%) — aggregate of commission
//     (0.1425% × broker_discount ~0.6) + slippage
//   - TaxRate: 0.003 (0.3%) — securities transaction tax (sell side only)
type TaiwanCostModel struct {
	AvgTradingCost float64
	TaxRate        float64
}

// NewTaiwanCostModel creates a cost model with the given rates.
//
// avgTradingCost should be sourced from ParametersConfig.Baseline.AvgTradingCost
// (default 0.00654). taxRate should be the securities transaction tax rate
// (typically domain.DefaultTaiwanTaxConfig().TransactionTaxRate = 0.003).
func NewTaiwanCostModel(avgTradingCost, taxRate float64) *TaiwanCostModel {
	return &TaiwanCostModel{
		AvgTradingCost: avgTradingCost,
		TaxRate:        taxRate,
	}
}

// RoundTripCost returns the total round-trip cost as a fraction of turnover.
// RoundTripCost = turnover * (AvgTradingCost + TaxRate)
func (cm *TaiwanCostModel) RoundTripCost(turnover float64) float64 {
	if turnover <= 0 {
		return 0
	}
	return turnover * (cm.AvgTradingCost + cm.TaxRate)
}

// NetReturn returns the net return after deducting round-trip trading costs.
func (cm *TaiwanCostModel) NetReturn(rawReturn, turnover float64) float64 {
	return rawReturn - cm.RoundTripCost(turnover)
}

// ApplyToSeries applies round-trip cost adjustments to a series of raw returns
// paired with corresponding turnover rates.
// Returns a new slice; does not mutate the input.
func (cm *TaiwanCostModel) ApplyToSeries(rawReturns, turnovers []float64) []float64 {
	n := min(len(turnovers), len(rawReturns))
	result := make([]float64, n)
	for i := 0; i < n; i++ {
		result[i] = cm.NetReturn(rawReturns[i], turnovers[i])
	}
	return result
}
