package orchestrator

import (
	"errors"
	"strings"
	"testing"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/sim"
)

// FU-20261016-03 regression tests for the candidate-selection call site.
//
// NextExperimentCandidate used to write `_ = ledger.RecordExperiment(...)` and
// `_ = ledger.RecordSessionExperiment(...)`. Together with a session-scoped
// INSERT that had no ON CONFLICT, that produced 894/894 production experiments
// rows with session_id = NULL and no signal that anything failed. These tests
// pin the observable contract:
//   - a session-link write failure is reported at ERROR (never silently dropped),
//   - and the candidate pipeline still returns its candidate (availability is
//     preserved on purpose — see the trade-off note in system.go).

// candidateSelectionStore is a ledger.OutcomeStore whose scorecard projection
// yields one weak agent, so NextExperimentCandidate produces a candidate and
// exercises the experiment writes.
type candidateSelectionStore struct {
	ledger.OutcomeStore
	globalExpErr  error
	sessionExpErr error
	globalCalls   int
	linkCalls     int
}

// errSessionLinkUnavailable stands in for a store outage.
var errSessionLinkUnavailable = errors.New("ledger store unavailable")

func (s *candidateSelectionStore) LoadScorecardOutcomes() ([]domain.RecommendationOutcome, error) {
	return []domain.RecommendationOutcome{{
		AgentID:       "taiwan-macro-01",
		Skill:         "taiwan_macro",
		Layer:         domain.LayerContext,
		Window:        "2026-10-W1",
		ForwardReturn: -0.02,
		Hit:           false,
	}}, nil
}

func (s *candidateSelectionStore) RecordExperiment(domain.ExperimentRecord) error {
	s.globalCalls++
	return s.globalExpErr
}

func (s *candidateSelectionStore) RecordSessionExperiment(
	domain.ReplaySession, domain.ExperimentRecord,
) error {
	s.linkCalls++
	return s.sessionExpErr
}

func newCandidateSelectionSystem(t *testing.T, store ledger.OutcomeStore) *System {
	t.Helper()
	return &System{
		SystemCore: &SystemCore{
			sim: SimulationCore{
				cfg:      config.Config{PrimaryMarket: "TW", LedgerDir: t.TempDir()},
				engine:   sim.NewEngine(domain.SimulationConstraints{StartingCash: 1_000_000}),
				registry: SeedRegistry(),
				session:  domain.ReplaySession{ID: "session-link-test"},
				ledger:   store,
			},
			plugins: NewPluginRegistry(),
		},
	}
}

// TestNextExperimentCandidate_SessionLinkFailureIsLoggedAndDoesNotAbort is the
// defect gate: when the session-scoped write fails, the failure must appear in
// the log at ERROR — and the caller must still receive its candidate.
func TestNextExperimentCandidate_SessionLinkFailureIsLoggedAndDoesNotAbort(t *testing.T) {
	logs := captureOrchestratorLogs(t)
	store := &candidateSelectionStore{sessionExpErr: errSessionLinkUnavailable}
	sys := newCandidateSelectionSystem(t, store)

	candidate, err := sys.NextExperimentCandidate()
	if err != nil {
		t.Fatalf("NextExperimentCandidate must not fail on a store write error: %v", err)
	}
	if candidate == nil {
		t.Fatal("candidate selection must still produce a candidate")
	}
	if store.linkCalls != 1 {
		t.Fatalf("RecordSessionExperiment calls = %d, want 1", store.linkCalls)
	}

	out := logs.String()
	for _, want := range []string{
		"level=ERROR",
		"record_session_experiment_failed",
		"experiment_id=" + candidate.Experiment.ID,
		"session_id=session-link-test",
		errSessionLinkUnavailable.Error(),
	} {
		if !strings.Contains(out, want) {
			t.Errorf("log is missing %q; got:\n%s", want, out)
		}
	}
}

// TestNextExperimentCandidate_GlobalWriteFailureIsLogged covers the sibling
// write: RecordExperiment was also `_ =` and the SQLite backend raised a
// UNIQUE violation for a retried experiment_id (fixed to DO NOTHING), so its
// error must be reported rather than discarded.
func TestNextExperimentCandidate_GlobalWriteFailureIsLogged(t *testing.T) {
	logs := captureOrchestratorLogs(t)
	store := &candidateSelectionStore{globalExpErr: errSessionLinkUnavailable}
	sys := newCandidateSelectionSystem(t, store)

	if _, err := sys.NextExperimentCandidate(); err != nil {
		t.Fatalf("NextExperimentCandidate must not fail on a store write error: %v", err)
	}
	if out := logs.String(); !strings.Contains(out, "record_experiment_failed") {
		t.Errorf("log is missing record_experiment_failed; got:\n%s", out)
	}
}

// TestNextExperimentCandidate_HealthyStoreLogsNoError is the negative control:
// with a working store the new logging must stay silent, so the ERROR line
// keeps meaning "a link was actually lost".
func TestNextExperimentCandidate_HealthyStoreLogsNoError(t *testing.T) {
	logs := captureOrchestratorLogs(t)
	store := &candidateSelectionStore{}
	sys := newCandidateSelectionSystem(t, store)

	if _, err := sys.NextExperimentCandidate(); err != nil {
		t.Fatalf("NextExperimentCandidate: %v", err)
	}
	if store.linkCalls != 1 || store.globalCalls == 0 {
		t.Fatalf("writes must still happen: link=%d global=%d", store.linkCalls, store.globalCalls)
	}
	if out := logs.String(); strings.Contains(out, "level=ERROR") {
		t.Errorf("a healthy store must not log ERROR; got:\n%s", out)
	}
}
