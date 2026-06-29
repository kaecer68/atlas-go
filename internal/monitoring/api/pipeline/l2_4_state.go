package pipeline

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"time"
)

var timeHHMMRegex = regexp.MustCompile(`^([01][0-9]|2[0-3]):[0-5][0-9]$`)

// L24ScheduleStatus tracks the runtime state of an L2.4 observation window.
type L24ScheduleStatus struct {
	Running           bool       `json:"running"`
	StartedAt         *time.Time `json:"started_at,omitempty"`
	EndsAt            *time.Time `json:"ends_at,omitempty"`
	CurrentPeriodDays int        `json:"current_period_days,omitempty"`
}

// L24ScheduleConfig holds operator-visible schedule configuration
// (defaults + overrides). Mirrors the L2_4ScheduleParameters
// governance struct in internal/config/parameters.go, but stored
// in the data file so that runtime overrides survive process restarts.
type L24ScheduleConfig struct {
	DefaultStartTime   string `json:"default_start_time"`
	DefaultPeriodDays  int    `json:"default_period_days"`
	OverrideStartTime  string `json:"override_start_time,omitempty"`
	OverridePeriodDays int    `json:"override_period_days,omitempty"`
	AutoEnabled        bool   `json:"auto_enabled"`
}

// L24Schedule is the on-disk schema persisted to
// data/state/l2-4-schedule.json.
type L24Schedule struct {
	Status    L24ScheduleStatus `json:"status"`
	Config    L24ScheduleConfig `json:"config"`
	UpdatedAt time.Time         `json:"updated_at"`
}

// L24StateManager owns the L2.4 observation schedule state.
// All public methods are safe for concurrent use.
type L24StateManager struct {
	mu     sync.Mutex
	path   string
	data   *L24Schedule
	logger *slog.Logger
}

// NewL24StateManager constructs a manager rooted at
// <workDir>/data/state/l2-4-schedule.json. Missing file is not
// an error: the manager starts with zero-value state and persists
// on the first write.
func NewL24StateManager(workDir string) *L24StateManager {
	m := &L24StateManager{
		path:   filepath.Join(workDir, "data/state/l2-4-schedule.json"),
		logger: slog.Default(),
	}
	// Best-effort load: do not fail construction on a missing or
	// malformed file. First successful write repairs the file.
	_ = m.load()
	return m
}

func (m *L24StateManager) load() error {
	data, err := os.ReadFile(m.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("l2_4_state: read %s: %w", m.path, err)
	}
	var s L24Schedule
	if err := json.Unmarshal(data, &s); err != nil {
		m.logger.Warn("l2_4_state: invalid JSON, starting fresh",
			"path", m.path,
			"err", err)
		return nil
	}
	m.data = &s
	return nil
}

func (m *L24StateManager) save() error {
	if m.data == nil {
		return nil
	}
	data, err := json.MarshalIndent(m.data, "", "  ")
	if err != nil {
		return fmt.Errorf("l2_4_state: marshal: %w", err)
	}
	// Ensure parent directory exists (data/state/) on first write.
	if err := os.MkdirAll(filepath.Dir(m.path), 0o755); err != nil {
		return fmt.Errorf("l2_4_state: mkdir: %w", err)
	}
	// Atomic write: write to .tmp then rename, so a crash mid-write
	// cannot leave the schedule file in a half-written state.
	tmp := m.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("l2_4_state: write tmp: %w", err)
	}
	if err := os.Rename(tmp, m.path); err != nil {
		return fmt.Errorf("l2_4_state: rename: %w", err)
	}
	return nil
}

// Get returns a copy of the current schedule state.
func (m *L24StateManager) Get() L24Schedule {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.data == nil {
		return L24Schedule{}
	}
	return *m.data
}

// Start marks the observation window as running. Returns an error
// if the window is already running. Effective period is
// override ?? default.
func (m *L24StateManager) Start() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.data == nil {
		m.data = &L24Schedule{}
	}
	if m.data.Status.Running {
		return fmt.Errorf("l2_4_state: already running")
	}
	period := m.data.Config.DefaultPeriodDays
	if m.data.Config.OverridePeriodDays > 0 {
		period = m.data.Config.OverridePeriodDays
	}
	now := time.Now()
	end := now.Add(time.Duration(period) * 24 * time.Hour)
	m.data.Status.Running = true
	m.data.Status.StartedAt = &now
	m.data.Status.EndsAt = &end
	m.data.Status.CurrentPeriodDays = period
	m.data.UpdatedAt = now
	return m.save()
}

// Stop marks the observation window as not running.
func (m *L24StateManager) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.data == nil || !m.data.Status.Running {
		return fmt.Errorf("l2_4_state: not running")
	}
	m.data.Status.Running = false
	m.data.UpdatedAt = time.Now()
	return m.save()
}

// ApplyOverride sets the operator override for start time and period.
// Validates HH:MM format and 1-30 day range.
func (m *L24StateManager) ApplyOverride(startTime string, periodDays int) error {
	if !timeHHMMRegex.MatchString(startTime) {
		return fmt.Errorf("l2_4_state: invalid time format %q (expected HH:MM)", startTime)
	}
	if periodDays < 1 || periodDays > 30 {
		return fmt.Errorf("l2_4_state: period %d out of range [1, 30]", periodDays)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.data == nil {
		m.data = &L24Schedule{}
	}
	m.data.Config.OverrideStartTime = startTime
	m.data.Config.OverridePeriodDays = periodDays
	m.data.UpdatedAt = time.Now()
	return m.save()
}

// Reset clears the operator override, restoring defaults.
// Status (running/stopped) is preserved.
func (m *L24StateManager) Reset() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.data == nil {
		return nil
	}
	m.data.Config.OverrideStartTime = ""
	m.data.Config.OverridePeriodDays = 0
	m.data.UpdatedAt = time.Now()
	return m.save()
}

// SetConfig updates the static config (defaults + auto_enabled) in
// the state file. Used at boot to mirror parameters.json into the
// runtime config.
func (m *L24StateManager) SetConfig(cfg L24ScheduleConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.data == nil {
		m.data = &L24Schedule{}
	}
	// Preserve overrides if any
	overrideStart := m.data.Config.OverrideStartTime
	overrideDays := m.data.Config.OverridePeriodDays
	m.data.Config = cfg
	m.data.Config.OverrideStartTime = overrideStart
	m.data.Config.OverridePeriodDays = overrideDays
	m.data.UpdatedAt = time.Now()
	return m.save()
}
