package system

// Q2 batch 2, unit 2: the capital-phase handler's session-history fallback
// reads through an injected backend-aware ledger store (PostgreSQL first,
// JSONL fallback) and keeps the legacy direct-file read under Svc.LedgerDir
// when no store is injected. These tests are hermetic (no PostgreSQL server):
// the real server half lives in capital_phase_pg_integration_test.go behind
// `-tags=integration`.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/monitoring/api/shared"
	"github.com/kaecer68/atlas-go/internal/monitoring/service"
)

const (
	q2b2Session13 = "session-20260413-daily"
	q2b2Session14 = "session-20260414-daily"
	q2b2Session15 = "session-20260415-daily"
)

// q2b2RecordingStore implements ledger.OutcomeStore by embedding the interface
// (nil) so only the summary read under test is implemented — any other method
// panics, which keeps the seam honest. Every read is counted so a test can
// prove the store answered instead of the files.
type q2b2RecordingStore struct {
	ledger.OutcomeStore

	summaries []domain.SessionSummary
	calls     int
	err       error

	// source/degraded are reported through the optional interfaces the
	// PG-first store implements, so the handler can label its response.
	source   string
	degraded bool
}

func (s *q2b2RecordingStore) LoadSessionSummaries() ([]domain.SessionSummary, error) {
	s.calls++
	return s.summaries, s.err
}

func (s *q2b2RecordingStore) SourceBackend() string { return s.source }

func (s *q2b2RecordingStore) Degraded() bool { return s.degraded }

// q2b2PlainStore reports no backend (no SourceBackend/Degraded methods), which
// is what a parity fixture needs: both paths then serialize identically.
type q2b2PlainStore struct {
	ledger.OutcomeStore
	summaries []domain.SessionSummary
	calls     int
}

func (s *q2b2PlainStore) LoadSessionSummaries() ([]domain.SessionSummary, error) {
	s.calls++
	return s.summaries, nil
}

// q2b2Summaries is the fixture series: it dips (1,020,000 → 900,000), so a
// non-zero max drawdown proves which series was read.
func q2b2Summaries() []domain.SessionSummary {
	return []domain.SessionSummary{
		{
			SessionID:      q2b2Session13,
			PortfolioValue: 1_000_000,
			EndingCash:     400_000,
			TotalTaxPaid:   1_000,
			RecordedAt:     time.Date(2026, 4, 13, 12, 0, 0, 0, time.UTC),
		},
		{
			SessionID:      q2b2Session14,
			PortfolioValue: 1_020_000,
			EndingCash:     410_000,
			TotalTaxPaid:   2_000,
			RecordedAt:     time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC),
		},
		{
			SessionID:      q2b2Session15,
			PortfolioValue: 900_000,
			EndingCash:     360_000,
			TotalTaxPaid:   3_000,
			RecordedAt:     time.Date(2026, 4, 15, 12, 0, 0, 0, time.UTC),
		},
	}
}

// q2b2WriteSummaries writes the given summaries to disk in the layout the
// legacy file path reads (LedgerDir/sessions/<id>/summary.json).
func q2b2WriteSummaries(t *testing.T, ledgerDir string, summaries []domain.SessionSummary) {
	t.Helper()
	for _, summary := range summaries {
		sessionDir := filepath.Join(ledgerDir, "sessions", summary.SessionID)
		if err := os.MkdirAll(sessionDir, 0o755); err != nil {
			t.Fatalf("os.MkdirAll: %v", err)
		}
		b, err := json.Marshal(summary)
		if err != nil {
			t.Fatalf("marshal summary: %v", err)
		}
		if err := os.WriteFile(filepath.Join(sessionDir, "summary.json"), b, 0o644); err != nil {
			t.Fatalf("write summary.json: %v", err)
		}
	}
}

// q2b2Handlers builds the handler the way the dashboard does: a SystemService
// with the given ledger dir, optionally wrapped with an injected store. The
// work dir holds no live state files, so nothing overrides the capital series.
func q2b2Handlers(t *testing.T, ledgerDir string) *Handlers {
	t.Helper()
	workDir := t.TempDir()
	svc := service.NewSystemService(workDir, ledgerDir, filepath.Join(workDir, "baseline.json"), ledger.NewStore(ledgerDir), nil, nil)
	return &Handlers{Svc: svc}
}

// q2b2CapitalPhase drives GET /api/dashboard/capital-phase through the shared
// GET adapter (the production path, so the JSON contract is exercised) and
// decodes the snapshot.
func q2b2CapitalPhase(t *testing.T, h *Handlers) domain.CapitalSnapshot {
	t.Helper()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/capital-phase", nil)
	shared.Get(h.HandleCapitalPhase).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("capital-phase status = %d, want %d", w.Code, http.StatusOK)
	}
	var snap domain.CapitalSnapshot
	if err := json.Unmarshal(w.Body.Bytes(), &snap); err != nil {
		t.Fatalf("decode capital-phase: %v", err)
	}
	return snap
}

// q2b2NoClock blanks the two fields that depend on the wall clock (days since
// the first session), so two payloads built in the same test compare equal
// across a midnight boundary.
func q2b2NoClock(s domain.CapitalSnapshot) domain.CapitalSnapshot {
	s.PhaseStartDate = time.Time{}
	s.DaysInPhase = 0
	return s
}

