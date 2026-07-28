package apigateway

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"golang.org/x/time/rate"
)

// =========================================================================
// ScheduledTask Tests — IsEnabled
// =========================================================================

func TestScheduledTask_IsEnabled_InitiallyFalse(t *testing.T) {
	task := &ScheduledTask{}
	if task.IsEnabled() {
		t.Error("IsEnabled should return false for a fresh ScheduledTask")
	}
}

func TestScheduledTask_IsEnabled_AfterSetEnabledTrue(t *testing.T) {
	task := &ScheduledTask{}
	task.SetEnabled(true)
	if !task.IsEnabled() {
		t.Error("IsEnabled should return true after SetEnabled(true)")
	}
}

func TestScheduledTask_IsEnabled_AfterSetEnabledFalse(t *testing.T) {
	task := &ScheduledTask{}
	task.SetEnabled(true)
	task.SetEnabled(false)
	if task.IsEnabled() {
		t.Error("IsEnabled should return false after SetEnabled(false)")
	}
}

func TestScheduledTask_IsEnabled_ToggleBackAndForth(t *testing.T) {
	task := &ScheduledTask{}

	task.SetEnabled(true)
	if !task.IsEnabled() {
		t.Error("expected true after first SetEnabled(true)")
	}

	task.SetEnabled(false)
	if task.IsEnabled() {
		t.Error("expected false after SetEnabled(false)")
	}

	task.SetEnabled(true)
	if !task.IsEnabled() {
		t.Error("expected true after second SetEnabled(true)")
	}
}

// =========================================================================
// ScheduledTask Tests — SetEnabled
// =========================================================================

func TestScheduledTask_SetEnabled_True(t *testing.T) {
	task := &ScheduledTask{}
	task.SetEnabled(true)
	if !task.Enabled {
		t.Error("Enabled field should be true after SetEnabled(true)")
	}
}

func TestScheduledTask_SetEnabled_False(t *testing.T) {
	task := &ScheduledTask{Enabled: true}
	task.SetEnabled(false)
	if task.Enabled {
		t.Error("Enabled field should be false after SetEnabled(false)")
	}
}

// =========================================================================
// ScheduledTask Tests — LastRun / SetLastRun
// =========================================================================

func TestScheduledTask_LastRun_InitiallyZeroTime(t *testing.T) {
	task := &ScheduledTask{}
	if !task.LastRun().IsZero() {
		t.Error("LastRun should return zero time for a fresh ScheduledTask")
	}
}

func TestScheduledTask_SetLastRun_ReturnsSetValue(t *testing.T) {
	task := &ScheduledTask{}
	now := time.Date(2026, 5, 27, 10, 0, 0, 0, time.UTC)
	task.SetLastRun(now)

	got := task.LastRun()
	if !got.Equal(now) {
		t.Errorf("LastRun = %v, want %v", got, now)
	}
}

func TestScheduledTask_SetLastRun_MultipleUpdates(t *testing.T) {
	task := &ScheduledTask{}

	t1 := time.Date(2026, 5, 27, 10, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 5, 27, 11, 0, 0, 0, time.UTC)
	t3 := time.Date(2026, 5, 27, 12, 0, 0, 0, time.UTC)

	task.SetLastRun(t1)
	if !task.LastRun().Equal(t1) {
		t.Error("first SetLastRun not reflected")
	}

	task.SetLastRun(t2)
	if !task.LastRun().Equal(t2) {
		t.Error("second SetLastRun not reflected")
	}

	task.SetLastRun(t3)
	if !task.LastRun().Equal(t3) {
		t.Error("third SetLastRun not reflected")
	}
}

func TestScheduledTask_LastRun_ThreadSafe(t *testing.T) {
	task := &ScheduledTask{}

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = task.LastRun()
		}()
	}
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			task.SetLastRun(time.Date(2026, 5, 27, 10, i%60, 0, 0, time.UTC))
		}(i)
	}
	wg.Wait()

	// After concurrent r/w, LastRun should not panic and return a valid time
	got := task.LastRun()
	if got.IsZero() {
		t.Error("LastRun should not be zero after concurrent writes")
	}
}

// =========================================================================
// ScheduledTask Tests — Failures / RecordFailure / RecordSuccess
// =========================================================================

func TestScheduledTask_Failures_InitiallyZero(t *testing.T) {
	task := &ScheduledTask{}
	if task.Failures() != 0 {
		t.Errorf("Failures = %d, want 0", task.Failures())
	}
}

func TestScheduledTask_RecordFailure_IncrementsCount(t *testing.T) {
	task := &ScheduledTask{}

	for i := 1; i <= 5; i++ {
		task.RecordFailure()
		if got := task.Failures(); got != i {
			t.Errorf("after %d RecordFailure calls: Failures = %d, want %d", i, got, i)
		}
	}
}

func TestScheduledTask_RecordFailure_ThenRecordSuccess(t *testing.T) {
	task := &ScheduledTask{}

	// Three failures
	task.RecordFailure()
	task.RecordFailure()
	task.RecordFailure()

	if got := task.Failures(); got != 3 {
		t.Errorf("after 3 failures: Failures = %d, want 3", got)
	}

	// RecordSuccess resets to zero
	task.RecordSuccess()
	if got := task.Failures(); got != 0 {
		t.Errorf("after RecordSuccess: Failures = %d, want 0", got)
	}
}

func TestScheduledTask_RecordSuccess_OnCleanTask(t *testing.T) {
	task := &ScheduledTask{}

	// RecordSuccess on a task with zero failures should stay zero
	task.RecordSuccess()
	if got := task.Failures(); got != 0 {
		t.Errorf("after RecordSuccess on clean task: Failures = %d, want 0", got)
	}
}

func TestScheduledTask_RecordFailure_RecordSuccess_MultipleCycles(t *testing.T) {
	task := &ScheduledTask{}

	for cycle := 0; cycle < 3; cycle++ {
		// Build up failures
		for i := 0; i < cycle+1; i++ {
			task.RecordFailure()
		}
		if got := task.Failures(); got != cycle+1 {
			t.Errorf("cycle %d after failures: Failures = %d, want %d", cycle, got, cycle+1)
		}

		// Reset
		task.RecordSuccess()
		if got := task.Failures(); got != 0 {
			t.Errorf("cycle %d after RecordSuccess: Failures = %d, want 0", cycle, got)
		}
	}
}

