package narrative

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kaecer68/atlas-go/internal/monitoring/service"
	"github.com/kaecer68/atlas-go/internal/narrative"
)

func TestHandleStressIndexCurrent(t *testing.T) {
	eng := narrative.NewNarrativeEngine()
	svc := service.NewNarrativeService("", eng, nil)
	h := &Handlers{Svc: svc}

	req := httptest.NewRequest(http.MethodGet, "/api/narrative/stress-index/current", nil)
	status, body := h.HandleStressIndexCurrent(req)

	if status != http.StatusOK {
		t.Fatalf("expected status 200, got %d", status)
	}

	m, ok := body.(map[string]any)
	if !ok {
		t.Fatalf("expected map body, got %T", body)
	}

	if _, ok := m["score"]; !ok {
		t.Error("expected 'score' field")
	}
	if _, ok := m["regime"]; !ok {
		t.Error("expected 'regime' field")
	}
	if _, ok := m["components"]; !ok {
		t.Error("expected 'components' field")
	}
	if _, ok := m["timestamp"]; !ok {
		t.Error("expected 'timestamp' field")
	}
}

func TestHandleStressIndexHistory(t *testing.T) {
	eng := narrative.NewNarrativeEngine()
	svc := service.NewNarrativeService("", eng, nil)
	h := &Handlers{Svc: svc}

	req := httptest.NewRequest(http.MethodGet, "/api/narrative/stress-index/history", nil)
	status, body := h.HandleStressIndexHistory(req)

	if status != http.StatusOK {
		t.Fatalf("expected status 200, got %d", status)
	}

	m, ok := body.(map[string]any)
	if !ok {
		t.Fatalf("expected map body, got %T", body)
	}

	if _, ok := m["history"]; !ok {
		t.Error("expected 'history' field")
	}

	req2 := httptest.NewRequest(http.MethodGet, "/api/narrative/stress-index/history?days=7", nil)
	status2, _ := h.HandleStressIndexHistory(req2)
	if status2 != http.StatusOK {
		t.Fatalf("expected status 200, got %d", status2)
	}

	req3 := httptest.NewRequest(http.MethodGet, "/api/narrative/stress-index/history?days=invalid", nil)
	status3, _ := h.HandleStressIndexHistory(req3)
	if status3 != http.StatusOK {
		t.Fatalf("expected status 200, got %d", status3)
	}
}

func TestHandleStressIndexThresholds(t *testing.T) {
	eng := narrative.NewNarrativeEngine()
	svc := service.NewNarrativeService("", eng, nil)
	h := &Handlers{Svc: svc}

	req := httptest.NewRequest(http.MethodGet, "/api/narrative/stress-index/thresholds", nil)
	status, body := h.HandleStressIndexThresholds(req)

	if status != http.StatusOK {
		t.Fatalf("expected status 200, got %d", status)
	}

	m, ok := body.(map[string]any)
	if !ok {
		t.Fatalf("expected map body, got %T", body)
	}

	if _, ok := m["crisis"]; !ok {
		t.Error("expected 'crisis' field")
	}
	if _, ok := m["high"]; !ok {
		t.Error("expected 'high' field")
	}
	if _, ok := m["alert"]; !ok {
		t.Error("expected 'alert' field")
	}
}
