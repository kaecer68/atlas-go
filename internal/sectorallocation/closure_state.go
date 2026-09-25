// Package sectorallocation — SA11.A closure state manager.
//
// SACClosureStateManager tracks the runtime state of the sector allocation
// closure feature: whether it is enabled, observation window, session count,
// and dark-launch metrics. Clones the L2.4 / C07 state-manager pattern
// (file-backed atomic writes, concurrent-safe get/set).
package sectorallocation

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// SACClosureStatus is the persisted runtime state of the closure feature.
type SACClosureStatus struct {
	// Enabled mirrors ATLAS_SECTOR_ALLOCATION_CLOSURE_ENABLED at the
	// time the state was last saved. If env and state disagree, env wins.
	Enabled bool `json:"enabled"`

	// ObservationWindow tracks whether dark-launch observation is active.
	ObservationWindow ObservationWindow `json:"observation_window"`

	// SessionCount is the number of snapshot-producing sessions observed
	// since the observation window started. A session that only stored a
	// snapshot counts here even when nothing consumed the policy.
	SessionCount int `json:"session_count"`

	// AppliedSessionCount is the number of sessions whose policy was actually
	// consumed (applied=true with a ConsumptionReceipt). It was added in #1944
	// Batch 4 / N-A3 because SessionCount alone let the promotion gate pass with
	// zero genuinely applied sessions — a stored-but-unconsumed snapshot is not
	// an applied policy (spec §8.3). Promotion requires ≥20 of these.
	AppliedSessionCount int `json:"applied_session_count"`

	// LastReceiptID is the most recent mutation receipt recorded.
	LastReceiptID string `json:"last_receipt_id,omitempty"`

	// InvariantViolations is the number of invariant violations detected
	// during observation. Promotion requires 0 violations.
	InvariantViolations int `json:"invariant_violations"`

	// UpdatedAt records the last mutation time.
	UpdatedAt time.Time `json:"updated_at"`
}

// ObservationWindow tracks the dark-launch observation period.
type ObservationWindow struct {
	Running   bool       `json:"running"`
	StartedAt *time.Time `json:"started_at,omitempty"`
	Days      int        `json:"days,omitempty"`
}

// SACClosureStateManager owns the runtime state of the closure feature.
// All public methods are safe for concurrent use.
type SACClosureStateManager struct {
	mu     sync.Mutex
	path   string
	data   *SACClosureStatus
	logger *slog.Logger
}

const sacStateFileName = "sac_closure_state.json"

// NewSACClosureStateManager creates a manager rooted at
// <workDir>/data/state/sac_closure_state.json.
func NewSACClosureStateManager(workDir string) *SACClosureStateManager {
	m := &SACClosureStateManager{
		path:   filepath.Join(workDir, "data", "state", sacStateFileName),
		logger: slog.Default(),
	}
	_ = m.load()
	return m
}

func (m *SACClosureStateManager) load() error {
	data, err := os.ReadFile(m.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("sac_closure_state: read %s: %w", m.path, err)
	}
	var s SACClosureStatus
	if err := json.Unmarshal(data, &s); err != nil {
		m.logger.Warn("sac_closure_state: invalid JSON, starting fresh",
			"path", m.path,
			"err", err)
		return nil
	}
	m.data = &s
	return nil
}

func (m *SACClosureStateManager) save() error {
	if m.data == nil {
		return nil
	}
	m.data.UpdatedAt = time.Now()
	data, err := json.MarshalIndent(m.data, "", "  ")
	if err != nil {
		return fmt.Errorf("sac_closure_state: marshal: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(m.path), 0o755); err != nil {
		return fmt.Errorf("sac_closure_state: mkdir: %w", err)
	}
	tmp := m.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("sac_closure_state: write tmp: %w", err)
	}
	if err := os.Rename(tmp, m.path); err != nil {
		return fmt.Errorf("sac_closure_state: rename: %w", err)
	}
	return nil
}

// Get returns a copy of the current state.
func (m *SACClosureStateManager) Get() SACClosureStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.data == nil {
		return SACClosureStatus{}
	}
	return *m.data
}

// SetEnabled updates the enabled flag and persists.
func (m *SACClosureStateManager) SetEnabled(v bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.data == nil {
		m.data = &SACClosureStatus{}
	}
	m.data.Enabled = v
	return m.save()
}

// StartObservation begins the dark-launch observation window.
func (m *SACClosureStateManager) StartObservation(days int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.data == nil {
		m.data = &SACClosureStatus{}
	}
	if m.data.ObservationWindow.Running {
		return fmt.Errorf("sac_closure_state: observation already running")
	}
	now := time.Now()
	m.data.ObservationWindow = ObservationWindow{
		Running:   true,
		StartedAt: &now,
		Days:      days,
	}
	m.data.SessionCount = 0
	m.data.InvariantViolations = 0
	return m.save()
}

// StopObservation ends the observation window.
func (m *SACClosureStateManager) StopObservation() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.data == nil || !m.data.ObservationWindow.Running {
		return fmt.Errorf("sac_closure_state: observation not running")
	}
	m.data.ObservationWindow.Running = false
	return m.save()
}

// RecordSession increments the session counter and records the receipt.
//
// This counts snapshot-producing sessions, NOT applied policies: see
// RecordAppliedSession and IsPromotable (N-A3).
func (m *SACClosureStateManager) RecordSession(receiptID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.data == nil {
		m.data = &SACClosureStatus{}
	}
	m.data.SessionCount++
	m.data.LastReceiptID = receiptID
	return m.save()
}

// RecordAppliedSession increments the applied-session counter. Call it only with
// consumption evidence (a ConsumptionReceipt), i.e. when the rotation really was
// applied. Promotion requires ≥20 applied sessions (N-A3).
func (m *SACClosureStateManager) RecordAppliedSession(receiptID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.data == nil {
		m.data = &SACClosureStatus{}
	}
	m.data.AppliedSessionCount++
	m.data.LastReceiptID = receiptID
	return m.save()
}

// RecordInvariantViolation increments the violation counter.
func (m *SACClosureStateManager) RecordInvariantViolation() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.data == nil {
		m.data = &SACClosureStatus{}
	}
	m.data.InvariantViolations++
	return m.save()
}

// IsPromotable returns true when conditions for promotion are met:
// ≥20 sessions, ≥20 of them actually applied, 0 violations, observation
// completed.
//
// The applied requirement is N-A3 (#1944 Batch 4): before it, recording 20
// stored-but-unconsumed snapshots was enough to satisfy the gate, even though
// not a single policy had been consumed.
func (m *SACClosureStateManager) IsPromotable() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.data == nil {
		return false
	}
	return m.data.SessionCount >= 20 &&
		m.data.AppliedSessionCount >= 20 &&
		m.data.InvariantViolations == 0 &&
		!m.data.ObservationWindow.Running
}
