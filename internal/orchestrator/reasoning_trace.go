package orchestrator

import (
	"time"
)

const (
	PhaseRegimeDetection     = "regime_detection"
	PhaseAgentRecommendation = "agent_recommendation"
	PhaseControlFilter       = "control_filter"
	PhasePortfolioBuild      = "portfolio_build"
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
}
