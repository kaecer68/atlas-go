package taskstate

import (
	"testing"
)

func TestManager(t *testing.T) {
	tmpDir := t.TempDir()
	mgr, err := NewManager(tmpDir)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	// Test CreateTask
	state := mgr.CreateTask("task-1", "Test task")
	if state.TaskID != "task-1" {
		t.Errorf("expected task-1, got %s", state.TaskID)
	}

	// Test HasToolBeenCalled (should be false initially)
	if mgr.HasToolBeenCalled("task-1", "grep", "pattern") {
		t.Error("expected false for uncalled tool")
	}

	// Test RecordToolCall
	if err := mgr.RecordToolCall("task-1", "grep", "pattern", "found 5 results"); err != nil {
		t.Fatalf("RecordToolCall: %v", err)
	}

	// Test HasToolBeenCalled (should be true now)
	if !mgr.HasToolBeenCalled("task-1", "grep", "pattern") {
		t.Error("expected true for called tool")
	}

	// Test GetTaskState
	retrieved, err := mgr.GetTaskState("task-1")
	if err != nil {
		t.Fatalf("GetTaskState: %v", err)
	}
	if len(retrieved.ToolCalls) != 1 {
		t.Errorf("expected 1 tool call, got %d", len(retrieved.ToolCalls))
	}

	// Test MarkComplete
	if err := mgr.MarkComplete("task-1", "Done"); err != nil {
		t.Fatalf("MarkComplete: %v", err)
	}

	completed, _ := mgr.GetTaskState("task-1")
	if !completed.IsComplete {
		t.Error("expected task to be complete")
	}

	// Test persistence
	mgr2, err := NewManager(tmpDir)
	if err != nil {
		t.Fatalf("NewManager reload: %v", err)
	}

	if !mgr2.HasToolBeenCalled("task-1", "grep", "pattern") {
		t.Error("expected persisted tool call to be found")
	}
}

func TestIsTaskSimilar(t *testing.T) {
	tmpDir := t.TempDir()
	mgr, _ := NewManager(tmpDir)

	mgr.CreateTask("task-1", "Analyze market data")

	if !mgr.IsTaskSimilar("Analyze market data", 30) {
		t.Error("expected similar task to be found")
	}

	if mgr.IsTaskSimilar("Different task", 30) {
		t.Error("expected different task to not match")
	}
}

func TestDuplicateToolCallPrevention(t *testing.T) {
	tmpDir := t.TempDir()
	mgr, _ := NewManager(tmpDir)

	mgr.CreateTask("task-1", "Test deduplication")

	// First call should succeed
	if err := mgr.RecordToolCall("task-1", "grep", "func.*New", "results"); err != nil {
		t.Fatalf("first call: %v", err)
	}

	// Check it's recorded
	if !mgr.HasToolBeenCalled("task-1", "grep", "func.*New") {
		t.Error("expected first call to be recorded")
	}

	// The system should prevent second call with same params
	// This is enforced by the agent prompt, not the code
	// But we verify the tracking works
}
