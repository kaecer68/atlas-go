package config

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
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
//
// # Two kinds of entry
//
//   - Named tunable (Path empty): the map key is a parameter-table name
//     (internal/config/param_table.go) and the value is a scalar float64. Used by
//     the calibrators whose parameter space is the table (risk self-calibration,
//     config.CalibrateParameters). Applied to the parsed struct.
//   - Dotted path (Path set): an explicit path into the parameters document
//     (e.g. "industry.cycle_thresholds.value"), with an arbitrary JSON value.
//     Used by calibrators that write nested blocks (factor weights, RSI-tw
//     scores, conviction thresholds, industry cycle thresholds). Applied to the
//     SSOT document before it is parsed, so nested and map-shaped values are
//     possible without teaching the parameter table about them.
//
// Both kinds share the same document, the same fail-closed rules and the same
// reporting.

// calibrationOverlayVersion is the schema version of the overlay document.
const calibrationOverlayVersion = "2"

// OverlaySSOTBaseline is the SSOT state an overlay entry was reconciled
// against. A nil baseline means "not reconciled yet" (the entry was written by a
// process that has not loaded it since); Present=false means the path/value did
// not exist in the SSOT at reconciliation time.
type OverlaySSOTBaseline struct {
	Present bool `json:"present"`
	Value   any  `json:"value,omitempty"`
}

