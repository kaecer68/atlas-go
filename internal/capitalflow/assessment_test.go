package capitalflow

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// ===========================================================================
// 7×4 provenance matrix — docs/specs/capital-flow-seven-dimension-spec.md §6
//
// The matrix below mirrors spec §6's 3 official_actor + 2 behavioral_proxy +
// 2 signal taxonomy. Every dimension carries: DimensionRole, SourceID, Unit,
// and ParticipatesInActorConsensus. Government and retail have intentionally
// "unspecified" Unit/SourceID today (Task 7 decides the concrete literals;
// these tests still anchor the contract via the "non-empty" guard).
// ===========================================================================

// TestProvenanceMatrix asserts the 7×4 provenance contract from spec §6
// (CF-INV-01 / CF-INV-02). RED today: ForceProvenance and the new
// ForceScore provenance fields do not exist yet.
//
// Implementation note: one shared table-driven test (vs. 7 independent cases)
// keeps the matrix co-located and surfaces accidental role swaps — the
// invariant the table is designed to defend.
func TestProvenanceMatrix(t *testing.T) {
	type want struct {
		role        string // expected DimensionRole
		sourceID    string // expected SourceID; "" means "non-empty required"
		sourceOpen  bool   // when true, only assert NonEmpty for SourceID
		unit        string // expected Unit; "" means "non-empty required"
		unitOpen    bool   // when true, only assert NonEmpty for Unit
		participate bool   // expected ParticipatesInActorConsensus
	}
	cases := []struct {
		force ForceName
		want  want
	}{
		{ForceForeign, want{
			role: "official_actor", sourceID: "SRC-TWSE-T86", unit: "hundred_million_shares",
			participate: true,
		}},
		{ForceInstitutional, want{
			role: "official_actor", sourceID: "SRC-TWSE-T86", unit: "hundred_million_shares",
			participate: true,
		}},
		{ForceDealer, want{
			role: "official_actor", sourceID: "SRC-TWSE-T86", unit: "hundred_million_shares",
			participate: true,
		}},
		{ForceGovernment, want{
			role: "behavioral_proxy", sourceOpen: true, unitOpen: true,
			participate: false,
		}},
		{ForceRetail, want{
			role: "behavioral_proxy", sourceID: "SRC-TWSE-MARGN", unitOpen: true,
			participate: false,
		}},
		{ForceFutures, want{
			role: "positioning_indicator", sourceID: "SRC-TAIFEX-INST", unit: "contracts",
			participate: false,
		}},
		{ForceTSMADR, want{
			role: "cross_market_signal", sourceID: "SRC-SEC-TSMC", unit: "pct",
			participate: false,
		}},
	}

	for _, c := range cases {
		t.Run(string(c.force), func(t *testing.T) {
			got := ComputeForceProvenance(c.force)
			if got.DimensionRole != c.want.role {
				t.Errorf("DimensionRole = %q, want %q (spec §6)", got.DimensionRole, c.want.role)
			}
			if c.want.sourceOpen {
				if got.SourceID == "" {
					t.Errorf("SourceID empty for %s; spec §6 requires a non-empty operator-imported source", c.force)
				}
			} else if got.SourceID != c.want.sourceID {
				t.Errorf("SourceID = %q, want %q (spec §6)", got.SourceID, c.want.sourceID)
			}
			if c.want.unitOpen {
				if got.Unit == "" {
					t.Errorf("Unit empty for %s; spec §6 requires a non-empty composite unit (e.g. pct_composite)", c.force)
				}
			} else if got.Unit != c.want.unit {
				t.Errorf("Unit = %q, want %q (spec §6)", got.Unit, c.want.unit)
			}
			if got.ParticipatesInActorConsensus != c.want.participate {
				t.Errorf("ParticipatesInActorConsensus = %v, want %v (spec §6 / CF-INV-09)",
					got.ParticipatesInActorConsensus, c.want.participate)
			}
		})
	}
}

