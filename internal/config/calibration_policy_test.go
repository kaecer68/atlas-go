package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

// Issue #1944 Batch 4, item I31. The nightly job could not fail (set +e plus a
// non-final cat) and could not be fixed naively (the checked-out parameters.json
// is structurally never fresh, and seven segments carry no representatives by
// design). These tests pin the resulting contract: a policy file declares the
// freshness scope and the accepted findings, everything else fails, and the
// legacy fail-closed entry point is unchanged.

func repoPath(parts ...string) string {
	return filepath.Join(append([]string{"..", ".."}, parts...)...)
}

func shippedPolicy(t *testing.T) *CalibrationValidationPolicy {
	t.Helper()
	policy, err := LoadCalibrationValidationPolicy(repoPath("configs", "calibration-validation-policy.json"))
	if err != nil {
		t.Fatalf("LoadCalibrationValidationPolicy(shipped): %v", err)
	}
	return policy
}

// TestValidateCalibrationWithOptions_NoPolicyMatchesLegacy fail-closes the default:
// without a policy the new entry point must produce exactly the legacy verdict, so
// nobody can widen the gate by forgetting to pass a policy.
func TestValidateCalibrationWithOptions_NoPolicyMatchesLegacy(t *testing.T) {
	path := repoPath("configs", "parameters.json")

	legacy, err := ValidateCalibration(path, 48*time.Hour)
	if err != nil {
		t.Fatalf("ValidateCalibration: %v", err)
	}
	scoped, err := ValidateCalibrationWithOptions(path, CalibrationValidationOptions{MaxAge: 48 * time.Hour})
	if err != nil {
		t.Fatalf("ValidateCalibrationWithOptions: %v", err)
	}
	if scoped.OK != legacy.OK {
		t.Fatalf("OK differs: scoped=%v legacy=%v", scoped.OK, legacy.OK)
	}
	if scoped.Scope != CalibrationScopeFull || !scoped.FreshnessEnforced {
		t.Fatalf("default scope = %q freshness_enforced=%v, want %q/true",
			scoped.Scope, scoped.FreshnessEnforced, CalibrationScopeFull)
	}
	if scoped.ObservationCount != 0 {
		t.Fatalf("default must not downgrade anything, got %d observations", scoped.ObservationCount)
	}
	if len(scoped.Findings) != len(legacy.Findings) {
		t.Fatalf("finding count changed: scoped=%d legacy=%d", len(scoped.Findings), len(legacy.Findings))
	}
	for i := range scoped.Findings {
		if scoped.Findings[i].Severity != CalibrationSeverityError {
			t.Fatalf("finding %d severity = %q, want %q",
				i, scoped.Findings[i].Severity, CalibrationSeverityError)
		}
	}
	if len(scoped.Issues) != len(scoped.Findings) {
		t.Fatalf("Issues (%d) must mirror Findings (%d)", len(scoped.Issues), len(scoped.Findings))
	}
	for i, f := range scoped.Findings {
		if scoped.Issues[i] != f.Message {
			t.Fatalf("Issues[%d] = %q, want message %q", i, scoped.Issues[i], f.Message)
		}
	}
	if scoped.ErrorCount != len(scoped.Findings) {
		t.Fatalf("error_count = %d, want %d", scoped.ErrorCount, len(scoped.Findings))
	}
}

