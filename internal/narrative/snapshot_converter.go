package narrative

import (
	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// MarketNarrativeDataFromSnapshot converts a macro data snapshot into the
// narrative detection input struct.  Fields not available in the snapshot
// (GeopoliticalGPR, RetailInstitutionalDivergence, MarginZScore) are zeroed.
// The caller may overlay query-param overrides for those missing fields.
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

	return MarketNarrativeData{
		US10YChangeBps:                us10yBps,
		DXYChangePct:                  snap.DXY.ChangePct,
		VIXLevel:                      snap.VIX.Value,
		USD_TWD_ChangePct:             snap.USD_TWD.ChangePct,
		OilChangePct:                  snap.Oil.ChangePct,
		GoldChangePct:                 snap.Gold.ChangePct,
		JPY_ChangePct:                 snap.JPY.ChangePct,
		AICapexSentiment:              aiSentiment,
		GeopoliticalGPR:               0, // not available in snapshot; caller should overlay
		RetailInstitutionalDivergence: 0, // not available in snapshot; caller should overlay
		MarginZScore:                  0, // not available in snapshot; caller should overlay
	}
}
