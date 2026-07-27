package capitalflow

import (
	"context"
	"testing"

	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// TestExtractForeignLeadingSignal verifies the foreign ForceScore carries
// the leading Z from the TAIFEX futures OI series (manifest #E01 + #E05).
//
// BK-15 makes Score history-driven, so the test supplies the prior
// futures OI samples via the history map instead of poking the
// (now-removed) ext.foreignFuturesWindow.push. The behavioral
// assertion — LeadingZ is non-zero when the futures series has
// variance — is unchanged.
func TestExtractForeignLeadingSignal(t *testing.T) {
	ext := NewForceExtractor()
	snap := marketdata.MacroDataSnapshot{
		ForeignInvestorNet:  marketdata.MacroDataPoint{Symbol: "FOREIGN_NET", Value: 50},
		ForeignFuturesOINet: marketdata.MacroDataPoint{Symbol: "TX_FOREIGN_OI_NET", Value: -84000},
		RecordedAt:          1704067200,
	}
	history := map[ForceName][]RollingSample{
		ForceFutures: {
			{RawValue: -50000},
			{RawValue: -60000},
			{RawValue: -70000},
			{RawValue: -80000},
			{RawValue: -90000},
		},
	}
	forces := ext.Score(snap, "", history)
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

// TestExtract_NoDataDoesNotPush verifies spec §8.3 / CF-INV-06: a
// ForceScore that reports DataAvailable=false (e.g. government
// Symbol empty) must never be persisted as a zero-valued
// RollingSample. The test mirrors the production Refresh writer
// contract — only forces with DataAvailable=true become samples —
// and asserts the unavailable dimension is absent from the store.
//
// Production Refresh wiring (Task 4) will reuse this same filter;
// until then the test is RED because RollingSample / SourceTWSET86
// do not exist.
func TestExtract_NoDataDoesNotPush(t *testing.T) {
	ext := NewForceExtractor()
	snap := marketdata.MacroDataSnapshot{
		ForeignInvestorNet: marketdata.MacroDataPoint{Symbol: "FOREIGN_NET", Value: 50},
		// GovernmentNet.Symbol intentionally empty.
		RecordedAt: 1704067200,
	}
	forces := ext.Extract(snap)

	// Sanity: the government ForceScore must report
	// DataAvailable=false so the writer knows to skip it.
	var gov ForceScore
	govFound := false
	for _, f := range forces {
		if f.Force == ForceGovernment {
			gov = f
			govFound = true
		}
	}
	if !govFound {
		t.Fatalf("government ForceScore missing from Extract output")
	}
	if gov.DataAvailable {
		t.Fatalf("government DataAvailable=true with empty Symbol; expected false")
	}

	// Mirrors the production writer: convert each available force
	// into a RollingSample and UpsertDay once for the trading date.
	store := &stubRollingStore{}
	var samples []RollingSample
	for _, f := range forces {
		if !f.DataAvailable {
			continue
		}
		samples = append(samples, RollingSample{
			TradingDate: "2026-07-17",
			Dimension:   f.Force,
			RawValue:    f.RawValue,
			Unit:        "hundred_million_shares",
			SourceID:    SourceTWSET86,
		})
	}
	if err := store.UpsertDay(context.Background(), "2026-07-17", samples); err != nil {
		t.Fatalf("UpsertDay: %v", err)
	}

	for _, s := range store.samples {
		if s.Dimension == ForceGovernment {
			t.Errorf("government RollingSample written despite DataAvailable=false (CF-INV-06 / spec §8.3)")
		}
		if s.RawValue == 0 {
			t.Errorf("zero-valued sample persisted for dimension %q (CF-INV-06)", s.Dimension)
		}
	}
}

// TestForceScore_ProvenanceAndRuntimeFields anchors the new E07 provenance
// + runtime fields on ForceScore for the foreign dimension (spec §6 / §7).
// RED today: ForceScore.DimensionRole / EvidenceClass / SourceID / Unit /
// AsOfTradingDate / SampleCount / CalibrationStatus / ParticipatesInActorConsensus /
// WeightDeprecated do not exist on the struct yet.
//
// This test mirrors the live-snapshot pattern from TestExtractForeignLeadingSignal
// above so the E05 test surface stays consistent: the same fixture (foreign
// spot + futures OI) drives both E05 legacy assertions and E07 new ones.
func TestForceScore_ProvenanceAndRuntimeFields(t *testing.T) {
	ext := NewForceExtractor()
	snap := marketdata.MacroDataSnapshot{
		ForeignInvestorNet:  marketdata.MacroDataPoint{Symbol: "FOREIGN_NET", Value: 50},
		ForeignFuturesOINet: marketdata.MacroDataPoint{Symbol: "TX_FOREIGN_OI_NET", Value: -84000},
		RecordedAt:          1704067200,
	}
	history := map[ForceName][]RollingSample{
		ForceFutures: {
			{RawValue: -50000},
			{RawValue: -60000},
			{RawValue: -70000},
			{RawValue: -80000},
			{RawValue: -90000},
		},
	}
	forces := ext.Score(snap, "2026-07-17", history)
	var foreign ForceScore
	for _, f := range forces {
		if f.Force == ForceForeign {
			foreign = f
		}
	}
	if foreign.Force != ForceForeign {
		t.Fatalf("foreign ForceScore missing from Extract output")
	}

	// spec §6 — foreign is an official_actor with the TWSE T86 source.
	if foreign.DimensionRole != "official_actor" {
		t.Errorf("DimensionRole = %q, want official_actor (spec §6)", foreign.DimensionRole)
	}
	if foreign.SourceID != "SRC-TWSE-T86" {
		t.Errorf("SourceID = %q, want SRC-TWSE-T86 (spec §5 / §6)", foreign.SourceID)
	}
	if foreign.Unit != "hundred_million_shares" {
		t.Errorf("Unit = %q, want hundred_million_shares (spec §6)", foreign.Unit)
	}
	// spec §7 — required runtime metadata fields.
	if foreign.AsOfTradingDate != "2026-07-17" {
		t.Errorf("AsOfTradingDate = %q, want 2026-07-17 (spec §7)", foreign.AsOfTradingDate)
	}
	if foreign.CalibrationStatus == "" {
		t.Errorf("CalibrationStatus empty; spec §7 requires calibrating/eligible/degraded")
	}
	// spec §6 — only the three official_actor dimensions vote in actor
	// consensus. Foreign is one of them.
	if !foreign.ParticipatesInActorConsensus {
		t.Errorf("ParticipatesInActorConsensus = false, want true for official_actor (spec §6)")
	}
	// spec §7.2 — legacy weight is hidden behind WeightDeprecated; the
	// numeric Weight must be 0 to prevent the cross-unit aggregate from
	// re-entering the API.
	if foreign.Weight != 0 {
		t.Errorf("Weight = %f, want 0 (spec §7.2 / CF-INV-07)", foreign.Weight)
	}
	if !foreign.WeightDeprecated {
		t.Errorf("WeightDeprecated = false, want true (spec §7.2)")
	}
}

// TestForceScore_LegacyRoleStillParseable locks in the E05 contract that
// the legacy Role / Deprecated fields keep their current values when E07
// layers provenance / role classification on top of them. E07 introduces
// a new DimensionRole field as additive metadata; the legacy Role is
// retained for back-compat with consumers that still key off
// "subject/leading_indicator/sentiment" (spec §7.1).
func TestForceScore_LegacyRoleStillParseable(t *testing.T) {
	cases := []struct {
		force ForceName
		wantR string
		wantD bool
	}{
		{ForceForeign, ForceRoleSubject, false},
		{ForceInstitutional, ForceRoleSubject, false},
		{ForceDealer, ForceRoleSubject, false},
		{ForceGovernment, ForceRoleSubject, false},
		{ForceRetail, ForceRoleSubject, false},
		{ForceFutures, ForceRoleLeadingIndicator, true},
		{ForceTSMADR, ForceRoleSentiment, true},
	}
	ext := NewForceExtractor()
	snap := marketdata.MacroDataSnapshot{
		ForeignInvestorNet:  marketdata.MacroDataPoint{Symbol: "FOREIGN_NET", Value: 50},
		ForeignFuturesOINet: marketdata.MacroDataPoint{Symbol: "TX_FOREIGN_OI_NET", Value: -84000},
		TSMADR:              marketdata.MacroDataPoint{Symbol: "TSM_ADR", Value: 100, ChangePct: 1.5},
		DomesticFundNet:     marketdata.MacroDataPoint{Symbol: "DII", Value: 25},
		DealerNet:           marketdata.MacroDataPoint{Symbol: "DEALER", Value: 10},
		GovernmentNet:       marketdata.MacroDataPoint{Symbol: "GOV", Value: 5},
		RetailMarginBalance: marketdata.MacroDataPoint{Symbol: "MARGIN", Value: 1000, ChangePct: 1},
		RetailShortBalance:  marketdata.MacroDataPoint{Symbol: "SHORT", Value: 100, ChangePct: 0.5},
		RecordedAt:          1704067200,
	}
	forces := ext.Extract(snap)
	got := map[ForceName]ForceScore{}
	for _, f := range forces {
		got[f.Force] = f
	}
	for _, c := range cases {
		c := c
		t.Run(string(c.force), func(t *testing.T) {
			f, ok := got[c.force]
			if !ok {
				t.Fatalf("dimension %s missing from Extract output", c.force)
			}
			if f.Role != c.wantR {
				t.Errorf("Role = %q, want %q (E05 legacy contract preserved)", f.Role, c.wantR)
			}
			if f.Deprecated != c.wantD {
				t.Errorf("Deprecated = %v, want %v (E05 legacy contract preserved)", f.Deprecated, c.wantD)
			}
		})
	}
}
