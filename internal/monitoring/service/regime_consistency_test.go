// File: regime_consistency_test.go
// Package: service
//
// Tests for the Phase 2 Reconciler v1 P1 regime three-endpoint consistency
// check. Fixtures combine all three endpoints with mixed vocabularies and
// unknown sessions so cross-walk and drift detection are asserted end-to-end.
package service

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/ledger"
)

// --- fixture helpers ------------------------------------------------------

// fixtureDates returns the last n trading dates ending today (UTC), oldest first.
func fixtureDates(t *testing.T, n int) []time.Time {
	t.Helper()
	now := time.Now().UTC()
	dates := make([]time.Time, n)
	for i := 0; i < n; i++ {
		dates[i] = now.AddDate(0, 0, -(n - 1 - i))
	}
	return dates
}

func fixtureDateStr(d time.Time) string { return d.Format("2006-01-02") }

func fixtureSessionID(d time.Time) string { return "session-" + d.Format("20060102") + "-daily" }

// writeSessionSummary writes a session summary.json (regime may be "" to
// simulate the backfill writer gap).
func writeSessionSummary(t *testing.T, ledgerDir string, sessionID string, regime domain.Regime, recordedAt time.Time) {
	t.Helper()
	dir := filepath.Join(ledgerDir, "sessions", sessionID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir session: %v", err)
	}
	sum := domain.SessionSummary{
		SessionID:    sessionID,
		Regime:       regime,
		OutcomeCount: 1,
		RecordedAt:   recordedAt,
	}
	data, err := json.Marshal(sum)
	if err != nil {
		t.Fatalf("marshal summary: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "summary.json"), data, 0o644); err != nil {
		t.Fatalf("write summary: %v", err)
	}
}

// writeOrphanSession creates a session dir with outcomes but no summary.json.
func writeOrphanSession(t *testing.T, ledgerDir string, sessionID string) {
	t.Helper()
	dir := filepath.Join(ledgerDir, "sessions", sessionID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir session: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "recommendation_outcomes.jsonl"), []byte("{}"), 0o644); err != nil {
		t.Fatalf("write outcomes: %v", err)
	}
}

type regimeFixture struct {
	svc     *PipelineService
	store   *ledger.SQLiteHistoricalStore
	ledger  string
	cleanup func()
}