// =========================================================================
// BackgroundTaskManager Tests — NewBackgroundTaskManager
// =========================================================================

func TestBackgroundTaskManager_NewWithNilGateway(t *testing.T) {
	m := NewBackgroundTaskManager(nil)
	if m == nil {
		t.Fatal("NewBackgroundTaskManager(nil) returned nil")
	}
	if m.gateway != nil {
		t.Error("gateway should be nil when passed nil")
	}
	if m.registry == nil {
		t.Error("registry should be initialized (non-nil map)")
	}
	if len(m.registry) != 0 {
		t.Errorf("registry should be empty, got %d entries", len(m.registry))
	}
}

func TestBackgroundTaskManager_NewWithNilGateway_ListReturnsEmpty(t *testing.T) {
	m := NewBackgroundTaskManager(nil)
	names := m.List()
	if len(names) != 0 {
		t.Errorf("List should return empty slice, got %v", names)
	}
}

// =========================================================================
// BackgroundTaskManager Tests — Register / Get
// =========================================================================

func TestBackgroundTaskManager_Register_EmptyChannelID(t *testing.T) {
	m := NewBackgroundTaskManager(nil)
	task := &ScheduledTask{
		Name:      "test-task",
		ChannelID: "", // empty → skips gateway check
		Interval:  1 * time.Hour,
		Task:      func(ctx context.Context) error { return nil },
	}

	err := m.Register(task)
	if err != nil {
		t.Fatalf("Register with empty ChannelID should succeed, got: %v", err)
	}
}

func TestBackgroundTaskManager_Register_AutoJitter(t *testing.T) {
	m := NewBackgroundTaskManager(nil)
	task := &ScheduledTask{
		Name:     "jitter-task",
		Interval: 1 * time.Hour,
		Jitter:   0, // should be auto-set
		Task:     func(ctx context.Context) error { return nil },
	}

	err := m.Register(task)
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}
	if task.Jitter == 0 {
		t.Error("Jitter should be auto-computed when Interval > 0 and Jitter == 0")
	}
}

func TestBackgroundTaskManager_Register_NoAutoJitterWhenZeroInterval(t *testing.T) {
	m := NewBackgroundTaskManager(nil)
	task := &ScheduledTask{
		Name:     "no-jitter-task",
		Interval: 0,
		Jitter:   0,
		Task:     func(ctx context.Context) error { return nil },
	}

	err := m.Register(task)
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}
	// Jitter should remain 0 because Interval is 0 (guard: task.Jitter == 0 && task.Interval > 0)
	if task.Jitter != 0 {
		t.Errorf("Jitter should remain 0 when Interval is 0, got %v", task.Jitter)
	}
}

func TestBackgroundTaskManager_Register_ExistingJitterPreserved(t *testing.T) {
	m := NewBackgroundTaskManager(nil)
	existingJitter := 30 * time.Second
	task := &ScheduledTask{
		Name:     "preset-jitter-task",
		Interval: 1 * time.Hour,
		Jitter:   existingJitter,
		Task:     func(ctx context.Context) error { return nil },
	}

	err := m.Register(task)
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}
	if task.Jitter != existingJitter {
		t.Errorf("Jitter should be preserved, got %v, want %v", task.Jitter, existingJitter)
	}
}

func TestBackgroundTaskManager_Get_ExistingTask(t *testing.T) {
	m := NewBackgroundTaskManager(nil)
	task := &ScheduledTask{
		Name:     "get-test-task",
		Interval: 1 * time.Hour,
		Task:     func(ctx context.Context) error { return nil },
	}

	_ = m.Register(task)

	got, ok := m.Get("get-test-task")
	if !ok {
		t.Fatal("Get should return true for registered task")
	}
	if got != task {
		t.Error("Get should return the same task pointer that was registered")
	}
	if got.Name != "get-test-task" {
		t.Errorf("returned task Name = %s, want get-test-task", got.Name)
	}
}

func TestBackgroundTaskManager_Get_NonExistingTask(t *testing.T) {
	m := NewBackgroundTaskManager(nil)

	got, ok := m.Get("nonexistent")
	if ok {
		t.Error("Get should return false for non-existing task")
	}
	if got != nil {
		t.Errorf("Get should return nil for non-existing task, got %v", got)
	}
}

// =========================================================================
// BackgroundTaskManager Tests — List
// =========================================================================

func TestBackgroundTaskManager_List_SingleTask(t *testing.T) {
	m := NewBackgroundTaskManager(nil)
	task := &ScheduledTask{
		Name:     "list-task-1",
		Interval: 1 * time.Hour,
		Task:     func(ctx context.Context) error { return nil },
	}
	_ = m.Register(task)

	names := m.List()
	if len(names) != 1 {
		t.Fatalf("List should return 1 name, got %d", len(names))
	}
	if names[0] != "list-task-1" {
		t.Errorf("List[0] = %s, want list-task-1", names[0])
	}
}

func TestBackgroundTaskManager_List_MultipleTasks(t *testing.T) {
	m := NewBackgroundTaskManager(nil)

	taskNames := []string{"task-alpha", "task-beta", "task-gamma"}
	for _, name := range taskNames {
		_ = m.Register(&ScheduledTask{
			Name:     name,
			Interval: 1 * time.Hour,
			Task:     func(ctx context.Context) error { return nil },
		})
	}

	names := m.List()
	if len(names) != len(taskNames) {
		t.Fatalf("List should return %d names, got %d", len(taskNames), len(names))
	}

	nameSet := make(map[string]bool, len(names))
	for _, n := range names {
		nameSet[n] = true
	}
	for _, want := range taskNames {
		if !nameSet[want] {
			t.Errorf("List missing expected task name: %s", want)
		}
	}
}

func TestBackgroundTaskManager_List_ReturnsFreshSlice(t *testing.T) {
	m := NewBackgroundTaskManager(nil)
	_ = m.Register(&ScheduledTask{
		Name:     "mutate-test",
		Interval: 1 * time.Hour,
		Task:     func(ctx context.Context) error { return nil },
	})

	names1 := m.List()
	if len(names1) != 1 {
		t.Fatal("expected 1 task")
	}
	names1[0] = "mutated" // mutate the returned slice

	names2 := m.List()
	if len(names2) != 1 {
		t.Fatal("expected 1 task on second call")
	}
	if names2[0] == "mutated" {
		t.Error("List should return a fresh copy, mutations should not affect subsequent calls")
	}
}

