package marketdata

// TSMADRProvider fetches Taiwan Semiconductor ADR (TSM) from Yahoo Finance.
type TSMADRProvider = YahooStockProvider

// NewTSMADRProvider creates a new TSM ADR provider.
func NewTSMADRProvider() *TSMADRProvider {
	return newYahooStockProvider("TSM", "tsm_adr", func(s *MacroDataSnapshot) *MacroDataPoint { return &s.TSMADR })
}
