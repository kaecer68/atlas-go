package live

// Q2 batch 2, unit 1: the live handlers read recommendation outcomes and
// session summaries through an injected backend-aware ledger store (PG first,
// JSONL fallback) and keep the legacy direct-file reads when no store is
// injected. These tests are hermetic (no PostgreSQL server): the real server
// half lives in outcomes_store_integration_test.go behind `-tags=integration`.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	domainshared "github.com/kaecer68/atlas-go/internal/domain/shared"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/monitoring/api/shared"
	"github.com/kaecer68/atlas-go/internal/testdb"
)

const (
	q2b1SessionOld = "session-20260413-daily"
	q2b1SessionNew = "session-20260414-daily"
	// q2b1UnreachableDSN points at a closed port: the pool parses the DSN and
	// never dials, so any read fails with a connection error (same trick as
	// the C6c fallback test).
	q2b1UnreachableDSN = "postgres://atlas:atlas@127.0.0.1:1/atlas_q2b1_unreachable?sslmode=disable"
)

// q2b1RecordingStore implements ledger.OutcomeStore by embedding the interface
// (nil) so only the two read paths under test are implemented — any other
// method panics, which keeps the seam honest. Every read is recorded so a test
// can prove the store answered instead of the files.
type q2b1RecordingStore struct {
	ledger.OutcomeStore

	summaries []domain.SessionSummary
	outcomes  map[string][]domain.RecommendationOutcome

	summaryCalls int
	summaryErr   error
	outcomeCalls []string
}

func (s *q2b1RecordingStore) LoadSessionSummaries() ([]domain.SessionSummary, error) {
	s.summaryCalls++
	return s.summaries, s.summaryErr
}

func (s *q2b1RecordingStore) LoadSessionOutcomes(sessionID string) ([]domain.RecommendationOutcome, error) {
	s.outcomeCalls = append(s.outcomeCalls, sessionID)
	return s.outcomes[sessionID], nil
}

// q2b1Outcomes returns the outcome rows of one session. Every session gets a
// different symbol and return so a read of the wrong session cannot pass.
func q2b1Outcomes(symbol string, forwardReturn float64) []domain.RecommendationOutcome {
	return []domain.RecommendationOutcome{{
		AgentID:       "agent-tech-1",
		Symbol:        symbol,
		Side:          domain.SideBuy,
		ForwardReturn: forwardReturn,
		PassedGuards:  true,
		FactorScores: domainshared.FactorScores{
			Momentum: 0.6, Value: 0.4, Quality: 0.7, Agent: 0.8, Total: 0.6,
		},
	}}
}

func q2b1Summaries() []domain.SessionSummary {
	return []domain.SessionSummary{
		{
			SessionID:      q2b1SessionOld,
			PortfolioValue: 1000000.0,
			RecordedAt:     time.Date(2026, 4, 13, 12, 0, 0, 0, time.UTC),
		},
		{
			SessionID:      q2b1SessionNew,
			PortfolioValue: 1020000.0,
			RecordedAt:     time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC),
		},
	}
}

func q2b1StoreFixture() *q2b1RecordingStore {
	return &q2b1RecordingStore{
		summaries: q2b1Summaries(),
		outcomes: map[string][]domain.RecommendationOutcome{
			q2b1SessionOld: q2b1Outcomes("2311", 0.01),
			q2b1SessionNew: append(q2b1Outcomes("2330", 0.05), q2b1Outcomes("2881", 0.02)...),
		},
	}
}

// q2b1WriteFixture writes the same sessions and outcomes to disk (the layout
// the legacy file path reads: sessions/<id>/summary.json + recommendation_outcomes.jsonl).
func q2b1WriteFixture(t *testing.T, dir string) {
	t.Helper()
	sessionsDir := filepath.Join(dir, "sessions")
	rows := map[string][]domain.RecommendationOutcome{
		q2b1SessionOld: q2b1Outcomes("2311", 0.01),
		q2b1SessionNew: append(q2b1Outcomes("2330", 0.05), q2b1Outcomes("2881", 0.02)...),
	}
	for _, summary := range q2b1Summaries() {
		sessionDir := filepath.Join(sessionsDir, summary.SessionID)
		if err := os.MkdirAll(sessionDir, 0o755); err != nil {
			t.Fatalf("os.MkdirAll: %v", err)
		}
		b, err := json.Marshal(summary)
		if err != nil {
			t.Fatalf("marshal summary: %v", err)
		}
		if err := os.WriteFile(filepath.Join(sessionDir, "summary.json"), b, 0o644); err != nil {
			t.Fatalf("os.WriteFile summary: %v", err)
		}
		var lines []string
		for _, oc := range rows[summary.SessionID] {
			b, err := json.Marshal(oc)
			if err != nil {
				t.Fatalf("marshal outcome: %v", err)
			}
			lines = append(lines, string(b))
		}
		if err := os.WriteFile(filepath.Join(sessionDir, "recommendation_outcomes.jsonl"), []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
			t.Fatalf("os.WriteFile outcomes: %v", err)
		}
	}
}

