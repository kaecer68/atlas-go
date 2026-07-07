package dailyreport

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestGenerate(t *testing.T) {
	dir, _ := os.MkdirTemp("", "rpt-test")
	defer os.RemoveAll(dir)

	gen := NewGenerator(dir)
	rep := gen.Generate()
	if rep.Date == "" {
		t.Error("report date should not be empty")
	}
	if rep.Global.Summary == "" {
		t.Error("report should have global summary")
	}
	if rep.Capital.Resonance != 1.0 {
		t.Errorf("expected resonance 1.0, got %.2f", rep.Capital.Resonance)
	}
}

func TestHandleLatest(t *testing.T) {
	dir, _ := os.MkdirTemp("", "rpt-test")
	defer os.RemoveAll(dir)

	gen := NewGenerator(dir)
	h := NewHandler(gen)

	req := httptest.NewRequest(http.MethodGet, "/api/reports/latest", nil)
	rec := httptest.NewRecorder()
	h.HandleLatest(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestHandleArchive(t *testing.T) {
	dir, _ := os.MkdirTemp("", "rpt-test")
	defer os.RemoveAll(dir)

	gen := NewGenerator(dir)
	gen.Generate()

	h := NewHandler(gen)
	req := httptest.NewRequest(http.MethodGet, "/api/reports/archive?date=2025-01-01", nil)
	rec := httptest.NewRecorder()
	h.HandleArchive(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown date, got %d", rec.Code)
	}
}

func TestHandleSubscribe(t *testing.T) {
	dir, _ := os.MkdirTemp("", "rpt-test")
	defer os.RemoveAll(dir)

	gen := NewGenerator(dir)
	h := NewHandler(gen)

	req := httptest.NewRequest(http.MethodPost, "/api/reports/subscribe", nil)
	rec := httptest.NewRecorder()
	h.HandleSubscribe(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}
