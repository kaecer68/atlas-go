package service

// Q2 batch 2, unit 2: the monitoring services' L-cold session-summary reads
// (LiveService equity curve / portfolio history, BacktestService run window) go
// through an injected backend-aware ledger store (PostgreSQL first, JSONL
// fallback) and keep the legacy JSONL ledger read when no store is injected.
// These tests are hermetic (no PostgreSQL server): the real server half lives
// in summaries_pg_integration_test.go behind `-tags=integration`.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/backtest"
	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/ledger"
)

const (
	q2b2Session13 = "session-20260413-daily"
	q2b2Session14 = "session-20260414-daily"
	q2b2Session15 = "session-20260415-daily"
)

// q2b2RecordingStore implements ledger.OutcomeStore by embedding the interface
// (nil) so only the two reads under test are implemented — any other method
// panics, which keeps the seam honest. Every read is counted so a test can
// prove the store answered instead of the files.
type q2b2RecordingStore struct {
	ledger.OutcomeStore

	summaries      []domain.SessionSummary
	summaryCalls   int
	scorecardCalls int

	source   string
	degraded bool
}

func (s *q2b2RecordingStore) LoadSessionSummaries() ([]domain.SessionSummary, error) {
	s.summaryCalls++
	return s.summaries, nil
}

func (s *q2b2RecordingStore) LoadAllSessionScorecards() ([]domain.Scorecard, []domain.RecommendationOutcome, error) {
	s.scorecardCalls++
	return nil, nil, nil
}

func (s *q2b2RecordingStore) SourceBackend() string { return s.source }

func (s *q2b2RecordingStore) Degraded() bool { return s.degraded }

// q2b2PlainStore reports no backend, which is what a parity fixture needs.
type q2b2PlainStore struct {
	ledger.OutcomeStore
	summaries []domain.SessionSummary
	calls     int
}

func (s *q2b2PlainStore) LoadSessionSummaries() ([]domain.SessionSummary, error) {
	s.calls++
	return s.summaries, nil
}

func (s *q2b2PlainStore) LoadAllSessionScorecards() ([]domain.Scorecard, []domain.RecommendationOutcome, error) {
	return nil, nil, nil
}

// q2b2StoreSummary is the fixture series: it dips (1,020,000 → 900,000), so a
// different last value proves which series was read.
func q2b2StoreSummary() []domain.SessionSummary {
	return []domain.SessionSummary{
		{SessionID: q2b2Session13, PortfolioValue: 1_000_000, EndingCash: 400_000, TotalTaxPaid: 1_000, RecordedAt: time.Date(2026, 4, 13, 12, 0, 0, 0, time.UTC)},
		{SessionID: q2b2Session14, PortfolioValue: 1_020_000, EndingCash: 410_000, TotalTaxPaid: 2_000, RecordedAt: time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC)},
		{SessionID: q2b2Session15, PortfolioValue: 900_000, EndingCash: 360_000, TotalTaxPaid: 3_000, RecordedAt: time.Date(2026, 4, 15, 12, 0, 0, 0, time.UTC)},
	}
}

// q2b2FileSummary is a DIFFERENT, shorter file series (monotonic 500,000 →
// 505,000): a reader that stayed on the files cannot produce the store values.
func q2b2FileSummary() []domain.SessionSummary {
	return []domain.SessionSummary{
		{SessionID: q2b2Session13, PortfolioValue: 500_000, EndingCash: 200_000, TotalTaxPaid: 0, RecordedAt: time.Date(2026, 4, 13, 12, 0, 0, 0, time.UTC)},
		{SessionID: q2b2Session14, PortfolioValue: 505_000, EndingCash: 205_000, TotalTaxPaid: 0, RecordedAt: time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC)},
	}
}

// q2b2WriteSummaries writes the given summaries to disk in the layout the
// legacy JSONL ledger reads (LedgerDir/sessions/<id>/summary.json).
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

