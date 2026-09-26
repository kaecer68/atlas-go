package config

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"time"
)

// requiredParamsKeys lists the top-level keys that must be present in params.json.
// These correspond to the non-omitempty fields on ParametersConfig.
// The ten struct-typed sub-config keys (factor_weight, narrative_conviction,
// sector_executor, risk_gate, engine, rsi_tw, tax, smart_universe,
// forward_return, stockpicker) are always serialized: omitempty is a no-op on
// struct fields, so removing the ineffective tags (Batch 1 of the omitzero
// cleanup) makes the list match runtime behavior.
var requiredParamsKeys = []string{
	"version",
	"updated_at",
	"darwinian",
	"factor",
	"factor_weight",
	"optimizer",
	"sizing",
	"health",
	"garch",
	"experiment",
	"baseline",
	"orchestrator",
	"risk",
	"drawdown",
	"realtime",
	"janus",
	"narrative",
	"narrative_conviction",
	"marketdata",
	"industry",
	"strategy",
	"precious_metals",
	"sector_executor",
	"alert",
	"capitalflow",
	"risk_gate",
	"engine",
	"rsi_tw",
	"tax",
	"sector_allocation",
	"reporting",
	"smart_universe",
	"forward_return",
	"stockpicker",
}

// CheckParamsIntegrity validates that the file at path is a non-empty, valid JSON
// object containing all required top-level keys for ParametersConfig.
// It returns a slice of errors; an empty (non-nil) slice means the file passed.
func CheckParamsIntegrity(path string) []error {
	data, err := os.ReadFile(path)
	if err != nil {
		return []error{fmt.Errorf("read params.json: %w", err)}
	}

	if len(data) == 0 {
		return []error{fmt.Errorf("params.json is empty")}
	}

	var obj map[string]json.RawMessage
	if err := json.Unmarshal(data, &obj); err != nil {
		return []error{fmt.Errorf("invalid JSON in params.json: %w", err)}
	}

	var errs []error
	for _, key := range requiredParamsKeys {
		raw, ok := obj[key]
		if !ok {
			errs = append(errs, fmt.Errorf("params.json missing required top-level key: %s", key))
			continue
		}

		var nested map[string]json.RawMessage
		if err := json.Unmarshal(raw, &nested); err != nil {
			// version and updated_at are scalar metadata fields; any JSON type is allowed.
			if key == "version" || key == "updated_at" {
				continue
			}

			var val any
			if json.Unmarshal(raw, &val) == nil {
				switch val.(type) {
				case string:
					errs = append(errs, fmt.Errorf("params.json key '%s' has type string, expected object", key))
				case float64:
					errs = append(errs, fmt.Errorf("params.json key '%s' has type number, expected object", key))
				case bool:
					errs = append(errs, fmt.Errorf("params.json key '%s' has type bool, expected object", key))
				case []any:
					errs = append(errs, fmt.Errorf("params.json key '%s' has type array, expected object", key))
				case nil:
					errs = append(errs, fmt.Errorf("params.json key '%s' has type null, expected object", key))
				default:
					errs = append(errs, fmt.Errorf("params.json key '%s' has type unknown, expected object", key))
				}
			} else {
				errs = append(errs, fmt.Errorf("params.json key '%s' has type unknown, expected object", key))
			}
			continue
		}

		if nested == nil {
			errs = append(errs, fmt.Errorf("params.json key '%s' is null", key))
			continue
		}

		if len(nested) == 0 {
			errs = append(errs, fmt.Errorf("params.json key '%s' is empty object", key))
		}
	}

	if errs == nil {
		return []error{}
	}
	return errs
}

