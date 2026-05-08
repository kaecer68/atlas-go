package pipeline

import (
	"net/http"
	"strings"

	"github.com/kaecer68/atlas-go/internal/orchestrator"
	"github.com/kaecer68/atlas-go/internal/reporting"
)

type ReasoningHandler struct {
	BaseDir string
}

type ReasoningTraceResponse struct {
	SessionID    string                        `json:"session_id"`
	Traces       []orchestrator.ReasoningTrace `json:"traces"`
	Explanations []string                     `json:"explanations,omitempty"`
}

func (h *ReasoningHandler) HandleReasoningTrace(r *http.Request) (int, any) {
	sessionID := strings.TrimSpace(r.URL.Query().Get("session_id"))
	if sessionID == "" {
		return http.StatusBadRequest, map[string]string{"error": "session_id is required"}
	}

	sp, err := orchestrator.LoadScratchpad(sessionID, h.BaseDir)
	if err != nil {
		return http.StatusNotFound, map[string]string{"error": "no traces found for session"}
	}

	traces := sp.Traces()
	if len(traces) == 0 {
		return http.StatusNotFound, map[string]string{"error": "no traces found for session"}
	}

	explanations := make([]string, len(traces))
	for i, trace := range traces {
		explanations[i] = reporting.ExplainTrace(trace)
	}

	resp := ReasoningTraceResponse{
		SessionID:    sessionID,
		Traces:       traces,
		Explanations: explanations,
	}
	return http.StatusOK, resp
}