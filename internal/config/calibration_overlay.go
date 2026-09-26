package config

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/kaecer68/atlas-go/internal/constants"
	"github.com/kaecer68/atlas-go/internal/logging"
)

// Calibrated-parameters overlay (FU-20260926-07).
//
// configs/parameters.json is the SSOT: it is baked into the image, reviewed in
// git, and it is the file a human edits. The self-calibration loops, however,
// adapt a few tunables at runtime. Writing those adaptations back into
// configs/parameters.json was wrong twice over:
//
//   - configs/ is not bind-mounted (only data/, reports/ and logs/ are), so the
//     write landed in the container's writable layer: invisible to git, and gone
//     on the next container recreate;
//   - while the container ran, nothing could tell the reviewed SSOT value apart
//     from an adapted one — production showed risk.max_daily_loss_pct 0.0108
//     where the repository says 0.03, with no trace of who changed what.
//
// This file implements the replacement contract:
//
//  1. calibration loops write an overlay document under the bind-mounted
//     data/ tree (constants.StateParametersCalibrated);
//  2. configs/parameters.json is left untouched, so "what the charter says"
//     and "what the process runs on" can always be diffed;
//  3. the overlay is layered on top of the SSOT at load time, and every applied
//     entry is logged with both values, so the effective value is never silent;
//  4. an entry whose SSOT value has moved since it was reconciled is dropped
//     with a warning: a reviewed charter edit always wins over a stale runtime
//     adaptation.
//
// The overlay is opt-in per process (SetCalibratedOverlayPath). With no path
// registered the loader behaves exactly as before, so tests and one-shot tools
// can never pick up a stray file.

// calibrationOverlayVersion is the schema version of the overlay document.
const calibrationOverlayVersion = "1"

// CalibrationOverlayEntry is one adapted tunable.
type CalibrationOverlayEntry struct {
	// Value is the calibrated (effective) value.
	Value float64 `json:"value"`
	// Before is the effective value this adaptation replaced.
	Before float64 `json:"before"`
	// SSOT is the configs/parameters.json value this entry was last reconciled
	// against (0 = not reconciled yet, i.e. written by a process that has not
	// restarted since). It is what makes a later charter edit detectable.
	SSOT float64 `json:"ssot"`
	// CalibratedAt is when the adaptation was computed.
	CalibratedAt time.Time `json:"calibrated_at"`
	// Method names the calibration routine that produced the value
	// (e.g. "bayesian_optimization").
	Method string `json:"method,omitempty"`
	// Rationale is the human-readable reason recorded by the calibration run.
	Rationale string `json:"rationale,omitempty"`
}

// CalibrationOverlay is the on-disk document at
// constants.StateParametersCalibrated.
type CalibrationOverlay struct {
	Version   string                             `json:"version"`
	UpdatedAt time.Time                          `json:"updated_at"`
	Source    string                             `json:"source,omitempty"`
	Entries   map[string]CalibrationOverlayEntry `json:"entries"`
}

// calibratedOverlayPath is the process-wide overlay path. Empty disables the
// overlay (default), which keeps every non-production caller on pure SSOT
// semantics.
var calibratedOverlayPath string

// SetCalibratedOverlayPath registers the overlay path for this process. An
// empty path disables the overlay. Call it before the first parameters load.
func SetCalibratedOverlayPath(path string) {
	calibratedOverlayPath = path
}

// GetCalibratedOverlayPath returns the registered overlay path ("" = disabled).
func GetCalibratedOverlayPath() string {
	return calibratedOverlayPath
}

// CalibrationOverlayPath returns the overlay path for a work dir.
func CalibrationOverlayPath(workDir string) string {
	return filepath.Join(workDir, filepath.FromSlash(constants.StateParametersCalibrated))
}

// LoadCalibrationOverlay reads the overlay document. A missing file is not an
// error: it returns (nil, nil), meaning "no adaptations".
func LoadCalibrationOverlay(path string) (*CalibrationOverlay, error) {
	if path == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read calibration overlay: %w", err)
	}
	var ov CalibrationOverlay
	if err := json.Unmarshal(data, &ov); err != nil {
		return nil, fmt.Errorf("parse calibration overlay %s: %w", path, err)
	}
	if ov.Entries == nil {
		ov.Entries = map[string]CalibrationOverlayEntry{}
	}
	return &ov, nil
}

