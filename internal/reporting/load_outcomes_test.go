package reporting

import (
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// countingOutcomeStore records which per-session read path loadAllOutcomes uses.
type countingOutcomeStore struct {
	*fakeSourceStore
	slimCalls int
	fullCalls int
}

func (c *countingOutcomeStore) LoadSessionScorecardOutcomes(string) ([]domain.RecommendationOutcome, error) {
	c.slimCalls++
	return []domain.RecommendationOutcome{{AgentID: "a", Hit: true, ForwardReturn: 0.01}}, nil
}

func (c *countingOutcomeStore) LoadSessionOutcomes(string) ([]domain.RecommendationOutcome, error) {
	c.fullCalls++
	return nil, nil
}

// TestLoadAllOutcomes_PrefersSlimPerSessionProjection is the regression for the
// 2026-09-17 OOM: the performance report accumulated every session's outcomes
// including the 644 MB metadata blob, allocating gigabytes per generation. When
// the store offers the slim per-session projection, it must be used.
func TestLoadAllOutcomes_PrefersSlimPerSessionProjection(t *testing.T) {
	store := &countingOutcomeStore{fakeSourceStore: &fakeSourceStore{}}
	summaries := []domain.SessionSummary{
		{SessionID: "session-20260916-daily"},
		{SessionID: "session-20260915-daily"},
	}

	got := loadAllOutcomes(store, summaries)

	if store.slimCalls != 2 {
		t.Errorf("expected the slim projection per session (2), got %d", store.slimCalls)
	}
	if store.fullCalls != 0 {
		t.Errorf("full metadata read must not be used when slim is available, got %d", store.fullCalls)
	}
	if len(got) != 2 {
		t.Errorf("expected 2 outcomes, got %d", len(got))
	}
}

// TestLoadAllOutcomes_FallsBackWhenSlimUnavailable keeps the jsonl/sqlite
// backends working: no slim method → full per-session read.
func TestLoadAllOutcomes_FallsBackWhenSlimUnavailable(t *testing.T) {
	store := &fakeSourceStore{}
	summaries := []domain.SessionSummary{{SessionID: "session-20260916-daily"}}

	if got := loadAllOutcomes(store, summaries); got != nil {
		t.Errorf("expected nil outcomes from the fake full read, got %v", got)
	}
}

// delayedStore records the concurrency it observes, so the parallel load path
// can be asserted without timing flakiness.
type delayedStore struct {
	*fakeSourceStore
	mu        sync.Mutex
	inFlight  int
	maxSeen   int
	sessions  []string
	perCallMs int
}

func (d *delayedStore) LoadSessionScorecardOutcomes(sessionID string) ([]domain.RecommendationOutcome, error) {
	d.mu.Lock()
	d.inFlight++
	if d.inFlight > d.maxSeen {
		d.maxSeen = d.inFlight
	}
	d.sessions = append(d.sessions, sessionID)
	d.mu.Unlock()

	time.Sleep(time.Duration(d.perCallMs) * time.Millisecond)

	d.mu.Lock()
	d.inFlight--
	d.mu.Unlock()
	return []domain.RecommendationOutcome{{AgentID: sessionID, Hit: true}}, nil
}

// TestLoadAllOutcomes_LoadsSessionsConcurrently guards the N+1 latency fix: the
// production ledger has ~200 sessions, and a sequential load pushed a cold
// generation past the 8 s request timeout.
func TestLoadAllOutcomes_LoadsSessionsConcurrently(t *testing.T) {
	store := &delayedStore{fakeSourceStore: &fakeSourceStore{}, perCallMs: 20}
	summaries := make([]domain.SessionSummary, 24)
	for i := range summaries {
		summaries[i] = domain.SessionSummary{SessionID: "session-" + strconv.Itoa(i) + "-daily"}
	}

	start := time.Now()
	got := loadAllOutcomes(store, summaries)
	elapsed := time.Since(start)

	if len(got) != len(summaries) {
		t.Fatalf("expected %d outcomes, got %d", len(summaries), len(got))
	}
	if store.maxSeen < 2 {
		t.Errorf("expected concurrent session loads, max in flight = %d", store.maxSeen)
	}
	// 24 calls x 20ms sequential = 480ms; with >=2 workers this must be lower.
	if elapsed >= 480*time.Millisecond {
		t.Errorf("load took %v, expected concurrency to beat the sequential bound", elapsed)
	}
	// Output order must follow the summary order (deterministic fan-in), even
	// though the per-session calls complete in arbitrary order.
	for i, o := range got {
		if o.AgentID != summaries[i].SessionID {
			t.Fatalf("outcome %d = %q, want %q (fan-in must preserve summary order)", i, o.AgentID, summaries[i].SessionID)
		}
	}
	if len(store.sessions) != len(summaries) {
		t.Errorf("expected %d session loads, got %d", len(summaries), len(store.sessions))
	}
}
