package marketdata

// US tech stock providers backed by YahooStockProvider.
// The parametrized YahooStockProvider eliminates ~150 lines of duplicate
// fetch→parse→bounds logic that previously lived in fetchUSTechSnapshot.

// NVDAProvider fetches NVIDIA Corp (NVDA) from Yahoo Finance.
type NVDAProvider = YahooStockProvider

// AAPLProvider fetches Apple Inc (AAPL) from Yahoo Finance.
type AAPLProvider = YahooStockProvider

// MSFTProvider fetches Microsoft Corp (MSFT) from Yahoo Finance.
type MSFTProvider = YahooStockProvider

// NewNVDAProvider creates a new NVDA provider.
func NewNVDAProvider() *NVDAProvider {
	return newYahooStockProvider("NVDA", "us_nvda", func(s *MacroDataSnapshot) *MacroDataPoint { return &s.NVDA })
}

// NewAAPLProvider creates a new AAPL provider.
func NewAAPLProvider() *AAPLProvider {
	return newYahooStockProvider("AAPL", "us_aapl", func(s *MacroDataSnapshot) *MacroDataPoint { return &s.AAPL })
}

// NewMSFTProvider creates a new MSFT provider.
func NewMSFTProvider() *MSFTProvider {
	return newYahooStockProvider("MSFT", "us_msft", func(s *MacroDataSnapshot) *MacroDataPoint { return &s.MSFT })
}
