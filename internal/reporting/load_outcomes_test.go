package reporting

import (
	"testing"

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