// CalibrationOverlayEntry is one adapted tunable or nested path.
type CalibrationOverlayEntry struct {
	// Path is the dotted JSON path inside the SSOT document. Empty = the map key
	// is a parameter-table name (named tunable).
	Path string `json:"path,omitempty"`
	// Value is the calibrated (effective) value. float64 for named tunables, any
	// JSON value for path entries.
	Value any `json:"value"`
	// Before is the effective value this adaptation replaced (audit only).
	Before any `json:"before,omitempty"`
	// SSOT is the SSOT state this entry was last reconciled against (nil = not
	// reconciled yet). It is what makes a later charter edit detectable.
	SSOT *OverlaySSOTBaseline `json:"ssot,omitempty"`
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
// previous round (or another calibrator) wrote and revert those values to the
// SSOT. An entry that already exists keeps its reconciliation baseline unless
// the caller supplies a fresh one.
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
	for key, entry := range entries {
		if prev, ok := ov.Entries[key]; ok && entry.SSOT == nil {
			entry.SSOT = prev.SSOT
		}
		if entry.CalibratedAt.IsZero() {
			entry.CalibratedAt = time.Now()
		}
		ov.Entries[key] = entry
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

// CalibratedEntryForParameter builds a named-tunable entry, resolving its SSOT
// baseline from the SSOT file. A baseline that cannot be resolved is left nil
// ("not reconciled yet") — the loader records it on the next start.
func CalibratedEntryForParameter(name string, value, before float64, method, rationale string, at time.Time) CalibrationOverlayEntry {
	return CalibrationOverlayEntry{
		Value:        value,
		Before:       before,
		SSOT:         SSOTParameterBaseline(name),
		CalibratedAt: at,
		Method:       method,
		Rationale:    rationale,
	}
}

// CalibratedEntryForPath builds a dotted-path entry, resolving its SSOT baseline
// from the SSOT document (Present=false when the path has no value there).
func CalibratedEntryForPath(path string, value, before any, method, rationale string, at time.Time) CalibrationOverlayEntry {
	return CalibrationOverlayEntry{
		Path:         path,
		Value:        value,
		Before:       before,
		SSOT:         SSOTPathBaseline(path),
		CalibratedAt: at,
		Method:       method,
		Rationale:    rationale,
	}
}

// SSOTParameterBaseline returns a named tunable's value as written in the SSOT
// file, ignoring the overlay, or nil when it cannot be resolved.
func SSOTParameterBaseline(name string) *OverlaySSOTBaseline {
	accessor, ok := parameterTable[name]
	if !ok {
		return nil
	}
	_, cfg, _, err := loadParametersSource(GetParametersConfigPath())
	if err != nil || cfg == nil {
		return nil
	}
	return &OverlaySSOTBaseline{Present: true, Value: accessor.get(cfg)}
}

// SSOTPathBaseline returns the value at a dotted path in the SSOT document,
// ignoring the overlay, or nil when the document or the path cannot be read.
func SSOTPathBaseline(path string) *OverlaySSOTBaseline {
	raw, _, _, err := loadParametersSource(GetParametersConfigPath())
	if err != nil || len(raw) == 0 {
		return nil
	}
	doc, err := decodeParametersDocument(raw)
	if err != nil {
		return nil
	}
	value, ok := getJSONPath(doc, path)
	if !ok {
		return &OverlaySSOTBaseline{Present: false}
	}
	return &OverlaySSOTBaseline{Present: true, Value: value}
}

// CalibrationOverlayDiff is one applied adaptation, carrying both the SSOT and
// the effective value so the caller can report the drift it just layered in.
type CalibrationOverlayDiff struct {
	Key          string    `json:"key"`
	Path         string    `json:"path,omitempty"`
	SSOT         any       `json:"ssot,omitempty"`
	Effective    any       `json:"effective"`
	Before       any       `json:"before,omitempty"`
	Ratio        float64   `json:"ratio,omitempty"`
	CalibratedAt time.Time `json:"calibrated_at"`
	Method       string    `json:"method,omitempty"`
}

// CalibrationOverlayReport summarizes one application of the overlay.
type CalibrationOverlayReport struct {
	Path        string
	Applied     []CalibrationOverlayDiff
	Unknown     []string // entries whose parameter/path does not exist in the SSOT
	Invalidated []string // entries dropped because the SSOT value moved
	Reconciled  bool     // the on-disk overlay was rewritten
	Err         error
}

// AppliedKeys returns the map keys of the entries the overlay changed.
func (r CalibrationOverlayReport) AppliedKeys() []string {
	keys := make([]string, 0, len(r.Applied))
	for _, d := range r.Applied {
		keys = append(keys, d.Key)
	}
	return keys
}

// ApplyCalibratedOverlayLayer layers the registered overlay onto cfg and returns
// the (possibly replaced) configuration plus a report of what changed. ssotRaw is
// the SSOT document cfg was parsed from (nil when the SSOT file is missing); it
// is what dotted-path entries are applied to.
//
// It is deliberately non-fatal: an unreadable or malformed overlay is logged and
// ignored, so a broken overlay can never stop the process from starting on the
// SSOT values. Entries naming an unknown parameter or path are dropped (they
// cannot be applied, and keeping them would only hide a typo), and entries whose
// SSOT baseline moved are dropped so a reviewed charter edit wins.
func ApplyCalibratedOverlayLayer(cfg *ParametersConfig, ssotRaw []byte) (*ParametersConfig, CalibrationOverlayReport) {
	report := CalibrationOverlayReport{Path: GetCalibratedOverlayPath()}
	if cfg == nil || report.Path == "" {
		return cfg, report
	}

	ov, err := LoadCalibrationOverlay(report.Path)
	if err != nil {
		report.Err = err
		logging.Warn("calibration_overlay", "overlay_unreadable",
			logging.FStr("path", report.Path), logging.Err(err))
		return cfg, report
	}
	if ov == nil || len(ov.Entries) == 0 {
		return cfg, report
	}

	keys := make([]string, 0, len(ov.Entries))
	for key := range ov.Entries {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	reconcile := false

	// ---- phase 1: dotted-path entries patch the SSOT document ----
	pathKeys := make([]string, 0, len(keys))
	for _, key := range keys {
		if ov.Entries[key].Path != "" {
			pathKeys = append(pathKeys, key)
		}
	}
	var doc map[string]any
	patched := false
	if len(pathKeys) > 0 {
		doc, err = decodeParametersDocument(ssotRaw)
		if err != nil {
			// No SSOT document to patch: the entries cannot be validated against
			// the SSOT, so they are dropped loudly rather than applied blind.
			for _, key := range pathKeys {
				report.Unknown = append(report.Unknown, key)
				delete(ov.Entries, key)
				reconcile = true
				logging.Warn("calibration_overlay", "overlay_path_entry_dropped_no_ssot_document",
					logging.FStr("key", key), logging.FStr("path", ov.Entries[key].Path), logging.Err(err))
			}
		} else {
			for _, key := range pathKeys {
				entry := ov.Entries[key]
				if !containerExists(doc, entry.Path) {
					report.Unknown = append(report.Unknown, key)
					delete(ov.Entries, key)
					reconcile = true
					logging.Warn("calibration_overlay", "overlay_entry_unknown_path",
						logging.FStr("key", key), logging.FStr("path", entry.Path))
					continue
				}
				curve, _ := getJSONPath(doc, entry.Path)
				if !baselineMatches(entry.SSOT, curve) {
					report.Invalidated = append(report.Invalidated, key)
					delete(ov.Entries, key)
					reconcile = true
					logging.Warn("calibration_overlay", "overlay_entry_invalidated_ssot_moved",
						logging.FStr("key", key), logging.FStr("path", entry.Path),
						logging.FStr("reconciled_against", describeBaseline(entry.SSOT)),
						logging.FStr("ssot_now", describeJSONValue(curve)))
					continue
				}
				if !setJSONPath(doc, entry.Path, entry.Value) {
					report.Unknown = append(report.Unknown, key)
					delete(ov.Entries, key)
					reconcile = true
					logging.Warn("calibration_overlay", "overlay_entry_path_not_settable",
						logging.FStr("key", key), logging.FStr("path", entry.Path))
					continue
				}
				patched = true
				diff := newOverlayDiff(key, entry, curve)
				report.Applied = append(report.Applied, diff)
				logOverlayApplication(diff)
				if entry.SSOT == nil {
					entry.SSOT = baselineOf(curve)
					ov.Entries[key] = entry
					reconcile = true
				}
			}
		}
	}
	if patched {
		out, err := json.Marshal(doc)
		if err != nil {
			report.Err = fmt.Errorf("marshal patched parameters document: %w", err)
			logging.Error("calibration_overlay", "overlay_patch_marshal_failed", logging.Err(err))
			return cfg, report
		}
		patchedCfg, err := parseParametersBytes(out)
		if err != nil {
			// Fail closed: keep the SSOT configuration rather than running on a
			// document we cannot validate.
			report.Err = err
			logging.Error("calibration_overlay", "overlay_patch_parse_failed", logging.Err(err))
			return cfg, report
		}
		cfg = patchedCfg
	}

	// ---- phase 2: named tunables, applied through the parameter table ----
	// Re-derive the key list: phase 1 may have dropped entries, and a dropped key
	// must not be re-read as a zero-valued named entry here.
	keys = keys[:0]
	for key := range ov.Entries {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		entry := ov.Entries[key]
		if entry.Path != "" {
			continue
		}
		accessor, ok := parameterTable[key]
		if !ok {
			report.Unknown = append(report.Unknown, key)
			delete(ov.Entries, key)
			reconcile = true
			logging.Warn("calibration_overlay", "overlay_entry_unknown_parameter",
				logging.FStr("param", key), logging.FStr("path", report.Path))
			continue
		}
		ssot := accessor.get(cfg)
		if !baselineMatches(entry.SSOT, ssot) {
			report.Invalidated = append(report.Invalidated, key)
			delete(ov.Entries, key)
			reconcile = true
			logging.Warn("calibration_overlay", "overlay_entry_invalidated_ssot_moved",
				logging.FStr("param", key),
				logging.FStr("reconciled_against", describeBaseline(entry.SSOT)),
				logging.FStr("ssot_now", describeJSONValue(ssot)),
				logging.FStr("overlay_value", describeJSONValue(entry.Value)))
			continue
		}
		value, ok := numericValue(entry.Value)
		if !ok {
			report.Unknown = append(report.Unknown, key)
			delete(ov.Entries, key)
			reconcile = true
			logging.Warn("calibration_overlay", "overlay_entry_non_numeric_parameter",
				logging.FStr("param", key), logging.FStr("value", describeJSONValue(entry.Value)))
			continue
		}
		accessor.set(cfg, value)
		diff := newOverlayDiff(key, entry, ssot)
		report.Applied = append(report.Applied, diff)
		logOverlayApplication(diff)
		if entry.SSOT == nil {
			entry.SSOT = baselineOf(ssot)
			ov.Entries[key] = entry
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
	return cfg, report
}

// LoadEffectiveParametersConfig loads the SSOT parameters file and layers the
// registered calibrated overlay on top. This is the configuration the process
// actually runs on; LoadParametersConfig stays the raw SSOT read used by audit
// paths (integrity checks, diffing, tooling).
func LoadEffectiveParametersConfig(path string) (*ParametersConfig, error) {
	raw, cfg, _, err := loadParametersSource(path)
	if err != nil {
		return nil, err
	}
	effective, _ := ApplyCalibratedOverlayLayer(cfg, raw)
	return effective, nil
}

// newOverlayDiff builds the audit/visibility record for one applied entry.
func newOverlayDiff(key string, entry CalibrationOverlayEntry, ssot any) CalibrationOverlayDiff {
	diff := CalibrationOverlayDiff{
		Key:          key,
		Path:         entry.Path,
		SSOT:         ssot,
		Effective:    entry.Value,
		Before:       entry.Before,
		CalibratedAt: entry.CalibratedAt,
		Method:       entry.Method,
	}
	if ssotNum, ok := numericValue(ssot); ok {
		if effNum, ok := numericValue(entry.Value); ok && ssotNum != 0 {
			diff.Ratio = effNum / ssotNum
		}
	}
	return diff
}

// logOverlayApplication reports one applied entry with both values. A numeric
// ratio outside the calibration loops' documented per-round window [0.3x, 3x]
// means the overlay is not the product of a single accepted step — a human
// should look.
func logOverlayApplication(diff CalibrationOverlayDiff) {
	logging.Info("calibration_overlay", "overlay_entry_applied",
		logging.FStr("key", diff.Key),
		logging.FStr("json_path", diff.Path),
		logging.FStr("ssot", describeJSONValue(diff.SSOT)),
		logging.FStr("effective", describeJSONValue(diff.Effective)),
		logging.FFloat64("ratio", diff.Ratio),
		logging.FStr("method", diff.Method),
		logging.FStr("calibrated_at", diff.CalibratedAt.Format(time.RFC3339)))
	if diff.Ratio != 0 && (diff.Ratio < 1.0/3.0 || diff.Ratio > 3.0) {
		logging.Warn("calibration_overlay", "overlay_entry_outside_single_step_window",
			logging.FStr("key", diff.Key),
			logging.FStr("ssot", describeJSONValue(diff.SSOT)),
			logging.FStr("effective", describeJSONValue(diff.Effective)),
			logging.FFloat64("ratio", diff.Ratio))
	}
}

// baselineOf wraps a value found in the SSOT.
func baselineOf(v any) *OverlaySSOTBaseline {
	return &OverlaySSOTBaseline{Present: true, Value: v}
}

// baselineMatches reports whether an entry's recorded SSOT baseline still
// describes the current SSOT value. A nil baseline means "not reconciled yet"
// and always matches (the loader records it).
func baselineMatches(baseline *OverlaySSOTBaseline, current any) bool {
	if baseline == nil {
		return true
	}
	if !baseline.Present {
		return current == nil
	}
	return sameJSONValue(baseline.Value, current)
}

// describeBaseline renders a baseline for logs.
func describeBaseline(baseline *OverlaySSOTBaseline) string {
	if baseline == nil {
		return "<not reconciled>"
	}
	if !baseline.Present {
		return "<absent>"
	}
	return describeJSONValue(baseline.Value)
}

// describeJSONValue renders any JSON value compactly for logs.
func describeJSONValue(v any) string {
	if v == nil {
		return "<absent>"
	}
	switch t := v.(type) {
	case string:
		return t
	case float64:
		return strconv.FormatFloat(t, 'g', -1, 64)
	case bool:
		return strconv.FormatBool(t)
	}
	encoded, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return string(encoded)
}

// numericValue extracts a float from an overlay/SSOT value.
func numericValue(v any) (float64, bool) {
	switch t := v.(type) {
	case float64:
		return t, true
	case float32:
		return float64(t), true
	case int:
		return float64(t), true
	case int64:
		return float64(t), true
	case json.Number:
		f, err := t.Float64()
		return f, err == nil
	}
	return 0, false
}

// sameJSONValue compares two JSON-decoded values.
func sameJSONValue(a, b any) bool {
	aNum, aOK := numericValue(a)
	bNum, bOK := numericValue(b)
	if aOK || bOK {
		if !aOK || !bOK {
			return false
		}
		return math.Abs(aNum-bNum) <= 1e-9*math.Max(1, math.Max(math.Abs(aNum), math.Abs(bNum)))
	}
	return reflect.DeepEqual(a, b)
}

// decodeParametersDocument decodes a parameters document into a generic JSON
// map. A nil or empty document is an error: there is nothing to patch against.
func decodeParametersDocument(raw []byte) (map[string]any, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("no parameters document to patch")
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("decode parameters document: %w", err)
	}
	if doc == nil {
		return nil, fmt.Errorf("parameters document is not a JSON object")
	}
	return doc, nil
}

// containerExists reports whether every segment of path except the last exists
// in doc and is a JSON object. The leaf itself may be absent: calibration loops
// legitimately create new leaves (e.g. a newly calibrated industry) inside an
// existing SSOT section, while a wrong section name must still be detected.
func containerExists(doc map[string]any, path string) bool {
	segments := strings.Split(path, ".")
	if len(segments) == 0 || segments[0] == "" {
		return false
	}
	var current any = doc
	for _, segment := range segments[:len(segments)-1] {
		obj, ok := current.(map[string]any)
		if !ok {
			return false
		}
		next, ok := obj[segment]
		if !ok {
			return false
		}
		current = next
	}
	_, ok := current.(map[string]any)
	return ok
}

// getJSONPath returns the value at a dotted path. It fails when any segment
// (including the last) is missing or when an intermediate segment is not an
// object. Only object segments are supported (no array indexing).
func getJSONPath(doc map[string]any, path string) (any, bool) {
	if path == "" {
		return nil, false
	}
	var current any = doc
	for _, segment := range strings.Split(path, ".") {
		obj, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		next, ok := obj[segment]
		if !ok {
			return nil, false
		}
		current = next
	}
	return current, true
}

// setJSONPath sets the value at a dotted path. The container must already exist
// (see containerExists); the leaf is created when absent.
func setJSONPath(doc map[string]any, path string, value any) bool {
	if !containerExists(doc, path) {
		return false
	}
	segments := strings.Split(path, ".")
	obj := doc
	for _, segment := range segments[:len(segments)-1] {
		next, ok := obj[segment].(map[string]any)
		if !ok {
			return false
		}
		obj = next
	}
	obj[segments[len(segments)-1]] = value
	return true
}
