package marketdata

// SOXIndexProvider fetches the Philadelphia Semiconductor Index (^SOX) from Yahoo Finance.
type SOXIndexProvider = YahooStockProvider

// NewSOXIndexProvider creates a new SOX index provider.
func NewSOXIndexProvider() *SOXIndexProvider {
	return newYahooStockProvider("^SOX", "sox_index", func(s *MacroDataSnapshot) *MacroDataPoint { return &s.SOXIndex })
}
