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

// loadSessionSummaryTrace reads summary.json for the session and returns
// synthetic traces for all four decision phases when no detailed trace file
// is available. This ensures the reasoning-trace UI always shows the full
// decision pipeline for audit and attribution purposes.
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

	// Build all four decision phases from the limited summary data.
	phases := []struct {
		phase      string
		component  string
		action     string
		reasoning  string
		confidence float64
		data       map[string]any
	}{
		{
			phase:      orchestrator.PhaseRegimeDetection,
			component:  "session_summary",
			action:     "detect_regime",
			reasoning:  "市場體制判定：依據回測期間的 macro 與價量數據判定當前體制",
			confidence: 0.7,
			data: map[string]any{
				"regime":        summary.Regime,
				"outcome_count": summary.OutcomeCount,
				"note":          "historical session — detailed regime trace not preserved",
			},
		},
		{
			phase:      orchestrator.PhaseAgentRecommendation,
			component:  "session_summary",
			action:     "agent_recommend",
			reasoning:  "代理推薦階段：AI Agent 根據體制與敘事事件產生標的推薦",
			confidence: 0.6,
			data: map[string]any{
				"order_count": summary.OrderCount,
				"note":        "historical session — detailed agent recommendation trace not preserved",
			},
		},
		{
			phase:      orchestrator.PhaseControlFilter,
			component:  "session_summary",
			action:     "control_filter",
			reasoning:  "控制層過濾：風控長與投資長對推薦標的進行風險與合規審查",
			confidence: 0.6,
			data: map[string]any{
				"orders_before_filter": summary.OrderCount,
				"note":                 "historical session — detailed control filter trace not preserved",
			},
		},
		{
			phase:      orchestrator.PhasePortfolioBuild,
			component:  "session_summary",
			action:     "portfolio_build",
			reasoning:  "組合構建：最終放行標的納入投資組合並計算績效",
			confidence: 0.7,
			data: map[string]any{
				"position_count":  summary.PositionCount,
				"portfolio_value": summary.PortfolioValue,
				"note":            "historical session — detailed portfolio build trace not preserved",
			},
		},
	}

	items := make([]ReasoningTraceItem, 0, len(phases))
	for i, p := range phases {
		trace := orchestrator.ReasoningTrace{
			SessionID:  sessionID,
			Timestamp:  now.Add(time.Duration(i) * time.Second), // preserve order
			Phase:      p.phase,
			Step:       i + 1,
			Component:  p.component,
			Action:     p.action,
			Reasoning:  p.reasoning,
			Confidence: p.confidence,
			IsFallback: true,
			Data:       p.data,
		}
		items = append(items, ReasoningTraceItem{
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
		})
	}
	return items
}
