package risk

// Q2 batch 2, unit 2: the risk handlers' L-cold session-summary reads go
// through an injected backend-aware ledger store (PostgreSQL first, JSONL
// fallback) and keep the legacy JSONL ledger read when no store is injected.
// These tests are hermetic (no PostgreSQL server): the real server half lives
// in summaries_pg_integration_test.go behind `-tags=integration`.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/monitoring/api/shared"
	"github.com/kaecer68/atlas-go/internal/risk"
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

	source   string
	degraded bool
}

func (s *q2b2RecordingStore) LoadSessionSummaries() ([]domain.SessionSummary, error) {
	s.calls++
	return s.summaries, s.err
}

func (s *q2b2RecordingStore) SourceBackend() string { return s.source }

func (s *q2b2RecordingStore) Degraded() bool { return s.degraded }

// q2b2PlainStore reports no backend, which is what a parity fixture needs: both
// paths then serialize identically.
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

// q2b2RiskMetrics drives GET /api/dashboard/risk through the shared GET adapter
// (the production path, so the JSON contract is exercised) and decodes it.
func q2b2RiskMetrics(t *testing.T, h *Handlers) map[string]any {
	t.Helper()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/risk", nil)
	shared.Get(h.HandleRiskMetrics).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("risk status = %d, want %d", w.Code, http.StatusOK)
	}
	var m map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil {
		t.Fatalf("decode risk: %v", err)
	}
	return m
}

// q2b2RiskCommentary drives GET /api/risk/commentary the same way.
func q2b2RiskCommentary(t *testing.T, h *Handlers) map[string]any {
	t.Helper()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/risk/commentary", nil)
	shared.Get(h.HandleRiskCommentary).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("commentary status = %d, want %d", w.Code, http.StatusOK)
	}
	var m map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil {
		t.Fatalf("decode commentary: %v", err)
	}
	return m
}

func q2b2Snapshot(t *testing.T, payload map[string]any) map[string]any {
	t.Helper()
	snap, ok := payload["risk_snapshot"].(map[string]any)
	if !ok {
		t.Fatalf("risk_snapshot is %T, want map", payload["risk_snapshot"])
	}
	return snap
}

func TestHandlers_RiskMetricsUsesInjectedStore(t *testing.T) {
	// The files hold a DIFFERENT, shorter series (two sessions, monotonic):
	// reading them would give session_count 2 and no drawdown-sourcing dip.
	ledgerDir := t.TempDir()
	q2b2WriteSummaries(t, ledgerDir, []domain.SessionSummary{
		{SessionID: q2b2Session13, PortfolioValue: 500_000, RecordedAt: time.Date(2026, 4, 13, 12, 0, 0, 0, time.UTC)},
		{SessionID: q2b2Session14, PortfolioValue: 505_000, RecordedAt: time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC)},
	})

	store := &q2b2RecordingStore{summaries: q2b2Summaries(), source: "postgres"}
	h := NewHandlers(ledgerDir).WithRiskGate(risk.NewRiskGate(nil, nil, nil)).WithStore(store)

	payload := q2b2RiskMetrics(t, h)

	if store.calls == 0 {
		t.Error("LoadSessionSummaries calls = 0; the risk metrics did not use the injected store")
	}
	if got := payload["session_count"]; got != float64(3) {
		t.Errorf("session_count = %v, want 3 (the store series; 2 would mean the files answered)", got)
	}
	snap := q2b2Snapshot(t, payload)
	if got := snap["data_points"]; got != float64(2) {
		t.Errorf("data_points = %v, want 2 (the store series has 3 points ⇒ 2 returns)", got)
	}
	if got := payload["source"]; got != "postgres" {
		t.Errorf("source = %v, want postgres (the injected store labels the L-cold read)", got)
	}
	if got := payload["degraded"]; got != false {
		t.Errorf("degraded = %v, want false", got)
	}
}

