package dailyreport

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// genWithReport creates a generator with a generated report for today.
func genWithReport(t *testing.T) (*Generator, *Tracker) {
	t.Helper()
	dir := t.TempDir()
	gen := NewGenerator(dir)
	trk := NewTracker(dir, filepath.Join(dir, "replay"))
	gen.SetTracker(trk)
	rep := gen.Generate()
	if rep.Date == "" {
		t.Fatal("generate failed")
	}
	return gen, trk
}

func TestHandleRevise_Success(t *testing.T) {
	gen, trk := genWithReport(t)
	h := NewHandler(gen, trk)
	date := gen.Latest().Date

	body := `{"note":"人工訂正","by":"ops-1","fields":[{"path":"strategy.active_strategy","value":"momentum"},{"path":"risk.risk_level","value":"high"}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/reports/"+date+"/revise", strings.NewReader(body))
	req.SetPathValue("date", date)
	rec := httptest.NewRecorder()
	h.HandleRevise(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("revise = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var rep Report
	if err := json.Unmarshal(rec.Body.Bytes(), &rep); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if rep.WorkflowStatus != WorkflowCorrected {
		t.Errorf("WorkflowStatus = %q, want corrected", rep.WorkflowStatus)
	}
	if rep.Strategy.Active != "momentum" {
		t.Errorf("strategy = %q, want momentum", rep.Strategy.Active)
	}
	if len(rep.RevisionHistory) != 1 {
		t.Fatalf("revision history = %d, want 1", len(rep.RevisionHistory))
	}
	if rep.RevisionHistory[0].By != "ops-1" || rep.RevisionHistory[0].Note != "人工訂正" {
		t.Errorf("history entry = %+v", rep.RevisionHistory[0])
	}
}

func TestHandleRevise_PersistsOverwriteWithHistory(t *testing.T) {
	dir := t.TempDir()
	gen := NewGenerator(dir)
	trk := NewTracker(dir, filepath.Join(dir, "replay"))
	gen.SetTracker(trk)
	rep := gen.Generate()
	date := rep.Date

	h := NewHandler(gen, trk)
	body := `{"note":"v1","fields":[{"path":"strategy.direction","value":"偏空"}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/reports/"+date+"/revise", strings.NewReader(body))
	req.SetPathValue("date", date)
	rec := httptest.NewRecorder()
	h.HandleRevise(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("first revise = %d", rec.Code)
	}

	// Second revision appends history.
	body2 := `{"note":"v2","fields":[{"path":"risk.warning","value":"警戒"}]}`
	req2 := httptest.NewRequest(http.MethodPost, "/api/reports/"+date+"/revise", strings.NewReader(body2))
	req2.SetPathValue("date", date)
	rec2 := httptest.NewRecorder()
	h.HandleRevise(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("second revise = %d", rec2.Code)
	}

	// The on-disk file must contain the full revision history (2 entries).
	data, err := os.ReadFile(filepath.Join(dir, "data", "reports", date+".json"))
	if err != nil {
		t.Fatalf("read persisted file: %v", err)
	}
	var persisted Report
	if err := json.Unmarshal(data, &persisted); err != nil {
		t.Fatalf("decode persisted: %v", err)
	}
	if persisted.WorkflowStatus != WorkflowCorrected {
		t.Errorf("persisted status = %q, want corrected", persisted.WorkflowStatus)
	}
	if len(persisted.RevisionHistory) != 2 {
		t.Fatalf("persisted history = %d, want 2", len(persisted.RevisionHistory))
	}
	if persisted.Strategy.Direction != "偏空" || persisted.Risk.Warning != "警戒" {
		t.Errorf("persisted fields wrong: %+v", persisted)
	}
}

func TestHandleRevise_WhitelistRejected(t *testing.T) {
	gen, trk := genWithReport(t)
	h := NewHandler(gen, trk)
	date := gen.Latest().Date

	body := `{"note":"hack","fields":[{"path":"capital.foreign","value":"篡改"}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/reports/"+date+"/revise", strings.NewReader(body))
	req.SetPathValue("date", date)
	rec := httptest.NewRecorder()
	h.HandleRevise(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("whitelist reject = %d, want 400; body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "not whitelisted") {
		t.Errorf("body = %s", rec.Body.String())
	}
}

func TestHandleRevise_NotFound(t *testing.T) {
	gen, trk := genWithReport(t)
	h := NewHandler(gen, trk)

	body := `{"note":"x","fields":[{"path":"risk.risk_level","value":"high"}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/reports/2099-01-01/revise", strings.NewReader(body))
	req.SetPathValue("date", "2099-01-01")
	rec := httptest.NewRecorder()
	h.HandleRevise(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown date revise = %d, want 404", rec.Code)
	}
}

func TestHandleRevise_BadJSON(t *testing.T) {
	gen, trk := genWithReport(t)
	h := NewHandler(gen, trk)
	date := gen.Latest().Date

	req := httptest.NewRequest(http.MethodPost, "/api/reports/"+date+"/revise", strings.NewReader("{not json"))
	req.SetPathValue("date", date)
	rec := httptest.NewRecorder()
	h.HandleRevise(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad JSON = %d, want 400", rec.Code)
	}
}

func TestHandleApprove_Success(t *testing.T) {
	dir := t.TempDir()
	gen := NewGenerator(dir)
	trk := NewTracker(dir, filepath.Join(dir, "replay"))
	gen.SetTracker(trk)
	date := gen.Generate().Date

	h := NewHandler(gen, trk)
	req := httptest.NewRequest(http.MethodPost, "/api/reports/"+date+"/approve", nil)
	req.SetPathValue("date", date)
	rec := httptest.NewRecorder()
	h.HandleApprove(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("approve = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var rep Report
	if err := json.Unmarshal(rec.Body.Bytes(), &rep); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if rep.WorkflowStatus != WorkflowApproved {
		t.Errorf("status = %q, want approved", rep.WorkflowStatus)
	}
	if len(rep.RevisionHistory) != 0 {
		t.Errorf("approve should not add revision history, got %d", len(rep.RevisionHistory))
	}
	// Approved state must be persisted back to the report file.
	data, err := os.ReadFile(filepath.Join(dir, "data", "reports", date+".json"))
	if err != nil {
		t.Fatalf("read persisted file: %v", err)
	}
	var persisted Report
	if err := json.Unmarshal(data, &persisted); err != nil {
		t.Fatalf("decode persisted: %v", err)
	}
	if persisted.WorkflowStatus != WorkflowApproved {
		t.Errorf("persisted status = %q, want approved", persisted.WorkflowStatus)
	}
}

func TestHandleApprove_NotFound(t *testing.T) {
	gen, trk := genWithReport(t)
	h := NewHandler(gen, trk)

	req := httptest.NewRequest(http.MethodPost, "/api/reports/2099-01-01/approve", nil)
	req.SetPathValue("date", "2099-01-01")
	rec := httptest.NewRecorder()
	h.HandleApprove(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown date approve = %d, want 404", rec.Code)
	}
}

func TestHandleTrackedClaims_Unavailable(t *testing.T) {
	gen, _ := genWithReport(t)
	h := NewHandler(gen, nil) // no tracker

	req := httptest.NewRequest(http.MethodGet, "/api/reports/tracked-claims", nil)
	rec := httptest.NewRecorder()
	h.HandleTrackedClaims(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("no tracker = %d, want 503", rec.Code)
	}
}

func TestHandleTrackedClaims_ListAndFilter(t *testing.T) {
	gen, trk := genWithReport(t)
	h := NewHandler(gen, trk)

	req := httptest.NewRequest(http.MethodGet, "/api/reports/tracked-claims", nil)
	rec := httptest.NewRecorder()
	h.HandleTrackedClaims(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list = %d, want 200", rec.Code)
	}
	var resp struct {
		Claims []TrackedClaim `json:"claims"`
		Count  int            `json:"count"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// Default provider report has no period/warning → 1 claim (strategy).
	if resp.Count != 1 {
		t.Fatalf("count = %d, want 1", resp.Count)
	}
	if resp.Claims[0].ClaimType != ClaimStrategyRecommendation {
		t.Errorf("claim type = %s", resp.Claims[0].ClaimType)
	}

	// Filter by status.
	req2 := httptest.NewRequest(http.MethodGet, "/api/reports/tracked-claims?status=verified", nil)
	rec2 := httptest.NewRecorder()
	h.HandleTrackedClaims(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("filter = %d", rec2.Code)
	}
	var resp2 struct {
		Claims []TrackedClaim `json:"claims"`
		Count  int            `json:"count"`
	}
	_ = json.Unmarshal(rec2.Body.Bytes(), &resp2)
	if resp2.Count != 0 {
		t.Errorf("verified filter count = %d, want 0", resp2.Count)
	}
}

func TestReviseRoutes_AdminAuth(t *testing.T) {
	dir := t.TempDir()
	gen := NewGenerator(dir)
	trk := NewTracker(dir, filepath.Join(dir, "replay"))
	mux := http.NewServeMux()
	RegisterRoutes(mux, gen, trk)
	date := gen.Generate().Date

	t.Run("wrong key 401", func(t *testing.T) {
		t.Setenv("ATLAS_API_KEY", "secret")
		req := httptest.NewRequest(http.MethodPost, "/api/reports/"+date+"/revise",
			strings.NewReader(`{"note":"x","fields":[{"path":"risk.risk_level","value":"high"}]}`))
		req.Header.Set("X-API-Key", "wrong")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("wrong key = %d, want 401", rec.Code)
		}
	})

	t.Run("correct key passes", func(t *testing.T) {
		t.Setenv("ATLAS_API_KEY", "secret")
		req := httptest.NewRequest(http.MethodPost, "/api/reports/"+date+"/revise",
			strings.NewReader(`{"note":"x","fields":[{"path":"risk.risk_level","value":"high"}]}`))
		req.Header.Set("X-API-Key", "secret")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("correct key = %d, want 200; body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("bearer token passes", func(t *testing.T) {
		t.Setenv("ATLAS_API_KEY", "secret")
		req := httptest.NewRequest(http.MethodPost, "/api/reports/"+date+"/approve", nil)
		req.Header.Set("Authorization", "Bearer secret")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("bearer = %d, want 200; body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("dev mode no key passes through", func(t *testing.T) {
		t.Setenv("ATLAS_API_KEY", "")
		t.Setenv("ATLAS_ENV", "")
		req := httptest.NewRequest(http.MethodPost, "/api/reports/"+date+"/approve", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("dev pass-through = %d, want 200", rec.Code)
		}
	})

	t.Run("production missing key 503", func(t *testing.T) {
		t.Setenv("ATLAS_ENV", "production")
		t.Setenv("ATLAS_API_KEY", "")
		req := httptest.NewRequest(http.MethodPost, "/api/reports/"+date+"/approve", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("prod no key = %d, want 503", rec.Code)
		}
	})
}