// =========================================================================
// BackgroundTaskManager Tests — Status
// =========================================================================

func TestBackgroundTaskManager_Status_Empty(t *testing.T) {
	m := NewBackgroundTaskManager(nil)
	statuses := m.Status()
	if len(statuses) != 0 {
		t.Errorf("Status should return empty slice for empty registry, got %d entries", len(statuses))
	}
}

func TestBackgroundTaskManager_Status_SingleTask(t *testing.T) {
	m := NewBackgroundTaskManager(nil)
	task := &ScheduledTask{
		Name:     "status-task",
		Interval: 30 * time.Minute,
		Task:     func(ctx context.Context) error { return nil },
	}
	task.SetEnabled(true)
	task.SetLastRun(time.Date(2026, 5, 27, 10, 0, 0, 0, time.UTC))
	task.RecordFailure()
	task.RecordFailure()
	_ = m.Register(task)

	statuses := m.Status()
	if len(statuses) != 1 {
		t.Fatalf("Status should return 1 entry, got %d", len(statuses))
	}

	s := statuses[0]
	if s.Name != "status-task" {
		t.Errorf("Status.Name = %s, want status-task", s.Name)
	}
	if s.ChannelID != "" {
		t.Errorf("Status.ChannelID = %s, want empty", s.ChannelID)
	}
	if !s.Enabled {
		t.Error("Status.Enabled should be true")
	}
	if s.Interval != 30*time.Minute {
		t.Errorf("Status.Interval = %v, want 30m", s.Interval)
	}
	if !s.LastRun.Equal(time.Date(2026, 5, 27, 10, 0, 0, 0, time.UTC)) {
		t.Errorf("Status.LastRun = %v, want 2026-05-27T10:00:00Z", s.LastRun)
	}
	if s.ConsecutiveFailures != 2 {
		t.Errorf("Status.ConsecutiveFailures = %d, want 2", s.ConsecutiveFailures)
	}
}

func TestBackgroundTaskManager_Status_MultipleTasks(t *testing.T) {
	m := NewBackgroundTaskManager(nil)

	for i := 0; i < 5; i++ {
		name := string(rune('A' + i))
		_ = m.Register(&ScheduledTask{
			Name:     string(name),
			Interval: time.Duration(i+1) * time.Hour,
			Task:     func(ctx context.Context) error { return nil },
		})
	}

	statuses := m.Status()
	if len(statuses) != 5 {
		t.Fatalf("Status should return 5 entries, got %d", len(statuses))
	}

	seen := make(map[string]bool)
	for _, s := range statuses {
		if s.Name == "" {
			t.Error("Status entry has empty Name")
		}
		if s.Interval <= 0 {
			t.Errorf("Status entry %s has non-positive Interval", s.Name)
		}
		if seen[s.Name] {
			t.Errorf("duplicate Status entry for %s", s.Name)
		}
		seen[s.Name] = true
	}
}

func TestBackgroundTaskManager_Status_ReturnsFreshSlice(t *testing.T) {
	m := NewBackgroundTaskManager(nil)
	_ = m.Register(&ScheduledTask{
		Name:     "fresh-slice-task",
		Interval: 1 * time.Hour,
		Task:     func(ctx context.Context) error { return nil },
	})

	s1 := m.Status()
	s2 := m.Status()

	if len(s1) != 1 || len(s2) != 1 {
		t.Fatal("expected 1 entry in Status results")
	}

	// Mutate s1, verify s2 is unaffected
	s1[0] = TaskStatus{Name: "corrupted"}
	if s2[0].Name == "corrupted" {
		t.Error("Status should return a fresh copy, mutations should not affect later calls")
	}
}

// =========================================================================
// BackgroundTaskManager Tests — SetFailureHandler
// =========================================================================

func TestBackgroundTaskManager_SetFailureHandler_VerifyCallback(t *testing.T) {
	m := NewBackgroundTaskManager(nil)

	handlerCalled := false
	m.SetFailureHandler(func(taskName string, consecutiveFailures int, err error) {
		handlerCalled = true
	})

	m.failureHandler("test-task", 1, errors.New("test error"))
	if !handlerCalled {
		t.Error("SetFailureHandler should store and use the handler callback")
	}
}

func TestBackgroundTaskManager_SetFailureHandler_NilHandler(t *testing.T) {
	m := NewBackgroundTaskManager(nil)

	// Set a handler first
	m.SetFailureHandler(func(taskName string, consecutiveFailures int, err error) {})

	// Set nil — this should NOT panic (just stores nil)
	m.SetFailureHandler(nil)

	// Verify no panic when calling nil handler via executeTask path would happen,
	// but we can't call that without goroutines. Just verify the field is nil.
	if m.failureHandler != nil {
		t.Error("failureHandler should be nil after SetFailureHandler(nil)")
	}
}

// =========================================================================
// TaskStatus Tests — Struct Fields
// =========================================================================

func TestTaskStatus_StructFields(t *testing.T) {
	now := time.Date(2026, 5, 27, 14, 30, 0, 0, time.UTC)
	ts := TaskStatus{
		Name:                "test-task",
		ChannelID:           "channel-1",
		Enabled:             true,
		Interval:            15 * time.Minute,
		LastRun:             now,
		ConsecutiveFailures: 3,
	}

	if ts.Name != "test-task" {
		t.Errorf("Name = %s, want test-task", ts.Name)
	}
	if ts.ChannelID != "channel-1" {
		t.Errorf("ChannelID = %s, want channel-1", ts.ChannelID)
	}
	if !ts.Enabled {
		t.Error("Enabled should be true")
	}
	if ts.Interval != 15*time.Minute {
		t.Errorf("Interval = %v, want 15m", ts.Interval)
	}
	if !ts.LastRun.Equal(now) {
		t.Errorf("LastRun = %v, want %v", ts.LastRun, now)
	}
	if ts.ConsecutiveFailures != 3 {
		t.Errorf("ConsecutiveFailures = %d, want 3", ts.ConsecutiveFailures)
	}
}