// q2b1GetPnL drives GET /api/dashboard/pnl-attribution through the shared GET
// adapter (the production path, so the JSON contract is exercised) and decodes
// the payload.
func q2b1GetPnL(t *testing.T, h *Handlers) PnLAttributionResponse {
	t.Helper()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/pnl-attribution", nil)
	shared.Get(h.HandlePnLAttribution).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("pnl-attribution status = %d, want %d", w.Code, http.StatusOK)
	}
	var resp PnLAttributionResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode pnl-attribution: %v", err)
	}
	return resp
}

// q2b1GetRisk drives GET /api/dashboard/risk-exposure through the shared GET
// adapter and decodes the payload.
func q2b1GetRisk(t *testing.T, h *Handlers) RiskExposureResponse {
	t.Helper()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/risk-exposure", nil)
	shared.Get(h.HandleRiskExposure).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("risk-exposure status = %d, want %d", w.Code, http.StatusOK)
	}
	var resp RiskExposureResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode risk-exposure: %v", err)
	}
	return resp
}

// q2b1AssertOutcomesEqual compares two outcome lists field by field: the Go
// structs (every field, tagged or not) and then their JSON encoding (every
// serialized field name), so a difference cannot hide behind a tag.
func q2b1AssertOutcomesEqual(t *testing.T, label string, want, got []domain.RecommendationOutcome) {
	t.Helper()
	if len(want) != len(got) {
		t.Fatalf("%s: len(outcomes) = %d, want %d", label, len(got), len(want))
	}
	for i := range want {
		if !reflect.DeepEqual(want[i], got[i]) {
			t.Errorf("%s: outcome[%d] differs\n store/files want = %+v\n got            = %+v", label, i, want[i], got[i])
			continue
		}
		wantJSON, err := json.Marshal(want[i])
		if err != nil {
			t.Fatalf("%s: marshal want: %v", label, err)
		}
		gotJSON, err := json.Marshal(got[i])
		if err != nil {
			t.Fatalf("%s: marshal got: %v", label, err)
		}
		if string(wantJSON) != string(gotJSON) {
			t.Errorf("%s: outcome[%d] JSON differs\n want = %s\n got  = %s", label, i, wantJSON, gotJSON)
		}
	}
}

// q2b1CanonicalPnL sorts the attribution slices so the map-iteration order of
// the handler does not decide the comparison.
func q2b1CanonicalPnL(resp PnLAttributionResponse) PnLAttributionResponse {
	slices.SortFunc(resp.AgentAttribution, func(a, b AgentAttribution) int {
		return strings.Compare(a.AgentID, b.AgentID)
	})
	slices.SortFunc(resp.SectorAttribution, func(a, b SectorAttribution) int {
		return strings.Compare(a.Sector, b.Sector)
	})
	slices.SortFunc(resp.SymbolAttribution, func(a, b SymbolAttribution) int {
		return strings.Compare(a.Symbol, b.Symbol)
	})
	return resp
}

// q2b1CanonicalRisk sorts the exposure slices (same reason) and zeroes the
// wall-clock snapshot stamp, which is not a data source difference.
func q2b1CanonicalRisk(resp RiskExposureResponse) RiskExposureResponse {
	resp.SnapshotTime = time.Time{}
	slices.SortFunc(resp.SectorExposure, func(a, b SectorExposure) int {
		return strings.Compare(a.Sector, b.Sector)
	})
	slices.SortFunc(resp.Concentration, func(a, b PositionConcentration) int {
		return strings.Compare(a.Symbol, b.Symbol)
	})
	return resp
}