// TestShippedConfigUnderShippedPolicyPassesStructureOnly is the alerting contract
// the workflow depends on: the shipped checkout passes under the shipped policy,
// and the eight known findings are still reported (accepted is visible, never
// silent).
func TestShippedConfigUnderShippedPolicyPassesStructureOnly(t *testing.T) {
	res, err := ValidateCalibrationWithOptions(repoPath("configs", "parameters.json"),
		CalibrationValidationOptions{MaxAge: 48 * time.Hour, Policy: shippedPolicy(t)})
	if err != nil {
		t.Fatalf("ValidateCalibrationWithOptions: %v", err)
	}
	if !res.OK {
		t.Fatalf("shipped config + shipped policy must pass (structure scope); errors=%d findings=%v",
			res.ErrorCount, res.Findings)
	}
	if res.Scope != CalibrationScopeStructure || res.FreshnessEnforced {
		t.Fatalf("scope = %q freshness_enforced=%v, want %q/false",
			res.Scope, res.FreshnessEnforced, CalibrationScopeStructure)
	}
	if res.ErrorCount != 0 {
		t.Fatalf("error_count = %d, want 0", res.ErrorCount)
	}
	// 1 freshness observation + 7 accepted by-design findings.
	if res.ObservationCount != 8 || len(res.Findings) != 8 {
		t.Fatalf("observations = %d findings = %d, want 8/8", res.ObservationCount, len(res.Findings))
	}

	var sawFreshness bool
	var sawAccepted int
	for _, f := range res.Findings {
		if f.Severity != CalibrationSeverityObservation {
			t.Fatalf("finding %s/%s severity = %q, want %q", f.Code, f.Segment, f.Severity, CalibrationSeverityObservation)
		}
		switch f.Code {
		case CalibrationFindingUpdatedAtStale:
			sawFreshness = true
		case CalibrationFindingL1NoRepresentatives, CalibrationFindingL2NoRepresentatives:
			sawAccepted++
		}
	}
	if !sawFreshness {
		t.Fatalf("freshness finding must still be reported as an observation; got %v", res.Findings)
	}
	if sawAccepted != 7 {
		t.Fatalf("accepted by-design findings = %d, want 7", sawAccepted)
	}
}

// TestShippedPolicyAcceptsOnlyByDesignFindings is an exact-set assertion: adding
// representatives to one of these buckets (or renaming a segment) must fail here
// so the policy and docs are updated in the same commit instead of drifting into
// "accepted forever".
func TestShippedPolicyAcceptsOnlyByDesignFindings(t *testing.T) {
	policy := shippedPolicy(t)
	got := make([]string, 0, len(policy.Accepted))
	for _, a := range policy.Accepted {
		got = append(got, string(a.Code)+"/"+a.Segment)
	}
	want := []string{
		"L1_NO_REPRESENTATIVES/etf_rotation",
		"L1_NO_REPRESENTATIVES/defensive",
		"L1_NO_REPRESENTATIVES/high_dividend",
		"L1_NO_REPRESENTATIVES/small_cap",
		"L1_NO_REPRESENTATIVES/tech",
		"L2_NO_REPRESENTATIVES/pcb",
		"L2_NO_REPRESENTATIVES/thermal",
	}
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("shipped policy accepted set changed:\n got %v\nwant %v", got, want)
	}

	// Freshness must never be accepted: it is policy-scope, not a by-design defect.
	for _, a := range policy.Accepted {
		if IsFreshnessFinding(a.Code) {
			t.Fatalf("policy must not hard-accept the freshness finding %s (segment %q)", a.Code, a.Segment)
		}
	}
}

// TestValidateCalibrationWithOptions_NewStructuralFindingFails proves the gate has
// teeth: a structural regression on a segment the policy does not accept still
// fails the run, and it is flagged at ERROR severity so the workflow exits 1.
func TestValidateCalibrationWithOptions_NewStructuralFindingFails(t *testing.T) {
	raw, err := os.ReadFile(repoPath("configs", "parameters.json"))
	if err != nil {
		t.Fatalf("read shipped params: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("unmarshal shipped params: %v", err)
	}
	segments := doc["industry"].(map[string]any)["classification_tree"].(map[string]any)["value"].(map[string]any)["segments"].([]any)
	cleared := ""
	for _, rawSeg := range segments {
		seg, ok := rawSeg.(map[string]any)
		if !ok {
			continue
		}
		if id, _ := seg["id"].(string); id == "semiconductor" {
			delete(seg, "representative_stocks")
			cleared = id
		}
	}
	if cleared == "" {
		t.Fatalf("fixture drift: L1 segment semiconductor not found in shipped params")
	}
	mutated, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal mutated params: %v", err)
	}
	path := filepath.Join(t.TempDir(), "parameters.json")
	if err := os.WriteFile(path, mutated, 0o600); err != nil {
		t.Fatalf("write mutated params: %v", err)
	}

	res, err := ValidateCalibrationWithOptions(path, CalibrationValidationOptions{
		MaxAge: 48 * time.Hour,
		Policy: shippedPolicy(t),
	})
	if err != nil {
		t.Fatalf("ValidateCalibrationWithOptions: %v", err)
	}
	if res.OK {
		t.Fatalf("a new structural finding must fail the run; findings=%v", res.Findings)
	}
	if res.ErrorCount != 1 {
		t.Fatalf("error_count = %d, want 1 (only the un-accepted semiconductor segment)", res.ErrorCount)
	}
	var found bool
	for _, f := range res.Findings {
		if f.Code == CalibrationFindingL1NoRepresentatives && f.Segment == "semiconductor" {
			if f.Severity != CalibrationSeverityError {
				t.Fatalf("severity = %q, want %q", f.Severity, CalibrationSeverityError)
			}
			found = true
		}
	}
	if !found {
		t.Fatalf("missing L1_NO_REPRESENTATIVES for semiconductor; got %v", res.Findings)
	}
}