func TestTaskStatus_ZeroValue(t *testing.T) {
	var ts TaskStatus

	if ts.Name != "" {
		t.Errorf("zero Name should be empty, got %s", ts.Name)
	}
	if ts.ChannelID != "" {
		t.Errorf("zero ChannelID should be empty, got %s", ts.ChannelID)
	}
	if ts.Enabled {
		t.Error("zero Enabled should be false")
	}
	if ts.Interval != 0 {
		t.Errorf("zero Interval should be 0, got %v", ts.Interval)
	}
	if !ts.LastRun.IsZero() {
		t.Errorf("zero LastRun should be zero time, got %v", ts.LastRun)
	}
	if ts.ConsecutiveFailures != 0 {
		t.Errorf("zero ConsecutiveFailures should be 0, got %d", ts.ConsecutiveFailures)
	}
}

// =========================================================================
// Concurrent Access Tests
// =========================================================================

func TestScheduledTask_ConcurrentIsEnabled(t *testing.T) {
	task := &ScheduledTask{}
	var wg sync.WaitGroup

	// 50 goroutines reading IsEnabled
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				_ = task.IsEnabled()
			}
		}()
	}

	// 50 goroutines writing SetEnabled
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				task.SetEnabled(j%2 == 0)
			}
		}()
	}

	wg.Wait()
	// If we get here without a panic/race, the test passes.
}

func TestScheduledTask_ConcurrentFailures(t *testing.T) {
	task := &ScheduledTask{}
	var wg sync.WaitGroup

	// 100 goroutines recording failures concurrently
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				task.RecordFailure()
			}
		}()
	}

	wg.Wait()

	// After 100 × 50 = 5000 RecordFailure calls, failures should be exactly 5000
	if task.Failures() != 5000 {
		t.Errorf("after 5000 concurrent RecordFailure calls: Failures = %d, want 5000", task.Failures())
	}
}

func TestScheduledTask_ConcurrentMixedOperations(t *testing.T) {
	task := &ScheduledTask{}
	var wg sync.WaitGroup

	// Readers
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				_ = task.IsEnabled()
				_ = task.LastRun()
				_ = task.Failures()
			}
		}()
	}

	// Writers
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			now := time.Now()
			for j := 0; j < 100; j++ {
				task.SetEnabled(j%2 == 0)
				task.SetLastRun(now)
				task.RecordFailure()
				if j%10 == 0 {
					task.RecordSuccess()
				}
			}
		}()
	}

	wg.Wait()
	// If we get here without a panic/race, the test passes.
}

func TestBackgroundTaskManager_ConcurrentRegisterAndGet(t *testing.T) {
	m := NewBackgroundTaskManager(nil)
	var wg sync.WaitGroup

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			name := "concurrent-task-" + string(rune('0'+i%10))
			_ = m.Register(&ScheduledTask{
				Name:     name,
				Interval: 1 * time.Hour,
				Task:     func(ctx context.Context) error { return nil },
			})
		}(i)
	}

	wg.Wait()

	// After concurrent registration, registry should have some tasks
	names := m.List()
	if len(names) == 0 {
		t.Error("expected some tasks after concurrent Register calls")
	}
}

func TestBackgroundTaskManager_ConcurrentListAndRegister(t *testing.T) {
	m := NewBackgroundTaskManager(nil)
	var wg sync.WaitGroup

	// Register some initial tasks
	for i := 0; i < 5; i++ {
		_ = m.Register(&ScheduledTask{
			Name:     "initial-task-" + string(rune('A'+i)),
			Interval: 1 * time.Hour,
			Task:     func(ctx context.Context) error { return nil },
		})
	}

	// Concurrent List and Register
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = m.List()
		}()
	}
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_ = m.Register(&ScheduledTask{
				Name:     "parallel-task-" + string(rune('0'+i)),
				Interval: 1 * time.Hour,
				Task:     func(ctx context.Context) error { return nil },
			})
		}(i)
	}

	wg.Wait()
	// If we get here without a panic/race, the test passes.
}

func TestBackgroundTaskManager_Status_NextRun_ZeroLastRun(t *testing.T) {
	mgr := NewBackgroundTaskManager(nil)
	mgr.Register(&ScheduledTask{
		Name:     "never_run",
		Interval: time.Hour,
		Task:     func(_ context.Context) error { return nil },
	})

	statuses := mgr.Status()
	if len(statuses) != 1 {
		t.Fatalf("got %d statuses, want 1", len(statuses))
	}
	if !statuses[0].NextRun.IsZero() {
		t.Errorf("NextRun = %v, want zero value (task has never run)", statuses[0].NextRun)
	}
}

func TestBackgroundTaskManager_Status_NextRun_WithLastRun(t *testing.T) {
	mgr := NewBackgroundTaskManager(nil)
	task := &ScheduledTask{
		Name:     "ran",
		Interval: time.Hour,
		Task:     func(_ context.Context) error { return nil },
	}
	knownRun := time.Date(2026, 5, 27, 10, 0, 0, 0, time.UTC)
	task.SetLastRun(knownRun)
	mgr.Register(task)

	statuses := mgr.Status()
	if len(statuses) != 1 {
		t.Fatalf("got %d statuses, want 1", len(statuses))
	}
	want := knownRun.Add(time.Hour)
	if !statuses[0].NextRun.Equal(want) {
		t.Errorf("NextRun = %v, want %v", statuses[0].NextRun, want)
	}
}

func TestBackgroundTaskManager_Status_AllFieldsPopulated(t *testing.T) {
	mgr := NewBackgroundTaskManager(nil)
	task := &ScheduledTask{
		Name:      "full",
		ChannelID: "ch1",
		Interval:  5 * time.Minute,
		Task:      func(_ context.Context) error { return nil },
	}
	task.SetEnabled(true)
	knownRun := time.Date(2026, 1, 15, 8, 30, 0, 0, time.UTC)
	task.SetLastRun(knownRun)
	mgr.Register(task)

	statuses := mgr.Status()
	if len(statuses) != 1 {
		t.Fatalf("got %d statuses, want 1", len(statuses))
	}
	s := statuses[0]
	if s.Name != "full" {
		t.Errorf("Name = %q, want %q", s.Name, "full")
	}
	if s.ChannelID != "ch1" {
		t.Errorf("ChannelID = %q, want %q", s.ChannelID, "ch1")
	}
	if !s.Enabled {
		t.Errorf("Enabled = false, want true")
	}
	if s.Interval != 5*time.Minute {
		t.Errorf("Interval = %v, want %v", s.Interval, 5*time.Minute)
	}
	if !s.LastRun.Equal(knownRun) {
		t.Errorf("LastRun = %v, want %v", s.LastRun, knownRun)
	}
	wantNext := knownRun.Add(5 * time.Minute)
	if !s.NextRun.Equal(wantNext) {
		t.Errorf("NextRun = %v, want %v", s.NextRun, wantNext)
	}
	if s.ConsecutiveFailures != 0 {
		t.Errorf("ConsecutiveFailures = %d, want 0", s.ConsecutiveFailures)
	}
}

