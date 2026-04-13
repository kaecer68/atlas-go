// Package prism implements PRISM (Parallel Regime-Specific Independent Systems for Multi-cohort training)
// Manages 5 independent training queues for different market regimes
package prism

import (
	"container/list"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// RegimeType represents distinct market regimes
type RegimeType int

const (
	RegimeRiskOn         RegimeType = iota // Bull market, expansion phase
	RegimeRiskOff                          // Bear market, contraction phase
	RegimeHighVolatility                   // High volatility, crisis mode
	RegimeLowVolatility                    // Low volatility, range-bound
	RegimeTransition                       // Regime change, uncertainty
	RegimeCount                            // Total number of regimes
)

// String returns human-readable regime name
func (r RegimeType) String() string {
	switch r {
	case RegimeRiskOn:
		return "Risk-On"
	case RegimeRiskOff:
		return "Risk-Off"
	case RegimeHighVolatility:
		return "High-Vol"
	case RegimeLowVolatility:
		return "Low-Vol"
	case RegimeTransition:
		return "Transition"
	default:
		return "Unknown"
	}
}

// TrainingTask represents a single training unit for an agent
type TrainingTask struct {
	ID          string
	AgentID     string
	AgentSkill  string
	WindowStart time.Time
	WindowEnd   time.Time
	Regime      RegimeType
	Priority    int
	Status      TaskStatus
	CreatedAt   time.Time
	StartedAt   *time.Time
	CompletedAt *time.Time
	Result      *TrainingResult
	RetryCount  int
}

// TaskStatus represents training task lifecycle
type TaskStatus int

const (
	TaskPending TaskStatus = iota
	TaskQueued
	TaskRunning
	TaskCompleted
	TaskFailed
	TaskCancelled
)

// TrainingResult contains outcome of training
type TrainingResult struct {
	HitRate      float64
	SharpeRatio  float64
	MaxDrawdown  float64
	TotalReturn  float64
	SignalsCount int
	WinCount     int
	LossCount    int
	Error        string
	Duration     time.Duration
}

// TrainingQueue manages a regime-specific training queue
type TrainingQueue struct {
	Regime   RegimeType
	tasks    *list.List
	taskByID map[string]*list.Element
	mu       sync.RWMutex
	maxSize  int
	workers  int
}

// NewTrainingQueue creates a new regime-specific queue
func NewTrainingQueue(regime RegimeType, maxSize int, workers int) *TrainingQueue {
	return &TrainingQueue{
		Regime:   regime,
		tasks:    list.New(),
		taskByID: make(map[string]*list.Element),
		maxSize:  maxSize,
		workers:  workers,
	}
}

// Enqueue adds a task to the queue
func (q *TrainingQueue) Enqueue(task *TrainingTask) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.tasks.Len() >= q.maxSize {
		return fmt.Errorf("queue %s is full (max %d)", q.Regime, q.maxSize)
	}

	task.Status = TaskQueued
	elem := q.tasks.PushBack(task)
	q.taskByID[task.ID] = elem

	return nil
}

// Dequeue removes and returns the highest priority task
func (q *TrainingQueue) Dequeue() (*TrainingTask, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.tasks.Len() == 0 {
		return nil, false
	}

	// Find highest priority task
	var highest *list.Element
	highestPriority := -1

	for elem := q.tasks.Front(); elem != nil; elem = elem.Next() {
		task := elem.Value.(*TrainingTask)
		if task.Status == TaskQueued && task.Priority > highestPriority {
			highestPriority = task.Priority
			highest = elem
		}
	}

	if highest == nil {
		return nil, false
	}

	task := highest.Value.(*TrainingTask)
	task.Status = TaskRunning
	now := time.Now()
	task.StartedAt = &now

	q.tasks.Remove(highest)
	delete(q.taskByID, task.ID)

	return task, true
}

// GetTask retrieves a task by ID
func (q *TrainingQueue) GetTask(taskID string) (*TrainingTask, bool) {
	q.mu.RLock()
	defer q.mu.RUnlock()

	if elem, ok := q.taskByID[taskID]; ok {
		return elem.Value.(*TrainingTask), true
	}
	return nil, false
}