func TestHandlers_CapitalPhaseUsesInjectedStore(t *testing.T) {
	// The files hold a DIFFERENT, monotonic series (500,000 → 505,000): if the
	// handler read them instead of the store, the totals and the drawdown would
	// not match the store's series.
	ledgerDir := t.TempDir()
	q2b2WriteSummaries(t, ledgerDir, []domain.SessionSummary{
		{SessionID: q2b2Session13, PortfolioValue: 500_000, RecordedAt: time.Date(2026, 4, 13, 12, 0, 0, 0, time.UTC)},
		{SessionID: q2b2Session14, PortfolioValue: 505_000, RecordedAt: time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC)},
	})

	store := &q2b2RecordingStore{summaries: q2b2Summaries(), source: "postgres"}
	h := q2b2Handlers(t, ledgerDir).WithStore(store)

	snap := q2b2CapitalPhase(t, h)

	if store.calls == 0 {
		t.Error("LoadSessionSummaries calls = 0; the capital-phase history did not use the injected store")
	}
	if snap.TotalCapital != 900_000 {
		t.Errorf("total_capital = %v, want 900000 (the store's latest session; 505000 would mean the files answered)", snap.TotalCapital)
	}
	if snap.ReserveCash != 360_000 {
		t.Errorf("reserve_cash = %v, want 360000 (the store's latest session)", snap.ReserveCash)
	}
	if snap.DeployedCapital != 540_000 {
		t.Errorf("deployed_capital = %v, want 540000 (total - reserve from the store series)", snap.DeployedCapital)
	}
	if snap.MaxDrawdown == 0 {
		t.Error("max_drawdown = 0; the store series dips 1020000 → 900000, so the file series (monotonic) was read")
	}
	if snap.Source != "postgres" {
		t.Errorf("source = %q, want postgres (the injected store labels the L-cold read)", snap.Source)
	}
}

func TestHandlers_CapitalPhaseUsesFilesWithoutStore(t *testing.T) {
	ledgerDir := t.TempDir()
	q2b2WriteSummaries(t, ledgerDir, q2b2Summaries())

	snap := q2b2CapitalPhase(t, q2b2Handlers(t, ledgerDir))

	// Golden pin of the file path (no store injected), so the parity test
	// below cannot pass by having BOTH paths broken in the same way.
	if snap.TotalCapital != 900_000 {
		t.Errorf("total_capital = %v, want 900000", snap.TotalCapital)
	}
	if snap.ReserveCash != 360_000 {
		t.Errorf("reserve_cash = %v, want 360000", snap.ReserveCash)
	}
	if snap.DeployedCapital != 540_000 {
		t.Errorf("deployed_capital = %v, want 540000", snap.DeployedCapital)
	}
	if diff := snap.MaxDrawdown - 0.11764705882352941; diff > 1e-9 || diff < -1e-9 {
		t.Errorf("max_drawdown = %v, want 0.11764705882352941 (1020000 → 900000)", snap.MaxDrawdown)
	}
	if snap.Source != "" || snap.Degraded {
		t.Errorf("source/degraded = %q/%v, want empty/false for the legacy file path", snap.Source, snap.Degraded)
	}
	if snap.PhaseStartDate.IsZero() {
		t.Error("phase_start_date is zero; the file series did not reach the payload")
	}
}

func TestHandlers_CapitalPhaseStoreAndFilePathsAgree(t *testing.T) {
	ledgerDir := t.TempDir()
	summaries := q2b2Summaries()
	q2b2WriteSummaries(t, ledgerDir, summaries)

	// A store that reports no backend: both paths must then produce the same
	// payload, so any difference is a real read-path difference.
	store := &q2b2PlainStore{summaries: summaries}
	fromStore := q2b2Handlers(t, ledgerDir).WithStore(store)
	fromFiles := q2b2Handlers(t, ledgerDir)

	storePoints := fromStore.capitalHistory()
	filePoints := fromFiles.capitalHistory()
	if !reflect.DeepEqual(storePoints, filePoints) {
		t.Fatalf("history points differ\n store = %+v\n files = %+v", storePoints, filePoints)
	}
	if store.calls == 0 {
		t.Error("LoadSessionSummaries calls = 0; the store path did not read the store")
	}

	want := q2b2NoClock(q2b2CapitalPhase(t, fromStore))
	got := q2b2NoClock(q2b2CapitalPhase(t, fromFiles))
	wantJSON, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("marshal store payload: %v", err)
	}
	gotJSON, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal file payload: %v", err)
	}
	if string(wantJSON) != string(gotJSON) {
		t.Errorf("store and file paths disagree\n store = %s\n files = %s", wantJSON, gotJSON)
	}
}

// TestHandlers_CapitalPhaseStoreErrorYieldsNoHistory pins the error branch:
// an unreadable store must not fabricate a series (the file fixture is present
// and must stay unused).
func TestHandlers_CapitalPhaseStoreErrorYieldsNoHistory(t *testing.T) {
	ledgerDir := t.TempDir()
	q2b2WriteSummaries(t, ledgerDir, q2b2Summaries())

	store := &q2b2RecordingStore{err: os.ErrPermission, source: "jsonl", degraded: true}
	snap := q2b2CapitalPhase(t, q2b2Handlers(t, ledgerDir).WithStore(store))

	if snap.TotalCapital != 0 {
		t.Errorf("total_capital = %v, want 0 (the store read failed; the file fixture must not answer)", snap.TotalCapital)
	}
}
