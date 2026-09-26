package config

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/internal/logging"
)

// Calibration writeback routing (FU-20260926-07).
//
// The calibration loops that run *inside a container* must not write
// configs/parameters.json: configs/ is not bind-mounted, so the write lands in
// the container's writable layer (invisible to git, gone on the next container
// recreate, and impossible to tell apart from the reviewed SSOT while the
// container runs). Those runs write the calibrated-parameters overlay under the
// bind-mounted data/ tree instead (see calibration_overlay.go).
//
// A human running the same tool on a checkout is in the opposite situation: the
// write IS the deliverable, and it should show up as a reviewable git diff.
//
// The routing is therefore explicit rather than inferred: every calibration
// command takes a -writeback flag, default `ssot` (the human/checkout behavior,
// unchanged), and the in-container spawner passes `overlay`. A command that
// asked for `overlay` and has no overlay path registered fails loudly instead of
// quietly falling back to the SSOT.
type CalibrationWriteback string

const (
	// WritebackSSOT writes the parameters file itself. Default: the write is a
	// reviewable git diff, which is what a human on a checkout wants.
	WritebackSSOT CalibrationWriteback = "ssot"
	// WritebackOverlay writes the calibrated-parameters overlay under data/.
	// Required for container/cron runs (configs/ is not mounted there).
	WritebackOverlay CalibrationWriteback = "overlay"
)

// WritebackFlagUsage is the flag description shared by the calibration CLIs.
const WritebackFlagUsage = "where calibration results are persisted: " +
	"ssot (default) rewrites configs/parameters.json, reviewable as a git diff; " +
	"overlay writes data/state/parameters.calibrated.json, required inside a container where configs/ is not bind-mounted"

// ParseCalibrationWriteback validates a -writeback flag value. The empty string
// selects the default (ssot).
func ParseCalibrationWriteback(v string) (CalibrationWriteback, error) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "", string(WritebackSSOT):
		return WritebackSSOT, nil
	case string(WritebackOverlay):
		return WritebackOverlay, nil
	default:
		return "", fmt.Errorf("unknown writeback %q (want %q or %q)", v, WritebackSSOT, WritebackOverlay)
	}
}

// IsOverlay reports whether the mode persists to the overlay.
func (m CalibrationWriteback) IsOverlay() bool { return m == WritebackOverlay }

// RegisterForWorkDir prepares the mode for a process rooted at workDir. In
// overlay mode it registers the work-dir-relative overlay path
// (<workDir>/data/state/parameters.calibrated.json); in ssot mode it is a no-op.
func (m CalibrationWriteback) RegisterForWorkDir(workDir string) error {
	if !m.IsOverlay() {
		return nil
	}
	path := CalibrationOverlayPath(workDir)
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("calibration writeback=overlay: overlay path for work dir %q is empty", workDir)
	}
	SetCalibratedOverlayPath(path)
	return nil
}

