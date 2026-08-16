package domain

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// SystemMaturity represents the statistical readiness phase of the system.
// The system transitions through three phases based on accumulated trading days:
//   - BURN_IN:     < 60 days — conservative defaults, no auto-adjustment
//   - CALIBRATING: 60–251 days — partial auto-calibration and evolution
//   - FULL_AUTO:   ≥ 252 days — all statistical engines fully active
type SystemMaturity string

const (
	MaturityBurnIn      SystemMaturity = "burn_in"
	MaturityCalibrating SystemMaturity = "calibrating"
	MaturityFullAuto    SystemMaturity = "full_auto"
)

// MaturityThresholds maps each phase to its minimum required system age in days.
var MaturityThresholds = map[SystemMaturity]int{
	MaturityBurnIn:      0,
	MaturityCalibrating: 60,
	MaturityFullAuto:    252,
}

// maturityTrackerState is the persisted form of the tracker.
type maturityTrackerState struct {
	FirstStartDate time.Time `json:"first_start_date"`
	LastChecked    time.Time `json:"last_checked"`
}

// MaturityTracker tracks system age and determines the current maturity phase.
// It is safe for concurrent use.
type MaturityTracker struct {
	firstStartDate time.Time
	current        SystemMaturity
	mu             sync.RWMutex
	subs           []func(oldM, newM SystemMaturity)
	subsMu         sync.RWMutex
}

// NewMaturityTracker creates a tracker. It attempts to load persisted state
// from disk; if absent, it sets firstStartDate to now and persists it.
func NewMaturityTracker(statePath string) (*MaturityTracker, error) {
	return NewMaturityTrackerSeeded(statePath, "")
}

// NewMaturityTrackerSeeded creates a tracker like NewMaturityTracker, but when
// no persisted state exists (fresh deployment) it seeds firstStartDate from
// firstStartSeed instead of now. firstStartSeed accepts RFC3339
// ("2026-06-01T05:10:28Z") or date-only ("2026-06-01"); an empty or
// unparseable seed falls back to now.
//
// The seed lets deployments carry the original system start date across
// data-directory loss (container/volume rebuild, machine re-provision),
// so the burn-in / calibrating / full-auto clock never silently resets.
// Precedence: persisted state file > seed > now.
func NewMaturityTrackerSeeded(statePath, firstStartSeed string) (*MaturityTracker, error) {
	state, err := loadMaturityState(statePath)
	if err != nil {
		return nil, fmt.Errorf("load maturity state: %w", err)
	}

	t := &MaturityTracker{
		firstStartDate: state.FirstStartDate,
		current:        MaturityBurnIn,
	}

	// Persist on first creation so subsequent restarts see the same start date.
	// firstStartDate must be set BEFORE refresh() — computing maturity from a
	// zero time would report full_auto on a brand-new deployment.
	if t.firstStartDate.IsZero() {
		t.firstStartDate = seedMaturityStart(firstStartSeed, time.Now().UTC())
		_ = t.save(statePath) // best-effort; caller can retry
	}

	t.refresh()
	return t, nil
}

// seedMaturityStart parses firstStartSeed (RFC3339 or date-only) and returns
// it; empty or unparseable seeds fall back to fallback.
func seedMaturityStart(firstStartSeed string, fallback time.Time) time.Time {
	if seed := strings.TrimSpace(firstStartSeed); seed != "" {
		for _, layout := range []string{time.RFC3339, "2006-01-02"} {
			if parsed, err := time.Parse(layout, seed); err == nil {
				return parsed.UTC()
			}
		}
	}
	return fallback
}

// NewMaturityTrackerWithStart creates a tracker with an explicit start date.
// Useful for testing or back-dating a deployment that already has history.
func NewMaturityTrackerWithStart(start time.Time) *MaturityTracker {
	t := &MaturityTracker{
		firstStartDate: start.UTC(),
		current:        MaturityBurnIn,
	}
	t.refresh()
	return t
}

// Current returns the cached maturity phase. Call Refresh before using if
// you need the absolutely latest value.
func (t *MaturityTracker) Current() SystemMaturity {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.current
}

// DaysSinceStart returns the number of calendar days since first start.
func (t *MaturityTracker) DaysSinceStart() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return int(time.Since(t.firstStartDate).Hours() / 24)
}

// DaysUntil returns the number of days until the given maturity phase is reached.
// Returns 0 if already at or past that phase.
func (t *MaturityTracker) DaysUntil(target SystemMaturity) int {
	threshold, ok := MaturityThresholds[target]
	if !ok {
		return 0
	}
	days := t.DaysSinceStart()
	if days >= threshold {
		return 0
	}
	return threshold - days
}

// Refresh recalculates maturity from the current wall-clock time and publishes
// transition events if the phase changed.
func (t *MaturityTracker) Refresh() {
	t.mu.Lock()
	old := t.current
	t.current = t.computeMaturity()
	newM := t.current
	t.mu.Unlock()

	if old != newM {
		t.subsMu.RLock()
		callbacks := make([]func(SystemMaturity, SystemMaturity), len(t.subs))
		copy(callbacks, t.subs)
		t.subsMu.RUnlock()
		for _, fn := range callbacks {
			fn(old, newM)
		}
	}
}

// OnTransition registers a callback invoked whenever the maturity phase changes.
func (t *MaturityTracker) OnTransition(fn func(oldM, newM SystemMaturity)) {
	t.subsMu.Lock()
	defer t.subsMu.Unlock()
	t.subs = append(t.subs, fn)
}

// FirstStartDate returns the immutable system start date.
func (t *MaturityTracker) FirstStartDate() time.Time {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.firstStartDate
}

// Save persists the tracker state to disk.
func (t *MaturityTracker) Save(statePath string) error {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.save(statePath)
}

func (t *MaturityTracker) save(statePath string) error {
	state := maturityTrackerState{
		FirstStartDate: t.firstStartDate,
		LastChecked:    time.Now().UTC(),
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal maturity state: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(statePath), 0o755); err != nil {
		return fmt.Errorf("create maturity dir: %w", err)
	}
	if err := os.WriteFile(statePath, data, 0o644); err != nil {
		return fmt.Errorf("write maturity state: %w", err)
	}
	return nil
}

func (t *MaturityTracker) computeMaturity() SystemMaturity {
	days := int(time.Since(t.firstStartDate).Hours() / 24)
	switch {
	case days >= MaturityThresholds[MaturityFullAuto]:
		return MaturityFullAuto
	case days >= MaturityThresholds[MaturityCalibrating]:
		return MaturityCalibrating
	default:
		return MaturityBurnIn
	}
}

func (t *MaturityTracker) refresh() {
	t.current = t.computeMaturity()
}

func loadMaturityState(path string) (maturityTrackerState, error) {
	var state maturityTrackerState
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return state, nil // fresh start
		}
		return state, fmt.Errorf("read %s: %w", path, err)
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return state, fmt.Errorf("unmarshal %s: %w", path, err)
	}
	return state, nil
}

// MaturityGated is a convenience helper that returns whether an operation
// should proceed based on minimum required maturity.
func MaturityGated(current SystemMaturity, minimum SystemMaturity) bool {
	order := map[SystemMaturity]int{
		MaturityBurnIn:      0,
		MaturityCalibrating: 1,
		MaturityFullAuto:    2,
	}
	return order[current] >= order[minimum]
}