// UpdateTaskStatus updates task status
func (q *TrainingQueue) UpdateTaskStatus(taskID string, status TaskStatus, result *TrainingResult) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if task, ok := q.taskByID[taskID]; ok {
		t := task.Value.(*TrainingTask)
		t.Status = status
		if result != nil {
			t.Result = result
		}
		if status == TaskCompleted || status == TaskFailed {
			now := time.Now()
			t.CompletedAt = &now
			if t.StartedAt != nil {
				result.Duration = now.Sub(*t.StartedAt)
			}
		}
	}
}

// Len returns current queue length
func (q *TrainingQueue) Len() int {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return q.tasks.Len()
}

// Clear removes all tasks from queue
func (q *TrainingQueue) Clear() {
	q.mu.Lock()
	defer q.mu.Unlock()

	q.tasks.Init()
	q.taskByID = make(map[string]*list.Element)
}

// GetAllTasks returns all tasks in queue
func (q *TrainingQueue) GetAllTasks() []*TrainingTask {
	q.mu.RLock()
	defer q.mu.RUnlock()

	tasks := make([]*TrainingTask, 0, q.tasks.Len())
	for elem := q.tasks.Front(); elem != nil; elem = elem.Next() {
		tasks = append(tasks, elem.Value.(*TrainingTask))
	}
	return tasks
}

// CompletedTrainingResult pairs a regime with its completed training result.
type CompletedTrainingResult struct {
	AgentID    string
	AgentSkill string
	Regime     RegimeType
	Result     TrainingResult
}

// TrainingExecutor executes a single training task and returns real metrics.
type TrainingExecutor interface {
	Execute(task TrainingTask) (TrainingResult, error)
}

// PRISMManager manages all 5 regime-specific training queues
type PRISMManager struct {
	queues    [RegimeCount]*TrainingQueue
	mu        sync.RWMutex
	isRunning bool
	stopCh    chan struct{}
	config    PRISMConfig
	executor  TrainingExecutor

	// Metrics
	totalTasks     int
	completedTasks int
	failedTasks    int

	// Completed result buffer for JANUS/meta-layer consumption
	completedResults []CompletedTrainingResult
	resultMu         sync.RWMutex
	maxResults       int
}

// PRISMConfig holds configuration for PRISM
type PRISMConfig struct {
	QueueSize           int
	WorkersPerQueue     int
	AutoBalance         bool
	PrioritizeNewAgents bool
}

// DefaultPRISMConfig returns recommended defaults
func DefaultPRISMConfig() PRISMConfig {
	return PRISMConfig{
		QueueSize:           100,
		WorkersPerQueue:     2,
		AutoBalance:         true,
		PrioritizeNewAgents: true,
	}
}

// NewPRISMManager creates a new PRISM manager with 5 regime queues
func NewPRISMManager(config PRISMConfig) *PRISMManager {
	pm := &PRISMManager{
		stopCh:     make(chan struct{}),
		config:     config,
		maxResults: 1000,
	}

	// Initialize 5 regime queues
	for i := 0; i < int(RegimeCount); i++ {
		regime := RegimeType(i)
		pm.queues[i] = NewTrainingQueue(regime, config.QueueSize, config.WorkersPerQueue)
	}

	return pm
}

// WithExecutor attaches a real training executor to the manager.
func (pm *PRISMManager) WithExecutor(ex TrainingExecutor) *PRISMManager {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.executor = ex
	return pm
}

// Start begins processing all queues
func (pm *PRISMManager) Start() {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if pm.isRunning {
		return
	}

	pm.isRunning = true
	pm.stopCh = make(chan struct{})

	// Start worker goroutines for each queue
	for i := 0; i < int(RegimeCount); i++ {
		for j := 0; j < pm.config.WorkersPerQueue; j++ {
			go pm.worker(pm.queues[i], pm.stopCh)
		}
	}

	// Start auto-balancer if enabled
	if pm.config.AutoBalance {
		go pm.autoBalancer(pm.stopCh)
	}

	log.Println("[PRISM] Started with 5 regime queues")
}

// Stop halts all queue processing
func (pm *PRISMManager) Stop() {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if !pm.isRunning {
		return
	}

	pm.isRunning = false
	close(pm.stopCh)

	log.Println("[PRISM] Stopped")
}