// CalibrationValidationResult captures the outcome of a post-backfill calibration
// integrity check. Used by the nightly-refresh workflow to detect a half-applied
// run (TWSE proxy fetched but ClassificationTree not actually updated) and alert.
//
// OK is derived from the ERROR-severity findings only (see
// ValidateCalibrationWithOptions). Observation-severity findings are reported but
// do not fail the run, so a caller can distinguish "the tree regressed" from
// "the tree is as expected but this checkout cannot be fresh".
type CalibrationValidationResult struct {
	OK            bool
	Issues        []string
	SegmentsCount int
	L1Count       int
	L2Count       int
	UpdatedAt     time.Time
	FileMTime     time.Time
	StaleBy       time.Duration

	// Findings classifies every entry in Issues with a stable machine-readable
	// code and the affected segment, so callers can distinguish finding classes
	// without parsing message text (issue #1944 Batch 3, item I31). Issues is kept
	// for backward compatibility and always mirrors Findings' messages.
	Findings []CalibrationFinding

	// Scope is the freshness policy the run was evaluated under:
	// CalibrationScopeFull (default) or CalibrationScopeStructure. Under
	// CalibrationScopeStructure the freshness codes are reported as observations
	// because a fresh checkout structurally cannot carry a fresh updated_at.
	Scope string `json:"scope"`
	// FreshnessEnforced is true when a stale/never-recorded updated_at fails the
	// run. It is the machine-readable form of the freshness policy decision
	// (issue #1944 Batch 4, item I31).
	FreshnessEnforced bool `json:"freshness_enforced"`
	// ErrorCount / ObservationCount split Findings by severity. OK is true iff
	// ErrorCount == 0. Observations are accepted findings — either accepted
	// explicitly by the policy, or freshness findings on a structure-scope run.
	// They stay in Findings so an accepted defect is visible, never silent.
	ErrorCount       int `json:"error_count"`
	ObservationCount int `json:"observation_count"`

	// raw holds findings before severity classification. Unexported and never
	// serialized: it only exists so collection and policy evaluation can be
	// separate steps.
	raw []CalibrationFinding
}

// CalibrationFindingCode is a stable identifier for one integrity finding.
// Callers MUST switch on the code, never on the message text.
type CalibrationFindingCode string

// Calibration finding codes. Every code currently carries SeverityError: this
// batch classifies findings, it does not downgrade any of them. Two codes are
// known to fire on by-design configuration in this repository — see
// docs/reference/inert-registry.md (I31) for the evidence and the open decision:
//
//	L1_NO_REPRESENTATIVES — the five size/style/asset-class buckets
//	  (defensive, etf_rotation, high_dividend, small_cap, tech) have no equity
//	  representatives by design; internal/industry's canonical mapper reports them
//	  as unmapped (no canonical L1 translation).
//	L2_NO_REPRESENTATIVES — pcb and thermal declare their members in
//	  configs/sector_symbols.json (sectormap namespace sector_symbols), not in the
//	  parameters tree, so adding members here would also change
//	  cmd/backfill-industry-tree weights and per-segment FinMind fetches.
const (
	CalibrationFindingParamsStatFailed    CalibrationFindingCode = "PARAMS_STAT_FAILED"
	CalibrationFindingMTimeStale          CalibrationFindingCode = "MTIME_STALE"
	CalibrationFindingParamsReadFailed    CalibrationFindingCode = "PARAMS_READ_FAILED"
	CalibrationFindingParamsInvalidJSON   CalibrationFindingCode = "PARAMS_INVALID_JSON"
	CalibrationFindingUpdatedAtZero       CalibrationFindingCode = "UPDATED_AT_ZERO"
	CalibrationFindingUpdatedAtStale      CalibrationFindingCode = "UPDATED_AT_STALE"
	CalibrationFindingSegmentsEmpty       CalibrationFindingCode = "SEGMENTS_EMPTY"
	CalibrationFindingL1EmptyID           CalibrationFindingCode = "L1_EMPTY_ID"
	CalibrationFindingL1NoRepresentatives CalibrationFindingCode = "L1_NO_REPRESENTATIVES"
	CalibrationFindingL2EmptyParentID     CalibrationFindingCode = "L2_EMPTY_PARENT_ID"
	CalibrationFindingL2UnknownParentID   CalibrationFindingCode = "L2_UNKNOWN_PARENT_ID"
	CalibrationFindingL2NoRepresentatives CalibrationFindingCode = "L2_NO_REPRESENTATIVES"
	CalibrationFindingNoL1Segments        CalibrationFindingCode = "NO_L1_SEGMENTS"
)

