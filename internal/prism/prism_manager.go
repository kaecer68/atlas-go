// Package prism implements PRISM (Parallel Regime-Specific Independent Systems for Multi-cohort training)
// Manages 5 independent training queues for different market regimes
package prism

import (
	"container/list"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/logging"
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

// MarshalJSON serializes RegimeType as an UPPER_SNAKE_CASE string so
// frontend consumers can match keys like RISK_ON / RISK_OFF without
// keeping an int<->string mapping table.
func (r RegimeType) MarshalJSON() ([]byte, error) {
	return []byte(`"` + r.jsonKey() + `"`), nil
}

// jsonKey returns the stable UPPER_SNAKE_CASE identifier for the regime.
func (r RegimeType) jsonKey() string {
	switch r {
	case RegimeRiskOn:
		return "RISK_ON"
	case RegimeRiskOff:
		return "RISK_OFF"
	case RegimeHighVolatility:
		return "HIGH_VOLATILITY"
	case RegimeLowVolatility:
		return "LOW_VOLATILITY"
	case RegimeTransition:
		return "TRANSITION"
	default:
		return "UNKNOWN"
	}
}

// UnmarshalJSON accepts both the UPPER_SNAKE_CASE string form (what
// MarshalJSON emits) and a raw integer (legacy callers, tests, and
// hand-written fixtures). Strings are matched case-insensitively.
func (r *RegimeType) UnmarshalJSON(data []byte) error {
	trimmed := strings.Trim(string(data), `"`)
	if trimmed == "" || trimmed == "null" {
		*r = RegimeTransition
		return nil
	}
	upper := strings.ToUpper(trimmed)
	for _, candidate := range []RegimeType{
		RegimeRiskOn, RegimeRiskOff,
		RegimeHighVolatility, RegimeLowVolatility,
		RegimeTransition,
	} {
		if candidate.jsonKey() == upper {
			*r = candidate
			return nil
		}
	}
	var asInt int
	if err := json.Unmarshal(data, &asInt); err == nil && asInt >= 0 && asInt < int(RegimeCount) {
		*r = RegimeType(asInt)
		return nil
	}
	return fmt.Errorf("prism: invalid RegimeType %q", trimmed)
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
	HitRate      float64       `json:"hit_rate"`
	SharpeRatio  float64       `json:"sharpe_ratio"`
	MaxDrawdown  float64       `json:"max_drawdown"`
	TotalReturn  float64       `json:"total_return"`
	SignalsCount int           `json:"signals_count"`
	WinCount     int           `json:"win_count"`
	LossCount    int           `json:"loss_count"`
	Error        string        `json:"error,omitempty"`
	Duration     time.Duration `json:"duration,omitempty"`  // nanoseconds; JS reads as number
	Synthetic    bool          `json:"synthetic,omitempty"` // true when no TrainingExecutor was available

	// Explanation is an optional natural-language explanation of the
	// training result, populated by the ScenarioExplainer hook when
	// LLM_PRISM_SCENARIO_ENABLED is true. Uses var indirection in
	// internal/orchestrator to avoid a prism→llm import cycle.
	Explanation string `json:"explanation,omitempty"`
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
	AgentID    string         `json:"agent_id"`
	AgentSkill string         `json:"agent_skill,omitempty"`
	Regime     RegimeType     `json:"regime"`
	Result     TrainingResult `json:"result"`
}

// TrainingExecutor runs a single training task and returns real metrics.
type TrainingExecutor interface {
	Run(task TrainingTask) (TrainingResult, error)
}

// PRISMManager manages all 5 regime-specific training queues
type PRISMManager struct {
	queues    [RegimeCount]*TrainingQueue
	mu        sync.RWMutex
	isRunning bool
	stopCh    chan struct{}
	wg        sync.WaitGroup
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

	// onCompleted fires when a training task completes, so JANUS gets results
	// immediately instead of waiting for the 6h cron. Set via SetOnCompleted.
	onCompleted func(CompletedTrainingResult)
}

// SetOnCompleted registers a callback fired on each training completion.
func (pm *PRISMManager) SetOnCompleted(fn func(CompletedTrainingResult)) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.onCompleted = fn
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
	for i := range int(RegimeCount) {
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
	// Start worker goroutines for each queue (tracked by WaitGroup).
	for i := range int(RegimeCount) {
		for j := 0; j < pm.config.WorkersPerQueue; j++ {
			pm.wg.Add(1)
			go pm.worker(pm.queues[i], pm.stopCh)
		}
	}

	// Start auto-balancer if enabled (tracked by WaitGroup).
	if pm.config.AutoBalance {
		pm.wg.Add(1)
		go pm.autoBalancer(pm.stopCh)
	}

	logging.Info("prism_manager", "started", "regime_queues", 5)
}

// Stop halts all queue processing and waits for goroutines to finish.
func (pm *PRISMManager) Stop() {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if !pm.isRunning {
		return
	}

	pm.isRunning = false
	close(pm.stopCh)
	pm.wg.Wait()

	logging.Info("prism_manager", "stopped")
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
	for i := range int(RegimeCount) {
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
		logging.Info("prism_manager", "queue_cleared", logging.FStr("regime", regime.String()))
	}
}

// Rebalance redistributes tasks based on current queue loads
func (pm *PRISMManager) Rebalance() {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	// Calculate average load
	totalLoad := 0
	for i := range int(RegimeCount) {
		totalLoad += pm.queues[i].Len()
	}
	avgLoad := totalLoad / int(RegimeCount)

	// Log imbalances
	for i := range int(RegimeCount) {
		queue := pm.queues[i]
		deviation := queue.Len() - avgLoad
		if abs(deviation) > avgLoad/2 {
			logging.Warn("prism_manager", "load_imbalance", logging.FStr("regime", queue.Regime.String()), logging.FInt("queue_size", queue.Len()), logging.FInt("avg_load", avgLoad))
		}
	}
}

// worker processes tasks from a single queue
func (pm *PRISMManager) worker(queue *TrainingQueue, stopCh <-chan struct{}) {
	defer pm.wg.Done()
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

		// Run training
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

// executeTraining delegates to the attached TrainingExecutor when available.
// When no executor is configured, the returned result is marked Synthetic so that
// callers can distinguish real backtest outcomes from empty placeholders.
func (pm *PRISMManager) executeTraining(task *TrainingTask) *TrainingResult {
	pm.mu.RLock()
	ex := pm.executor
	pm.mu.RUnlock()
	if ex != nil {
		result, err := ex.Run(*task)
		if err == nil {
			cloned := result
			return &cloned
		}
		return &TrainingResult{Error: err.Error()}
	}
	return &TrainingResult{
		Synthetic: true,
		Error:     "no training executor configured",
	}
}

// autoBalancer periodically rebalances queues
func (pm *PRISMManager) autoBalancer(stopCh <-chan struct{}) {
	defer pm.wg.Done()
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

// classifyRegime resolves the regime for a training window.
//
// When the window carries an explicit regime override (set by the orchestrator's
// regime-detection pipeline), that value is authoritative and returned directly.
// When no override is present, the function defaults to RegimeTransition so that
// the caller never relies on synthetic time-based guesses.
func (pm *PRISMManager) classifyRegime(window TrainingWindow) RegimeType {
	if window.RegimeSet {
		return window.Regime
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
	for i := range int(RegimeCount) {
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
	Start     time.Time
	End       time.Time
	Regime    RegimeType // regime override (only meaningful when RegimeSet is true)
	RegimeSet bool       // true when Regime was explicitly provided by the caller
}

// recordCompletedResult appends a completed training result to the internal buffer
// and fires the OnCompleted callback when set.
func (pm *PRISMManager) recordCompletedResult(task *TrainingTask, result TrainingResult) {
	pm.resultMu.Lock()
	fn := pm.onCompleted

	pm.completedResults = append(pm.completedResults, CompletedTrainingResult{
		AgentID:    task.AgentID,
		AgentSkill: task.AgentSkill,
		Regime:     task.Regime,
		Result:     result,
	})
	if len(pm.completedResults) > pm.maxResults {
		pm.completedResults = pm.completedResults[len(pm.completedResults)-pm.maxResults:]
	}
	pm.resultMu.Unlock()

	if fn != nil && !result.Synthetic {
		fn(CompletedTrainingResult{
			AgentID:    task.AgentID,
			AgentSkill: task.AgentSkill,
			Regime:     task.Regime,
			Result:     result,
		})
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