// SaveCalibrationOverlay writes the document atomically (tmp → rename).
func SaveCalibrationOverlay(path string, ov *CalibrationOverlay) error {
	if path == "" {
		return fmt.Errorf("calibration overlay path is empty")
	}
	if ov.Entries == nil {
		ov.Entries = map[string]CalibrationOverlayEntry{}
	}
	ov.Version = calibrationOverlayVersion
	if ov.UpdatedAt.IsZero() {
		ov.UpdatedAt = time.Now()
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create calibration overlay dir: %w", err)
	}
	if err := writeJSONFile(path, ov); err != nil {
		return fmt.Errorf("write calibration overlay %s: %w", path, err)
	}
	return nil
}

// UpdateCalibrationOverlay merges entries into the overlay at path (creating the
// document when absent) and writes it back.
//
// The merge is deliberate: a calibration round reports only the tunables it
// actually changed, so a plain overwrite would silently drop the entries a
// previous round wrote and revert those parameters to the SSOT value. An entry
// that already exists keeps its reconciliation baseline unless the caller
// supplies a fresh one.
func UpdateCalibrationOverlay(path, source string, entries map[string]CalibrationOverlayEntry) (*CalibrationOverlay, error) {
	if path == "" {
		return nil, fmt.Errorf("calibration overlay path is empty")
	}
	ov, err := LoadCalibrationOverlay(path)
	if err != nil {
		return nil, err
	}
	if ov == nil {
		ov = &CalibrationOverlay{Entries: map[string]CalibrationOverlayEntry{}}
	}
	for name, entry := range entries {
		if prev, ok := ov.Entries[name]; ok && entry.SSOT == 0 {
			entry.SSOT = prev.SSOT
		}
		ov.Entries[name] = entry
	}
	if source != "" {
		ov.Source = source
	}
	ov.UpdatedAt = time.Now()
	if err := SaveCalibrationOverlay(path, ov); err != nil {
		return nil, err
	}
	return ov, nil
}

// SSOTParameterValue returns a parameter's value as written in the SSOT file,
// ignoring the overlay. It re-reads the file so callers get the on-disk truth
// rather than this process's live (possibly overlaid) value, which is exactly
// what an overlay entry needs as its reconciliation baseline.
func SSOTParameterValue(name string) (float64, bool) {
	accessor, ok := parameterTable[name]
	if !ok {
		return 0, false
	}
	path := GetParametersConfigPath()
	if path == "" {
		return 0, false
	}
	cfg, err := LoadParametersConfig(path)
	if err != nil || cfg == nil {
		return 0, false
	}
	return accessor.get(cfg), true
}

// CalibrationOverlayDiff is one applied adaptation, carrying both the SSOT and
// the effective value so the caller can report the drift it just layered in.
type CalibrationOverlayDiff struct {
	Name         string    `json:"name"`
	SSOT         float64   `json:"ssot"`
	Effective    float64   `json:"effective"`
	Before       float64   `json:"before"`
	Ratio        float64   `json:"ratio"`
	CalibratedAt time.Time `json:"calibrated_at"`
	Method       string    `json:"method,omitempty"`
}

// CalibrationOverlayReport summarizes one application of the overlay.
type CalibrationOverlayReport struct {
	Path        string
	Applied     []CalibrationOverlayDiff
	Unknown     []string // entries naming a parameter the parameter table does not know
	Invalidated []string // entries dropped because the SSOT value moved
	Reconciled  bool     // the on-disk overlay was rewritten
	Err         error
}

// AppliedNames returns the names of the parameters the overlay changed.
func (r CalibrationOverlayReport) AppliedNames() []string {
	names := make([]string, 0, len(r.Applied))
	for _, d := range r.Applied {
		names = append(names, d.Name)
	}
	return names
}