func TestHandlers_RiskMetricsUsesFilesWithoutStore(t *testing.T) {
	ledgerDir := t.TempDir()
	q2b2WriteSummaries(t, ledgerDir, q2b2Summaries())

	h := NewHandlers(ledgerDir).WithRiskGate(risk.NewRiskGate(nil, nil, nil))
	payload := q2b2RiskMetrics(t, h)

	// Golden pin of the file path (no store injected), so the parity test below
	// cannot pass by having BOTH paths broken in the same way.
	if got := payload["session_count"]; got != float64(3) {
		t.Errorf("session_count = %v, want 3", got)
	}
	snap := q2b2Snapshot(t, payload)
	if got := snap["data_points"]; got != float64(2) {
		t.Errorf("data_points = %v, want 2", got)
	}
	if got := snap["insufficient_data"]; got != float64(1) {
		t.Errorf("insufficient_data = %v, want 1 (2 returns < %d)", got, risk.MinObservationsForVaR)
	}
	if got := snap["var_95"]; got != float64(0) {
		t.Errorf("var_95 = %v, want 0 (P0-1: no provisional VaR below the observation gate)", got)
	}
	dd, ok := snap["max_drawdown_pct"].(float64)
	if !ok || dd < 0.11764705882352940 || dd > 0.11764705882352942 {
		t.Errorf("max_drawdown_pct = %v, want 0.11764705882352941 (1020000 → 900000)", snap["max_drawdown_pct"])
	}
	// The legacy JSONL store reports no backend, so the payload keeps the exact
	// pre-seam shape (field-contract: no new keys on the file path).
	if _, present := payload["source"]; present {
		t.Error("source present on the file path; the legacy store must not invent a backend label")
	}
	if _, present := payload["degraded"]; present {
		t.Error("degraded present on the file path; the legacy store must not invent a backend label")
	}
}

func TestHandlers_RiskMetricsStoreAndFilePathsAgree(t *testing.T) {
	ledgerDir := t.TempDir()
	summaries := q2b2Summaries()
	q2b2WriteSummaries(t, ledgerDir, summaries)

	store := &q2b2PlainStore{summaries: summaries}
	fromStore := q2b2RiskMetrics(t, NewHandlers(ledgerDir).WithRiskGate(risk.NewRiskGate(nil, nil, nil)).WithStore(store))
	fromFiles := q2b2RiskMetrics(t, NewHandlers(ledgerDir).WithRiskGate(risk.NewRiskGate(nil, nil, nil)))

	if store.calls == 0 {
		t.Error("LoadSessionSummaries calls = 0; the store path did not read the store")
	}
	wantJSON, err := json.Marshal(fromStore)
	if err != nil {
		t.Fatalf("marshal store payload: %v", err)
	}
	gotJSON, err := json.Marshal(fromFiles)
	if err != nil {
		t.Fatalf("marshal file payload: %v", err)
	}
	if string(wantJSON) != string(gotJSON) {
		t.Errorf("store and file paths disagree\n store = %s\n files = %s", wantJSON, gotJSON)
	}
}

func TestHandlers_RiskCommentaryFallbackUsesInjectedStore(t *testing.T) {
	ledgerDir := t.TempDir()
	commentary := "風險水位穩定，曝險低於閾值，維持現行配置。"
	summaries := []domain.SessionSummary{
		{SessionID: q2b2Session13, PortfolioValue: 1_000_000, RiskCommentary: commentary, RecordedAt: time.Date(2026, 4, 13, 12, 0, 0, 0, time.UTC)},
		// A newer session WITHOUT commentary must not win over the older one
		// that carries it.
		{SessionID: q2b2Session14, PortfolioValue: 1_010_000, RecordedAt: time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC)},
	}
	// The files hold only the newer, commentary-less session, so the injected
	// store is the only source that can answer.
	q2b2WriteSummaries(t, ledgerDir, summaries[1:])

	store := &q2b2PlainStore{summaries: summaries}
	h := NewHandlers(ledgerDir).WithRiskGate(risk.NewRiskGate(nil, nil, nil)).WithStore(store)

	payload := q2b2RiskCommentary(t, h)
	if got := payload["session_id"]; got != q2b2Session13 {
		t.Errorf("session_id = %v, want %s (the store's commentary-carrying session)", got, q2b2Session13)
	}
	if got := payload["reason_zh"]; got != commentary {
		t.Errorf("reason_zh = %v, want the store's persisted commentary", got)
	}
	if store.calls == 0 {
		t.Error("LoadSessionSummaries calls = 0; the commentary fallback did not use the injected store")
	}
}

// TestHandlers_RiskCommentaryFallbackIsStoreOnly pins the pre-existing seam
// boundary: the persisted-commentary fallback exists only when a store is
// injected, so the legacy file path keeps answering "not_available" (the file
// fixture holds a commentary that must stay unused).
func TestHandlers_RiskCommentaryFallbackIsStoreOnly(t *testing.T) {
	ledgerDir := t.TempDir()
	q2b2WriteSummaries(t, ledgerDir, []domain.SessionSummary{{
		SessionID:      q2b2Session13,
		PortfolioValue: 1_000_000,
		RiskCommentary: "檔案裡的評語（store 未注入時不得使用）",
		RecordedAt:     time.Date(2026, 4, 13, 12, 0, 0, 0, time.UTC),
	}})

	h := NewHandlers(ledgerDir).WithRiskGate(risk.NewRiskGate(nil, nil, nil))
	payload := q2b2RiskCommentary(t, h)

	if got := payload["status"]; got != "not_available" {
		t.Errorf("status = %v, want not_available (no store injected)", got)
	}
}
