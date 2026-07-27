package orchestrator

import (
	"time"
)

const (
	PhaseRegimeDetection     = "regime_detection"
	PhaseAgentRecommendation = "agent_recommendation"
	PhaseControlFilter       = "control_filter"
	PhasePortfolioBuild      = "portfolio_build"
	PhaseSystem              = "system_flow"
	PhaseMacroFlow           = "macro_flow"
)

type ReasoningTrace struct {
	SessionID  string    `json:"session_id"`
	Timestamp  time.Time `json:"timestamp"`
	Phase      string    `json:"phase"`
	Step       int       `json:"step"`
	Component  string    `json:"component"`
	Action     string    `json:"action"`
	Reasoning  string    `json:"reasoning"`
	Data       any       `json:"data,omitempty"`
	Confidence float64   `json:"confidence"`
	IsFallback bool      `json:"is_fallback"`

	// B5 P1: causal chain layer tracing (per ATLAS_METHODOLOGY.md §2 layers 0-7).
	LayerID       string `json:"layer_id,omitempty"`        // e.g. "layer_0".."layer_7"
	LayerParentID string `json:"layer_parent_id,omitempty"` // upstream layer trace reference
}