// TestHandlers_OutcomeReadsUseInjectedStore pins the three file reads this unit
// converted: the P&L attribution session listing, its outcome read, and the
// risk-exposure outcome read must all answer from the injected store.
//
// LedgerDir points at a path that does not exist, so any surviving
// os.ReadDir/loadRecommendationOutcomes call over it answers empty: a
// non-empty response can only come from the store. The recorded calls then pin
// which session each site asked for.
func TestHandlers_OutcomeReadsUseInjectedStore(t *testing.T) {
	tmpDir := t.TempDir()
	missingLedger := filepath.Join(tmpDir, "no-such-ledger")
	store := q2b1StoreFixture()
	h := (&Handlers{LedgerDir: missingLedger, WorkDir: tmpDir}).WithStore(store)

	pnl := q2b1GetPnL(t, h)
	if pnl.SessionID != q2b1SessionNew {
		t.Errorf("pnl session_id = %q, want %q (the store's latest session)", pnl.SessionID, q2b1SessionNew)
	}
	if len(pnl.AgentAttribution) != 1 || pnl.AgentAttribution[0].AgentID != "agent-tech-1" {
		t.Errorf("pnl agent_attribution = %+v, want the store's agent-tech-1 rows", pnl.AgentAttribution)
	}
	if pnl.CurrentValue != 1020000.0 || pnl.StartingValue != 1000000.0 {
		t.Errorf("pnl values = %v→%v, want 1000000→1020000 from the store summaries", pnl.StartingValue, pnl.CurrentValue)
	}

	risk := q2b1GetRisk(t, h)
	if len(risk.SectorExposure) == 0 {
		t.Errorf("risk sector_exposure empty; the risk-exposure outcome read did not use the store")
	}

	if store.summaryCalls < 2 {
		t.Errorf("LoadSessionSummaries calls = %d, want >= 2 (P&L session listing + latest-session resolution)", store.summaryCalls)
	}
	wantOutcomeCalls := []string{q2b1SessionNew, q2b1SessionNew}
	if !slices.Equal(store.outcomeCalls, wantOutcomeCalls) {
		t.Errorf("LoadSessionOutcomes calls = %v, want %v (P&L attribution + risk exposure, both resolving the latest session)",
			store.outcomeCalls, wantOutcomeCalls)
	}
}

// TestHandlers_OutcomeReadsUseFilesWithoutStore pins the additive contract:
// with no store injected the handlers keep reading LedgerDir/sessions, so the
// production wiring can be rolled back by dropping the injection.
func TestHandlers_OutcomeReadsUseFilesWithoutStore(t *testing.T) {
	dir := t.TempDir()
	q2b1WriteFixture(t, dir)
	h := &Handlers{LedgerDir: dir, WorkDir: dir}

	pnl := q2b1GetPnL(t, h)
	if pnl.SessionID != q2b1SessionNew {
		t.Errorf("pnl session_id = %q, want %q", pnl.SessionID, q2b1SessionNew)
	}
	if pnl.CurrentValue != 1020000.0 || pnl.CumulativePnL != 20000.0 {
		t.Errorf("pnl values = current %v / cumulative %v, want 1020000 / 20000", pnl.CurrentValue, pnl.CumulativePnL)
	}

	risk := q2b1GetRisk(t, h)
	if len(risk.SectorExposure) == 0 {
		t.Errorf("risk sector_exposure empty; the file path must still read recommendation_outcomes.jsonl")
	}

	for _, tc := range []struct {
		sessionID string
		want      []string
	}{
		{sessionID: "", want: []string{"2330", "2881"}},
		{sessionID: q2b1SessionOld, want: []string{"2311"}},
		{sessionID: q2b1SessionNew, want: []string{"2330", "2881"}},
	} {
		outcomes, err := h.recommendationOutcomes(tc.sessionID)
		if err != nil {
			t.Fatalf("recommendationOutcomes(%q): %v", tc.sessionID, err)
		}
		var symbols []string
		for _, oc := range outcomes {
			symbols = append(symbols, oc.Symbol)
		}
		if !slices.Equal(symbols, tc.want) {
			t.Errorf("recommendationOutcomes(%q) symbols = %v, want %v", tc.sessionID, symbols, tc.want)
		}
	}
}

