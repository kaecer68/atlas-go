package config

import (
	"encoding/json"
	"fmt"
	"os"
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
	CalibrationSeverityError = "error"
)

// CalibrationFinding is one integrity finding with its class, severity, the
// segment it concerns (empty when the finding is file-wide) and the human message.
type CalibrationFinding struct {
	Code     CalibrationFindingCode `json:"code"`
	Severity string                 `json:"severity"`
	Segment  string                 `json:"segment,omitempty"`
	Message  string                 `json:"message"`
}

// addFinding records a finding and mirrors its message into the legacy Issues
// slice, keeping the JSON contract backward compatible.
func (r *CalibrationValidationResult) addFinding(code CalibrationFindingCode, segment, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	r.OK = false
	r.Issues = append(r.Issues, msg)
	r.Findings = append(r.Findings, CalibrationFinding{
		Code:     code,
		Severity: CalibrationSeverityError,
		Segment:  segment,
		Message:  msg,
	})
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
func ValidateCalibration(path string, maxAge time.Duration) (*CalibrationValidationResult, error) {
	res := &CalibrationValidationResult{OK: true}

	info, err := os.Stat(path)
	if err != nil {
		res.addFinding(CalibrationFindingParamsStatFailed, "", "params.json stat failed: %v", err)
		return res, nil
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
		return res, nil
	}

	var cfg ParametersConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		res.addFinding(CalibrationFindingParamsInvalidJSON, "", "params.json invalid JSON: %v", err)
		return res, nil
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
		return res, nil
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

	return res, nil
}
