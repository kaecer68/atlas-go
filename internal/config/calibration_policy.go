package config

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"
	"time"
)

// Calibration validation policy (issue #1944 Batch 4, item I31).
//
// The nightly workflow used to run cmd/calibration-validate inside `set +e` with
// a non-final `cat`, so the step could not fail; the Slack notification was
// skipped when SLACK_WEBHOOK_URL was unset. The verdict was therefore never
// visible, and the gate was simultaneously useless (it could not fail) and
// unusable (it would always fail if fixed naively, because the checked-out
// configs/parameters.json carries an updated_at that a CI runner cannot refresh,
// and seven segments carry no representatives by design).
//
// This policy file is the missing piece: it declares (a) the freshness scope of
// the run and (b) the findings that are known and accepted, each with a reason.
// Anything else fails the run. It is intentionally data, not code, so adding an
// accepted finding is a reviewable diff instead of a silent code change.

// CalibrationAcceptedFinding is one finding the policy accepts as known.
//
// Segment may be empty to accept the code for every segment (used for file-wide
// codes). Code must be one of CalibrationFindingCodes(); Reason must be a real
// explanation — an empty or TODO-prefixed reason is rejected, mirroring the
// reason-required rule of scripts/ci/inert-baseline.json.
type CalibrationAcceptedFinding struct {
	Code    CalibrationFindingCode `json:"code"`
	Segment string                 `json:"segment,omitempty"`
	Reason  string                 `json:"reason"`
}

// CalibrationValidationPolicy is the JSON schema of
// configs/calibration-validation-policy.json.
type CalibrationValidationPolicy struct {
	// Scope is "full" (default) or "structure".
	Scope string `json:"scope"`
	// Note is a free-text explanation surfaced in the CLI text output.
	Note string `json:"note,omitempty"`
	// Accepted lists findings that do not fail the run.
	Accepted []CalibrationAcceptedFinding `json:"accepted"`
}

// EffectiveScope returns Scope with the documented default applied.
func (p *CalibrationValidationPolicy) EffectiveScope() string {
	if p == nil {
		return CalibrationScopeFull
	}
	if strings.TrimSpace(p.Scope) == "" {
		return CalibrationScopeFull
	}
	return p.Scope
}

// RequiresFreshness reports whether a stale/never-recorded updated_at fails the
// run under this policy.
func (p *CalibrationValidationPolicy) RequiresFreshness() bool {
	return p.EffectiveScope() != CalibrationScopeStructure
}

// Accepts reports whether the policy explicitly accepts (code, segment).
func (p *CalibrationValidationPolicy) Accepts(code CalibrationFindingCode, segment string) bool {
	if p == nil {
		return false
	}
	for _, a := range p.Accepted {
		if a.Code != code {
			continue
		}
		if a.Segment == "" || a.Segment == segment {
			return true
		}
	}
	return false
}

// LoadCalibrationValidationPolicy reads and validates a policy file. It fails
// closed: an unknown scope, an unknown finding code, a missing reason or a
// TODO-prefixed reason is an error, so a typo cannot silence a real finding.
func LoadCalibrationValidationPolicy(path string) (*CalibrationValidationPolicy, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read calibration validation policy: %w", err)
	}
	var policy CalibrationValidationPolicy
	if err := json.Unmarshal(data, &policy); err != nil {
		return nil, fmt.Errorf("parse calibration validation policy %s: %w", path, err)
	}
	if err := policy.Validate(); err != nil {
		return nil, fmt.Errorf("invalid calibration validation policy %s: %w", path, err)
	}
	return &policy, nil
}

// Validate checks the policy against the closed sets this package owns.
func (p *CalibrationValidationPolicy) Validate() error {
	switch p.EffectiveScope() {
	case CalibrationScopeFull, CalibrationScopeStructure:
	default:
		return fmt.Errorf("unknown scope %q (want %q or %q)",
			p.Scope, CalibrationScopeFull, CalibrationScopeStructure)
	}
	for i, a := range p.Accepted {
		if a.Code == "" {
			return fmt.Errorf("accepted[%d]: code is required", i)
		}
		if !IsKnownCalibrationFindingCode(a.Code) {
			return fmt.Errorf("accepted[%d]: unknown finding code %q; known codes: %v",
				i, a.Code, CalibrationFindingCodes())
		}
		reason := strings.TrimSpace(a.Reason)
		if reason == "" {
			return fmt.Errorf("accepted[%d] (%s/%s): reason is required — an accepted finding without a reason is itself a defect",
				i, a.Code, a.Segment)
		}
		if strings.HasPrefix(strings.ToUpper(reason), "TODO") {
			return fmt.Errorf("accepted[%d] (%s/%s): reason must not start with TODO", i, a.Code, a.Segment)
		}
	}
	return nil
}

// AcceptedCodes returns the distinct codes the policy accepts, sorted. Used by
// tests and by the CLI header so the accepted set is visible in the output.
func (p *CalibrationValidationPolicy) AcceptedCodes() []CalibrationFindingCode {
	if p == nil {
		return nil
	}
	set := make(map[CalibrationFindingCode]bool, len(p.Accepted))
	for _, a := range p.Accepted {
		set[a.Code] = true
	}
	out := make([]CalibrationFindingCode, 0, len(set))
	for code := range set {
		out = append(out, code)
	}
	slices.Sort(out)
	return out
}

// CalibrationValidationOptions configures ValidateCalibrationWithOptions.
type CalibrationValidationOptions struct {
	// MaxAge bounds file freshness. Zero keeps the caller's default (48h).
	MaxAge time.Duration
	// Policy is the validation policy. Nil means: full freshness scope, no
	// accepted findings — i.e. any finding fails the run (legacy behaviour).
	Policy *CalibrationValidationPolicy
}