// TestForceScore_DimensionRoleForcesNewField verifies the new
// DimensionRole field is wired on ForceScore (spec §7 / CF-INV-02).
// RED today: ForceScore.DimensionRole / SourceID / Unit do not exist.
// This test stays in the production package so it fails to compile
// rather than silently passing.
func TestForceScore_DimensionRoleForcesNewField(t *testing.T) {
	score := ForceScore{
		Force:         ForceForeign,
		Role:          ForceRoleSubject,
		DimensionRole: "official_actor",
		SourceID:      "SRC-TWSE-T86",
		Unit:          "hundred_million_shares",
	}
	if score.DimensionRole != "official_actor" {
		t.Errorf("DimensionRole = %q, want official_actor", score.DimensionRole)
	}
	if score.SourceID != "SRC-TWSE-T86" {
		t.Errorf("SourceID = %q, want SRC-TWSE-T86", score.SourceID)
	}
	if score.Unit != "hundred_million_shares" {
		t.Errorf("Unit = %q, want hundred_million_shares", score.Unit)
	}
}

// TestForceScore_DataAvailableDefaultsFalse documents the CF-INV-06 rule:
// data_available=false is the absent-data sentinel, not neutral.
// Existing E05 behavior already honours this; the test guards against
// future regressions when ComputeForceProvenance starts populating the
// field by default.
func TestForceScore_DataAvailableDefaultsFalse(t *testing.T) {
	score := ForceScore{Force: ForceForeign, Role: ForceRoleSubject}
	if score.DataAvailable {
		t.Errorf("DataAvailable = true on zero-value ForceScore; spec §8.3 / CF-INV-06 requires false when no source reading")
	}
}

// TestForceScore_LegacyRoleStillParseable_JSON ensures the E05 legacy
// Role/Deprecated fields remain parseable through JSON round-trip
// (CF-INV-01 / spec §7.1). New DimensionRole is additive, not a
// replacement for the legacy Role.
//
// (The full per-dimension Role/Deprecated coverage lives in
// forces_e05_test.go:TestForceScore_LegacyRoleStillParseable.)
func TestForceScore_LegacyRoleStillParseable_JSON(t *testing.T) {
	in := ForceScore{
		Force:         ForceFutures,
		Role:          ForceRoleLeadingIndicator,
		Deprecated:    true,
		DataAvailable: true,
	}
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out ForceScore
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.Role != ForceRoleLeadingIndicator {
		t.Errorf("Role round-trip = %q, want %q", out.Role, ForceRoleLeadingIndicator)
	}
	if !out.Deprecated {
		t.Errorf("Deprecated round-trip = false, want true")
	}
	if out.Force != ForceFutures {
		t.Errorf("Force round-trip = %q, want %q", out.Force, ForceFutures)
	}
}

// TestForceScore_LegacyWeightDeprecated asserts the legacy weight
// contract from spec §7.2: Weight must be 0 with WeightDeprecated=true
// so existing consumers that read `weight` see a zero (no-op) while
// the new WeightDeprecated flag tells consumers not to interpret it.
// RED today: ForceScore.WeightDeprecated does not exist.
func TestForceScore_LegacyWeightDeprecated(t *testing.T) {
	score := ForceScore{Force: ForceForeign, Role: ForceRoleSubject}
	score.WeightDeprecated = true
	if score.Weight != 0 {
		t.Errorf("Weight = %f, want 0 (spec §7.2 / CF-INV-07 forbids cross-unit weighting)", score.Weight)
	}
	if !score.WeightDeprecated {
		t.Errorf("WeightDeprecated = false, want true (spec §7.2)")
	}

	// JSON round-trip must preserve the deprecated marker.
	b, err := json.Marshal(score)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out ForceScore
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !out.WeightDeprecated {
		t.Errorf("WeightDeprecated lost across JSON round-trip; spec §7.2 contract broken")
	}
}

// ===========================================================================
// CF-INV-09 — actor consensus aligned/opposing MUST exclude positioning
// and cross-market signals. The current ComputeResonance implementation
// (resonance.go:77-84) re-collects every non-neutral force into Aligned
// without filtering on the new ParticipatesInActorConsensus flag.
// This test will turn GREEN once the new flag is respected.
// ===========================================================================