// =========================================================================
// BackgroundTaskManager Tests — Panic Recovery
// =========================================================================

func TestExecuteTask_PanicInTaskFunc_RecoversAndRecordsFailure(t *testing.T) {
	m := NewBackgroundTaskManager(nil)

	var failureHandlerMu sync.Mutex
	var failureHandlerCalled bool
	var failureHandlerName string
	var failureHandlerErr error
	var failureHandlerFailures int

	m.SetFailureHandler(func(taskName string, consecutiveFailures int, err error) {
		failureHandlerMu.Lock()
		defer failureHandlerMu.Unlock()
		failureHandlerCalled = true
		failureHandlerName = taskName
		failureHandlerErr = err
		failureHandlerFailures = consecutiveFailures
	})

	task := &ScheduledTask{
		Name:     "panicking-task",
		Interval: 1 * time.Hour,
		Jitter:   0,
		Task: func(ctx context.Context) error {
			panic("intentional test panic for recovery test")
		},
	}
	task.SetEnabled(true)

	m.executeTask(context.Background(), task)

	if task.Failures() == 0 {
		t.Error("expected Failures() > 0 after panic, got 0 (recover() should call RecordFailure)")
	}

	failureHandlerMu.Lock()
	defer failureHandlerMu.Unlock()
	if !failureHandlerCalled {
		t.Error("expected failureHandler to be called after panic, was not")
	}
	if failureHandlerName != "panicking-task" {
		t.Errorf("failureHandler name = %q, want %q", failureHandlerName, "panicking-task")
	}
	if failureHandlerErr == nil {
		t.Error("expected failureHandler err to be non-nil after panic")
	}
	if failureHandlerFailures < 1 {
		t.Errorf("expected failureHandler consecutiveFailures >= 1, got %d", failureHandlerFailures)
	}
}

func TestExecuteTask_FailureHandlerPanics_ManagerSurvives(t *testing.T) {
	m := NewBackgroundTaskManager(nil)

	m.SetFailureHandler(func(taskName string, consecutiveFailures int, err error) {
		panic("intentional test panic in failureHandler")
	})

	task := &ScheduledTask{
		Name:     "task-with-panicking-handler",
		Interval: 1 * time.Hour,
		Jitter:   0,
		Task:     func(ctx context.Context) error { return errors.New("normal task error") },
	}
	task.SetEnabled(true)

	m.executeTask(context.Background(), task)

	if task.Failures() == 0 {
		t.Error("expected Failures() > 0 after normal task error (RecordFailure should still run)")
	}
}

