package pipeline

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/orchestrator"
)

func TestHandleReasoningTrace_Success(t *testing.T) {
	tmpDir := t.TempDir()
	tracesDir := filepath.Join(tmpDir, "traces")
	if err := os.MkdirAll(tracesDir, 0755); err != nil {
		t.Fatalf("failed to create traces dir: %v", err)
	}

	sessionID := "session-test-001"
	traceFile := filepath.Join(tracesDir, sessionID+".jsonl")
	f, err := os.Create(traceFile)
	if err != nil {
		t.Fatalf("failed to create trace file: %v", err)
	}
	defer f.Close()

	traces := []orchestrator.ReasoningTrace{
		{
			SessionID:  sessionID,
			Timestamp:  time.Date(2026, 4, 13, 9, 30, 0, 0, time.UTC),
			Phase:      orchestrator.PhaseRegimeDetection,
			Step:       1,
			Component:  "janus",
			Action:     "detect_regime",
			Reasoning:  "Market showing bullish momentum indicators",
			Confidence: 0.78,
			IsFallback: false,
		},
		{
			SessionID:  sessionID,
			Timestamp:  time.Date(2026, 4, 13, 9, 31, 0, 0, time.UTC),
			Phase:      orchestrator.PhaseAgentRecommendation,
			Step:       2,
			Component:  "semiconductor_desk",
			Action:     "generate_recommendation",
			Reasoning:  "Strong order book growth in semiconductor sector",
			Confidence: 0.85,
			IsFallback: false,
		},
	}

	enc := json.NewEncoder(f)
	for _, trace := range traces {
		if err := enc.Encode(trace); err != nil {
			t.Fatalf("failed to encode trace: %v", err)
		}
	}
	f.Close()

	h := &ReasoningHandler{BaseDir: tmpDir}
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/reasoning-trace?session_id="+sessionID, nil)

	status, data := h.HandleReasoningTrace(req)

	if status != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, status)
	}

	resp, ok := data.(ReasoningTraceResponse)
	if !ok {
		t.Fatalf("expected ReasoningTraceResponse, got %T", data)
	}

	if resp.SessionID != sessionID {
		t.Errorf("expected session_id %q, got %q", sessionID, resp.SessionID)
	}

	if len(resp.Traces) != 2 {
		t.Errorf("expected 2 traces, got %d", len(resp.Traces))
	}

	if len(resp.Explanations) != 2 {
		t.Errorf("expected 2 explanations, got %d", len(resp.Explanations))
	}

	if len(resp.Explanations[0]) == 0 {
		t.Error("expected non-empty explanation for first trace")
	}
}

func TestHandleReasoningTrace_MissingSession(t *testing.T) {
	h := &ReasoningHandler{BaseDir: "/tmp/nonexistent"}
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/reasoning-trace", nil)

	status, data := h.HandleReasoningTrace(req)

	if status != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, status)
	}

	errResp, ok := data.(map[string]string)
	if !ok {
		t.Fatalf("expected map[string]string, got %T", data)
	}

	if errResp["error"] != "session_id is required" {
		t.Errorf("expected error message about session_id, got %q", errResp["error"])
	}
}

func TestHandleReasoningTrace_SessionNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	tracesDir := filepath.Join(tmpDir, "traces")
	if err := os.MkdirAll(tracesDir, 0755); err != nil {
		t.Fatalf("failed to create traces dir: %v", err)
	}

	h := &ReasoningHandler{BaseDir: tmpDir}
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/reasoning-trace?session_id=nonexistent", nil)

	status, data := h.HandleReasoningTrace(req)

	if status != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, status)
	}

	errResp, ok := data.(map[string]string)
	if !ok {
		t.Fatalf("expected map[string]string, got %T", data)
	}

	if errResp["error"] != "no traces found for session" {
		t.Errorf("expected 'no traces found for session', got %q", errResp["error"])
	}
}