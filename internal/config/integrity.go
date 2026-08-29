package config

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// requiredParamsKeys lists the top-level keys that must be present in params.json.
// These correspond to the non-omitempty fields on ParametersConfig.
var requiredParamsKeys = []string{
	"version",
	"updated_at",
	"darwinian",
	"factor",
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
	"marketdata",
	"industry",
	"strategy",
	"precious_metals",
	"alert",
	"capitalflow",
	"sector_allocation",
	"reporting",
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
		res.OK = false
		res.Issues = append(res.Issues, fmt.Sprintf("params.json stat failed: %v", err))
		return res, nil
	}
	res.FileMTime = info.ModTime()
	res.StaleBy = time.Since(res.FileMTime) - maxAge
	if res.StaleBy > 0 {
		res.OK = false
		res.Issues = append(res.Issues, fmt.Sprintf("params.json mtime %s is stale by %s (threshold %s)",
			res.FileMTime.Format(time.RFC3339), res.StaleBy.Truncate(time.Minute), maxAge))
	}

	data, err := os.ReadFile(path)
	if err != nil {
		res.OK = false
		res.Issues = append(res.Issues, fmt.Sprintf("params.json read failed: %v", err))
		return res, nil
	}

	var cfg ParametersConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		res.OK = false
		res.Issues = append(res.Issues, fmt.Sprintf("params.json invalid JSON: %v", err))
		return res, nil
	}

	res.UpdatedAt = cfg.UpdatedAt
	if res.UpdatedAt.IsZero() {
		res.OK = false
		res.Issues = append(res.Issues, "params.json has zero updated_at — calibration never recorded")
	} else if time.Since(res.UpdatedAt) > maxAge {
		res.OK = false
		res.Issues = append(res.Issues, fmt.Sprintf("params.json updated_at %s is stale by %s (threshold %s)",
			res.UpdatedAt.Format(time.RFC3339), time.Since(res.UpdatedAt).Truncate(time.Minute), maxAge))
	}

	segments := cfg.Industry.ClassificationTree.Value.Segments
	res.SegmentsCount = len(segments)
	if len(segments) == 0 {
		res.OK = false
		res.Issues = append(res.Issues, "Industry.ClassificationTree.Value.Segments is empty")
		return res, nil
	}

	l1IDs := make(map[string]bool)
	for _, s := range segments {
		switch s.Level {
		case 1:
			res.L1Count++
			if s.ID == "" {
				res.OK = false
				res.Issues = append(res.Issues, "L1 segment has empty ID")
			}
			l1IDs[s.ID] = true
			if len(s.RepresentativeStocks) == 0 {
				res.OK = false
				res.Issues = append(res.Issues, fmt.Sprintf("L1 segment %q has no RepresentativeStocks", s.ID))
			}
		case 2:
			res.L2Count++
			if s.ParentID == "" {
				res.OK = false
				res.Issues = append(res.Issues, fmt.Sprintf("L2 segment %q has empty ParentID", s.ID))
			} else if !l1IDs[s.ParentID] {
				res.OK = false
				res.Issues = append(res.Issues, fmt.Sprintf("L2 segment %q references unknown ParentID %q", s.ID, s.ParentID))
			}
			if len(s.RepresentativeStocks) == 0 {
				res.OK = false
				res.Issues = append(res.Issues, fmt.Sprintf("L2 segment %q has no RepresentativeStocks", s.ID))
			}
		}
	}

	if res.L1Count == 0 {
		res.OK = false
		res.Issues = append(res.Issues, "no L1 (top-level) segments found")
	}

	return res, nil
}