// ScheduleTraining schedules an agent for training in appropriate regime queues
func (pm *PRISMManager) ScheduleTraining(agent domain.AgentSpec, windows []TrainingWindow) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	for _, window := range windows {
		regime := pm.classifyRegime(window)

		task := &TrainingTask{
			ID:          generateTaskID(agent.ID, window),
			AgentID:     agent.ID,
			AgentSkill:  agent.Skill,
			WindowStart: window.Start,
			WindowEnd:   window.End,
			Regime:      regime,
			Priority:    pm.calculatePriority(agent),
			Status:      TaskPending,
			CreatedAt:   time.Now(),
		}

		queue := pm.queues[int(regime)]
		if err := queue.Enqueue(task); err != nil {
			return fmt.Errorf("failed to schedule for regime %s: %w", regime, err)
		}

		pm.totalTasks++
	}

	return nil
}

// GetQueueStats returns statistics for all queues
func (pm *PRISMManager) GetQueueStats() []QueueStats {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	stats := make([]QueueStats, RegimeCount)
	for i := 0; i < int(RegimeCount); i++ {
		queue := pm.queues[i]
		stats[i] = QueueStats{
			Regime:  queue.Regime,
			Size:    queue.Len(),
			MaxSize: queue.maxSize,
			Workers: queue.workers,
		}
	}

	return stats
}

// GetOverallStats returns system-wide statistics
func (pm *PRISMManager) GetOverallStats() PRISMStats {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	return PRISMStats{
		TotalTasks:     pm.totalTasks,
		CompletedTasks: pm.completedTasks,
		FailedTasks:    pm.failedTasks,
		ActiveQueues:   pm.countActiveQueues(),
	}
}

// ClearQueue removes all tasks from a specific regime queue
func (pm *PRISMManager) ClearQueue(regime RegimeType) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if int(regime) < int(RegimeCount) {
		pm.queues[int(regime)].Clear()
		log.Printf("[PRISM] Cleared queue for regime %s", regime)
	}
}

// Rebalance redistributes tasks based on current queue loads
func (pm *PRISMManager) Rebalance() {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	// Calculate average load
	totalLoad := 0
	for i := 0; i < int(RegimeCount); i++ {
		totalLoad += pm.queues[i].Len()
	}
	avgLoad := totalLoad / int(RegimeCount)

	// Log imbalances
	for i := 0; i < int(RegimeCount); i++ {
		queue := pm.queues[i]
		deviation := queue.Len() - avgLoad
		if abs(deviation) > avgLoad/2 {
			log.Printf("[PRISM] Queue %s load imbalance: %d (avg: %d)",
				queue.Regime, queue.Len(), avgLoad)
		}
	}
}

// worker processes tasks from a single queue
func (pm *PRISMManager) worker(queue *TrainingQueue, stopCh <-chan struct{}) {
	for {
		select {
		case <-stopCh:
			return
		default:
		}

		task, ok := queue.Dequeue()
		if !ok {
			time.Sleep(100 * time.Millisecond)
			continue
		}

		// Execute training
		result := pm.executeTraining(task)

		// Update status
		if result.Error == "" {
			queue.UpdateTaskStatus(task.ID, TaskCompleted, result)
			pm.mu.Lock()
			pm.completedTasks++
			pm.mu.Unlock()
			pm.recordCompletedResult(task, *result)
		} else {
			queue.UpdateTaskStatus(task.ID, TaskFailed, result)
			pm.mu.Lock()
			pm.failedTasks++
			pm.mu.Unlock()
		}
	}
}

// executeTraining performs actual training (placeholder - integrate with backtest)
func (pm *PRISMManager) executeTraining(task *TrainingTask) *TrainingResult {
	// If a real executor is attached, use it.
	pm.mu.RLock()
	ex := pm.executor
	pm.mu.RUnlock()
	if ex != nil {
		result, err := ex.Execute(*task)
		if err == nil {
			return &result
		}
		// Fall back to simulation on executor error
		return &TrainingResult{
			HitRate:      0.5,
			SharpeRatio:  0.0,
			MaxDrawdown:  0.0,
			TotalReturn:  0.0,
			SignalsCount: 0,
			WinCount:     0,
			LossCount:    0,
			Error:        err.Error(),
		}
	}

	// Simulate training with realistic results when no executor is present (legacy/tests)
	time.Sleep(50 * time.Millisecond)

	result := &TrainingResult{
		HitRate:      0.5 + float64(task.Priority)*0.02,
		SharpeRatio:  0.6,
		MaxDrawdown:  -0.12,
		TotalReturn:  0.08,
		SignalsCount: 50,
		WinCount:     25,
		LossCount:    25,
	}

	switch task.Regime {
	case RegimeRiskOn:
		result.SharpeRatio = 0.8
		result.TotalReturn = 0.15
	case RegimeRiskOff:
		result.SharpeRatio = 0.4
		result.TotalReturn = -0.05
	case RegimeHighVolatility:
		result.SharpeRatio = 0.3
		result.MaxDrawdown = -0.25
	case RegimeLowVolatility:
		result.SharpeRatio = 0.9
		result.MaxDrawdown = -0.05
	}

	return result
}

