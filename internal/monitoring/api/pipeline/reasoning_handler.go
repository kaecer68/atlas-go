package pipeline

import (
	"net/http"
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/internal/monitoring/api/shared"
	"github.com/kaecer68/atlas-go/internal/orchestrator"
	"github.com/kaecer68/atlas-go/internal/reporting"
)

type ReasoningHandler struct {
	BaseDir string
}

// ReasoningTraceItem wraps orchestrator.ReasoningTrace with per-trace
// explanation and raw_data fields that the frontend expects directly on
// each trace object.
type ReasoningTraceItem struct {
	SessionID   string    `json:"session_id"`
	Timestamp   time.Time `json:"timestamp"`
	Phase       string    `json:"phase"`
	Step        int       `json:"step"`
	Component   string    `json:"component"`
	Action      string    `json:"action"`
	Reasoning   string    `json:"reasoning"`
	Data        any       `json:"data,omitempty"`
	Confidence  float64   `json:"confidence"`
	IsFallback  bool      `json:"is_fallback"`
	Explanation string    `json:"explanation"`
	RawData     any       `json:"raw_data,omitempty"`
}

type ReasoningTraceResponse struct {
	SessionID string               `json:"session_id"`
	Traces    []ReasoningTraceItem `json:"traces"`
}

func (h *ReasoningHandler) HandleReasoningTrace(r *http.Request) (int, any) {
	sessionID := strings.TrimSpace(r.URL.Query().Get("session_id"))
	if sessionID == "" {
		return http.StatusBadRequest, map[string]string{"error": "session_id is required"}
	}
	if err := shared.ValidateSessionID(sessionID); err != nil {
		return http.StatusBadRequest, map[string]string{"error": err.Error()}
	}

	sp, err := orchestrator.LoadScratchpad(sessionID, h.BaseDir)
	if err != nil {
		// Session may exist (in sessions/ dir) but no trace file written yet
		// (trace files are only created when scratchpad.ExportJSONL() is called
		// during simulation). Return empty traces rather than 404 so the UI
		// shows "no trace data" instead of "fetch failed".
		return http.StatusOK, ReasoningTraceResponse{SessionID: sessionID, Traces: []ReasoningTraceItem{}}
	}

	traces := sp.Traces()
	if len(traces) == 0 {
		return http.StatusOK, ReasoningTraceResponse{SessionID: sessionID, Traces: []ReasoningTraceItem{}}
	}

	items := make([]ReasoningTraceItem, len(traces))
	for i, trace := range traces {
		items[i] = ReasoningTraceItem{
			SessionID:   trace.SessionID,
			Timestamp:   trace.Timestamp,
			Phase:       trace.Phase,
			Step:        trace.Step,
			Component:   trace.Component,
			Action:      trace.Action,
			Reasoning:   trace.Reasoning,
			Data:        trace.Data,
			Confidence:  trace.Confidence,
			IsFallback:  trace.IsFallback,
			Explanation: reporting.ExplainTrace(trace),
			RawData:     trace.Data,
		}
	}

	resp := ReasoningTraceResponse{
		SessionID: sessionID,
		Traces:    items,
	}
	return http.StatusOK, resp
}
