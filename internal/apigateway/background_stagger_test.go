package apigateway

import (
	"context"
	"testing"
	"time"
)

// TestStartupStaggerDelay_DeterministicAndBounded covers the #1763 fix: fresh
// process starts must spread first runs deterministically across a bounded
// window instead of firing every task simultaneously.
func TestStartupStaggerDelay_DeterministicAndBounded(t *testing.T) {
	// Deterministic: same name+interval -> same delay.
	a := startupStaggerDelay("channel_health_finmind", time.Hour)
	b := startupStaggerDelay("channel_health_finmind", time.Hour)
	if a != b {
		t.Fatalf("not deterministic: %v vs %v", a, b)
	}

	// Bounded: never exceeds min(2min, interval/10).
	for _, tc := range []struct {
		name string
		itvl time.Duration
		max  time.Duration
	}{
		{"hourly-probe", time.Hour, 2 * time.Minute},
		{"fast-probe", 5 * time.Minute, 30 * time.Second}, // interval/10 caps
		{"daily-task", 24 * time.Hour, 2 * time.Minute},
	} {
		d := startupStaggerDelay(tc.name, tc.itvl)
		if d < 0 || d > tc.max {
			t.Errorf("%s: delay %v outside (0,%v]", tc.name, d, tc.max)
		}
	}

	// Spread: distinct names produce distinct delays (hash coverage sanity).
	seen := map[time.Duration]bool{}
	for _, n := range []string{"a", "b", "c", "d", "e", "f", "g", "h", "auto_quote_backfill", "auto_twse_insider", "channel_health_finmind", "channel_health_bdi"} {
		seen[startupStaggerDelay(n, time.Hour)] = true
	}
	if len(seen) < 2 {
		t.Errorf("all sample names collapsed to one delay; hash not spreading")
	}

	// Zero interval -> no delay.
	if d := startupStaggerDelay("x", 0); d != 0 {
		t.Errorf("interval=0 -> delay %v, want 0", d)
	}
}

// TestBackgroundTaskManager_Start_WithStagger_FirstRunDeferred verifies the
// production wiring (#1763): with WithStartupStagger(true), a fresh start
// does NOT execute immediately — the first run waits for the hashed stagger
// delay (bounded by min(2min, interval/10)).
func TestBackgroundTaskManager_Start_WithStagger_FirstRunDeferred(t *testing.T) {
	m := NewBackgroundTaskManager(nil).WithStartupStagger(true)

	ran := make(chan struct{}, 1)
	task := &ScheduledTask{
		Name:     "stagger-deferred-task",
		Interval: time.Hour,
		Task: func(ctx context.Context) error {
			select {
			case ran <- struct{}{}:
			default:
			}
			return nil
		},
	}
	task.SetEnabled(true)
	if err := m.Register(task); err != nil {
		t.Fatalf("Register: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m.Start(ctx)

	// The name-hash delay for this task under a 6-min window (interval/10 of
	// 1h) is startupStaggerDelay("stagger-deferred-task", time.Hour); if the
	// hash happens to be 0 the first run is legitimately immediate — skip.
	if startupStaggerDelay("stagger-deferred-task", time.Hour) == 0 {
		t.Skip("hash delay is 0 for this name; nothing to assert")
	}

	select {
	case <-ran:
		t.Fatal("task executed before stagger delay elapsed")
	case <-time.After(300 * time.Millisecond):
		// Expected: still deferred within the stagger window.
	}
}
