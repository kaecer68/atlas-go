package prism

import (
	"fmt"
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
				Start:     time.Now().AddDate(0, 0, -90),
				End:       time.Now(),
				Regime:    RegimeRiskOn,
				RegimeSet: true,
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
					Start:     time.Now().AddDate(0, 0, -30),
					End:       time.Now(),
					Regime:    RegimeRiskOn,
					RegimeSet: true,
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
				Start:     time.Now().AddDate(0, 0, -30),
				End:       time.Now(),
				Regime:    RegimeRiskOn,
				RegimeSet: true,
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

func TestPRISMManagerStartStop(t *testing.T) {
	t.Run("start_twice_is_idempotent", func(t *testing.T) {
		pm := NewPRISMManager(DefaultPRISMConfig())
		pm.Start()
		pm.Start() // must not panic or double-start workers
		pm.Stop()
	})
}

// TestAutoBalanceEnabled verifies the BTM migration gate (Issue #1447):
// AutoBalanceEnabled must reflect the config so cmd/atlas only registers the
// prism_auto_balancer BackgroundTaskManager task when rebalancing is on.
func TestAutoBalanceEnabled(t *testing.T) {
	t.Run("default_config_enables_autobalance", func(t *testing.T) {
		pm := NewPRISMManager(DefaultPRISMConfig())
		if !pm.AutoBalanceEnabled() {
			t.Fatalf("DefaultPRISMConfig AutoBalance=true, AutoBalanceEnabled() = false")
		}
	})
	t.Run("disabled_config_reports_false", func(t *testing.T) {
		cfg := DefaultPRISMConfig()
		cfg.AutoBalance = false
		pm := NewPRISMManager(cfg)
		if pm.AutoBalanceEnabled() {
			t.Fatalf("AutoBalance=false, AutoBalanceEnabled() = true")
		}
	})
}

func TestPRISMManagerGetTask(t *testing.T) {
	pm := NewPRISMManager(DefaultPRISMConfig())
	agent := domain.AgentSpec{ID: "agt", Skill: "s", Layer: domain.LayerSector}
	_ = pm.ScheduleTraining(agent, []TrainingWindow{
		{Start: time.Now(), End: time.Now().Add(time.Hour), Regime: RegimeRiskOn, RegimeSet: true},
	})
}

func TestPRISMManagerUpdateTaskStatus(t *testing.T) {
	pm := NewPRISMManager(DefaultPRISMConfig())
	queue := pm.queues[int(RegimeRiskOn)]

	// Enqueue a task directly to avoid auto-generated IDs.
	task := &TrainingTask{
		ID:      "direct-task-001",
		AgentID: "agt",
		Status:  TaskPending,
	}
	if err := queue.Enqueue(task); err != nil {
		t.Fatalf("enqueue failed: %v", err)
	}

	dequeued, ok := queue.Dequeue()
	if !ok {
		t.Fatal("expected task from queue")
	}
	dequeued.Result = &TrainingResult{}
	queue.UpdateTaskStatus(dequeued.ID, TaskFailed, &TrainingResult{Error: "test failure"})

	// After dequeueing, GetTask by the original ID should return false.
	_, ok = queue.GetTask(dequeued.ID)
	if ok {
		t.Error("dequeued task should not be found by GetTask")
	}
}

func TestPRISMManagerGetAllTasks(t *testing.T) {
	pm := NewPRISMManager(DefaultPRISMConfig())
	agent := domain.AgentSpec{ID: "agt", Skill: "s", Layer: domain.LayerSector}
	_ = pm.ScheduleTraining(agent, []TrainingWindow{
		{Start: time.Now(), End: time.Now().Add(time.Hour), Regime: RegimeRiskOn, RegimeSet: true},
	})

	tasks := pm.queues[int(RegimeRiskOn)].GetAllTasks()
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}
	if tasks[0].AgentID != "agt" {
		t.Errorf("expected agent agt, got %s", tasks[0].AgentID)
	}
}

