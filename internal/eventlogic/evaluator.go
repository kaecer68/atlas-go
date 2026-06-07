package eventlogic

import (
	"strings"

	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// SnapshotToValidationContext converts a MacroDataSnapshot into a ValidationContext
// suitable for evaluating cross-market event rules. activeThemes is the current
// set of active narrative themes from the lifecycle manager, used to populate
// the NarrativeTheme string field for rule matching.
func SnapshotToValidationContext(snap marketdata.MacroDataSnapshot, activeThemes []string) *ValidationContext {
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

	ctx.StringFields["NarrativeTheme"] = strings.Join(activeThemes, ",")
	return ctx
}

// EvaluateActiveRules evaluates all active rules against a snapshot and returns
// which rules fired. activeThemes are the current narrative themes that should
// be considered for rule matching.
func EvaluateActiveRules(v *RuleValidator, snap marketdata.MacroDataSnapshot, activeThemes []string) []string {
	ctx := SnapshotToValidationContext(snap, activeThemes)
	var fired []string
	for _, r := range v.registry.ListActive() {
		if v.EvaluateRule(r, ctx) {
			fired = append(fired, r.ID)
		}
	}
	return fired
}
