// File: handlers_regime_consistency_test.go
// Package: pipeline
//
// End-to-end tests for GET /api/dashboard/regime-consistency (and its
// /api/regime/consistency alias): the endpoint must surface the regime
// three-endpoint consistency numbers and let drift be detected.
package pipeline

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/monitoring/service"
)

// newConsistencyHandler builds a PipelineService wired to a SQLite
// HistoricalStore and returns its handler plus the store (for drift
// manipulation).
func newConsistencyHandler(t *testing.T) (*Handlers, ledger.HistoricalStore) {
	t.Helper()
	baseDir := t.TempDir()
	db, err := ledger.OpenSQLiteDB(filepath.Join(t.TempDir(), "handler_test.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := ledger.InitSchema(db); err != nil {
		_ = db.Close()
		t.Fatalf("init schema: %v", err)
	}
	store := ledger.NewSQLiteHistoricalStore(db)
	t.Cleanup(func() { _ = db.Close() })

	svc := service.NewPipelineService(baseDir, baseDir, ledger.NewStore(baseDir)).
		WithHistoricalStore(store)

	ctx := context.Background()
	now := time.Now().UTC()
	date := now.Format("2006-01-02")
	nowT := now
	if err := store.UpsertRegime(ctx, ledger.RegimeRow{Date: date, Regime: "RISK_ON", Source: "macro_ingest", RecordedAt: nowT, CapturedAt: nowT}); err != nil {
		t.Fatalf("upsert regime: %v", err)
	}
	if err := store.UpsertStress(ctx, ledger.StressRow{Date: date, Score: 30, Regime: "low", Source: "taiwan_stress", CapturedAt: nowT}); err != nil {
		t.Fatalf("upsert stress: %v", err)
	}
	sessionID := "session-" + now.Format("20060102") + "-daily"
	dir := filepath.Join(baseDir, "sessions", sessionID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir session: %v", err)
	}
	sum := domain.SessionSummary{SessionID: sessionID, Regime: domain.RegimeRiskOn, OutcomeCount: 1, RecordedAt: now}
	data, _ := json.Marshal(sum)
	if err := os.WriteFile(filepath.Join(dir, "summary.json"), data, 0o644); err != nil {
		t.Fatalf("write summary: %v", err)
	}
	return NewHandlers(svc), store
}

func TestHandleRegimeConsistency_OK(t *testing.T) {
	h, _ := newConsistencyHandler(t)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	for _, path := range []string{"/api/dashboard/regime-consistency", "/api/regime/consistency"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("%s status = %d, want 200 (body=%s)", path, rr.Code, rr.Body.String())
		}
		var rep service.RegimeConsistencyReport
		if err := json.Unmarshal(rr.Body.Bytes(), &rep); err != nil {
			t.Fatalf("%s unmarshal: %v", path, err)
		}
		if rep.Authoritative != "regime_history" {
			t.Errorf("%s authoritative = %q, want regime_history", path, rep.Authoritative)
		}
		if rep.Status != service.RegimeConsistencyOK {
			t.Errorf("%s status = %q, want ok", path, rep.Status)
		}
		if rep.Drifts != 0 || rep.Matches != 1 {
			t.Errorf("%s matches/drifts = %d/%d, want 1/0", path, rep.Matches, rep.Drifts)
		}
		if rep.Sessions.UnknownCount != 0 {
			t.Errorf("%s unknown = %d, want 0", path, rep.Sessions.UnknownCount)
		}
	}
}

// TestHandleRegimeConsistency_DriftDetected drives the fixture to a drift
// (stress crisis vs authoritative RISK_ON) and asserts the endpoint reports it.
func TestHandleRegimeConsistency_DriftDetected(t *testing.T) {
	h, store := newConsistencyHandler(t)
	// override the stress row to disagree: crisis → RISK_OFF vs RISK_ON.
	ctx := context.Background()
	now := time.Now().UTC()
	date := now.Format("2006-01-02")
	if err := store.UpsertStress(ctx, ledger.StressRow{Date: date, Score: 90, Regime: "crisis", Source: "taiwan_stress", CapturedAt: now}); err != nil {
		t.Fatalf("upsert stress: %v", err)
	}

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/regime-consistency", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body=%s)", rr.Code, rr.Body.String())
	}
	var rep service.RegimeConsistencyReport
	if err := json.Unmarshal(rr.Body.Bytes(), &rep); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if rep.Status != service.RegimeConsistencyDrift {
		t.Errorf("status = %q, want %q", rep.Status, service.RegimeConsistencyDrift)
	}
	if rep.Drifts != 1 || len(rep.DriftDetails) != 1 {
		t.Fatalf("drifts/details = %d/%d, want 1/1", rep.Drifts, len(rep.DriftDetails))
	}
	dd := rep.DriftDetails[0]
	if dd.Endpoint != "stress_index" || dd.Actual != "crisis" || dd.Normalized != "RISK_OFF" {
		t.Errorf("drift detail = %+v, want endpoint=stress_index actual=crisis normalized=RISK_OFF", dd)
	}
}

func TestHandleRegimeConsistency_DaysParam(t *testing.T) {
	h, _ := newConsistencyHandler(t)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/regime-consistency?days=7", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body=%s)", rr.Code, rr.Body.String())
	}
	var rep service.RegimeConsistencyReport
	if err := json.Unmarshal(rr.Body.Bytes(), &rep); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if rep.WindowDays != 7 {
		t.Errorf("window_days = %d, want 7", rep.WindowDays)
	}
}

func TestHandleRegimeConsistency_InvalidDays(t *testing.T) {
	h, _ := newConsistencyHandler(t)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	for _, q := range []string{"days=0", "days=-3", "days=abc"} {
		req := httptest.NewRequest(http.MethodGet, "/api/dashboard/regime-consistency?"+q, nil)
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Errorf("?%s status = %d, want 400 (body=%s)", q, rr.Code, rr.Body.String())
		}
	}
}