// TestValidateCalibrationWithOptions_FullScopePolicyEnforcesFreshness pins the
// other half of the decision: on the host that actually refreshes the file,
// freshness is enforced (the same policy file with scope=full).
func TestValidateCalibrationWithOptions_FullScopePolicyEnforcesFreshness(t *testing.T) {
	policy := shippedPolicy(t)
	policy.Scope = CalibrationScopeFull

	res, err := ValidateCalibrationWithOptions(repoPath("configs", "parameters.json"),
		CalibrationValidationOptions{MaxAge: 48 * time.Hour, Policy: policy})
	if err != nil {
		t.Fatalf("ValidateCalibrationWithOptions: %v", err)
	}
	if res.OK {
		t.Fatalf("stale updated_at must fail under scope=full")
	}
	if !res.FreshnessEnforced {
		t.Fatalf("freshness_enforced = false, want true under scope=full")
	}
	var found bool
	for _, f := range res.Findings {
		if f.Code == CalibrationFindingUpdatedAtStale {
			if f.Severity != CalibrationSeverityError {
				t.Fatalf("freshness severity = %q, want %q", f.Severity, CalibrationSeverityError)
			}
			found = true
		}
	}
	if !found {
		t.Fatalf("missing UPDATED_AT_STALE; got %v", res.Findings)
	}
}

// TestValidateCalibrationWithOptions_ZeroMaxAgeUsesDefault guards the documented
// default: a zero MaxAge must not silently degrade into "everything is stale".
func TestValidateCalibrationWithOptions_ZeroMaxAgeUsesDefault(t *testing.T) {
	now := time.Now()
	path := writeIntegrityParams(t, integrityParamsJSON(t, now,
		[]map[string]any{segmentJSON("electronics", "", 1, []string{"2317.TW"})}))

	res, err := ValidateCalibrationWithOptions(path, CalibrationValidationOptions{})
	if err != nil {
		t.Fatalf("ValidateCalibrationWithOptions: %v", err)
	}
	if !res.OK {
		t.Fatalf("zero MaxAge must fall back to the 48h default, got findings %v", res.Findings)
	}
}

func TestCalibrationValidationPolicy_ValidateFailsClosed(t *testing.T) {
	cases := []struct {
		name    string
		policy  CalibrationValidationPolicy
		wantErr bool
	}{
		{name: "default_scope_is_full_and_valid", policy: CalibrationValidationPolicy{}},
		{name: "structure_scope_valid", policy: CalibrationValidationPolicy{Scope: CalibrationScopeStructure}},
		{name: "unknown_scope_rejected", policy: CalibrationValidationPolicy{Scope: "yolo"}, wantErr: true},
		{name: "unknown_code_rejected", policy: CalibrationValidationPolicy{Accepted: []CalibrationAcceptedFinding{
			{Code: "NOT_A_CODE", Reason: "looks plausible"},
		}}, wantErr: true},
		{name: "empty_code_rejected", policy: CalibrationValidationPolicy{Accepted: []CalibrationAcceptedFinding{
			{Reason: "no code"},
		}}, wantErr: true},
		{name: "missing_reason_rejected", policy: CalibrationValidationPolicy{Accepted: []CalibrationAcceptedFinding{
			{Code: CalibrationFindingNoL1Segments},
		}}, wantErr: true},
		{name: "todo_reason_rejected", policy: CalibrationValidationPolicy{Accepted: []CalibrationAcceptedFinding{
			{Code: CalibrationFindingNoL1Segments, Reason: "TODO explain later"},
		}}, wantErr: true},
		{name: "whitespace_reason_rejected", policy: CalibrationValidationPolicy{Accepted: []CalibrationAcceptedFinding{
			{Code: CalibrationFindingNoL1Segments, Reason: "   "},
		}}, wantErr: true},
		{name: "documented_reason_accepted", policy: CalibrationValidationPolicy{
			Scope: CalibrationScopeStructure,
			Accepted: []CalibrationAcceptedFinding{
				{Code: CalibrationFindingNoL1Segments, Reason: "documented by-design bucket"},
			},
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.policy.Validate()
			if tc.wantErr && err == nil {
				t.Fatalf("Validate() = nil, want error")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("Validate() = %v, want nil", err)
			}
		})
	}
}