// newRegimeFixture builds a PipelineService wired to a fresh SQLite
// HistoricalStore plus an empty ledger dir.
func newRegimeFixture(t *testing.T) *regimeFixture {
	t.Helper()
	ledgerDir := t.TempDir()
	db, err := ledger.OpenSQLiteDB(filepath.Join(t.TempDir(), "regime_test.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := ledger.InitSchema(db); err != nil {
		_ = db.Close()
		t.Fatalf("init schema: %v", err)
	}
	store := ledger.NewSQLiteHistoricalStore(db)
	svc := NewPipelineService(ledgerDir, ledgerDir, ledger.NewStore(ledgerDir)).
		WithHistoricalStore(store)
	return &regimeFixture{
		svc:     svc,
		store:   store,
		ledger:  ledgerDir,
		cleanup: func() { _ = db.Close() },
	}
}

func (f *regimeFixture) upsertRegime(t *testing.T, date, regime string) {
	t.Helper()
	ctx := context.Background()
	if err := f.store.UpsertRegime(ctx, ledger.RegimeRow{
		Date:       date,
		Regime:     regime,
		RecordedAt: time.Now().UTC(),
		CapturedAt: time.Now().UTC(),
		Source:     "macro_ingest",
	}); err != nil {
		t.Fatalf("upsert regime %s: %v", date, err)
	}
}

func (f *regimeFixture) upsertStress(t *testing.T, date, regime string) {
	t.Helper()
	ctx := context.Background()
	if err := f.store.UpsertStress(ctx, ledger.StressRow{
		Date:       date,
		Score:      50,
		Regime:     regime,
		CapturedAt: time.Now().UTC(),
		Source:     "taiwan_stress",
	}); err != nil {
		t.Fatalf("upsert stress %s: %v", date, err)
	}
}

func (f *regimeFixture) mustCheck(t *testing.T, days int) *RegimeConsistencyReport {
	t.Helper()
	defer f.cleanup()
	rep, err := f.svc.CheckRegimeConsistency(context.Background(), days)
	if err != nil {
		t.Fatalf("CheckRegimeConsistency: %v", err)
	}
	return rep
}

// --- tests ----------------------------------------------------------------

// TestRegimeConsistency_AllAgree: all three endpoints agree after cross-walk.
func TestRegimeConsistency_AllAgree(t *testing.T) {
	f := newRegimeFixture(t)
	dates := fixtureDates(t, 3)
	for i, d := range dates {
		f.upsertRegime(t, fixtureDateStr(d), []string{"RISK_ON", "RISK_OFF", "NEUTRAL"}[i])
		f.upsertStress(t, fixtureDateStr(d), []string{"low", "high", "alert"}[i])
		writeSessionSummary(t, f.ledger, fixtureSessionID(d), domain.Regime([]string{"RISK_ON", "RISK_OFF", "NEUTRAL"}[i]), d)
	}

	rep := f.mustCheck(t, 7)

	if rep.Status != RegimeConsistencyOK {
		t.Errorf("status = %q, want %q", rep.Status, RegimeConsistencyOK)
	}
	if rep.ComparedDays != 3 {
		t.Errorf("compared_days = %d, want 3", rep.ComparedDays)
	}
	if rep.Matches != 3 || rep.Drifts != 0 {
		t.Errorf("matches/drifts = %d/%d, want 3/0", rep.Matches, rep.Drifts)
	}
	if rep.UnknownCount != 0 || rep.UnknownRatio != 0 {
		t.Errorf("unknown = %d (ratio %.2f), want 0", rep.UnknownCount, rep.UnknownRatio)
	}
	if len(rep.DriftDetails) != 0 {
		t.Errorf("drift details = %v, want none", rep.DriftDetails)
	}
	// Cross-walk: stress rows must appear under their canonical labels.
	if got := rep.StressIndex.Normalized["RISK_ON"]; got != 1 {
		t.Errorf("stress normalized RISK_ON = %d, want 1 (low → RISK_ON)", got)
	}
	if got := rep.StressIndex.Normalized["RISK_OFF"]; got != 1 {
		t.Errorf("stress normalized RISK_OFF = %d, want 1 (high → RISK_OFF)", got)
	}
	if got := rep.StressIndex.Normalized["NEUTRAL"]; got != 1 {
		t.Errorf("stress normalized NEUTRAL = %d, want 1 (alert → NEUTRAL)", got)
	}
	if rep.StressIndex.Regimes["low"] != 1 || rep.StressIndex.Regimes["high"] != 1 || rep.StressIndex.Regimes["alert"] != 1 {
		t.Errorf("stress raw vocab = %v, want low/high/alert each 1", rep.StressIndex.Regimes)
	}
	if rep.WriterGap != nil {
		t.Errorf("writer gap = %+v, want nil", rep.WriterGap)
	}
}

// TestRegimeConsistency_Drift_StressDisagrees: stress vocabulary (crisis → RISK_OFF)
// disagrees with authoritative RISK_ON → drift, even though the session agrees.
func TestRegimeConsistency_Drift_StressDisagrees(t *testing.T) {
	f := newRegimeFixture(t)
	d := time.Now().UTC()
	f.upsertRegime(t, fixtureDateStr(d), "RISK_ON")
	f.upsertStress(t, fixtureDateStr(d), "crisis")
	writeSessionSummary(t, f.ledger, fixtureSessionID(d), domain.RegimeRiskOn, d)

	rep := f.mustCheck(t, 7)

	if rep.Status != RegimeConsistencyDrift {
		t.Errorf("status = %q, want %q", rep.Status, RegimeConsistencyDrift)
	}
	if rep.Drifts != 1 || rep.Matches != 0 {
		t.Errorf("matches/drifts = %d/%d, want 0/1", rep.Matches, rep.Drifts)
	}
	if len(rep.DriftDetails) != 1 {
		t.Fatalf("drift details = %d, want 1", len(rep.DriftDetails))
	}
	dd := rep.DriftDetails[0]
	if dd.Endpoint != "stress_index" || dd.Actual != "crisis" || dd.Normalized != "RISK_OFF" || dd.Authoritative != "RISK_ON" {
		t.Errorf("drift detail = %+v, want endpoint=stress_index actual=crisis normalized=RISK_OFF authoritative=RISK_ON", dd)
	}
}

// TestRegimeConsistency_Drift_SessionDisagrees: a session regime that
// contradicts the authoritative endpoint is a drift too.
func TestRegimeConsistency_Drift_SessionDisagrees(t *testing.T) {
	f := newRegimeFixture(t)
	d := time.Now().UTC()
	f.upsertRegime(t, fixtureDateStr(d), "RISK_OFF")
	f.upsertStress(t, fixtureDateStr(d), "high") // high → RISK_OFF, agrees
	writeSessionSummary(t, f.ledger, fixtureSessionID(d), domain.RegimeRiskOn, d)

	rep := f.mustCheck(t, 7)

	if rep.Status != RegimeConsistencyDrift {
		t.Errorf("status = %q, want %q", rep.Status, RegimeConsistencyDrift)
	}
	if len(rep.DriftDetails) != 1 {
		t.Fatalf("drift details = %d, want 1", len(rep.DriftDetails))
	}
	dd := rep.DriftDetails[0]
	if dd.Endpoint != "session" || dd.Actual != "RISK_ON" || dd.Authoritative != "RISK_OFF" {
		t.Errorf("drift detail = %+v, want endpoint=session actual=RISK_ON authoritative=RISK_OFF", dd)
	}
}

// TestRegimeConsistency_UnknownMix_WriterGap: empty-regime summary + orphan
// session produce unknown sessions with correct writer-gap attribution; the
// high ratio flips status to unknown_high while known sessions still match.
func TestRegimeConsistency_UnknownMix_WriterGap(t *testing.T) {
	f := newRegimeFixture(t)
	d := time.Now().UTC()
	f.upsertRegime(t, fixtureDateStr(d), "RISK_ON")
	f.upsertStress(t, fixtureDateStr(d), "low")
	// one known, one empty-regime summary (backfill writer gap), one orphan.
	writeSessionSummary(t, f.ledger, fixtureSessionID(d)+"-known", domain.RegimeRiskOn, d)
	writeSessionSummary(t, f.ledger, fixtureSessionID(d)+"-empty", domain.Regime(""), d)
	writeOrphanSession(t, f.ledger, fixtureSessionID(d)+"-orphan")

	rep := f.mustCheck(t, 7)

	if rep.Sessions.Total != 3 {
		t.Errorf("sessions total = %d, want 3", rep.Sessions.Total)
	}
	if rep.UnknownCount != 2 {
		t.Errorf("unknown count = %d, want 2", rep.UnknownCount)
	}
	if rep.UnknownRatio < 0.66 || rep.UnknownRatio > 0.67 {
		t.Errorf("unknown ratio = %.3f, want ≈0.667", rep.UnknownRatio)
	}
	if rep.Status != RegimeConsistencyUnknownHigh {
		t.Errorf("status = %q, want %q", rep.Status, RegimeConsistencyUnknownHigh)
	}
	// unknown sessions are writer gaps, not drifts.
	if rep.Drifts != 0 || rep.Matches != 1 {
		t.Errorf("matches/drifts = %d/%d, want 1/0", rep.Matches, rep.Drifts)
	}
	if rep.WriterGap == nil {
		t.Fatal("writer gap = nil, want populated")
	}
	if rep.WriterGap.EmptyRegimeInSummary != 1 || rep.WriterGap.MissingSummary != 1 {
		t.Errorf("writer gap = empty:%d missing:%d, want 1/1",
			rep.WriterGap.EmptyRegimeInSummary, rep.WriterGap.MissingSummary)
	}
	if len(rep.WriterGap.UnknownSessionIDs) != 2 {
		t.Errorf("unknown session ids = %v, want 2", rep.WriterGap.UnknownSessionIDs)
	}
	if rep.WriterGap.RootCause == "" {
		t.Error("writer gap root cause empty, want explanation")
	}
}

// TestRegimeConsistency_CrossWalk_StressVocabulary exercises the full
// four-way stress → regime mapping via the report's normalized distribution.
func TestRegimeConsistency_CrossWalk_StressVocabulary(t *testing.T) {
	f := newRegimeFixture(t)
	dates := fixtureDates(t, 4)
	stress := []string{"low", "alert", "high", "crisis"}
	regimes := []string{"RISK_ON", "NEUTRAL", "RISK_OFF", "RISK_OFF"}
	for i, d := range dates {
		f.upsertRegime(t, fixtureDateStr(d), regimes[i])
		f.upsertStress(t, fixtureDateStr(d), stress[i])
	}
	rep := f.mustCheck(t, 7)

	if rep.Drifts != 0 {
		t.Errorf("drifts = %d, want 0 (all stress tokens cross-walk to their regime)", rep.Drifts)
	}
	if got := rep.StressIndex.Normalized["RISK_ON"]; got != 1 {
		t.Errorf("low → RISK_ON count = %d, want 1", got)
	}
	if got := rep.StressIndex.Normalized["NEUTRAL"]; got != 1 {
		t.Errorf("alert → NEUTRAL count = %d, want 1", got)
	}
	if got := rep.StressIndex.Normalized["RISK_OFF"]; got != 2 {
		t.Errorf("high+crisis → RISK_OFF count = %d, want 2", got)
	}
}

// TestRegimeConsistency_WindowExcludesOldSessions verifies the look-back
// window filters both sessions and store rows.
func TestRegimeConsistency_WindowExcludesOldSessions(t *testing.T) {
	f := newRegimeFixture(t)
	old := time.Now().UTC().AddDate(0, 0, -14)
	writeSessionSummary(t, f.ledger, fixtureSessionID(old), domain.RegimeRiskOn, old)
	// store rows outside the window must not count either.
	f.upsertRegime(t, fixtureDateStr(old), "RISK_ON")

	rep := f.mustCheck(t, 7)

	if rep.Sessions.Total != 0 {
		t.Errorf("sessions total = %d, want 0 (outside 7-day window)", rep.Sessions.Total)
	}
	if rep.RegimeHistory.Rows != 0 {
		t.Errorf("regime_history rows = %d, want 0 (outside window)", rep.RegimeHistory.Rows)
	}
	if rep.ComparedDays != 0 || rep.Matches != 0 {
		t.Errorf("compared/matches = %d/%d, want 0/0", rep.ComparedDays, rep.Matches)
	}
	if rep.Status != RegimeConsistencyOK {
		t.Errorf("status = %q, want ok (no drift, no unknown in window)", rep.Status)
	}
}

// TestRegimeConsistency_Degraded_NoHistoricalStore: without a HistoricalStore
// the report degrades to session-only instead of failing.
func TestRegimeConsistency_Degraded_NoHistoricalStore(t *testing.T) {
	ledgerDir := t.TempDir()
	svc := NewPipelineService(ledgerDir, ledgerDir, ledger.NewStore(ledgerDir))
	d := time.Now().UTC()
	writeSessionSummary(t, ledgerDir, fixtureSessionID(d), domain.RegimeRiskOn, d)

	rep, err := svc.CheckRegimeConsistency(context.Background(), 7)
	if err != nil {
		t.Fatalf("CheckRegimeConsistency: %v", err)
	}
	if rep.Status != RegimeConsistencyDegraded {
		t.Errorf("status = %q, want %q", rep.Status, RegimeConsistencyDegraded)
	}
	if rep.Availability.RegimeHistory || rep.Availability.StressIndex {
		t.Errorf("availability = %+v, want regime_history/stress_index false", rep.Availability)
	}
	if !rep.Availability.Sessions {
		t.Error("availability.sessions = false, want true")
	}
	if rep.Sessions.Total != 1 || rep.Sessions.UnknownCount != 0 {
		t.Errorf("sessions = total:%d unknown:%d, want 1/0", rep.Sessions.Total, rep.Sessions.UnknownCount)
	}
}

// TestClampRegimeWindow verifies the days clamp used by the checker.
func TestClampRegimeWindow(t *testing.T) {
	cases := []struct {
		in   int
		want int
	}{
		{0, RegimeConsistencyDefaultDays},
		{-5, RegimeConsistencyDefaultDays},
		{7, 7},
		{RegimeConsistencyMaxDays, RegimeConsistencyMaxDays},
		{9999, RegimeConsistencyMaxDays},
	}
	for _, tc := range cases {
		if got := clampRegimeWindow(tc.in); got != tc.want {
			t.Errorf("clampRegimeWindow(%d) = %d, want %d", tc.in, got, tc.want)
		}
	}
}