func TestBackgroundTaskManager_Start_PanicInTask_DoesNotCrashManager(t *testing.T) {
	m := NewBackgroundTaskManager(nil)

	eventCh := make(chan struct{}, 2)

	panickingTask := &ScheduledTask{
		Name:     "panicking-task-e2e",
		Interval: 1 * time.Hour,
		Jitter:   1 * time.Millisecond,
		Task: func(ctx context.Context) error {
			panic("intentional test panic — manager should survive")
		},
	}
	panickingTask.SetEnabled(true)
	if err := m.Register(panickingTask); err != nil {
		t.Fatalf("Register panickingTask: %v", err)
	}

	safeTask := &ScheduledTask{
		Name:     "safe-task-e2e",
		Interval: 1 * time.Hour,
		Jitter:   1 * time.Millisecond,
		Task: func(ctx context.Context) error {
			eventCh <- struct{}{}
			return nil
		},
	}
	safeTask.SetEnabled(true)
	if err := m.Register(safeTask); err != nil {
		t.Fatalf("Register safeTask: %v", err)
	}

	m.SetFailureHandler(func(name string, consecutiveFailures int, err error) {
		eventCh <- struct{}{}
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	m.Start(ctx)
	defer m.Stop()

	select {
	case <-eventCh:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for first event — manager likely crashed")
	}
	select {
	case <-eventCh:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for second event — other task never ran")
	}

	if panickingTask.Failures() == 0 {
		t.Error("expected panickingTask.Failures() > 0 (failure not recorded)")
	}
}

// =========================================================================
// BackgroundTaskManager — SetRecoveryHandler
// =========================================================================

func TestBackgroundTaskManager_SetRecoveryHandler_SetsHandler(t *testing.T) {
	m := NewBackgroundTaskManager(nil)
	var called bool
	var recoveredName string
	var recoveredFrom int

	m.SetRecoveryHandler(func(name string, from int) {
		called = true
		recoveredName = name
		recoveredFrom = from
	})

	// Simulate a task that failed then succeeded:
	// 1. Create a task with 2 failures
	// 2. Execute it with a successful task func
	// 3. Recovery handler should be called
	task := &ScheduledTask{
		Name:     "recovering-task",
		Interval: 1 * time.Hour,
		Jitter:   0,
		Task:     func(ctx context.Context) error { return nil },
	}
	task.SetEnabled(true)

	// Artificially set failure count > 0
	task.RecordFailure()
	task.RecordFailure()

	m.executeTask(context.Background(), task)

	if !called {
		t.Error("SetRecoveryHandler callback was not invoked after task recovered")
	}
	if recoveredName != "recovering-task" {
		t.Errorf("recovery handler name = %q, want recovering-task", recoveredName)
	}
	if recoveredFrom != 2 {
		t.Errorf("recovery handler from = %d, want 2", recoveredFrom)
	}
	if task.Failures() != 0 {
		t.Errorf("Failures after recovery = %d, want 0", task.Failures())
	}
}

func TestBackgroundTaskManager_SetRecoveryHandler_NilHandler(t *testing.T) {
	m := NewBackgroundTaskManager(nil)
	// Setting nil handler should not panic and should clear
	m.SetRecoveryHandler(nil)

	task := &ScheduledTask{
		Name:     "task-no-recovery-handler",
		Interval: 1 * time.Hour,
		Task:     func(ctx context.Context) error { return nil },
	}
	task.SetEnabled(true)
	task.RecordFailure()

	// Execute without a handler should not panic
	m.executeTask(context.Background(), task)
	// Task should have recovered (failures reset to 0)
	if task.Failures() != 0 {
		t.Errorf("Failures = %d, want 0", task.Failures())
	}
}

func TestBackgroundTaskManager_SetRecoveryHandler_NoFailuresBefore(t *testing.T) {
	m := NewBackgroundTaskManager(nil)
	var called bool

	m.SetRecoveryHandler(func(name string, from int) {
		called = true
	})

	task := &ScheduledTask{
		Name:     "fresh-task",
		Interval: 1 * time.Hour,
		Task:     func(ctx context.Context) error { return nil },
	}
	task.SetEnabled(true)
	// No failures - handler should NOT be called
	m.executeTask(context.Background(), task)

	if called {
		t.Error("recovery handler was called when there were no prior failures")
	}
}

func TestTaskRecoveryHandler_Signature(t *testing.T) {
	var h TaskRecoveryHandler = func(taskName string, recoveredFrom int) {
	}
	_ = h
}

func TestBackgroundTaskManager_Register_ChannelIDValidation(t *testing.T) {
	g := newTestGateway(t)
	m := NewBackgroundTaskManager(g)
	task := &ScheduledTask{
		Name:      "invalid-channel-task",
		ChannelID: "nonexistent_channel",
		Interval:  1 * time.Hour,
		Task:      func(ctx context.Context) error { return nil },
	}
	err := m.Register(task)
	if err == nil {
		t.Error("Register should fail when ChannelID is not registered in gateway")
	}
}

func TestBackgroundTaskManager_Register_ChannelIDValidation_EmptyChannel(t *testing.T) {
	g := newTestGateway(t)
	m := NewBackgroundTaskManager(g)
	task := &ScheduledTask{
		Name:      "no-channel-task",
		ChannelID: "",
		Interval:  1 * time.Hour,
		Task:      func(ctx context.Context) error { return nil },
	}
	err := m.Register(task)
	if err != nil {
		t.Errorf("Register with empty ChannelID should succeed, got: %v", err)
	}
}

func TestBackgroundTaskManager_Register_JitterDefault(t *testing.T) {
	g := newTestGateway(t)
	m := NewBackgroundTaskManager(g)
	task := &ScheduledTask{
		Name:     "jitery-task",
		Interval: 10 * time.Second,
		Jitter:   0,
		Task:     func(ctx context.Context) error { return nil },
	}
	err := m.Register(task)
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}
	if task.Jitter == 0 {
		t.Error("Jitter should be auto-set to non-zero for tasks with Interval > 0")
	}
	if task.Jitter != time.Duration(0.1*float64(10*time.Second)) {
		t.Errorf("Jitter = %v, want %v", task.Jitter, time.Duration(1*time.Second))
	}
}

func TestBackgroundTaskManager_ExecuteTask_Disabled(t *testing.T) {
	m := NewBackgroundTaskManager(nil)
	task := &ScheduledTask{
		Name:     "disabled-task",
		Interval: 1 * time.Hour,
		Task:     func(ctx context.Context) error { return errors.New("should not be called") },
	}
	task.SetEnabled(false)
	m.executeTask(context.Background(), task)
}

func TestBackgroundTaskManager_SafeCallFailureHandler_NilHandler(t *testing.T) {
	m := NewBackgroundTaskManager(nil)
	m.safeCallFailureHandler("test-task", 3, errors.New("test error"))
}

func TestLogAndWrapPanic_NilRecover(t *testing.T) {
	err := logAndWrapPanic("test-task", "test-event", nil)
	if err != nil {
		t.Errorf("logAndWrapPanic with nil recover should return nil, got: %v", err)
	}
}

// TestExecuteTask_DoesNotSkipWhenBreakerOpen is a regression test for the
// 2026-06 fubon channel deadlock: the previous implementation called
// breaker.IsOpen() in executeTask and returned early when open. If the
// BackgroundTaskManager is the channel's only caller (e.g.
// channel_health_fubon), the half-open probe inside breaker.Call() never
// fired, leaving the breaker permanently open. The fix is to always
// invoke task.Task(ctx); gateway.Fetch's breaker.Call() handles the
// Open→HalfOpen→Closed transitions itself.
func TestExecuteTask_DoesNotSkipWhenBreakerOpen(t *testing.T) {
	g := newTestGateway(t)
	breaker, err := g.breakers.Get("fubon")
	if err != nil {
		t.Fatalf("Get(fubon) failed: %v", err)
	}
	breaker.ForceOpen()
	if !breaker.IsOpen() {
		t.Fatal("precondition: breaker should be open after ForceOpen")
	}

	m := NewBackgroundTaskManager(g)
	called := 0
	task := &ScheduledTask{
		Name:      "channel_health_fubon",
		ChannelID: "fubon",
		Interval:  1 * time.Hour,
		Task:      func(ctx context.Context) error { called++; return nil },
	}
	task.SetEnabled(true)
	m.executeTask(context.Background(), task)

	if called != 1 {
		t.Errorf("task.Task should be invoked even when breaker is open, got called=%d (regression: deadlock returns early)", called)
	}
}

func TestExecuteTask_Baseline_InvokesTask(t *testing.T) {
	g := newTestGateway(t)
	m := NewBackgroundTaskManager(g)
	called := 0
	task := &ScheduledTask{
		Name:      "channel_health_fubon",
		ChannelID: "fubon",
		Interval:  1 * time.Hour,
		Task:      func(ctx context.Context) error { called++; return nil },
	}
	task.SetEnabled(true)
	m.executeTask(context.Background(), task)
	if called != 1 {
		t.Errorf("task.Task should be invoked when breaker is closed, got called=%d", called)
	}
}

// TestExecuteTask_HalfOpenTransition verifies that executeTask can trigger a
// breaker Open→HalfOpen→Closed transition through Gateway.Fetch().
//
// This is the core integration test for the 2026-06 fubon channel fix:
// earlier code pre-checked breaker.IsOpen() in executeTask and returned early,
// which meant no probe ever reached breaker.Call() — so for tasks that were
// the channel's sole caller, the breaker stayed Open forever. The fix was to
// remove the pre-check and let breaker.Call() inside Gateway.Fetch() manage
// the transition.
//
// Because CircuitBreakerRecoveryTimeout is 5 minutes, we fast-forward
// lastFailure to bypass the recovery check and exercise the actual
// state machine transition through the real gateway path.
func TestExecuteTask_HalfOpenTransition(t *testing.T) {
	g := newTestGateway(t)

	provider := &HTTPProvider{
		name:    "fubon",
		limiter: rate.NewLimiter(rate.Inf, 0),
		meta:    ChannelMetadata{ChannelID: "fubon", Country: "TW"},
		fetcher: func(ctx context.Context) ([]byte, error) {
			return []byte(`{"status":"ok"}`), nil
		},
		healthFunc: func(ctx context.Context) (HealthStatus, error) {
			return HealthStatus{Status: "ok", CheckType: "liveness"}, nil
		},
	}
	g.registry.Register("fubon", provider)

	breaker, err := g.breakers.Get("fubon")
	if err != nil {
		t.Fatalf("Get(fubon) failed: %v", err)
	}
	breaker.ForceOpen()
	if breaker.State() != StateOpen {
		t.Fatalf("precondition: breaker should be Open after ForceOpen, got State()=%v", breaker.State())
	}

	breaker.mu.Lock()
	breaker.lastFailure = time.Now().Add(-2 * CircuitBreakerRecoveryTimeout)
	breaker.mu.Unlock()

	m := NewBackgroundTaskManager(g)
	var fetchResult *FetchResult
	task := &ScheduledTask{
		Name:      "channel_health_fubon",
		ChannelID: "fubon",
		Interval:  1 * time.Hour,
		Task: func(ctx context.Context) error {
			result, err := g.Fetch(ctx, "fubon")
			if err != nil {
				return err
			}
			fetchResult = result
			return nil
		},
	}
	task.SetEnabled(true)

	m.executeTask(context.Background(), task)

	if breaker.State() != StateClosed {
		t.Errorf("breaker should be Closed after successful half-open probe, got State()=%v", breaker.State())
	}

	if fetchResult == nil {
		t.Fatal("fetchResult is nil — Gateway.Fetch was not called by executeTask")
	}
	if fetchResult.Fallback {
		t.Error("got fallback data but expected a fresh fetch through half-open probe")
	}
	if string(fetchResult.Data) != `{"status":"ok"}` {
		t.Errorf("unexpected fetch result data: got %q, want %q", string(fetchResult.Data), `{"status":"ok"}`)
	}
}

// =========================================================================
// BackgroundTaskManager — 啟動抖動（Startup Jitter）回歸測試
//
// 設計目的：fresh deploy 時所有 task 的首次執行應立即啟動（不套用抖動），
// 避免長週期任務（如 government_flow_aggregate 28h）在新部署上閒置數小時。
// 後續週期（LastRun 非零值）仍套用 Jitter 以防止多 process 同時重啟時
// 的 thundering herd。
//
// 若未來有人誤刪 `!task.LastRun().IsZero()` 條件，這兩個測試會立即失敗。
// =========================================================================

// TestBackgroundTaskManager_RunTask_AppliesStartupJitter 驗證 runTask
// 的抖動行為分兩階段：
//  1. 首次執行（LastRun==零值）→ 不套用抖動，近乎立即執行。
//  2. 後續週期 → 套用 Jitter 抖動。
//
// 測試技巧（統計式容錯）：
//   - 首次執行應在 50ms 內（Go 排程誤差）。
//   - 設定 LastRun 後再次 Start → 新週期首次應有 Jitter。
func TestBackgroundTaskManager_RunTask_AppliesStartupJitter(t *testing.T) {
	const (
		targetJitter = 500 * time.Millisecond
		firstMax     = 50 * time.Millisecond
		minElapsed   = 1 * time.Millisecond
		maxElapsed   = 700 * time.Millisecond
	)
	m := NewBackgroundTaskManager(nil)

	firstRun := make(chan time.Time, 2)
	runCount := 0
	task := &ScheduledTask{
		Name:     "startup-jitter-task",
		Interval: 1 * time.Hour,
		Jitter:   targetJitter,
		Task: func(ctx context.Context) error {
			select {
			case firstRun <- time.Now():
			default:
			}
			runCount++
			return nil
		},
	}
	task.SetEnabled(true)
	if err := m.Register(task); err != nil {
		t.Fatalf("Register: %v", err)
	}

	// ── Phase A: 首次執行 ──
	ctx, cancelA := context.WithCancel(context.Background())
	defer cancelA()
	startTime := time.Now()
	m.Start(ctx)

	select {
	case execTime := <-firstRun:
		elapsed := execTime.Sub(startTime)
		if elapsed > firstMax {
			t.Errorf("first run (no previous LastRun): elapsed=%v, expected ≤ %v. "+
				"First execution should bypass jitter.", elapsed, firstMax)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout: task did not execute within 2s")
	}
	m.Stop()
	cancelA()

	// ── Phase B: 模擬後續週期（LastRun 非零值 → 應套用抖動）──
	task.SetLastRun(time.Now().Add(-2 * time.Hour))
	ctxB, cancelB := context.WithCancel(context.Background())
	defer cancelB()
	m2 := NewBackgroundTaskManager(nil)
	if err := m2.Register(task); err != nil {
		t.Fatalf("Register phase B: %v", err)
	}
	startTime = time.Now()
	m2.Start(ctxB)
	defer m2.Stop()

	select {
	case execTime := <-firstRun:
		elapsed := execTime.Sub(startTime)
		if elapsed < minElapsed {
			t.Errorf("subsequent run (LastRun non-zero): elapsed=%v, expected ≥ %v. "+
				"Jitter should be applied.", elapsed, minElapsed)
		}
		if elapsed > maxElapsed {
			t.Errorf("subsequent run too late: elapsed=%v, expected within %v",
				elapsed, maxElapsed)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout: task did not execute within 2s")
	}
}

// TestBackgroundTaskManager_RunTask_DesynchronizesMultipleTasks 驗證
// 後續週期中（LastRun 非零值），多個 task 的執行時間因 Jitter 而分散。
//
// 測試技巧：為每個 task 設 SetLastRun(time.Now()) 模擬後續週期，
// 全部用 300ms Jitter。若 runTask 套用隨機抖動，10 個 task 的
// 執行時間應分散在 [0, 300ms) 區間，最晚與最早的差距 ≥ 50ms。
//
// 若未來有人誤刪 `!task.LastRun().IsZero()` 條件或將 rand 改成固定值，
// 所有 task 會在 t≈0 同時執行，差距趨近於 0，本測試會失敗。
func TestBackgroundTaskManager_RunTask_DesynchronizesMultipleTasks(t *testing.T) {
	const (
		numTasks     = 10
		targetJitter = 300 * time.Millisecond
		minSpread    = 50 * time.Millisecond
	)
	m := NewBackgroundTaskManager(nil)

	firstRuns := make([]time.Time, numTasks)
	doneCh := make(chan int, numTasks)
	var mu sync.Mutex

	for i := 0; i < numTasks; i++ {
		idx := i
		task := &ScheduledTask{
			Name:     "desync-task-" + string(rune('A'+i)),
			Interval: 1 * time.Hour,
			Jitter:   targetJitter,
			Task: func(ctx context.Context) error {
				mu.Lock()
				if firstRuns[idx].IsZero() {
					firstRuns[idx] = time.Now()
				}
				mu.Unlock()
				doneCh <- idx
				return nil
			},
		}
		task.SetEnabled(true)
		// 模擬後續週期：設 LastRun 為非零值以觸發 jitter
		task.SetLastRun(time.Now().Add(-2 * time.Hour))
		if err := m.Register(task); err != nil {
			t.Fatalf("Register task %d: %v", i, err)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	startTime := time.Now()
	m.Start(ctx)
	defer m.Stop()

	for received := 0; received < numTasks; {
		select {
		case <-doneCh:
			received++
		case <-time.After(2 * time.Second):
			t.Fatalf("timeout: only %d/%d tasks executed within 2s", received, numTasks)
		}
	}

	mu.Lock()
	defer mu.Unlock()

	var earliest, latest time.Time
	for i, ft := range firstRuns {
		if ft.IsZero() {
			t.Fatalf("task %d first run time not recorded", i)
		}
		rel := ft.Sub(startTime)
		t.Logf("task %d execution at +%v (relative to Start)", i, rel)
		if earliest.IsZero() || rel < earliest.Sub(startTime) {
			earliest = ft
		}
		if rel > latest.Sub(startTime) || latest.IsZero() {
			latest = ft
		}
	}

	spread := latest.Sub(earliest)
	if spread < minSpread {
		t.Errorf("tasks are not desynchronized: spread = %v, expected ≥ %v "+
			"(jitter=%v, numTasks=%d). All tasks clustered near t=0 indicates "+
			"the jitter was replaced with a constant or zero value.", spread, minSpread, targetJitter, numTasks)
	}

	maxAllowed := targetJitter + 200*time.Millisecond
	if latest.Sub(startTime) > maxAllowed {
		t.Errorf("latest task execution too late: %v after Start, expected within %v",
			latest.Sub(startTime), maxAllowed)
	}
}

func TestExecuteTask_SkippedPreservesFailures(t *testing.T) {
	m := NewBackgroundTaskManager(nil)
	calls := 0
	task := &ScheduledTask{
		Name:     "fail-then-skip",
		Interval: 1 * time.Hour,
		Task: func(ctx context.Context) error {
			calls++
			if calls == 1 {
				return errors.New("boom")
			}
			return ErrTaskSkipped
		},
	}
	task.SetEnabled(true)

	m.executeTask(context.Background(), task)
	if got := task.Failures(); got != 1 {
		t.Fatalf("after failure: Failures = %d, want 1", got)
	}
	// No-op ticks must NOT wash the failure away.
	for i := 0; i < 3; i++ {
		task.SetLastRun(time.Now().Add(-2 * time.Hour))
		m.executeTask(context.Background(), task)
	}
	if got := task.Failures(); got != 1 {
		t.Fatalf("after skips: Failures = %d, want 1 (skip is not success)", got)
	}
}

func TestExecuteTask_LastErrorRecordedAndCleared(t *testing.T) {
	m := NewBackgroundTaskManager(nil)
	fail := true
	task := &ScheduledTask{
		Name:     "last-error-task",
		Interval: 1 * time.Hour,
		Task: func(ctx context.Context) error {
			if fail {
				return errors.New("db timeout")
			}
			return nil
		},
	}
	task.SetEnabled(true)

	m.executeTask(context.Background(), task)
	if got := task.LastError(); got != "db timeout" {
		t.Fatalf("LastError = %q, want %q", got, "db timeout")
	}

	fail = false
	task.SetLastRun(time.Now().Add(-2 * time.Hour))
	m.executeTask(context.Background(), task)
	if got := task.LastError(); got != "" {
		t.Fatalf("LastError after success = %q, want empty", got)
	}
}

func TestExecuteTask_OverlapToleranceAbsorbsJitter(t *testing.T) {
	m := NewBackgroundTaskManager(nil)
	ran := false
	task := &ScheduledTask{
		Name:     "jittered-tick",
		Interval: 1 * time.Hour,
		Task: func(ctx context.Context) error {
			ran = true
			return nil
		},
	}
	task.SetEnabled(true)
	// Tick fires 500ms "early" relative to the last run start — within the
	// overlap tolerance, so it must execute (old code skipped it).
	task.SetLastRun(time.Now().Add(-1*time.Hour + 500*time.Millisecond))

	m.executeTask(context.Background(), task)
	if !ran {
		t.Fatal("tick within overlap tolerance was skipped; want executed")
	}
}

func TestExecuteTask_TrueOverlapStillSkipped(t *testing.T) {
	m := NewBackgroundTaskManager(nil)
	ran := false
	task := &ScheduledTask{
		Name:     "true-overlap",
		Interval: 1 * time.Hour,
		Task: func(ctx context.Context) error {
			ran = true
			return nil
		},
	}
	task.SetEnabled(true)
	// Half an interval early — well beyond the tolerance, must be debounced.
	task.SetLastRun(time.Now().Add(-30 * time.Minute))

	m.executeTask(context.Background(), task)
	if ran {
		t.Fatal("tick at half-interval executed; want skipped")
	}
}
