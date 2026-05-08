package prism

import (
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

func TestPRISMManager(t *testing.T) {
	t.Run("NewPRISMManager", func(t *testing.T) {
		config := DefaultPRISMConfig()
		manager := NewPRISMManager(config)

		if manager == nil {
			t.Fatal("Expected non-nil manager")
		}

		if len(manager.queues) != int(RegimeCount) {
			t.Errorf("Expected %d queues, got %d", int(RegimeCount), len(manager.queues))
		}
	})

	t.Run("RegimeTypes", func(t *testing.T) {
		regimes := []RegimeType{
			RegimeRiskOn,
			RegimeRiskOff,
			RegimeHighVolatility,
			RegimeLowVolatility,
			RegimeTransition,
		}

		for _, r := range regimes {
			name := r.String()
			if name == "Unknown" {
				t.Errorf("Regime type %d should have a known name", r)
			}
		}

		if int(RegimeCount) != 5 {
			t.Errorf("Expected 5 regime types, got %d", int(RegimeCount))
		}
	})

	t.Run("ScheduleTraining", func(t *testing.T) {
		config := DefaultPRISMConfig()
		manager := NewPRISMManager(config)

		agent := domain.AgentSpec{
			ID:    "test_agent_001",
			Skill: "test_skill",
			Layer: domain.LayerSector,
		}

		windows := []TrainingWindow{
			{
				Start:  time.Now().AddDate(0, 0, -90),
				End:    time.Now(),
				Regime: RegimeRiskOn,
			},
		}

		err := manager.ScheduleTraining(agent, windows)
		if err != nil {
			t.Fatalf("Failed to schedule training: %v", err)
		}

		// Check queue stats to verify task was added
		stats := manager.GetQueueStats()
		totalTasks := 0
		for _, s := range stats {
			totalTasks += s.Size
		}
		if totalTasks == 0 {
			t.Error("Expected at least one task in queues after scheduling")
		}
	})

	t.Run("GetQueueStats", func(t *testing.T) {
		config := DefaultPRISMConfig()
		manager := NewPRISMManager(config)

		stats := manager.GetQueueStats()
		if len(stats) != int(RegimeCount) {
			t.Errorf("Expected %d queue stats, got %d", int(RegimeCount), len(stats))
		}

		// All queues should have zero size initially
		for _, stat := range stats {
			if stat.Size != 0 {
				t.Errorf("Expected 0 initial size for regime %s, got %d", stat.Regime, stat.Size)
			}
		}
	})

	t.Run("GetOverallStats", func(t *testing.T) {
		config := DefaultPRISMConfig()
		manager := NewPRISMManager(config)

		stats := manager.GetOverallStats()
		if stats.TotalTasks != 0 {
			t.Errorf("Expected 0 total tasks initially, got %d", stats.TotalTasks)
		}
	})

	t.Run("Rebalance", func(t *testing.T) {
		config := DefaultPRISMConfig()
		manager := NewPRISMManager(config)

		// Add tasks to only one queue to create imbalance
		agent := domain.AgentSpec{ID: "test_agent", Skill: "test", Layer: domain.LayerSector}
		for range 10 {
			windows := []TrainingWindow{
				{
					Start:  time.Now().AddDate(0, 0, -30),
					End:    time.Now(),
					Regime: RegimeRiskOn,
				},
			}
			manager.ScheduleTraining(agent, windows)
		}

		// Rebalance should not panic
		manager.Rebalance()

		// Verify stats
		stats := manager.GetQueueStats()
		t.Logf("Queue stats after rebalance: %+v", stats)
	})

	t.Run("ClearQueue", func(t *testing.T) {
		config := DefaultPRISMConfig()
		manager := NewPRISMManager(config)

		agent := domain.AgentSpec{ID: "test_agent", Skill: "test", Layer: domain.LayerSector}
		windows := []TrainingWindow{
			{
				Start:  time.Now().AddDate(0, 0, -30),
				End:    time.Now(),
				Regime: RegimeRiskOn,
			},
		}
		manager.ScheduleTraining(agent, windows)

		manager.ClearQueue(RegimeRiskOn)

		stats := manager.GetQueueStats()
		for _, s := range stats {
			if s.Regime == RegimeRiskOn && s.Size != 0 {
				t.Errorf("Expected cleared Risk-On queue, got size %d", s.Size)
			}
		}
	})
}

func TestTrainingTask(t *testing.T) {
	t.Run("TaskCreation", func(t *testing.T) {
		task := &TrainingTask{
			ID:         "task_001",
			AgentID:    "agent_001",
			AgentSkill: "test_skill",
			Status:     TaskPending,
			CreatedAt:  time.Now(),
		}

		if task.ID != "task_001" {
			t.Errorf("Task ID mismatch")
		}

		if task.Status != TaskPending {
			t.Errorf("Expected status pending, got %d", task.Status)
		}
	})

	t.Run("TaskStatusValues", func(t *testing.T) {
		// Verify task status constants are distinct
		statuses := []TaskStatus{
			TaskPending,
			TaskQueued,
			TaskRunning,
			TaskCompleted,
			TaskFailed,
			TaskCancelled,
		}

		seen := make(map[TaskStatus]bool)
		for _, s := range statuses {
			if seen[s] {
				t.Errorf("Duplicate task status value: %d", s)
			}
			seen[s] = true
		}
	})
}

func TestTrainingQueue(t *testing.T) {
	t.Run("EnqueueDequeue", func(t *testing.T) {
		queue := NewTrainingQueue(RegimeRiskOn, 10, 2)

		task := &TrainingTask{
			ID:       "task_001",
			AgentID:  "agent_001",
			Priority: 5,
			Status:   TaskPending,
		}

		err := queue.Enqueue(task)
		if err != nil {
			t.Fatalf("Enqueue failed: %v", err)
		}

		if queue.Len() != 1 {
			t.Errorf("Expected queue length 1, got %d", queue.Len())
		}

		dequeued, ok := queue.Dequeue()
		if !ok {
			t.Fatal("Dequeue returned false")
		}

		if dequeued.ID != "task_001" {
			t.Errorf("Expected task_001, got %s", dequeued.ID)
		}
	})

	t.Run("QueueCapacity", func(t *testing.T) {
		queue := NewTrainingQueue(RegimeRiskOff, 2, 1)

		queue.Enqueue(&TrainingTask{ID: "t1", Priority: 1})
		queue.Enqueue(&TrainingTask{ID: "t2", Priority: 2})

		err := queue.Enqueue(&TrainingTask{ID: "t3", Priority: 3})
		if err == nil {
			t.Error("Expected error when queue is full")
		}
	})

	t.Run("Clear", func(t *testing.T) {
		queue := NewTrainingQueue(RegimeHighVolatility, 10, 1)
		queue.Enqueue(&TrainingTask{ID: "t1"})
		queue.Enqueue(&TrainingTask{ID: "t2"})

		queue.Clear()

		if queue.Len() != 0 {
			t.Errorf("Expected empty queue after clear, got %d", queue.Len())
		}
	})
}