// TestHandlers_StoreAndFilePathsAgreeOnOutcomes is the parity test: over one
// fixture, the store path (the backend-aware JSONL store, i.e. the non-Postgres
// branch of ledger.NewReportOutcomeStore) and the file path must return the
// same data — outcome lists field by field and session summaries, plus the two
// endpoint payloads themselves. The file side is additionally pinned to golden
// values, so a break of BOTH paths cannot make the parity check pass.
func TestHandlers_StoreAndFilePathsAgreeOnOutcomes(t *testing.T) {
	dir := t.TempDir()
	q2b1WriteFixture(t, dir)

	fileH := &Handlers{LedgerDir: dir, WorkDir: dir}
	storeH := (&Handlers{LedgerDir: dir, WorkDir: dir}).WithStore(ledger.NewStore(dir))

	// Golden pin on the file path (the "未注入 ⇒ 維持現行檔案行為" contract).
	fileOutcomes, err := fileH.recommendationOutcomes(q2b1SessionNew)
	if err != nil {
		t.Fatalf("file recommendationOutcomes: %v", err)
	}
	q2b1AssertOutcomesEqual(t, "file fixture", []domain.RecommendationOutcome{
		q2b1Outcomes("2330", 0.05)[0], q2b1Outcomes("2881", 0.02)[0],
	}, fileOutcomes)

	for _, sessionID := range []string{"", q2b1SessionOld, q2b1SessionNew} {
		want, wantErr := fileH.recommendationOutcomes(sessionID)
		got, gotErr := storeH.recommendationOutcomes(sessionID)
		if (wantErr == nil) != (gotErr == nil) {
			t.Fatalf("recommendationOutcomes(%q): file err = %v, store err = %v; the two paths must agree", sessionID, wantErr, gotErr)
		}
		if wantErr != nil {
			continue
		}
		q2b1AssertOutcomesEqual(t, "recommendationOutcomes("+sessionID+")", want, got)
	}

	wantSummaries, err := fileH.sessionSummaries()
	if err != nil {
		t.Fatalf("file sessionSummaries: %v", err)
	}
	gotSummaries, err := storeH.sessionSummaries()
	if err != nil {
		t.Fatalf("store sessionSummaries: %v", err)
	}
	sortBySessionID := func(s []domain.SessionSummary) {
		slices.SortFunc(s, func(a, b domain.SessionSummary) int {
			return strings.Compare(a.SessionID, b.SessionID)
		})
	}
	sortBySessionID(wantSummaries)
	sortBySessionID(gotSummaries)
	if !reflect.DeepEqual(wantSummaries, gotSummaries) {
		t.Errorf("session summaries differ\n file = %+v\n store = %+v", wantSummaries, gotSummaries)
	}

	wantPnL, gotPnL := q2b1CanonicalPnL(q2b1GetPnL(t, fileH)), q2b1CanonicalPnL(q2b1GetPnL(t, storeH))
	if !reflect.DeepEqual(wantPnL, gotPnL) {
		t.Errorf("pnl-attribution payload differs\n file = %+v\n store = %+v", wantPnL, gotPnL)
	}
	wantRisk, gotRisk := q2b1CanonicalRisk(q2b1GetRisk(t, fileH)), q2b1CanonicalRisk(q2b1GetRisk(t, storeH))
	if !reflect.DeepEqual(wantRisk, gotRisk) {
		t.Errorf("risk-exposure payload differs\n file = %+v\n store = %+v", wantRisk, gotRisk)
	}
}

// TestHandlers_OutcomeReadsFallBackToJSONLWhenPostgresUnavailable pins the
// production degraded path without a server: the store is the real PG-first
// store whose PostgreSQL half cannot dial, so it must fall back to its JSONL
// half and still answer. LedgerDir points somewhere else entirely, so the
// answer can only travel through the store.
func TestHandlers_OutcomeReadsFallBackToJSONLWhenPostgresUnavailable(t *testing.T) {
	dir := t.TempDir()
	q2b1WriteFixture(t, dir)
	pool := testdb.UnconnectedPool(t, q2b1UnreachableDSN)
	store := ledger.NewPGFirstOutcomeStore(ledger.NewPostgresLedgerStore(pool), ledger.NewStore(dir))
	h := (&Handlers{LedgerDir: filepath.Join(dir, "no-such-ledger"), WorkDir: dir}).WithStore(store)

	pnl := q2b1GetPnL(t, h)
	if pnl.SessionID != q2b1SessionNew {
		t.Errorf("pnl session_id = %q, want %q served by the JSONL fallback", pnl.SessionID, q2b1SessionNew)
	}
	if len(pnl.AgentAttribution) != 1 {
		t.Errorf("pnl agent_attribution = %+v, want the JSONL fixture rows", pnl.AgentAttribution)
	}
	if !store.Degraded() {
		t.Error("store is not marked degraded; the JSONL fallback must be visible to operators")
	}
	if src := store.SourceBackend(); src != "jsonl" {
		t.Errorf("SourceBackend() = %q, want %q", src, "jsonl")
	}
}