// autoBalancer periodically rebalances queues
func (pm *PRISMManager) autoBalancer(stopCh <-chan struct{}) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-stopCh:
			return
		case <-ticker.C:
			pm.Rebalance()
		}
	}
}

// classifyRegime determines regime type based on market conditions during window
func (pm *PRISMManager) classifyRegime(window TrainingWindow) RegimeType {
	// This would analyze market data during the window
	// For now, use time-based classification as a placeholder

	hour := window.Start.Hour()
	month := int(window.Start.Month())

	// Simple heuristic based on month (seasonal patterns)
	switch {
	case month >= 1 && month <= 3:
		return RegimeTransition // Q1 uncertainty
	case month >= 4 && month <= 6:
		return RegimeRiskOn // Q2 typically bullish
	case month >= 7 && month <= 9:
		return RegimeLowVolatility // Q3 often range-bound
	case month >= 10 && month <= 12:
		return RegimeRiskOff // Q4 often volatile/risk-off
	}

	// Time of day heuristic for intraday
	if hour >= 9 && hour <= 11 {
		return RegimeHighVolatility // Opening volatility
	}

	return RegimeTransition
}

// calculatePriority determines task priority
func (pm *PRISMManager) calculatePriority(agent domain.AgentSpec) int {
	priority := 50 // Base priority

	// Boost for new agents
	if agent.DarwinianWeight == 1.0 && pm.config.PrioritizeNewAgents {
		priority += 30
	}

	// Boost for Superinvestor layer
	if agent.Layer == domain.LayerSuperinvestor {
		priority += 20
	}

	// Reduce priority for disabled agents
	if !agent.Enabled {
		priority -= 40
	}

	return priority
}

// countActiveQueues returns number of queues with pending tasks
func (pm *PRISMManager) countActiveQueues() int {
	count := 0
	for i := 0; i < int(RegimeCount); i++ {
		if pm.queues[i].Len() > 0 {
			count++
		}
	}
	return count
}

// QueueStats provides statistics for a single queue
type QueueStats struct {
	Regime      RegimeType
	Size        int
	MaxSize     int
	Workers     int
	Utilization float64
}

// PRISMStats provides system-wide statistics
type PRISMStats struct {
	TotalTasks     int
	CompletedTasks int
	FailedTasks    int
	ActiveQueues   int
}

// TrainingWindow defines a time period for training
type TrainingWindow struct {
	Start  time.Time
	End    time.Time
	Regime RegimeType // Optional override
}

// recordCompletedResult appends a completed training result to the internal buffer.
func (pm *PRISMManager) recordCompletedResult(task *TrainingTask, result TrainingResult) {
	pm.resultMu.Lock()
	defer pm.resultMu.Unlock()

	pm.completedResults = append(pm.completedResults, CompletedTrainingResult{
		AgentID:    task.AgentID,
		AgentSkill: task.AgentSkill,
		Regime:     task.Regime,
		Result:     result,
	})
	if len(pm.completedResults) > pm.maxResults {
		pm.completedResults = pm.completedResults[len(pm.completedResults)-pm.maxResults:]
	}
}

// GetCompletedResults returns a copy of all recorded completed training results.
func (pm *PRISMManager) GetCompletedResults() []CompletedTrainingResult {
	pm.resultMu.RLock()
	defer pm.resultMu.RUnlock()

	out := make([]CompletedTrainingResult, len(pm.completedResults))
	copy(out, pm.completedResults)
	return out
}

// ClearCompletedResults empties the completed result buffer.
func (pm *PRISMManager) ClearCompletedResults() {
	pm.resultMu.Lock()
	defer pm.resultMu.Unlock()
	pm.completedResults = pm.completedResults[:0]
}

// Utility functions

func generateTaskID(agentID string, window TrainingWindow) string {
	return fmt.Sprintf("task_%s_%d_%d", agentID, window.Start.Unix(), window.End.Unix())
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