// WriteDocumentOverlay persists the leaves that a calibration run changed,
// computed by diffing the parameters document as loaded (the SSOT) against the
// document as updated. The SSOT file is not touched.
//
// It is the overlay-mode counterpart of "marshal the document and write the
// parameters file": callers keep their existing ssot path unchanged, and use
// this only when the operator asked for overlay mode.
func WriteDocumentOverlay(source string, base, updated map[string]any, at time.Time) (int, error) {
	overlayPath := GetCalibratedOverlayPath()
	if overlayPath == "" {
		return 0, fmt.Errorf("calibration writeback=overlay requires a registered overlay path (see CalibrationWriteback.RegisterForWorkDir)")
	}
	if at.IsZero() {
		at = time.Now()
	}

	changed, skipped := DiffParametersDocuments(base, updated)
	for _, path := range skipped {
		// A JSON path segment containing a dot cannot be addressed by the overlay
		// (paths are dot-separated). Reported rather than dropped silently.
		logging.Warn("calibration_writeback", "change_not_addressable",
			logging.FStr("source", source), logging.FStr("json_path", path))
	}
	if len(changed) == 0 {
		logging.Info("calibration_writeback", "nothing_changed", logging.FStr("source", source))
		return 0, nil
	}

	paths := make([]string, 0, len(changed))
	for p := range changed {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	entries := make(map[string]CalibrationOverlayEntry, len(paths))
	for _, p := range paths {
		before, present := getJSONPath(base, p) // absent leaves are legitimate
		entries[p] = CalibrationOverlayEntry{
			Path:         p,
			Value:        changed[p],
			Before:       before,
			CalibratedAt: at,
			Method:       source,
			Rationale:    "calibration run diffed against the SSOT document",
			// The baseline comes from the document this run actually loaded (the
			// SSOT as the caller saw it), not from a second read of the
			// process-wide path: a caller may be pointed at a different file.
			SSOT: &OverlaySSOTBaseline{Present: present, Value: before},
		}
	}

	ov, err := UpdateCalibrationOverlay(overlayPath, source, entries)
	if err != nil {
		return 0, fmt.Errorf("write calibration overlay: %w", err)
	}
	logging.Info("calibration_writeback", "overlay_written",
		logging.FStr("source", source),
		logging.FStr("path", overlayPath),
		logging.FInt("changed", len(entries)),
		logging.FInt("entries", len(ov.Entries)))
	return len(entries), nil
}

// WriteConfigOverlay is the typed counterpart of WriteDocumentOverlay: it diffs two
// ParametersConfig values (the one loaded before the calibration and the one
// after it) and persists the changed leaves to the overlay.
//
// Both values are marshaled first, so the diff is computed on the same JSON
// shape the SSOT file uses.
func WriteConfigOverlay(source string, base, updated *ParametersConfig, at time.Time) (int, error) {
	if base == nil || updated == nil {
		return 0, fmt.Errorf("calibration writeback: base and updated configs are required")
	}
	baseDoc, err := configToDocument(base)
	if err != nil {
		return 0, err
	}
	updatedDoc, err := configToDocument(updated)
	if err != nil {
		return 0, err
	}
	return WriteDocumentOverlay(source, baseDoc, updatedDoc, at)
}

// configToDocument marshals a ParametersConfig into a generic JSON document.
func configToDocument(cfg *ParametersConfig) (map[string]any, error) {
	raw, err := json.Marshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("marshal parameters config: %w", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("decode parameters config document: %w", err)
	}
	return doc, nil
}

// DiffParametersDocuments returns the changed leaves between two parameters
// documents as dotted-path → updated value, plus the paths it could not address.
//
// Rules:
//   - a leaf that changed is emitted with its whole updated value (scalars,
//     arrays and replaced objects alike);
//   - a key whose name contains a dot cannot be addressed by a dotted path, so it
//     is reported in the second return value instead of being persisted wrongly;
//   - removals are not represented: the overlay can set values, not delete keys.
//     A removed key therefore stays in the SSOT (which the calibration must not
//     rewrite anyway).
func DiffParametersDocuments(base, updated map[string]any) (map[string]any, []string) {
	changed := make(map[string]any)
	var skipped []string

	var walk func(prefix string, b, u any)
	walk = func(prefix string, b, u any) {
		bMap, bIsMap := b.(map[string]any)
		uMap, uIsMap := u.(map[string]any)
		if bIsMap && uIsMap {
			keys := make([]string, 0, len(uMap))
			for k := range uMap {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				path := k
				if prefix != "" {
					path = prefix + "." + k
				}
				if strings.Contains(k, ".") {
					if !sameJSONValue(bMap[k], uMap[k]) {
						skipped = append(skipped, path)
					}
					continue
				}
				walk(path, bMap[k], uMap[k])
			}
			return
		}
		if prefix == "" {
			return
		}
		if !sameJSONValue(b, u) {
			changed[prefix] = u
		}
	}

	if base == nil || updated == nil {
		return changed, skipped
	}
	walk("", base, updated)
	sort.Strings(skipped)
	return changed, skipped
}

// CloneParametersConfig returns a deep copy of cfg via a JSON round-trip. It is
// for callers that must diff the configuration as loaded against the
// configuration after a calibration.
func CloneParametersConfig(cfg *ParametersConfig) (*ParametersConfig, error) {
	if cfg == nil {
		return nil, fmt.Errorf("clone parameters config: nil config")
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("clone parameters config: marshal: %w", err)
	}
	var clone ParametersConfig
	if err := json.Unmarshal(raw, &clone); err != nil {
		return nil, fmt.Errorf("clone parameters config: unmarshal: %w", err)
	}
	return &clone, nil
}
