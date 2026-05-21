package pipeline

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
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
		// No trace file — generate a summary trace from session metadata.
		summaryTraces := h.loadSessionSummaryTrace(sessionID)
		if len(summaryTraces) > 0 {
			return http.StatusOK, ReasoningTraceResponse{SessionID: sessionID, Traces: summaryTraces}
		}
		return http.StatusOK, ReasoningTraceResponse{SessionID: sessionID, Traces: []ReasoningTraceItem{}}
	}

	traces := sp.Traces()
	if len(traces) == 0 {
		summaryTraces := h.loadSessionSummaryTrace(sessionID)
		if len(summaryTraces) > 0 {
			return http.StatusOK, ReasoningTraceResponse{SessionID: sessionID, Traces: summaryTraces}
		}
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

// loadSessionSummaryTrace reads summary.json for the session and returns a
// synthetic trace showing regime and outcome stats when no detailed trace
// file is available.
func (h *ReasoningHandler) loadSessionSummaryTrace(sessionID string) []ReasoningTraceItem {
	summaryPath := filepath.Join(h.BaseDir, "sessions", sessionID, "summary.json")
	data, err := os.ReadFile(summaryPath)
	if err != nil {
		return nil
	}
	var summary struct {
		SessionID      string    `json:"session_id"`
		Regime         string    `json:"regime"`
		OrderCount     int       `json:"order_count"`
		PositionCount  int       `json:"position_count"`
		OutcomeCount   int       `json:"outcome_count"`
		PortfolioValue float64   `json:"portfolio_value"`
		RecordedAt     time.Time `json:"recorded_at"`
		TotalTaxPaid   float64   `json:"total_tax_paid"`
	}
	if err := json.Unmarshal(data, &summary); err != nil {
		return nil
	}
	if summary.OutcomeCount == 0 {
		return nil
	}
	now := summary.RecordedAt
	if now.IsZero() {
		now = time.Now()
	}
	trace := orchestrator.ReasoningTrace{
		SessionID:  sessionID,
		Timestamp:  now,
		Phase:      orchestrator.PhaseRegimeDetection,
		Step:       1,
		Component:  "session_summary",
		Action:     "load_summary",
		Reasoning:  "此場次為歷史記錄，僅保留摘要數據，詳細決策鏈未寫入",
		Confidence: 0.5,
		IsFallback: true,
		Data: map[string]any{
			"regime":          summary.Regime,
			"outcome_count":   summary.OutcomeCount,
			"order_count":     summary.OrderCount,
			"position_count":  summary.PositionCount,
			"portfolio_value": summary.PortfolioValue,
			"note":            "historical session — detailed reasoning trace not preserved",
		},
	}
	return []ReasoningTraceItem{{
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
	}}
}