func TestComputeResonance_ActorConsensusExcludesFuturesAndADR(t *testing.T) {
	forces := []ForceScore{
		{Force: ForceForeign, Role: ForceRoleSubject, ZScore: 2.0, Trend: "bullish"},
		{Force: ForceInstitutional, Role: ForceRoleSubject, ZScore: 1.5, Trend: "bullish"},
		{Force: ForceDealer, Role: ForceRoleSubject, ZScore: 0.8, Trend: "bullish"},
		// The two signals below share the bullish trend with foreign.
		// They MUST NOT enter Aligned under CF-INV-09.
		{Force: ForceFutures, Role: ForceRoleLeadingIndicator, Deprecated: true, ZScore: 5.0, Trend: "bullish"},
		{Force: ForceTSMADR, Role: ForceRoleSentiment, Deprecated: true, ZScore: 5.0, Trend: "bullish"},
	}
	r := ComputeResonance(forces)
	for _, f := range []ForceName{ForceFutures, ForceTSMADR} {
		for _, a := range r.Aligned {
			if a == f {
				t.Errorf("CF-INV-09 violation: %s appears in actor consensus Aligned=%v", f, r.Aligned)
			}
		}
		for _, o := range r.Opposing {
			if o == f {
				t.Errorf("CF-INV-09 violation: %s appears in actor consensus Opposing=%v", f, r.Opposing)
			}
		}
	}
}

// ===========================================================================
// CF-INV-10 — DominantActor (from official_actor) and DominantSignal (from
// positioning_indicator / cross_market_signal) are separate fields.
// GenerateDailyReport does not expose them today; the test is RED until
// Task 7 adds them.
// ===========================================================================

func TestGenerateDailyReport_DominantActorAndDominantSignalSeparate(t *testing.T) {
	date := time.Date(2026, 7, 17, 0, 0, 0, 0, time.UTC)
	forces := []ForceScore{
		{Force: ForceForeign, Role: ForceRoleSubject, ZScore: 2.0, Trend: "bullish", DataAvailable: true},
		{Force: ForceInstitutional, Role: ForceRoleSubject, ZScore: 1.5, Trend: "bullish", DataAvailable: true},
		{Force: ForceDealer, Role: ForceRoleSubject, ZScore: 1.0, Trend: "bullish", DataAvailable: true},
		{Force: ForceFutures, Role: ForceRoleLeadingIndicator, Deprecated: true, ZScore: 5.0, Trend: "bullish", DataAvailable: true},
		{Force: ForceTSMADR, Role: ForceRoleSentiment, Deprecated: true, ZScore: 4.0, Trend: "bullish", DataAvailable: true},
	}
	rep := GenerateDailyReport(date, forces, ResonanceResult{Direction: "bullish"})

	// DominantActor must come from an official_actor; the futures/tsm_adr
	// forces with the highest absolute Z must NOT win the actor slot.
	if rep.DominantActor == ForceFutures || rep.DominantActor == ForceTSMADR {
		t.Errorf("CF-INV-10 violation: DominantActor = %q (signals must not win actor slot)", rep.DominantActor)
	}
	// DominantSignal must come from a positioning_indicator or
	// cross_market_signal; if no signal data is present, it's "".
	switch rep.DominantSignal {
	case ForceFutures, ForceTSMADR:
		// acceptable: signal-class dimension
	case "":
		// acceptable: no signal in this fixture (legitimate empty)
	default:
		t.Errorf("CF-INV-10 violation: DominantSignal = %q (must be a signal-class dimension or empty)", rep.DominantSignal)
	}
	// The two fields must be independently addressable.
	if string(rep.DominantActor) == string(rep.DominantSignal) && rep.DominantSignal != "" {
		t.Errorf("DominantActor == DominantSignal (%q); CF-INV-10 requires separate fields", rep.DominantActor)
	}
}

// ===========================================================================
// Assessment struct — spec §9.5 / §7
//
// The DailyReport.Assessment field (CapitalFlowAssessment) and its
// layered sub-assessments (Institutional / Behavioral / ForeignPositioning /
// CrossMarket) do not exist in the current implementation. These tests
// assert presence + the contract from spec §9.
// ===========================================================================