func TestLiveService_SummaryStoreSeam(t *testing.T) {
	ledgerDir := t.TempDir()
	q2b2WriteSummaries(t, ledgerDir, q2b2FileSummary())

	// 1) No injected store: the legacy JSONL ledger answers (file behavior).
	fromFiles := NewLiveService(t.TempDir(), ledgerDir)
	filePoints := fromFiles.HistoryPoints()
	if len(filePoints) != 2 || filePoints[len(filePoints)-1].PortfolioValue != 505_000 {
		t.Fatalf("file path HistoryPoints = %+v, want the 2 file sessions ending at 505000", filePoints)
	}
	if src := fromFiles.HistorySource(); src != "" {
		t.Errorf("file path HistorySource = %q, want empty (no backend to report)", src)
	}

	// 2) Injected store: it answers, not the files.
	store := &q2b2RecordingStore{summaries: q2b2StoreSummary(), source: "postgres"}
	fromStore := NewLiveService(t.TempDir(), ledgerDir).WithSummaryStore(store)
	storePoints := fromStore.HistoryPoints()

	if store.summaryCalls == 0 {
		t.Error("LoadSessionSummaries calls = 0; the equity curve did not use the injected store")
	}
	if len(storePoints) != 3 {
		t.Fatalf("store path HistoryPoints = %+v, want the 3 store sessions", storePoints)
	}
	if got := storePoints[len(storePoints)-1].PortfolioValue; got != 900_000 {
		t.Errorf("latest portfolio value = %v, want 900000 (505000 would mean the files answered)", got)
	}
	if src := fromStore.HistorySource(); src != "postgres" {
		t.Errorf("HistorySource = %q, want postgres (the injected store labels the L-cold read)", src)
	}
	if degraded := fromStore.HistoryDegraded(); degraded {
		t.Error("HistoryDegraded = true, want false for a healthy store")
	}

	curve := fromStore.buildEquityCurve()
	if len(curve) != 3 {
		t.Fatalf("buildEquityCurve = %d points, want 3", len(curve))
	}
	if curve[2].Label != q2b2Session15 || curve[2].Value != 900_000 || curve[2].TaxPaid != 3_000 {
		t.Errorf("curve[2] = %+v, want the store's newest session 900000/3000", curve[2])
	}
}

func TestLiveService_SummaryStoreDegradedIsLabeled(t *testing.T) {
	store := &q2b2RecordingStore{summaries: q2b2StoreSummary(), source: "jsonl", degraded: true}
	svc := NewLiveService(t.TempDir(), t.TempDir()).WithSummaryStore(store)

	svc.HistoryPoints()

	if src := svc.HistorySource(); src != "jsonl" {
		t.Errorf("HistorySource = %q, want jsonl (a degraded read must be visible to operators)", src)
	}
	if !svc.HistoryDegraded() {
		t.Error("HistoryDegraded = false, want true after a JSONL fallback")
	}
}

func TestLiveService_SummaryStoreAndFilePathsAgree(t *testing.T) {
	ledgerDir := t.TempDir()
	summaries := q2b2StoreSummary()
	q2b2WriteSummaries(t, ledgerDir, summaries)

	store := &q2b2PlainStore{summaries: summaries}
	fromStore := NewLiveService(t.TempDir(), ledgerDir).WithSummaryStore(store)
	fromFiles := NewLiveService(t.TempDir(), ledgerDir)

	storePoints := fromStore.HistoryPoints()
	filePoints := fromFiles.HistoryPoints()
	if !reflect.DeepEqual(storePoints, filePoints) {
		t.Fatalf("history points differ\n store = %+v\n files = %+v", storePoints, filePoints)
	}
	if store.calls == 0 {
		t.Error("LoadSessionSummaries calls = 0; the store path did not read the store")
	}
	if !reflect.DeepEqual(fromStore.buildEquityCurve(), fromFiles.buildEquityCurve()) {
		t.Errorf("equity curves differ\n store = %+v\n files = %+v", fromStore.buildEquityCurve(), fromFiles.buildEquityCurve())
	}
}

