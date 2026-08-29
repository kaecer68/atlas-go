package anomaly

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/alerting"
)

// recordingPublisher captures every PublishAnomaly call for inspection.
// It is goroutine-safe; used to verify the emitter fan-out without
// actually opening a network connection.
type recordingPublisher struct {
	mu    sync.Mutex
	Calls []alerting.AnomalyEvent
	Err   error // optional: when non-nil, PublishAnomaly returns this
}

func (r *recordingPublisher) PublishAnomaly(_ context.Context, ev alerting.AnomalyEvent) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Calls = append(r.Calls, ev)
	return r.Err
}

func (r *recordingPublisher) Count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.Calls)
}

// captureAckStore wraps MemoryStore to capture Saved records; the underlying
// store is accessible for round-trip checks.
type captureAckStore struct {
	*MemoryStore
	mu    sync.Mutex
	Saved []StoredAnomaly
}

func (c *captureAckStore) Save(ev AnomalyEvent) (StoredAnomaly, error) {
	sa, err := c.MemoryStore.Save(ev)
	if err != nil {
		return sa, err
	}
	c.mu.Lock()
	c.Saved = append(c.Saved, sa)
	c.mu.Unlock()
	return sa, nil
}

// countingMetrics satisfies both ScoreRecorder and AnomalyObserver so a
// single test double can feed the detector and the emitter.
type countingMetrics struct {
	mu    sync.Mutex
	Calls []scoreCall
}

func (c *countingMetrics) SetAnomalyScore(tenantID, anomalyType string, score float64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Calls = append(c.Calls, scoreCall{tenantID: tenantID, anomalyType: anomalyType, score: score})
}

func (c *countingMetrics) ObserveAnomaly(tenantID, anomalyType, _ string, score float64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Calls = append(c.Calls, scoreCall{tenantID: tenantID, anomalyType: anomalyType, score: score})
}

// Test_Emitter_ProcessOnce_dispatches_new_events verifies that a single
// ProcessOnce call fans an unseen AnomalyEvent out to publisher + ack store
// + metrics. This is the hot path of the alert/eventbus integration.
func Test_Emitter_ProcessOnce_dispatches_new_events(t *testing.T) {
	now := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	detectorStore := NewStore(100)
	detector := NewDetector(Config{ShortWindow: time.Hour, LongWindow: 24 * time.Hour, BurstZScoreThreshold: 1.0}, &countingMetrics{}, detectorStore)
	detector.now = fixedClock(now)
	for range 20 {
		detector.Observe(testAuditEntry{version: 2, ts: now, tool: "t", tenant: "tenant-a", status: "ok"})
	}

	pub := &recordingPublisher{}
	ackStore := &captureAckStore{MemoryStore: NewMemoryStore(100)}
	em := NewEmitter(EmitterConfig{
		Detector:   detector,
		Publisher:  pub,
		AckStore:   ackStore,
		Observer:   &countingMetrics{},
		BatchSize:  50,
		SeverityFn: fixedSeverity,
	})

	if err := em.ProcessOnce(context.Background()); err != nil {
		t.Fatalf("ProcessOnce: %v", err)
	}
	if pub.Count() != 1 {
		t.Errorf("expected 1 publish, got %d", pub.Count())
	}
	if len(ackStore.Saved) != 1 {
		t.Errorf("expected 1 save, got %d", len(ackStore.Saved))
	}
	if got := pub.Calls[0].AnomalyID; got != ackStore.Saved[0].AnomalyID {
		t.Errorf("AnomalyID mismatch: publisher=%q store=%q", got, ackStore.Saved[0].AnomalyID)
	}
	if got := pub.Calls[0].TenantID; got != "tenant-a" {
		t.Errorf("TenantID: %q", got)
	}
	if got := pub.Calls[0].Type; got != "burst" {
		t.Errorf("Type: %q", got)
	}
	if got := pub.Calls[0].Severity; got != "test-severity" {
		t.Errorf("Severity: %q", got)
	}
}

// Test_Emitter_ProcessOnce_idempotent verifies that re-running ProcessOnce
// without new detector events does NOT re-publish or re-save. Critical
// because the polling goroutine ticks every second.
func Test_Emitter_ProcessOnce_idempotent(t *testing.T) {
	now := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	detectorStore := NewStore(100)
	detector := NewDetector(Config{ShortWindow: time.Hour, LongWindow: 24 * time.Hour, BurstZScoreThreshold: 1.0}, &countingMetrics{}, detectorStore)
	detector.now = fixedClock(now)
	for range 20 {
		detector.Observe(testAuditEntry{version: 2, ts: now, tool: "t", tenant: "tenant-a", status: "ok"})
	}

	pub := &recordingPublisher{}
	em := NewEmitter(EmitterConfig{
		Detector:   detector,
		Publisher:  pub,
		AckStore:   NewMemoryStore(100),
		BatchSize:  50,
		SeverityFn: fixedSeverity,
	})

	for i := range 3 {
		if err := em.ProcessOnce(context.Background()); err != nil {
			t.Fatalf("ProcessOnce %d: %v", i, err)
		}
	}
	if pub.Count() != 1 {
		t.Errorf("expected exactly 1 publish across 3 ticks, got %d", pub.Count())
	}
}