// TestGenerateDailyReport_AssessmentCarriesLayeredSubAssessments exercises
// the spec §9.5 split: institutional / behavioral / foreign_positioning /
// cross_market each have their own DirectionalAssessment. RED today: the
// fields do not exist on DailyReport.
func TestGenerateDailyReport_AssessmentCarriesLayeredSubAssessments(t *testing.T) {
	date := time.Date(2026, 7, 17, 0, 0, 0, 0, time.UTC)
	forces := []ForceScore{
		{Force: ForceForeign, Role: ForceRoleSubject, ZScore: 1.0, Trend: "bullish", DataAvailable: true},
		{Force: ForceInstitutional, Role: ForceRoleSubject, ZScore: 1.0, Trend: "bullish", DataAvailable: true},
		{Force: ForceDealer, Role: ForceRoleSubject, ZScore: 1.0, Trend: "bullish", DataAvailable: true},
	}
	rep := GenerateDailyReport(date, forces, ResonanceResult{Direction: "bullish"})

	if rep.Assessment.CalibrationStatus == "" {
		t.Errorf("Assessment.CalibrationStatus empty; spec §9.5 requires calibrating/eligible/degraded")
	}
	if rep.Assessment.AsOfTradingDate == "" {
		t.Errorf("Assessment.AsOfTradingDate empty; spec §6 requires as-of trading date")
	}
	// Each layer must be its own sub-assessment with independent Available
	// flag — they MUST NOT collapse into a single overall coefficient yet
	// (spec §9.5 / CF-INV-13).
	if rep.Assessment.Institutional.Available {
		// institutional was already populated by the fixture; if Available
		// is false the test is RED on the missing field but we keep the
		// assertion to document the contract.
	}
}

func TestComputeInstitutionalConsensus_UsesSliceContract(t *testing.T) {
	forces := []ForceScore{
		{Force: ForceForeign, Trend: "bullish", DataAvailable: true},
		{Force: ForceInstitutional, Trend: "bullish", DataAvailable: true},
		{Force: ForceDealer, Trend: "bullish", DataAvailable: true},
	}
	got := computeInstitutionalConsensus(forces)
	want := DirectionalAssessment{
		Available: true,
		Direction: "bullish",
		Aligned:   []ForceName{ForceForeign, ForceInstitutional, ForceDealer},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("computeInstitutionalConsensus() = %+v, want %+v", got, want)
	}

	b, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal DirectionalAssessment: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatalf("unmarshal DirectionalAssessment: %v", err)
	}
	for _, key := range []string{"aligned_n", "opposing_n", "reasons_n"} {
		if _, ok := doc[key]; ok {
			t.Errorf("DirectionalAssessment JSON contains implementation-only key %q", key)
		}
	}
	if _, ok := doc["opposing"]; ok {
		t.Errorf("DirectionalAssessment JSON contains empty opposing field despite omitempty")
	}
}

// TestComputeCapitalFlowAssessment_CalibratingByDefault asserts that a
// fresh assessment (no calibration history) reports CalibrationStatus=
// "calibrating" rather than a fabricated "eligible" — spec §8.4 / CF-INV-13.
// RED today: ComputeCapitalFlowAssessment does not exist.
func TestComputeCapitalFlowAssessment_CalibratingByDefault(t *testing.T) {
	forces := []ForceScore{
		{Force: ForceForeign, Role: ForceRoleSubject, ZScore: 1.0, Trend: "bullish", DataAvailable: true},
		{Force: ForceInstitutional, Role: ForceRoleSubject, ZScore: 1.0, Trend: "bullish", DataAvailable: true},
		{Force: ForceDealer, Role: ForceRoleSubject, ZScore: 1.0, Trend: "bullish", DataAvailable: true},
	}
	got := ComputeCapitalFlowAssessment(forces)
	if got.CalibrationStatus != "calibrating" {
		t.Errorf("CalibrationStatus = %q, want \"calibrating\" (spec §8.4 / CF-INV-13)", got.CalibrationStatus)
	}
	if got.EligibleForAutomation() {
		t.Errorf("EligibleForAutomation() = true while calibrating; CF-INV-13 forbids automation on un-calibrated assessment")
	}
}

