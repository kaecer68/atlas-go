package repository

import (
	"context"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// stubSessionSummaryStore is a configurable mock that lets tests control
// what LoadSessionSummaries returns. It satisfies SessionSummaryStore.
type stubSessionSummaryStore struct {
	loadedSummaries []domain.SessionSummary
	loadErr         error
}

func (s *stubSessionSummaryStore) RecordSessionSummary(_ domain.ReplaySession, summary domain.SessionSummary) error {
	return nil
}

func (s *stubSessionSummaryStore) LoadSessionSummaries() ([]domain.SessionSummary, error) {
	return s.loadedSummaries, s.loadErr
}

func (s *stubSessionSummaryStore) LoadAllSessionScorecards() ([]domain.Scorecard, []domain.RecommendationOutcome, error) {
	return nil, nil, nil
}

// makeStubSessionSummaries builds n synthetic session summaries for tests.
func makeStubSessionSummaries(n int) []domain.SessionSummary {
	out := make([]domain.SessionSummary, n)
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < n; i++ {
		out[i] = domain.SessionSummary{
			SessionID:      "sess-test-" + string(rune('A'+i)),
			Regime:         domain.RegimeRiskOn,
			OrderCount:     i,
			OutcomeCount:   i * 2,
			PortfolioValue: 1000000 + float64(i)*1000,
			RecordedAt:     base.Add(time.Duration(i) * 24 * time.Hour),
		}
	}
	return out
}

// S1 — LoadAllSessionSummaries MUST fall back to JSONL when PG is unavailable.
// REGRESSION for empty Evolution page: production server had r.pg == nil
// (no PostgreSQL env vars), buggy code returned (nil, nil), JSONL fallback
// was never invoked, evolution page rendered "system has not accumulated
// enough session data" despite 109 sessions in data/state/sessions/.
func TestDualWriteRepository_LoadAllSessionSummaries_PGNil_FallsBackToJSONL(t *testing.T) {
	want := makeStubSessionSummaries(3)
	jsonl := &JSONLRepository{
		sessionSummaryStore: &stubSessionSummaryStore{loadedSummaries: want},
	}
	repo := &DualWriteRepository{jsonl: jsonl} // pg == nil — production case

	got, err := repo.LoadAllSessionSummaries(context.Background())
	if err != nil {
		t.Fatalf("LoadAllSessionSummaries returned error: %v", err)
	}
	if len(got) != len(want) {
		t.Fatalf("expected %d summaries from JSONL fallback, got %d (nil=%v)", len(want), len(got), got == nil)
	}
	for i := range got {
		if got[i].SessionID != want[i].SessionID {
			t.Errorf("summary[%d].SessionID = %q, want %q", i, got[i].SessionID, want[i].SessionID)
		}
	}
}

// S2 — LoadSessionSummary (singular) MUST fall back to JSONL when PG is
// unavailable. Same regression as S1; the pipeline calls both methods
// (singular for chart anchors, plural for the full history list).
func TestDualWriteRepository_LoadSessionSummary_PGNil_FallsBackToJSONL(t *testing.T) {
	want := makeStubSessionSummaries(3)
	jsonl := &JSONLRepository{
		sessionSummaryStore: &stubSessionSummaryStore{loadedSummaries: want},
	}
	repo := &DualWriteRepository{jsonl: jsonl}

	got, err := repo.LoadSessionSummary(context.Background(), "sess-test-B")
	if err != nil {
		t.Fatalf("LoadSessionSummary returned error: %v", err)
	}
	if got == nil {
		t.Fatalf("LoadSessionSummary returned nil; expected fallback to JSONL summary sess-test-B")
	}
	if got.SessionID != "sess-test-B" {
		t.Errorf("SessionID = %q, want %q", got.SessionID, "sess-test-B")
	}
	if got.OrderCount != 1 {
		t.Errorf("OrderCount = %d, want 1 (the B-summary)", got.OrderCount)
	}
}

// Regression guard — when JSONL is also empty (cold start), must return
// empty slice with nil error, not a panic or a wrapped error.
func TestDualWriteRepository_LoadAllSessionSummaries_BothEmpty_ReturnsEmpty(t *testing.T) {
	jsonl := &JSONLRepository{
		sessionSummaryStore: &stubSessionSummaryStore{loadedSummaries: nil},
	}
	repo := &DualWriteRepository{jsonl: jsonl}

	got, err := repo.LoadAllSessionSummaries(context.Background())
	if err != nil {
		t.Fatalf("expected nil error on cold start, got %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected 0 summaries on cold start, got %d", len(got))
	}
}