// CalibrationFinding severity values.
const (
	// CalibrationSeverityError fails the run. Everything not explicitly
	// downgraded by a policy is an error — the default is fail-closed.
	CalibrationSeverityError = "error"
	// CalibrationSeverityObservation is reported but does not fail the run. It is
	// used for (a) freshness findings when the run is evaluated under
	// CalibrationScopeStructure, and (b) findings matched by an explicit accepted
	// entry of the validation policy (issue #1944 Batch 4, item I31).
	CalibrationSeverityObservation = "observation"
)

// Calibration freshness scopes.
const (
	// CalibrationScopeFull enforces freshness: a stale mtime or a stale/absent
	// updated_at is an error. This is the default and the legacy behavior.
	CalibrationScopeFull = "full"
	// CalibrationScopeStructure reports freshness as an observation and only
	// fails on structural findings. It exists because the CI checkout of
	// configs/parameters.json is shipped with the repository, so its updated_at
	// can never be within max-age — enforcing freshness there produced a run that
	// could not distinguish "config is stale" from "the nightly job is a no-op"
	// (issue #1944 I30/I31). Freshness must be checked where the file is actually
	// refreshed, i.e. on the production host.
	CalibrationScopeStructure = "structure"
)

// freshnessFindingCodes are the updated_at / mtime findings. They are the only
// codes downgraded to observations by CalibrationScopeStructure.
var freshnessFindingCodes = map[CalibrationFindingCode]bool{
	CalibrationFindingMTimeStale:     true,
	CalibrationFindingUpdatedAtStale: true,
	CalibrationFindingUpdatedAtZero:  true,
}

// IsFreshnessFinding reports whether code is a file-freshness finding (mtime or
// updated_at) rather than a structural defect of the classification tree.
func IsFreshnessFinding(code CalibrationFindingCode) bool {
	return freshnessFindingCodes[code]
}

// knownCalibrationFindingCodes is the closed set of codes ValidateCalibration can
// emit. A policy that names a code outside this set is rejected at load time, so
// a typo cannot silently accept a real finding.
var knownCalibrationFindingCodes = map[CalibrationFindingCode]bool{
	CalibrationFindingParamsStatFailed:    true,
	CalibrationFindingMTimeStale:          true,
	CalibrationFindingParamsReadFailed:    true,
	CalibrationFindingParamsInvalidJSON:   true,
	CalibrationFindingUpdatedAtZero:       true,
	CalibrationFindingUpdatedAtStale:      true,
	CalibrationFindingSegmentsEmpty:       true,
	CalibrationFindingL1EmptyID:           true,
	CalibrationFindingL1NoRepresentatives: true,
	CalibrationFindingL2EmptyParentID:     true,
	CalibrationFindingL2UnknownParentID:   true,
	CalibrationFindingL2NoRepresentatives: true,
	CalibrationFindingNoL1Segments:        true,
}

// CalibrationFindingCodes returns the closed set of known finding codes, sorted
// for deterministic output. Callers use it to validate policy files.
func CalibrationFindingCodes() []CalibrationFindingCode {
	out := make([]CalibrationFindingCode, 0, len(knownCalibrationFindingCodes))
	for code := range knownCalibrationFindingCodes {
		out = append(out, code)
	}
	slices.Sort(out)
	return out
}

// IsKnownCalibrationFindingCode reports whether code is a code that
// ValidateCalibration can actually emit.
func IsKnownCalibrationFindingCode(code CalibrationFindingCode) bool {
	return knownCalibrationFindingCodes[code]
}

