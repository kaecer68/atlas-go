package capitalflow

// Verdict-change detection between two consecutive validation reports.
// Used by the scheduled cf_hypothesis_validation task to notice when a
// hypothesis's status changes — most importantly the "data unlock"
// transition INSUFFICIENT_DATA → PASS/PASS(improved)/FAIL once the
// sample crosses the pre-registered 252-trading-day threshold.
//
// Governance boundary: detection only informs (log + monitor alert).
// A verdict change NEVER flips automation eligibility, never writes
// config, and never substitutes for the human gate that reads the
// report before drafting the eligibility PR.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// VerdictChangeKind classifies a detected status transition.
type VerdictChangeKind string

const (
	// VerdictChangeDataUnlock: INSUFFICIENT_DATA → a real verdict
	// (PASS / PASS(improved) / FAIL). This is the expected "backfill
	// crossed the sample threshold" transition the auto-rerun exists
	// for.
	VerdictChangeDataUnlock VerdictChangeKind = "data_unlock"
	// VerdictChangeFlip: a judged status changed to a different
	// status (e.g. PASS → FAIL after new data, or a judged status
	// regressing to INSUFFICIENT_DATA). Rare; always worth eyes.
	VerdictChangeFlip VerdictChangeKind = "verdict_flip"
)

// VerdictChange is one hypothesis's status transition between two
// consecutive reports.
// No json tags by design: VerdictChange is consumed by logs and alert
// metadata only, never serialized to a frontend — keeping it tag-free
// keeps it out of the gentags generated field types.
type VerdictChange struct {
	HypothesisID string
	FromStatus   string
	ToStatus     string
	Kind         VerdictChangeKind
	SampleCount  int
}

// String renders the change in one human-readable line for logs.
func (c VerdictChange) String() string {
	return fmt.Sprintf("%s: %s -> %s (%s, n=%d)", c.HypothesisID, c.FromStatus, c.ToStatus, c.Kind, c.SampleCount)
}

func isJudgedStatus(status string) bool {
	switch status {
	case ValidationPass, ValidationPassImproved, ValidationFail:
		return true
	}
	return false
}

// DetectVerdictChanges diffs the previous report's hypothesis statuses
// against the current run. Statuses are matched by hypothesis ID; a
// hypothesis absent from prev (first run) yields no change so the very
// first scheduled run stays quiet. Order follows cur.
func DetectVerdictChanges(prev, cur []HypothesisResult) []VerdictChange {
	if len(cur) == 0 {
		return nil
	}
	prevByID := make(map[string]string, len(prev))
	for _, p := range prev {
		prevByID[p.ID] = p.Status
	}
	var out []VerdictChange
	for _, c := range cur {
		from, seen := prevByID[c.ID]
		if !seen || from == c.Status {
			continue
		}
		kind := VerdictChangeFlip
		if from == ValidationInsufficientData && isJudgedStatus(c.Status) {
			kind = VerdictChangeDataUnlock
		}
		out = append(out, VerdictChange{
			HypothesisID: c.ID,
			FromStatus:   from,
			ToStatus:     c.Status,
			Kind:         kind,
			SampleCount:  c.SampleCount,
		})
	}
	return out
}

// validationReportFilePrefix is the conventional report filename stem
// (see cmd/validate-capital-flow-hypotheses: cf-hypotheses-<date>.json).
const validationReportFilePrefix = "cf-hypotheses-"

// FindLatestValidationReport returns the most recent previously written
// validation report under reportsDir whose filename date is strictly
// before beforeDate (YYYY-MM-DD), so the diff always compares against
// the last run rather than today's own output. Returns found=false
// when no earlier report exists (first run). Malformed/unrelated
// filenames are skipped, not errors.
func FindLatestValidationReport(reportsDir, beforeDate string) (ValidationReport, string, bool, error) {
	entries, err := os.ReadDir(reportsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return ValidationReport{}, "", false, nil
		}
		return ValidationReport{}, "", false, fmt.Errorf("read reports dir: %w", err)
	}
	var latestDate string
	var latestPath string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasPrefix(name, validationReportFilePrefix) || !strings.HasSuffix(name, ".json") {
			continue
		}
		date := strings.TrimSuffix(strings.TrimPrefix(name, validationReportFilePrefix), ".json")
		if _, err := time.Parse("2006-01-02", date); err != nil {
			continue
		}
		if date >= beforeDate {
			continue
		}
		if date > latestDate {
			latestDate = date
			latestPath = filepath.Join(reportsDir, name)
		}
	}
	if latestPath == "" {
		return ValidationReport{}, "", false, nil
	}
	data, err := os.ReadFile(latestPath)
	if err != nil {
		return ValidationReport{}, "", false, fmt.Errorf("read previous report %s: %w", latestPath, err)
	}
	var report ValidationReport
	if err := json.Unmarshal(data, &report); err != nil {
		return ValidationReport{}, "", false, fmt.Errorf("parse previous report %s: %w", latestPath, err)
	}
	return report, latestPath, true, nil
}
