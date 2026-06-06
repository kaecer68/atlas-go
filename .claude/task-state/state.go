// Package taskstate provides task execution tracking to prevent infinite loops
// in agent dispatch scenarios.
package taskstate

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// ToolCall records a single tool invocation
type ToolCall struct {
	Tool      string    `json:"tool"`
	Params    string    `json:"params"`
	Result    string    `json:"result"`
	Timestamp time.Time `json:"timestamp"`
}

// TaskState tracks the execution state of a dispatched task
type TaskState struct {
	TaskID        string     `json:"task_id"`
	Description   string     `json:"description"`
	ToolCalls     []ToolCall `json:"tool_calls"`
	StartTime     time.Time  `json:"start_time"`
	LastUpdate    time.Time  `json:"last_update"`
	IsComplete    bool       `json:"is_complete"`
	CompletionMsg string     `json:"completion_msg,omitempty"`
}

// Manager handles task state persistence and queries
type Manager struct {
	storageDir string
	mu         sync.RWMutex
	states     map[string]*TaskState
}

// NewManager creates a new task state manager
func NewManager(storageDir string) (*Manager, error) {
	if err := os.MkdirAll(storageDir, 0o755); err != nil {
		return nil, fmt.Errorf("create storage dir: %w", err)
	}

	m := &Manager{
		storageDir: storageDir,
		states:     make(map[string]*TaskState),
	}

	// Load existing states
	if err := m.loadAll(); err != nil {
		return nil, fmt.Errorf("load existing states: %w", err)
	}

	return m, nil
}

// CreateTask initializes tracking for a new task
func (m *Manager) CreateTask(taskID, description string) *TaskState {
	m.mu.Lock()
	defer m.mu.Unlock()

	state := &TaskState{
		TaskID:      taskID,
		Description: description,
		ToolCalls:   make([]ToolCall, 0),
		StartTime:   time.Now(),
		LastUpdate:  time.Now(),
	}

	m.states[taskID] = state
	m.persist(state)

	return state
}

// HasToolBeenCalled checks if a specific tool+params combination was already used
func (m *Manager) HasToolBeenCalled(taskID, tool, params string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	state, ok := m.states[taskID]
	if !ok {
		return false
	}

	for _, call := range state.ToolCalls {
		if call.Tool == tool && call.Params == params {
			return true
		}
	}

	return false
}

// RecordToolCall logs a tool invocation for a task
func (m *Manager) RecordToolCall(taskID, tool, params, result string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	state, ok := m.states[taskID]
	if !ok {
		return fmt.Errorf("task %s not found", taskID)
	}

	call := ToolCall{
		Tool:      tool,
		Params:    params,
		Result:    result,
		Timestamp: time.Now(),
	}

	state.ToolCalls = append(state.ToolCalls, call)
	state.LastUpdate = time.Now()

	return m.persist(state)
}

// GetTaskState retrieves the current state of a task
func (m *Manager) GetTaskState(taskID string) (*TaskState, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	state, ok := m.states[taskID]
	if !ok {
		return nil, fmt.Errorf("task %s not found", taskID)
	}

	return state, nil
}

// MarkComplete marks a task as completed
func (m *Manager) MarkComplete(taskID, completionMsg string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	state, ok := m.states[taskID]
	if !ok {
		return fmt.Errorf("task %s not found", taskID)
	}

	state.IsComplete = true
	state.CompletionMsg = completionMsg
	state.LastUpdate = time.Now()

	return m.persist(state)
}

// GetRecentTasks returns tasks from the last N minutes
func (m *Manager) GetRecentTasks(minutes int) []*TaskState {
	m.mu.RLock()
	defer m.mu.RUnlock()

	cutoff := time.Now().Add(-time.Duration(minutes) * time.Minute)
	var recent []*TaskState

	for _, state := range m.states {
		if state.StartTime.After(cutoff) {
			recent = append(recent, state)
		}
	}

	return recent
}

// IsTaskSimilar checks if a similar task was recently dispatched
func (m *Manager) IsTaskSimilar(description string, minutes int) bool {
	recent := m.GetRecentTasks(minutes)

	for _, task := range recent {
		// Simple string similarity - in production, use Levenshtein distance
		if task.Description == description {
			return true
		}
	}

	return false
}

// persist saves a task state to disk
func (m *Manager) persist(state *TaskState) error {
	path := filepath.Join(m.storageDir, state.TaskID+".json")
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write state file: %w", err)
	}

	return nil
}

// loadAll loads all existing task states from disk
func (m *Manager) loadAll() error {
	entries, err := os.ReadDir(m.storageDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read storage dir: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		path := filepath.Join(m.storageDir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue // Skip unreadable files
		}

		var state TaskState
		if err := json.Unmarshal(data, &state); err != nil {
			continue // Skip invalid files
		}

		m.states[state.TaskID] = &state
	}

	return nil
}
