package narrative

import (
	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// MarketNarrativeDataFromSnapshot converts a macro data snapshot into the
// narrative detection input struct.  Fields not available in the snapshot
// (GeopoliticalGPR, EarningsSurprisePct) are zeroed; the caller may overlay
// query-param overrides for those missing fields.
func MarketNarrativeDataFromSnapshot(snap marketdata.MacroDataSnapshot) MarketNarrativeData {
	us10yBps := 0.0
	if snap.US10Y.Symbol != "" {
		// Approximate daily change in basis points from percent change.
		// ChangePct is the daily % change; Value is the yield level in %.
		// bps ≈ (ChangePct/100) * Value * 100 = ChangePct * Value.
		us10yBps = snap.US10Y.ChangePct * snap.US10Y.Value
	}

	aiSentiment := 0.0
	if snap.TSMCRevenue.Symbol != "" {
		aiSentiment = computeAICapexSentiment(snap.TSMCRevenue.ChangePct)
	}

	// Compute retail-institutional divergence from capital-flow + margin data.
	// Positive = retail is more bullish than institutions (crowding risk).
	retailDiv := 0.0
	if snap.ForeignInvestorNet.Symbol != "" && snap.RetailMarginBalance.Symbol != "" {
		// Foreign signal: selling (<0) → positive contribution to divergence.
		// Typical foreign daily flow range: ±100B TWD → normalize by 50.
		foreignSignal := -snap.ForeignInvestorNet.Value / 50.0
		// Retail signal: margin balance increasing → positive contribution.
		// Typical daily margin change: ±5% → normalize by 2.5.
		retailSignal := snap.RetailMarginBalance.ChangePct / 2.5
		retailDiv = (retailSignal + foreignSignal) / 2.0
	}

	// Compute a heuristic margin z-score from margin balance change %.
	// No historical distribution available → use a simple scaling heuristic.
	marginZScore := 0.0
	if snap.RetailMarginBalance.Symbol != "" {
		marginZScore = snap.RetailMarginBalance.ChangePct / 2.0
	}

	return MarketNarrativeData{
		US10YChangeBps:                us10yBps,
		DXYChangePct:                  snap.DXY.ChangePct,
		VIXLevel:                      snap.VIX.Value,
		USD_TWD_ChangePct:             snap.USD_TWD.ChangePct,
		OilChangePct:                  snap.Oil.ChangePct,
		GoldChangePct:                 snap.Gold.ChangePct,
		GoldLevel:                     snap.Gold.Value,
		JPY_ChangePct:                 snap.JPY.ChangePct,
		JPYLevel:                      snap.JPY.Value,
		AICapexSentiment:              aiSentiment,
		GeopoliticalGPR:               0, // not available in snapshot; caller should overlay
		RetailInstitutionalDivergence: retailDiv,
		MarginZScore:                  marginZScore,
		EarningsSurprisePct:           0, // not available in snapshot; caller should overlay
		// Extended macro inputs
		CPIYoY:                     snap.CPIYoY.Value,
		BDIChangePct:               snap.Bdi.ChangePct,
		CopperChangePct:            snap.Copper.ChangePct,
		ExportElectronicsChangePct: snap.ExportElectronics.ChangePct,
		SOXIndexChangePct:          snap.SOXIndex.ChangePct,
		DRAMSpotPriceChangePct:     snap.DRAMSpotPrice.ChangePct,
	}
}