// Test_Emitter_ProcessOnce_dispatches_only_new verifies that the emitter
// catches up when the detector emits multiple events between polls.
func Test_Emitter_ProcessOnce_dispatches_only_new(t *testing.T) {
	now := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	detectorStore := NewStore(100)
	detector := NewDetector(Config{
		ShortWindow:          time.Hour,
		LongWindow:           24 * time.Hour,
		BurstZScoreThreshold: 1.0,
		ErrorRateThreshold:   0.5,
		MinObservations:      3,
	}, &countingMetrics{}, detectorStore)
	detector.now = fixedClock(now)
	// First anomaly: burst for tenant-a
	for range 20 {
		detector.Observe(testAuditEntry{version: 2, ts: now, tool: "t", tenant: "tenant-a", status: "ok"})
	}
	pub := &recordingPublisher{}
	em := NewEmitter(EmitterConfig{
		Detector:   detector,
		Publisher:  pub,
		AckStore:   NewMemoryStore(100),
		BatchSize:  50,
		SeverityFn: fixedSeverity,
	})
	if err := em.ProcessOnce(context.Background()); err != nil {
		t.Fatalf("first ProcessOnce: %v", err)
	}
	first := pub.Count()
	if first != 1 {
		t.Fatalf("expected 1 after first tick, got %d", first)
	}

	// Second anomaly: tool_error_spike (use a different tool name with errors)
	for range 5 {
		detector.Observe(testAuditEntry{version: 2, ts: now.Add(time.Millisecond), tool: "fail-tool", tenant: "tenant-a", status: "error"})
	}
	if err := em.ProcessOnce(context.Background()); err != nil {
		t.Fatalf("second ProcessOnce: %v", err)
	}
	if pub.Count() != 2 {
		t.Errorf("expected 2 cumulative publishes, got %d", pub.Count())
	}
	// The new publish should be the tool_error_spike, not a re-burst.
	latest := pub.Calls[pub.Count()-1]
	if latest.Type != "tool_error_spike" {
		t.Errorf("expected tool_error_spike, got %q", latest.Type)
	}
}

// Test_Emitter_ProcessOnce_swallows_publisher_error verifies that a webhook
// failure does NOT stop subsequent events from being dispatched. The
// publisher error is logged but the emitter continues — the detector must
// never block on alert sink failures.
func Test_Emitter_ProcessOnce_swallows_publisher_error(t *testing.T) {
	now := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	detectorStore := NewStore(100)
	detector := NewDetector(Config{ShortWindow: time.Hour, LongWindow: 24 * time.Hour, BurstZScoreThreshold: 1.0}, &countingMetrics{}, detectorStore)
	detector.now = fixedClock(now)
	for range 20 {
		detector.Observe(testAuditEntry{version: 2, ts: now, tool: "t", tenant: "tenant-a", status: "ok"})
	}
	pub := &recordingPublisher{Err: errors.New("webhook down")}
	ackStore := &captureAckStore{MemoryStore: NewMemoryStore(100)}
	em := NewEmitter(EmitterConfig{
		Detector:   detector,
		Publisher:  pub,
		AckStore:   ackStore,
		BatchSize:  50,
		SeverityFn: fixedSeverity,
	})
	if err := em.ProcessOnce(context.Background()); err != nil {
		t.Fatalf("ProcessOnce should not propagate publisher error, got %v", err)
	}
	// The store save should still have happened.
	if len(ackStore.Saved) != 1 {
		t.Errorf("expected 1 save even when publisher errors, got %d", len(ackStore.Saved))
	}
}

// Test_Emitter_Run_stops_on_context_cancel verifies the goroutine exits
// cleanly when the parent context is cancelled.
func Test_Emitter_Run_stops_on_context_cancel(t *testing.T) {
	now := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	detectorStore := NewStore(100)
	detector := NewDetector(Config{ShortWindow: time.Hour, LongWindow: 24 * time.Hour, BurstZScoreThreshold: 1.0}, &countingMetrics{}, detectorStore)
	detector.now = fixedClock(now)
	em := NewEmitter(EmitterConfig{
		Detector:   detector,
		Publisher:  &recordingPublisher{},
		AckStore:   NewMemoryStore(100),
		BatchSize:  50,
		SeverityFn: fixedSeverity,
		Interval:   5 * time.Millisecond,
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		em.Run(ctx)
		close(done)
	}()
	cancel()
	select {
	case <-done:
		// ok
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not stop within 2s of context cancel")
	}
}

// Test_Emitter_ProcessOnce_handles_empty_store verifies the empty case.
func Test_Emitter_ProcessOnce_handles_empty_store(t *testing.T) {
	detectorStore := NewStore(100)
	detector := NewDetector(Config{}, &countingMetrics{}, detectorStore)
	pub := &recordingPublisher{}
	em := NewEmitter(EmitterConfig{
		Detector:   detector,
		Publisher:  pub,
		AckStore:   NewMemoryStore(100),
		BatchSize:  50,
		SeverityFn: fixedSeverity,
	})
	if err := em.ProcessOnce(context.Background()); err != nil {
		t.Fatalf("ProcessOnce on empty store: %v", err)
	}
	if pub.Count() != 0 {
		t.Errorf("expected 0 publishes, got %d", pub.Count())
	}
}

// Test_defaultSeverityFn maps score ranges to severity strings. Documented
// here so SREs have a single place to find the threshold table.
func Test_defaultSeverityFn(t *testing.T) {
	cases := []struct {
		score float64
		want  string
	}{
		{0.0, "low"},
		{1.0, "low"},
		{3.0, "medium"},
		{5.0, "high"},
		{10.0, "high"},
	}
	for _, tc := range cases {
		got := defaultSeverityFn("burst", tc.score)
		if got != tc.want {
			t.Errorf("score=%v want=%q got=%q", tc.score, tc.want, got)
		}
	}
}

// fixedSeverity returns a constant severity string — used when the test
// doesn't care about the severity mapping.
func fixedSeverity(_ string, _ float64) string { return "test-severity" }