func TestLoadCalibrationValidationPolicy_RejectsUnknownFileAndJSON(t *testing.T) {
	if _, err := LoadCalibrationValidationPolicy(filepath.Join(t.TempDir(), "absent.json")); err == nil {
		t.Fatalf("missing policy file must be an error")
	}
	bad := filepath.Join(t.TempDir(), "bad.json")
	if err := os.WriteFile(bad, []byte("{not json"), 0o600); err != nil {
		t.Fatalf("write bad policy: %v", err)
	}
	if _, err := LoadCalibrationValidationPolicy(bad); err == nil {
		t.Fatalf("invalid JSON policy must be an error")
	}
}

func TestCalibrationFindingCodes_AreClosedAndSorted(t *testing.T) {
	codes := CalibrationFindingCodes()
	if len(codes) != 13 {
		t.Fatalf("known finding codes = %d, want 13 (adding a code must update the policy contract)", len(codes))
	}
	if !slices.IsSorted(codes) {
		t.Fatalf("codes must be sorted for deterministic output, got %v", codes)
	}
	if !IsKnownCalibrationFindingCode(CalibrationFindingUpdatedAtStale) {
		t.Fatalf("UPDATED_AT_STALE must be a known code")
	}
	if IsKnownCalibrationFindingCode("NOPE") {
		t.Fatalf("unknown code must not be reported as known")
	}
}

func TestIsFreshnessFinding(t *testing.T) {
	for _, code := range []CalibrationFindingCode{
		CalibrationFindingMTimeStale,
		CalibrationFindingUpdatedAtStale,
		CalibrationFindingUpdatedAtZero,
	} {
		if !IsFreshnessFinding(code) {
			t.Fatalf("%s must be classified as a freshness finding", code)
		}
	}
	for _, code := range []CalibrationFindingCode{
		CalibrationFindingL1NoRepresentatives,
		CalibrationFindingL2NoRepresentatives,
		CalibrationFindingSegmentsEmpty,
		CalibrationFindingNoL1Segments,
	} {
		if IsFreshnessFinding(code) {
			t.Fatalf("%s must NOT be classified as a freshness finding", code)
		}
	}
}

func TestCalibrationValidationResult_JSONContract(t *testing.T) {
	res, err := ValidateCalibrationWithOptions(repoPath("configs", "parameters.json"),
		CalibrationValidationOptions{MaxAge: 48 * time.Hour, Policy: shippedPolicy(t)})
	if err != nil {
		t.Fatalf("ValidateCalibrationWithOptions: %v", err)
	}
	raw, err := json.Marshal(res)
	if err != nil {
		t.Fatalf("marshal result: %v", err)
	}
	var decoded map[string]json.RawMessage
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	for _, key := range []string{"OK", "Issues", "Findings", "scope", "freshness_enforced", "error_count", "observation_count"} {
		if _, ok := decoded[key]; !ok {
			t.Fatalf("missing json key %q in %s", key, raw)
		}
	}
	var findings []map[string]any
	if err := json.Unmarshal(decoded["Findings"], &findings); err != nil {
		t.Fatalf("unmarshal findings: %v", err)
	}
	if len(findings) == 0 {
		t.Fatalf("expected findings")
	}
	for _, f := range findings {
		if _, ok := f["severity"]; !ok {
			t.Fatalf("finding missing severity: %v", f)
		}
	}
}