// CalibrationFinding is one integrity finding with its class, severity, the
// segment it concerns (empty when the finding is file-wide) and the human message.
type CalibrationFinding struct {
	Code     CalibrationFindingCode `json:"code"`
	Severity string                 `json:"severity"`
	Segment  string                 `json:"segment,omitempty"`
	Message  string                 `json:"message"`
}

// addFinding records a raw finding. Severity and OK are decided afterwards by
// finalizeFindings, because whether a finding fails the run depends on the
// validation policy (issue #1944 Batch 4, item I31).
func (r *CalibrationValidationResult) addFinding(code CalibrationFindingCode, segment, format string, args ...any) {
	r.raw = append(r.raw, CalibrationFinding{
		Code:    code,
		Segment: segment,
		Message: fmt.Sprintf(format, args...),
	})
}

// finalizeFindings classifies the raw findings under the given policy and fills
// the outward slices. It mirrors every message into Issues in collection order,
// keeping the legacy JSON contract byte-compatible for the default (no policy)
// path.
func (r *CalibrationValidationResult) finalizeFindings(policy *CalibrationValidationPolicy) {
	r.Scope = CalibrationScopeFull
	if policy != nil {
		r.Scope = policy.EffectiveScope()
	}
	r.FreshnessEnforced = r.Scope != CalibrationScopeStructure
	r.OK = true
	for _, f := range r.raw {
		switch {
		case !r.FreshnessEnforced && IsFreshnessFinding(f.Code):
			// Freshness is out of scope for this run: still reported, never silent.
			f.Severity = CalibrationSeverityObservation
		case policy != nil && policy.Accepts(f.Code, f.Segment):
			f.Severity = CalibrationSeverityObservation
		default:
			f.Severity = CalibrationSeverityError
		}
		if f.Severity == CalibrationSeverityError {
			r.ErrorCount++
			r.OK = false
		} else {
			r.ObservationCount++
		}
		r.Issues = append(r.Issues, f.Message)
		r.Findings = append(r.Findings, f)
	}
	r.raw = nil
}

// ValidateCalibration checks that params.json at path reflects a fresh, structurally
// valid industry classification tree calibration. It is designed to be called after
// cmd/backfill-industry-tree runs, to detect the failure mode where the backfill
// process exited early (TWSE fetch failed or save was skipped) without producing
// a usable classification tree.
//
// maxAge bounds file freshness: if the file's mtime is older than now-maxAge,
// ValidateCalibration reports OK=false with a "file is stale" issue. A typical
// nightly workflow passes maxAge=48h to absorb weekend gaps.
//
// This is the fail-closed entry point (full freshness scope, no accepted
// findings): every finding is an error. Use ValidateCalibrationWithOptions when
// a validation policy is in play.
func ValidateCalibration(path string, maxAge time.Duration) (*CalibrationValidationResult, error) {
	return ValidateCalibrationWithOptions(path, CalibrationValidationOptions{MaxAge: maxAge})
}

// DefaultCalibrationMaxAge mirrors the historical 48h default and absorbs
// weekend gaps.
//
// It is exported because it is now a **cross-package policy value**, not an
// internal default: `cmd/calibration-validate` uses it as the `--max-age` flag
// default, and `internal/monitoring` uses it as the alerting contract
// (`CalibrationFreshnessContract`). Three independent copies of "48 * time.Hour"
// would let the CLI verdict and the monitoring verdict drift apart — exactly the
// contradiction (CLI says stale, monitoring says fresh) this contract exists to
// prevent.
const DefaultCalibrationMaxAge = 48 * time.Hour