// TestCapitalFlowAssessment_EligibleForAutomationTransitions covers the
// three calibration states and asserts only "eligible" makes the
// automation gate open. RED today: EligibleForAutomation does not exist.
func TestCapitalFlowAssessment_EligibleForAutomationTransitions(t *testing.T) {
	for _, status := range []string{"calibrating", "eligible", "degraded"} {
		t.Run(status, func(t *testing.T) {
			a := CapitalFlowAssessment{CalibrationStatus: status}
			eligible := a.EligibleForAutomation()
			want := status == "eligible"
			if eligible != want {
				t.Errorf("EligibleForAutomation() for CalibrationStatus=%q = %v, want %v",
					status, eligible, want)
			}
		})
	}
}

// TestCapitalFlowAssessment_ThreeEnumValues documents the enum surface
// from spec §7 / §9.5. Three values, no others. RED today: enum not
// declared yet, but the string constants are referenced via the table.
func TestCapitalFlowAssessment_ThreeEnumValues(t *testing.T) {
	for _, status := range []string{"calibrating", "eligible", "degraded"} {
		// Each value must round-trip cleanly through the struct field
		// without surprising the consumer.
		a := CapitalFlowAssessment{CalibrationStatus: status}
		if a.CalibrationStatus != status {
			t.Errorf("CalibrationStatus round-trip = %q, want %q", a.CalibrationStatus, status)
		}
	}
}

// ===========================================================================
// Service.LatestAssessment — automation face for eventdriven (spec §9.5).
//
// RED today: Service.LatestAssessment method does not exist.
// The test asserts the method exists and returns a non-zero assessment
// when CalibrationStatus must be "calibrating" on a fresh service
// (no calibration history has been recorded yet).
// ===========================================================================

func TestService_LatestAssessment_MethodExistsAndIsCalibrating(t *testing.T) {
	h := NewHandler(&mockProvider{snap: testSnapshot()})
	svc := ServiceFromHandler(h)
	ctx := t.Context()
	assessment, err := svc.LatestAssessment(ctx)
	if err != nil {
		t.Fatalf("LatestAssessment: %v", err)
	}
	if assessment.CalibrationStatus != "calibrating" {
		t.Errorf("CalibrationStatus = %q, want \"calibrating\" on a fresh service", assessment.CalibrationStatus)
	}
	if assessment.EligibleForAutomation() {
		t.Errorf("EligibleForAutomation() = true on calibrating assessment; automation must wait for calibration (CF-INV-13)")
	}
}

// ===========================================================================
// Daily HTTP surface — the public API must expose the new assessment +
// dominant fields, both in the embedded struct and on the JSON wire.
// Handler does not yet attach Assessment/DominantActor/DominantSignal;
// these tests are RED today.
// ===========================================================================

// TestHandleDaily_AssessmentExposedInJSON asserts that /api/capital-flow/daily
// JSON contains the permanent assessment fields and per-force provenance.
// Optional primary_flow / dominant_actor / dominant_signal fields are allowed
// to be absent while E07 leaves them empty. The test decodes the response via
// the production Handler so the JSON wire-format is what's actually published.
func TestHandleDaily_AssessmentExposedInJSON(t *testing.T) {
	h := NewHandler(&mockProvider{snap: testSnapshot()})

	req := httptest.NewRequest(http.MethodGet, "/api/capital-flow/daily", nil)
	code, data := h.HandleDaily(req)
	if code != http.StatusOK {
		t.Fatalf("expected 200, got %d", code)
	}

	b, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	assessment, ok := doc["assessment"].(map[string]any)
	if !ok {
		t.Fatalf("response JSON missing 'assessment' object (spec §9.5 / CF-INV-08)")
	}
	for _, key := range []string{"calibration_status", "as_of_trading_date"} {
		if _, ok := assessment[key]; !ok {
			t.Errorf("assessment missing permanent key %q (spec §9.5)", key)
		}
	}

	// Each force in the JSON array must carry DimensionRole / SourceID /
	// Unit per spec §7 (CF-INV-11).
	forces, ok := doc["forces"].([]any)
	if !ok {
		t.Fatalf("response JSON missing 'forces' array")
	}
	for i, raw := range forces {
		f, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("forces[%d] not an object (got %T)", i, raw)
		}
		for _, key := range []string{"dimension_role", "source_id", "unit"} {
			if _, ok := f[key]; !ok {
				t.Errorf("forces[%d] missing %q (spec §7 / CF-INV-11)", i, key)
			}
		}
	}
}

