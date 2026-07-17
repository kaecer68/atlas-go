package capitalflow

import (
	"testing"

	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// TestExtractForeignLeadingSignal verifies the foreign ForceScore carries
// the leading Z from the TAIFEX futures OI series (manifest #E01 + #E05).
func TestExtractForeignLeadingSignal(t *testing.T) {
	ext := NewForceExtractor()
	snap := marketdata.MacroDataSnapshot{
		ForeignInvestorNet:  marketdata.MacroDataPoint{Symbol: "FOREIGN_NET", Value: 50},
		ForeignFuturesOINet: marketdata.MacroDataPoint{Symbol: "TX_FOREIGN_OI_NET", Value: -84000},
		RecordedAt:          1704067200,
	}
	// Push a few values to make LeadingZ non-zero (window needs variance).
	for i := 0; i < 5; i++ {
		ext.foreignFuturesWindow.push(float64(-50000 - i*10000))
	}
	forces := ext.Extract(snap)
	var foreign ForceScore
	for _, f := range forces {
		if f.Force == ForceForeign {
			foreign = f
		}
	}
	if foreign.Role != ForceRoleSubject {
		t.Errorf("foreign role=%s, want subject", foreign.Role)
	}
	if foreign.LeadingZ == 0 {
		t.Error("expected non-zero LeadingZ when futures series has variance")
	}
	if foreign.LeadingTrend != "bullish" && foreign.LeadingTrend != "bearish" {
		t.Errorf("LeadingTrend=%s, want bullish/bearish", foreign.LeadingTrend)
	}
}

// TestExtractForeignLeadingNeutralWhenNoData ensures LeadingZ stays at zero
// and trend is neutral when the E01 channel produced no reading (Symbol empty).
func TestExtractForeignLeadingNeutralWhenNoData(t *testing.T) {
	ext := NewForceExtractor()
	snap := marketdata.MacroDataSnapshot{
		ForeignInvestorNet: marketdata.MacroDataPoint{Symbol: "FOREIGN_NET", Value: 50},
		// ForeignFuturesOINet.Symbol intentionally empty
		RecordedAt: 1704067200,
	}
	forces := ext.Extract(snap)
	for _, f := range forces {
		if f.Force == ForceForeign {
			if f.LeadingZ != 0 {
				t.Errorf("LeadingZ=%f, want 0 when no futures data", f.LeadingZ)
			}
			if f.LeadingTrend != "neutral" {
				t.Errorf("LeadingTrend=%s, want neutral", f.LeadingTrend)
			}
		}
	}
}

// TestExtractGovernmentDataAvailable verifies the data_available flag toggles
// based on the E04 官股 channel presence (manifest #E04).
func TestExtractGovernmentDataAvailable(t *testing.T) {
	ext := NewForceExtractor()

	// Empty snapshot — government force has no data.
	forces := ext.Extract(marketdata.MacroDataSnapshot{RecordedAt: 1704067200})
	var gov ForceScore
	for _, f := range forces {
		if f.Force == ForceGovernment {
			gov = f
		}
	}
	if gov.DataAvailable {
		t.Error("expected DataAvailable=false when no government reading")
	}
	if gov.Role != ForceRoleSubject {
		t.Errorf("role=%s, want subject", gov.Role)
	}

	// With reading.
	forces = ext.Extract(marketdata.MacroDataSnapshot{
		GovernmentNet: marketdata.MacroDataPoint{Symbol: "GOV_FLOW_NET", Value: 25},
		RecordedAt:    1704067200,
	})
	for _, f := range forces {
		if f.Force == ForceGovernment {
			if !f.DataAvailable {
				t.Error("expected DataAvailable=true with reading")
			}
		}
	}
}

// TestExtractBackCompatFlags verifies the deprecated + leading_indicator /
// sentiment flags are present per §7 taxonomy.
func TestExtractBackCompatFlags(t *testing.T) {
	ext := NewForceExtractor()
	forces := ext.Extract(marketdata.MacroDataSnapshot{
		ForeignFuturesOINet: marketdata.MacroDataPoint{Symbol: "TX_FOREIGN_OI_NET", Value: -84000},
		TSMADR:              marketdata.MacroDataPoint{Symbol: "TSM_ADR", Value: 100, ChangePct: 1.5},
		RecordedAt:          1704067200,
	})
	got := map[ForceName]ForceScore{}
	for _, f := range forces {
		got[f.Force] = f
	}
	if !got[ForceFutures].Deprecated || got[ForceFutures].Role != ForceRoleLeadingIndicator {
		t.Errorf("futures flags wrong: %+v", got[ForceFutures])
	}
	if !got[ForceTSMADR].Deprecated || got[ForceTSMADR].Role != ForceRoleSentiment {
		t.Errorf("tsm_adr flags wrong: %+v", got[ForceTSMADR])
	}
}

// TestResonanceIgnoresDeprecated ensures leading_indicator and sentiment
// forces do not influence resonance (manifest #E05).
func TestResonanceIgnoresDeprecated(t *testing.T) {
	forces := []ForceScore{
		{Force: ForceForeign, Role: ForceRoleSubject, ZScore: 0.0, Trend: "neutral"},
		{Force: ForceInstitutional, Role: ForceRoleSubject, ZScore: 0.0, Trend: "neutral"},
		{Force: ForceGovernment, Role: ForceRoleSubject, ZScore: 0.0, Trend: "neutral"},
		// These two should NOT enter resonance logic.
		{Force: ForceFutures, Role: ForceRoleLeadingIndicator, Deprecated: true, ZScore: 5.0, Trend: "bullish"},
		{Force: ForceTSMADR, Role: ForceRoleSentiment, Deprecated: true, ZScore: 5.0, Trend: "bullish"},
	}
	r := ComputeResonance(forces)
	if r.Direction != "mixed" {
		t.Errorf("3 neutral subjects + 2 strong deprecated = %s, want mixed", r.Direction)
	}
	if r.Coefficient != 1.0 {
		t.Errorf("coefficient=%f, want 1.0 (deprecated forces must not raise max bound)", r.Coefficient)
	}
}