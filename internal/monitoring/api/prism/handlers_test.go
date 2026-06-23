package prism

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	prismpkg "github.com/kaecer68/atlas-go/internal/prism"
)

// stubTrainingExecutor implements prism.TrainingExecutor for tests.
type stubTrainingExecutor struct {
	explanation string
}

func (s *stubTrainingExecutor) Run(task prismpkg.TrainingTask) (prismpkg.TrainingResult, error) {
	return prismpkg.TrainingResult{
		Explanation:  s.explanation,
		HitRate:      0.65,
		SharpeRatio:  1.2,
		SignalsCount: 8,
	}, nil
}

func TestHandleTrainingResults_ReturnsExplanation(t *testing.T) {
	pm := prismpkg.NewPRISMManager(prismpkg.DefaultPRISMConfig())
	pm.WithExecutor(&stubTrainingExecutor{explanation: "Scenario X suggests defensive tilt"})

	// Schedule a training task so that the worker processes it and records a result.
	agent := domain.AgentSpec{
		ID:      "test-agent",
		Skill:   "sector",
		Enabled: true,
		Layer:   domain.LayerSector,
	}
	window := prismpkg.TrainingWindow{
		Start:     time.Now().Add(-24 * time.Hour),
		End:       time.Now(),
		Regime:    prismpkg.RegimeRiskOn,
		RegimeSet: true,
	}
	if err := pm.ScheduleTraining(agent, []prismpkg.TrainingWindow{window}); err != nil {
		t.Fatalf("ScheduleTraining: %v", err)
	}

	pm.Start()
	time.Sleep(300 * time.Millisecond) // let the worker pick up and process the task
	pm.Stop()

	h := NewHandlers(pm)
	req := httptest.NewRequest(http.MethodGet, "/api/prism/training-results", nil)

	status, body := h.HandleTrainingResults(req)
	if status != http.StatusOK {
		t.Errorf("expected status 200, got %d", status)
	}

	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	if !strings.Contains(string(raw), `"explanation":"Scenario X suggests defensive tilt"`) {
		t.Errorf("response missing explanation; got: %s", string(raw))
	}
}

func TestHandleTrainingResults_Empty(t *testing.T) {
	pm := prismpkg.NewPRISMManager(prismpkg.DefaultPRISMConfig())
	h := NewHandlers(pm)
	req := httptest.NewRequest(http.MethodGet, "/api/prism/training-results", nil)

	status, body := h.HandleTrainingResults(req)
	if status != http.StatusOK {
		t.Errorf("expected status 200, got %d", status)
	}

	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	// Should be an empty JSON array, not null.
	if string(raw) != "[]" {
		t.Errorf("expected empty array `[]`, got: %s", string(raw))
	}
}

func TestRegisterRoutes(t *testing.T) {
	pm := prismpkg.NewPRISMManager(prismpkg.DefaultPRISMConfig())
	h := NewHandlers(pm)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/prism/training-results", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code == 0 {
		t.Error("route /api/prism/training-results not registered (no handler)")
	}
	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}