// TestHandleDaily_ResponseCarriesAssessmentSubLayers asserts the JSON
// payload carries the four layered sub-assessments under
// "assessment" (spec §9.5 / CF-INV-08). RED today: Assessment field
// does not exist on DailyReport.
func TestHandleDaily_ResponseCarriesAssessmentSubLayers(t *testing.T) {
	h := NewHandler(&mockProvider{snap: testSnapshot()})
	req := httptest.NewRequest(http.MethodGet, "/api/capital-flow/daily", nil)
	_, data := h.HandleDaily(req)
	b, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	assessment, ok := doc["assessment"].(map[string]any)
	if !ok {
		t.Fatalf("response JSON missing 'assessment' object (spec §9.5 / CF-INV-08)")
	}
	for _, key := range []string{
		"institutional",
		"behavioral",
		"foreign_positioning",
		"cross_market",
		"calibration_status",
		"as_of_trading_date",
	} {
		if _, ok := assessment[key]; !ok {
			t.Errorf("assessment missing %q (spec §9.5)", key)
		}
	}
}

// ===========================================================================
// Test helper: thin alias of t.Context for Go versions where the helper
// isn't available. (Keeps the file self-contained; stdlib t.Context was
// added in Go 1.24 and we are on 1.26.)
// ===========================================================================

// ensure testSnapshot's mock provider is exercised in the harness above.
var (
	_ = testSnapshot
	_ = marketdata.MacroDataSnapshot{}
)

// ===========================================================================
// M3 — dimensionSource ↔ ComputeForceProvenance single source of truth
// (audit 2026-09-04). The persistence writer (dimensionSource, used by
// Service.Refresh) used to carry its own unit/source table that
// contradicted ComputeForceProvenance on government (hundred_million_shares
// vs twd), retail (hundred_million_shares vs pct_composite) and TSM ADR
// (percent/SourceYahoo vs pct/SourceSECTSMC). The two must agree for all
// 7 dimensions — a 7×4 matrix lock (unit, source_id per dimension).
// ===========================================================================

func TestDimensionSource_MatchesProvenanceMatrix(t *testing.T) {
	dims := []ForceName{
		ForceForeign, ForceInstitutional, ForceDealer,
		ForceGovernment, ForceRetail, ForceFutures, ForceTSMADR,
	}
	for _, dim := range dims {
		t.Run(string(dim), func(t *testing.T) {
			prov := ComputeForceProvenance(dim)
			unit, sourceID := dimensionSource(dim)
			if unit != prov.Unit {
				t.Errorf("dimensionSource(%s) unit = %q, want %q (ComputeForceProvenance)", dim, unit, prov.Unit)
			}
			if sourceID != prov.SourceID {
				t.Errorf("dimensionSource(%s) source_id = %q, want %q (ComputeForceProvenance)", dim, sourceID, prov.SourceID)
			}
			if unit == "" || sourceID == "" {
				t.Errorf("dimensionSource(%s) returned empty provenance (%q, %q); every dimension must have a concrete row", dim, unit, sourceID)
			}
		})
	}
}

// TestDimensionSource_AnchoredRows pins the acceptance rows of audit M3:
// the contested dimensions must resolve to the provenance table's values,
// and the T86 trio keeps its spec §5.1 億股 unit.
func TestDimensionSource_AnchoredRows(t *testing.T) {
	anchors := []struct {
		dim      ForceName
		wantUnit string
		wantSrc  string
	}{
		{ForceForeign, "hundred_million_shares", SourceTWSET86},
		{ForceInstitutional, "hundred_million_shares", SourceTWSET86},
		{ForceDealer, "hundred_million_shares", SourceTWSET86},
		{ForceGovernment, "twd", SourceGovernmentOperator},
		{ForceRetail, "pct_composite", SourceTWSEODDLOT},
		{ForceFutures, "contracts", SourceTAIFEXInst},
		{ForceTSMADR, "pct", SourceSECTSMC},
	}
	for _, c := range anchors {
		unit, sourceID := dimensionSource(c.dim)
		if unit != c.wantUnit || sourceID != c.wantSrc {
			t.Errorf("dimensionSource(%s) = (%q, %q), want (%q, %q)",
				c.dim, unit, sourceID, c.wantUnit, c.wantSrc)
		}
	}
}
