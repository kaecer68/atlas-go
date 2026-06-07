package eventlogic

import "github.com/kaecer68/atlas-go/internal/marketdata"

// SnapshotToValidationContext converts a MacroDataSnapshot into a ValidationContext
// suitable for evaluating cross-market event rules. Fields are keyed by the
// dotted paths used in seed rule Conditions (e.g., "SOXIndex.ChangePct").
func SnapshotToValidationContext(snap marketdata.MacroDataSnapshot) *ValidationContext {
	ctx := &ValidationContext{
		NumericFields: make(map[string]float64),
		StringFields:  make(map[string]string),
	}

	ctx.NumericFields["SOXIndex.ChangePct"] = snap.SOXIndex.ChangePct
	ctx.NumericFields["DXY.ChangePct"] = snap.DXY.ChangePct
	ctx.NumericFields["Bdi.ChangePct"] = snap.Bdi.ChangePct
	ctx.NumericFields["USD_TWD.Value"] = snap.USD_TWD.Value
	ctx.NumericFields["VIX.Value"] = snap.VIX.Value
	ctx.NumericFields["SPXIndex.ChangePct"] = snap.SPXIndex.ChangePct
	ctx.NumericFields["NDXIndex.ChangePct"] = snap.NDXIndex.ChangePct
	ctx.NumericFields["DJIIndex.ChangePct"] = snap.DJIIndex.ChangePct
	ctx.NumericFields["TSMADR.ChangePct"] = snap.TSMADR.ChangePct
	ctx.NumericFields["Gold.ChangePct"] = snap.Gold.ChangePct
	ctx.NumericFields["Oil.ChangePct"] = snap.Oil.ChangePct
	ctx.NumericFields["JPY.ChangePct"] = snap.JPY.ChangePct
	ctx.NumericFields["US10Y.ChangePct"] = snap.US10Y.ChangePct
	ctx.NumericFields["ForeignInvestorNet.Value"] = snap.ForeignInvestorNet.Value
	ctx.NumericFields["TSMCRevenue.ChangePct"] = snap.TSMCRevenue.ChangePct

	// Narrative theme detection: set to non-zero if any active theme matches.
	// This is a stub placeholder for future NarrativeEvent→ValidationContext integration.
	ctx.NumericFields["NarrativeTheme"] = 0.0

	return ctx
}

// EvaluateActiveRules evaluates all active rules against a snapshot and returns
// which rules fired. Unlike ValidateAll, this does not record hit/miss outcomes
// since we don't know the actual market direction yet.
func EvaluateActiveRules(v *RuleValidator, snap marketdata.MacroDataSnapshot) []string {
	ctx := SnapshotToValidationContext(snap)
	var fired []string
	for _, r := range v.registry.ListActive() {
		if v.EvaluateRule(r, ctx) {
			fired = append(fired, r.ID)
		}
	}
	return fired
}