// TestHandlers_LatestSessionComesFromSummaries pins the deliberate split in the
// "latest session" resolution that independent review of this unit found
// (2026-10-15): the store path takes the newest session SUMMARY, the legacy
// file path the newest session DIRECTORY. The fixture is a half-written tree —
// the newest directory holds outcomes but never got its summary.json, the older
// one is complete:
//
//   - file path  ⇒ the newest directory's orphan outcomes (historical rule);
//   - store path ⇒ the newest recorded session, i.e. the older one (the SSoT
//     backend defines a session by its summary row; outcomes whose session was
//     never summarized belong to no session).
//
// The endpoint-level assertions make the consequence visible: risk-exposure has
// no session_id in its payload, so on such a tree the two data sources report
// different factor exposure with nothing in the response naming the session.
// This is the documented boundary of the parity contract — on the well-formed
// fixture TestHandlers_StoreAndFilePathsAgreeOnOutcomes requires equality.
func TestHandlers_LatestSessionComesFromSummaries(t *testing.T) {
	tmpDir := t.TempDir()
	dir := filepath.Join(tmpDir, "ledger")
	sessionsDir := filepath.Join(dir, "sessions")

	withMomentum := func(symbol string, forwardReturn, momentum float64) domain.RecommendationOutcome {
		oc := q2b1Outcomes(symbol, forwardReturn)[0]
		oc.FactorScores = domainshared.FactorScores{Momentum: momentum, Total: momentum}
		return oc
	}
	writeOutcomes := func(sessionID string, rows []domain.RecommendationOutcome) {
		t.Helper()
		sessionDir := filepath.Join(sessionsDir, sessionID)
		if err := os.MkdirAll(sessionDir, 0o755); err != nil {
			t.Fatalf("os.MkdirAll: %v", err)
		}
		var lines []string
		for _, oc := range rows {
			b, err := json.Marshal(oc)
			if err != nil {
				t.Fatalf("marshal outcome: %v", err)
			}
			lines = append(lines, string(b))
		}
		if err := os.WriteFile(filepath.Join(sessionDir, "recommendation_outcomes.jsonl"), []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
			t.Fatalf("write outcomes: %v", err)
		}
	}

	// Newest directory: outcomes only — no summary.json.
	writeOutcomes(q2b1SessionNew, []domain.RecommendationOutcome{withMomentum("2311", 0.01, 0.9)})
	// Older directory: a complete session.
	writeOutcomes(q2b1SessionOld, []domain.RecommendationOutcome{withMomentum("2881", 0.02, 0.6)})
	summary := q2b1Summaries()[0] // SessionID == q2b1SessionOld
	b, err := json.Marshal(summary)
	if err != nil {
		t.Fatalf("marshal summary: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sessionsDir, q2b1SessionOld, "summary.json"), b, 0o644); err != nil {
		t.Fatalf("write summary.json: %v", err)
	}

	fileH := &Handlers{LedgerDir: dir, WorkDir: tmpDir}
	storeH := (&Handlers{LedgerDir: dir, WorkDir: tmpDir}).WithStore(ledger.NewStore(dir))

	fileOutcomes, err := fileH.recommendationOutcomes("")
	if err != nil {
		t.Fatalf("file recommendationOutcomes(\"\"): %v", err)
	}
	storeOutcomes, err := storeH.recommendationOutcomes("")
	if err != nil {
		t.Fatalf("store recommendationOutcomes(\"\"): %v", err)
	}
	q2b1AssertOutcomesEqual(t, "file path (newest directory)", []domain.RecommendationOutcome{withMomentum("2311", 0.01, 0.9)}, fileOutcomes)
	q2b1AssertOutcomesEqual(t, "store path (newest summary)", []domain.RecommendationOutcome{withMomentum("2881", 0.02, 0.6)}, storeOutcomes)

	// Endpoint level: the same split reaches risk-exposure, whose payload names
	// no session and therefore cannot show which one it attributed.
	fileRisk := q2b1GetRisk(t, fileH)
	storeRisk := q2b1GetRisk(t, storeH)
	if fileRisk.FactorExposure.Momentum != 0.9 {
		t.Errorf("file path factor_exposure.momentum = %v, want 0.9 (newest directory's outcomes)", fileRisk.FactorExposure.Momentum)
	}
	if storeRisk.FactorExposure.Momentum != 0.6 {
		t.Errorf("store path factor_exposure.momentum = %v, want 0.6 (newest recorded session's outcomes)", storeRisk.FactorExposure.Momentum)
	}
}
