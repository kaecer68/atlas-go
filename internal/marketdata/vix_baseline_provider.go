package marketdata

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

const vixBaselineMaxDays = 252

// VIXBaselineTracker maintains a file-backed rolling history of VIX daily
// values and provides the 252-day median baseline used by JANUS as the
// panic threshold (see MacroDataSnapshot.VIXBaseline).
//
// History accumulates over time from the 5m macro_ingest cron. Until 252
// data points are collected, the median is computed from available points;
// at zero points Value() returns 0, which JANUS treats as "use fixed 20
// fallback" (legacy behavior).
type VIXBaselineTracker struct {
	path  string
	mu    sync.RWMutex
	value float64
}

// NewVIXBaselineTracker creates a tracker backed by a JSON file at the
// given path. If the file exists, it is loaded on first Update or Value call.
func NewVIXBaselineTracker(path string) *VIXBaselineTracker {
	return &VIXBaselineTracker{path: path}
}

// Update appends a VIX value to the rolling history, trims to maxSize,
// recomputes the median baseline, and persists to disk.
func (t *VIXBaselineTracker) Update(vix float64) {
	if vix <= 0 {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()

	history := t.loadLocked()
	history = append(history, vix)
	if len(history) > vixBaselineMaxDays {
		history = history[len(history)-vixBaselineMaxDays:]
	}
	t.value = median(history)
	t.saveLocked(history)
}

// Value returns the current 252-day VIX median, or 0 if no data collected.
// On first call when no Update has been made, loads persisted history.
func (t *VIXBaselineTracker) Value() float64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.value == 0 && t.path != "" {
		history := t.loadLocked()
		if len(history) > 0 {
			t.value = median(history)
		}
	}
	return t.value
}

func (t *VIXBaselineTracker) loadLocked() []float64 {
	data, err := os.ReadFile(t.path)
	if err != nil {
		return nil
	}
	var history []float64
	if err := json.Unmarshal(data, &history); err != nil {
		return nil
	}
	return history
}

func (t *VIXBaselineTracker) saveLocked(history []float64) {
	if err := os.MkdirAll(filepath.Dir(t.path), 0o755); err != nil {
		return
	}
	data, err := json.Marshal(history)
	if err != nil {
		return
	}
	_ = os.WriteFile(t.path, data, 0o644)
}

func median(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := make([]float64, len(values))
	copy(sorted, values)
	sort.Float64s(sorted)
	n := len(sorted)
	if n%2 == 0 {
		return (sorted[n/2-1] + sorted[n/2]) / 2
	}
	return sorted[n/2]
}