func TestPRISMManagerWithExecutor(t *testing.T) {
	pm := NewPRISMManager(DefaultPRISMConfig())
	mock := &stubExecutor{}
	pm.WithExecutor(mock)
	pm.mu.RLock()
	ex := pm.executor
	pm.mu.RUnlock()
	if ex == nil {
		t.Fatal("expected executor to be attached")
	}
}

func TestPRISMManagerCompletedResults(t *testing.T) {
	pm := NewPRISMManager(DefaultPRISMConfig())
	if len(pm.GetCompletedResults()) != 0 {
		t.Error("expected empty results initially")
	}

	pm.recordCompletedResult(&TrainingTask{ID: "t1", AgentID: "a1", Regime: RegimeRiskOn}, TrainingResult{HitRate: 0.8})
	results := pm.GetCompletedResults()
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].AgentID != "a1" {
		t.Errorf("expected agent a1, got %s", results[0].AgentID)
	}

	pm.ClearCompletedResults()
	if len(pm.GetCompletedResults()) != 0 {
		t.Error("expected empty results after clear")
	}
}

func TestClassifyRegime(t *testing.T) {
	pm := NewPRISMManager(DefaultPRISMConfig())

	// With explicit regime override, return it directly.
	tw := TrainingWindow{Start: time.Now(), End: time.Now(), Regime: RegimeRiskOff, RegimeSet: true}
	if got := pm.classifyRegime(tw); got != RegimeRiskOff {
		t.Fatalf("expected RiskOff override, got %v", got)
	}

	// Without explicit regime, default to Transition.
	twNoOverride := TrainingWindow{Start: time.Now(), End: time.Now()}
	if got := pm.classifyRegime(twNoOverride); got != RegimeTransition {
		t.Fatalf("expected Transition default, got %v", got)
	}
}

func TestExecuteTraining(t *testing.T) {
	t.Run("with_executor_returns_real_result", func(t *testing.T) {
		pm := NewPRISMManager(DefaultPRISMConfig())
		pm.WithExecutor(&stubExecutor{sharpe: 1.5})
		task := &TrainingTask{ID: "t1", AgentID: "a1", Regime: RegimeRiskOn}
		result := pm.executeTraining(task)
		if result.Error != "" {
			t.Fatalf("expected no error, got %s", result.Error)
		}
		if result.Synthetic {
			t.Fatal("expected non-synthetic result with executor")
		}
		if result.SharpeRatio != 1.5 {
			t.Errorf("expected sharpe 1.5, got %f", result.SharpeRatio)
		}
	})

	t.Run("with_failing_executor_returns_error", func(t *testing.T) {
		pm := NewPRISMManager(DefaultPRISMConfig())
		pm.WithExecutor(&stubExecutor{fail: true})
		task := &TrainingTask{ID: "t1", AgentID: "a1", Regime: RegimeRiskOn}
		result := pm.executeTraining(task)
		if result.Error == "" {
			t.Fatal("expected error from failing executor")
		}
	})

	t.Run("without_executor_returns_synthetic_result", func(t *testing.T) {
		pm := NewPRISMManager(DefaultPRISMConfig())
		task := &TrainingTask{ID: "t1", AgentID: "a1", Regime: RegimeRiskOn}
		result := pm.executeTraining(task)
		if !result.Synthetic {
			t.Fatal("expected Synthetic result with no executor")
		}
		if result.Error == "" {
			t.Fatal("expected error message in synthetic result")
		}
	})
}

// stubExecutor implements TrainingExecutor for tests.
type stubExecutor struct {
	sharpe float64
	fail   bool
}

func (s *stubExecutor) Run(task TrainingTask) (TrainingResult, error) {
	if s.fail {
		return TrainingResult{}, fmt.Errorf("stub failure")
	}
	return TrainingResult{
		SharpeRatio:  s.sharpe,
		HitRate:      0.6,
		SignalsCount: 10,
	}, nil
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