func TestBacktestService_SummariesStoreSeam(t *testing.T) {
	ledgerDir := t.TempDir()
	q2b2WriteSummaries(t, ledgerDir, q2b2FileSummary())
	cfg := config.Config{LedgerDir: ledgerDir}

	// 1) No injected store: the legacy JSONL ledger answers (file behavior).
	fromFiles := NewBacktestService(cfg)
	fileSummaries, err := fromFiles.summariesStore().LoadSessionSummaries()
	if err != nil {
		t.Fatalf("file path LoadSessionSummaries: %v", err)
	}
	if len(fileSummaries) != 2 {
		t.Fatalf("file path summaries = %d, want the 2 file sessions", len(fileSummaries))
	}

	// 2) Injected store: it answers, not the files.
	store := &q2b2RecordingStore{summaries: q2b2StoreSummary(), source: "postgres"}
	svc := NewBacktestService(cfg).WithStore(store)
	summaries, err := svc.summariesStore().LoadSessionSummaries()
	if err != nil {
		t.Fatalf("store path LoadSessionSummaries: %v", err)
	}
	if store.summaryCalls == 0 {
		t.Error("LoadSessionSummaries calls = 0; the backtest runner did not use the injected store")
	}
	if len(summaries) != 3 {
		t.Errorf("store path summaries = %d, want 3 (2 would mean the files answered)", len(summaries))
	}
}

// TestBacktestService_RunnerReadsSummariesFromTheResolvedStore proves the
// resolved store is the one the backtest runner actually reads: the runner
// report built from the injected store differs from the one built from the
// files, and matches it again when both carry the same fixture.
func TestBacktestService_RunnerReadsSummariesFromTheResolvedStore(t *testing.T) {
	ledgerDir := t.TempDir()
	fileSummaries := q2b2FileSummary()
	q2b2WriteSummaries(t, ledgerDir, fileSummaries)
	cfg := config.Config{LedgerDir: ledgerDir}
	window := domain.BacktestWindowSummary{
		WindowID:     "q2b2-window",
		StartDate:    time.Date(2026, 4, 13, 0, 0, 0, 0, time.UTC),
		EndDate:      time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC),
		SessionCount: 3,
	}

	store := &q2b2RecordingStore{summaries: q2b2StoreSummary()}
	fromStore, err := backtest.NewRunner(cfg, NewBacktestService(cfg).WithStore(store).summariesStore()).GenerateReport(window)
	if err != nil {
		t.Fatalf("GenerateReport(store): %v", err)
	}
	if store.summaryCalls == 0 || store.scorecardCalls == 0 {
		t.Fatalf("store calls = summaries %d / scorecards %d, want both > 0 (the runner did not read the injected store)",
			store.summaryCalls, store.scorecardCalls)
	}

	same, err := backtest.NewRunner(cfg, NewBacktestService(cfg).WithStore(&q2b2PlainStore{summaries: fileSummaries}).summariesStore()).GenerateReport(window)
	if err != nil {
		t.Fatalf("GenerateReport(store=file fixture): %v", err)
	}
	fromFiles, err := backtest.NewRunner(cfg, NewBacktestService(cfg).summariesStore()).GenerateReport(window)
	if err != nil {
		t.Fatalf("GenerateReport(files): %v", err)
	}

	if q2b2StripGeneratedAt(fromStore) == q2b2StripGeneratedAt(fromFiles) {
		t.Error("the runner report is identical with the store's series and with the file series; the store's summaries did not reach the runner")
	}
	if q2b2StripGeneratedAt(same) != q2b2StripGeneratedAt(fromFiles) {
		t.Errorf("the same fixture rendered differently through the two paths\n store = %s\n files = %s", same, fromFiles)
	}
}

// q2b2StripGeneratedAt drops the wall-clock line so two reports can be
// compared field by field.
func q2b2StripGeneratedAt(report string) string {
	var kept []string
	for _, line := range strings.Split(report, "\n") {
		if strings.HasPrefix(line, "*Generated at ") {
			continue
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
}
