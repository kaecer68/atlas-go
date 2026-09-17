package ledger

import (
	"errors"
	"testing"

	"github.com/kaecer68/atlas-go/internal/domain"
)

type projectionTestStore struct {
	OutcomeStore
	slim     []domain.RecommendationOutcome
	fromSess []domain.RecommendationOutcome
	flat     []domain.RecommendationOutcome
	sessErr  error
	slimErr  error
}

func (p *projectionTestStore) LoadScorecardOutcomes() ([]domain.RecommendationOutcome, error) {
	if p.slimErr != nil {
		return nil, p.slimErr
	}
	return p.slim, nil
}

func (p *projectionTestStore) LoadOutcomesFromSessions() ([]domain.RecommendationOutcome, error) {
	if p.sessErr != nil {
		return nil, p.sessErr
	}
	return p.fromSess, nil
}

func (p *projectionTestStore) LoadOutcomes() ([]domain.RecommendationOutcome, error) {
	return p.flat, nil
}

// TestLoadScorecardProjection_PrefersSlim locks the 2026-09-17 OOM fix: every
// scorecard consumer must read the slim projection instead of the full
// metadata scan when the store provides it.
func TestLoadScorecardProjection_PrefersSlim(t *testing.T) {
	store := &projectionTestStore{
		slim:     []domain.RecommendationOutcome{{AgentID: "slim"}},
		fromSess: []domain.RecommendationOutcome{{AgentID: "full"}},
	}
	got, err := LoadScorecardProjection(store)
	if err != nil {
		t.Fatalf("LoadScorecardProjection: %v", err)
	}
	if len(got) != 1 || got[0].AgentID != "slim" {
		t.Fatalf("expected the slim projection, got %+v", got)
	}
}

// TestLoadScorecardProjection_SlimErrorFallsBack keeps a failing slim read from
// breaking callers.
func TestLoadScorecardProjection_SlimErrorFallsBack(t *testing.T) {
	store := &projectionTestStore{
		slimErr:  errors.New("projection unavailable"),
		fromSess: []domain.RecommendationOutcome{{AgentID: "full"}},
	}
	got, err := LoadScorecardProjection(store)
	if err != nil {
		t.Fatalf("LoadScorecardProjection: %v", err)
	}
	if len(got) != 1 || got[0].AgentID != "full" {
		t.Fatalf("expected the full read, got %+v", got)
	}
}

// TestLoadScorecardProjection_FlatFallback covers JSONL/SQLite ledgers that keep
// outcomes only in the flat file, and ledgers that do not exist at all (the
// read must stay empty-but-successful, not an error).
func TestLoadScorecardProjection_FlatFallback(t *testing.T) {
	store := &projectionTestStore{sessErr: errors.New("no sessions dir"), flat: []domain.RecommendationOutcome{{AgentID: "flat"}}}
	got, err := LoadScorecardProjection(store)
	if err != nil {
		t.Fatalf("LoadScorecardProjection: %v", err)
	}
	if len(got) != 1 || got[0].AgentID != "flat" {
		t.Fatalf("expected the flat read, got %+v", got)
	}
}