// ValidateCalibrationWithOptions is ValidateCalibration plus a validation policy
// (issue #1944 Batch 4, item I31).
//
// The policy decides two things: whether freshness is part of this run's scope,
// and which known findings are accepted. Everything not accepted still fails the
// run, so the gate keeps its teeth while a checkout that structurally cannot be
// fresh no longer produces a verdict nobody can act on. Accepted findings stay in
// Findings with severity "observation" — accepted is visible, never silent.
func ValidateCalibrationWithOptions(path string, opts CalibrationValidationOptions) (*CalibrationValidationResult, error) {
	maxAge := opts.MaxAge
	if maxAge <= 0 {
		maxAge = DefaultCalibrationMaxAge
	}
	if opts.Policy != nil {
		if err := opts.Policy.Validate(); err != nil {
			return nil, fmt.Errorf("calibration validation policy: %w", err)
		}
	}
	res := collectCalibrationFindings(path, maxAge)
	res.finalizeFindings(opts.Policy)
	return res, nil
}

// collectCalibrationFindings runs every check and records raw findings. It does
// not decide severities or OK — that is finalizeFindings' job — so the same
// checks serve both the fail-closed and the policy-scoped entry points.
func collectCalibrationFindings(path string, maxAge time.Duration) *CalibrationValidationResult {
	res := &CalibrationValidationResult{OK: true}

	info, err := os.Stat(path)
	if err != nil {
		res.addFinding(CalibrationFindingParamsStatFailed, "", "params.json stat failed: %v", err)
		return res
	}
	res.FileMTime = info.ModTime()
	res.StaleBy = time.Since(res.FileMTime) - maxAge
	if res.StaleBy > 0 {
		res.addFinding(CalibrationFindingMTimeStale, "", "params.json mtime %s is stale by %s (threshold %s)",
			res.FileMTime.Format(time.RFC3339), res.StaleBy.Truncate(time.Minute), maxAge)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		res.addFinding(CalibrationFindingParamsReadFailed, "", "params.json read failed: %v", err)
		return res
	}

	var cfg ParametersConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		res.addFinding(CalibrationFindingParamsInvalidJSON, "", "params.json invalid JSON: %v", err)
		return res
	}

	res.UpdatedAt = cfg.UpdatedAt
	if res.UpdatedAt.IsZero() {
		res.addFinding(CalibrationFindingUpdatedAtZero, "", "params.json has zero updated_at — calibration never recorded")
	} else if time.Since(res.UpdatedAt) > maxAge {
		res.addFinding(CalibrationFindingUpdatedAtStale, "", "params.json updated_at %s is stale by %s (threshold %s)",
			res.UpdatedAt.Format(time.RFC3339), time.Since(res.UpdatedAt).Truncate(time.Minute), maxAge)
	}

	segments := cfg.Industry.ClassificationTree.Value.Segments
	res.SegmentsCount = len(segments)
	if len(segments) == 0 {
		res.addFinding(CalibrationFindingSegmentsEmpty, "", "Industry.ClassificationTree.Value.Segments is empty")
		return res
	}

	l1IDs := make(map[string]bool)
	for _, s := range segments {
		switch s.Level {
		case 1:
			res.L1Count++
			if s.ID == "" {
				res.addFinding(CalibrationFindingL1EmptyID, s.Name, "L1 segment has empty ID")
			}
			l1IDs[s.ID] = true
			if len(s.RepresentativeStocks) == 0 {
				res.addFinding(CalibrationFindingL1NoRepresentatives, s.ID, "L1 segment %q has no RepresentativeStocks", s.ID)
			}
		case 2:
			res.L2Count++
			if s.ParentID == "" {
				res.addFinding(CalibrationFindingL2EmptyParentID, s.ID, "L2 segment %q has empty ParentID", s.ID)
			} else if !l1IDs[s.ParentID] {
				res.addFinding(CalibrationFindingL2UnknownParentID, s.ID, "L2 segment %q references unknown ParentID %q", s.ID, s.ParentID)
			}
			if len(s.RepresentativeStocks) == 0 {
				res.addFinding(CalibrationFindingL2NoRepresentatives, s.ID, "L2 segment %q has no RepresentativeStocks", s.ID)
			}
		}
	}

	if res.L1Count == 0 {
		res.addFinding(CalibrationFindingNoL1Segments, "", "no L1 (top-level) segments found")
	}

	return res
}
