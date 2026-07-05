package marketdata

// US index providers backed by YahooStockProvider.
// The parametrized YahooStockProvider eliminates ~50 lines of duplicate
// fetch→parse→bounds logic that previously lived in fetchUSIndexSnapshot.

// SPXIndexProvider fetches S&P 500 (^GSPC) from Yahoo Finance.
type SPXIndexProvider = YahooStockProvider

// NDXIndexProvider fetches NASDAQ Composite (^IXIC) from Yahoo Finance.
type NDXIndexProvider = YahooStockProvider

// DJIIndexProvider fetches Dow Jones Industrial Average (^DJI) from Yahoo Finance.
type DJIIndexProvider = YahooStockProvider

// NewSPXIndexProvider creates a new S&P 500 provider.
func NewSPXIndexProvider() *SPXIndexProvider {
	return newYahooStockProvider("^GSPC", "us_spx", func(s *MacroDataSnapshot) *MacroDataPoint { return &s.SPXIndex })
}

// NewNDXIndexProvider creates a new NASDAQ provider.
func NewNDXIndexProvider() *NDXIndexProvider {
	return newYahooStockProvider("^IXIC", "us_ndx", func(s *MacroDataSnapshot) *MacroDataPoint { return &s.NDXIndex })
}

// NewDJIIndexProvider creates a new Dow Jones provider.
func NewDJIIndexProvider() *DJIIndexProvider {
	return newYahooStockProvider("^DJI", "us_dji", func(s *MacroDataSnapshot) *MacroDataPoint { return &s.DJIIndex })
}
