package marketdata

// TSMADRPremium computes the premium/discount of TSM ADR (NYSE: TSM) relative
// to TSMC.TW (2330). A positive premium means the ADR trades above the
// Taiwan-listed equivalent, typically driven by USD/TWD appreciation,
// US market risk-on sentiment, and foreign capital inflows to Taiwan.
//
// Formula: premium% = (adrPriceUSD * usdTwd / tsmeShPerAdr - tsmcTWD) / tsmcTWD * 100
//
// Parameters:
//
//	adrPriceUSD - TSM closing price on NYSE (USD)
//	tsmcTWD     - TSMC 2330 closing price on TWSE (TWD)
//	usdTwd      - USD/TWD exchange rate
//	sharesPer   - ADR ratio (default 5: 1 ADR = 5 TSMC.TW shares)
//
// From the 2024-2025 research report: TSM ADR returned +33.34% vs TSMC.TW +9.50%,
// a 23.84 percentage-point gap driven by USD strength and US AI sentiment premium.
// Widening premium signals US-led risk appetite diverging from local Taiwan pricing.
func TSMADRPremium(adrPriceUSD, tsmcTWD, usdTwd, sharesPer float64) (premiumPct float64) {
	if sharesPer <= 0 {
		sharesPer = 5 // standard TSMC ADR ratio
	}
	if tsmcTWD <= 0 {
		return 0
	}
	adrEquivalentTWD := adrPriceUSD * usdTwd / sharesPer
	return (adrEquivalentTWD - tsmcTWD) / tsmcTWD * 100
}