// ApplyCalibratedOverlayLayer layers the registered overlay onto cfg and returns
// a report of what changed.
//
// It is deliberately non-fatal: an unreadable or malformed overlay is logged and
// ignored, so a broken overlay can never stop the process from starting on the
// SSOT values. Entries for unknown parameters are dropped (they cannot be
// applied, and keeping them would only hide a typo), and entries whose SSOT
// baseline moved are dropped so a reviewed charter edit wins.
func ApplyCalibratedOverlayLayer(cfg *ParametersConfig) CalibrationOverlayReport {
	report := CalibrationOverlayReport{Path: GetCalibratedOverlayPath()}
	if cfg == nil || report.Path == "" {
		return report
	}

	ov, err := LoadCalibrationOverlay(report.Path)
	if err != nil {
		report.Err = err
		logging.Warn("calibration_overlay", "overlay_unreadable",
			logging.FStr("path", report.Path), logging.Err(err))
		return report
	}
	if ov == nil || len(ov.Entries) == 0 {
		return report
	}

	names := make([]string, 0, len(ov.Entries))
	for name := range ov.Entries {
		names = append(names, name)
	}
	sort.Strings(names)

	reconcile := false
	for _, name := range names {
		entry := ov.Entries[name]
		accessor, ok := parameterTable[name]
		if !ok {
			report.Unknown = append(report.Unknown, name)
			delete(ov.Entries, name)
			reconcile = true
			logging.Warn("calibration_overlay", "overlay_entry_unknown_parameter",
				logging.FStr("param", name), logging.FStr("path", report.Path))
			continue
		}

		ssot := accessor.get(cfg)
		if entry.SSOT != 0 && !almostEqual(entry.SSOT, ssot) {
			// The SSOT value moved after this entry was reconciled (a reviewed
			// edit to configs/parameters.json, or an unrelated calibrator that
			// still writes that file). The adaptation was derived from the old
			// value, so it is stale: drop it, let the charter win, and say so.
			report.Invalidated = append(report.Invalidated, name)
			delete(ov.Entries, name)
			reconcile = true
			logging.Warn("calibration_overlay", "overlay_entry_invalidated_ssot_moved",
				logging.FStr("param", name),
				logging.FFloat64("reconciled_against", entry.SSOT),
				logging.FFloat64("ssot_now", ssot),
				logging.FFloat64("overlay_value", entry.Value))
			continue
		}

		accessor.set(cfg, entry.Value)

		ratio := 0.0
		if ssot != 0 {
			ratio = entry.Value / ssot
		}
		report.Applied = append(report.Applied, CalibrationOverlayDiff{
			Name:         name,
			SSOT:         ssot,
			Effective:    entry.Value,
			Before:       entry.Before,
			Ratio:        ratio,
			CalibratedAt: entry.CalibratedAt,
			Method:       entry.Method,
		})

		// Operational visibility (requirement 3 above): every applied entry is
		// announced with both values. A ratio outside the calibration loops'
		// documented per-round window [0.3x, 3x] means the overlay is not the
		// product of a single accepted step — a human should look.
		logging.Info("calibration_overlay", "overlay_entry_applied",
			logging.FStr("param", name),
			logging.FFloat64("ssot", ssot),
			logging.FFloat64("effective", entry.Value),
			logging.FFloat64("ratio", ratio),
			logging.FStr("method", entry.Method),
			logging.FStr("calibrated_at", entry.CalibratedAt.Format(time.RFC3339)))
		if ssot != 0 && (ratio < 1.0/3.0 || ratio > 3.0) {
			logging.Warn("calibration_overlay", "overlay_entry_outside_single_step_window",
				logging.FStr("param", name),
				logging.FFloat64("ssot", ssot),
				logging.FFloat64("effective", entry.Value),
				logging.FFloat64("ratio", ratio))
		}

		if entry.SSOT == 0 {
			// First startup after this entry was written: record the SSOT it is
			// layered on, so a later charter edit can invalidate it.
			entry.SSOT = ssot
			ov.Entries[name] = entry
			reconcile = true
		}
	}

	if reconcile {
		if err := SaveCalibrationOverlay(report.Path, ov); err != nil {
			logging.Warn("calibration_overlay", "overlay_reconcile_failed",
				logging.FStr("path", report.Path), logging.Err(err))
		} else {
			report.Reconciled = true
		}
	}

	if len(report.Applied) > 0 || len(report.Invalidated) > 0 || len(report.Unknown) > 0 {
		logging.Info("calibration_overlay", "overlay_applied",
			logging.FStr("path", report.Path),
			logging.FInt("applied", len(report.Applied)),
			logging.FInt("invalidated", len(report.Invalidated)),
			logging.FInt("unknown", len(report.Unknown)))
	}
	return report
}

// LoadEffectiveParametersConfig loads the SSOT parameters file and layers the
// registered calibrated overlay on top. This is the configuration the process
// actually runs on; LoadParametersConfig stays the raw SSOT read used by audit
// paths (integrity checks, diffing, tooling).
func LoadEffectiveParametersConfig(path string) (*ParametersConfig, error) {
	cfg, err := LoadParametersConfig(path)
	if err != nil {
		return nil, err
	}
	ApplyCalibratedOverlayLayer(cfg)
	return cfg, nil
}

// almostEqual compares two parameter values with a tolerance that absorbs the
// float64 round-trip through JSON without hiding a real change.
func almostEqual(a, b float64) bool {
	scale := math.Max(1, math.Max(math.Abs(a), math.Abs(b)))
	return math.Abs(a-b) <= 1e-9*scale
}
