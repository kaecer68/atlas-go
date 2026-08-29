package server

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerLLMTraceTools(mcpSrv *mcp.Server, s *server) {
	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "llm_get_cost",
		Description: autoDescOr("llm_get_cost", "LLM cost snapshot — recent spend, by model, by capability."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: new(false)},
	}, s.handleLLMGetCost)

	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "llm_get_health",
		Description: autoDescOr("llm_get_health", "LLM router health — provider status (DeepSeek, MiniMax/M3), circuit-breaker, fallback chain."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: new(false)},
	}, s.handleLLMGetHealth)

	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "trace_get_sim_latest",
		Description: autoDescOr("trace_get_sim_latest", "Latest simulation reasoning trace (most recent agent run)."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: new(false)},
	}, s.handleTraceGetSimLatest)

	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "trace_get_agent_observatory",
		Description: autoDescOr("trace_get_agent_observatory", "Current agent activity observatory (which agents are active, what they're working on)."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: new(false)},
	}, s.handleTraceGetAgentObservatory)

	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "trace_get_decision_chain",
		Description: autoDescOr("trace_get_decision_chain", "Decision chain for a symbol (full causal trace from macro → sector → agent recommendation)."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: new(false)},
	}, s.handleTraceGetDecisionChain)

	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "trace_get_reasoning",
		Description: autoDescOr("trace_get_reasoning", "Reasoning trace (RAG/CoT steps) for a recent recommendation."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: new(false)},
	}, s.handleTraceGetReasoning)
}

type llmTraceBaseOutput struct {
	Result *map[string]any `json:"result"`
}

func (s *server) handleLLMGetCost(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, llmTraceBaseOutput, error) {
	var out llmTraceBaseOutput
	if err := s.withAudit(ctx, "llm_get_cost", nil, func() error {
		return s.cli.Get(ctx, "/api/llm_annotator/cost", nil, &out.Result)
	}); err != nil {
		return nil, llmTraceBaseOutput{}, err
	}
	return nil, out, nil
}

func (s *server) handleLLMGetHealth(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, llmTraceBaseOutput, error) {
	var out llmTraceBaseOutput
	if err := s.withAudit(ctx, "llm_get_health", nil, func() error {
		return s.cli.Get(ctx, "/api/llm/health", nil, &out.Result)
	}); err != nil {
		return nil, llmTraceBaseOutput{}, err
	}
	return nil, out, nil
}

// traceSimLatestOutput decodes the JSON array returned by
// GET /api/traces/sim-latest. Items stay as map[string]any to keep MCP
// schema decoupled from the trace record type.
type traceSimLatestOutput struct {
	Traces []map[string]any `json:"traces"`
}

func (s *server) handleTraceGetSimLatest(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, traceSimLatestOutput, error) {
	var out traceSimLatestOutput
	if err := s.withAudit(ctx, "trace_get_sim_latest", nil, func() error {
		return s.cli.Get(ctx, "/api/traces/sim-latest", nil, &out.Traces)
	}); err != nil {
		return nil, traceSimLatestOutput{}, err
	}
	return nil, out, nil
}

func (s *server) handleTraceGetAgentObservatory(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, llmTraceBaseOutput, error) {
	var out llmTraceBaseOutput
	if err := s.withAudit(ctx, "trace_get_agent_observatory", nil, func() error {
		return s.cli.Get(ctx, "/api/dashboard/agent-observatory", nil, &out.Result)
	}); err != nil {
		return nil, llmTraceBaseOutput{}, err
	}
	return nil, out, nil
}

func (s *server) handleTraceGetDecisionChain(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, llmTraceBaseOutput, error) {
	var out llmTraceBaseOutput
	if err := s.withAudit(ctx, "trace_get_decision_chain", nil, func() error {
		return s.cli.Get(ctx, "/api/dashboard/decision-chain", nil, &out.Result)
	}); err != nil {
		return nil, llmTraceBaseOutput{}, err
	}
	return nil, out, nil
}

// traceReasoningInput accepts an optional session_id. When omitted, the
// handler resolves the most recent session via GET /api/dashboard/sessions
// (which is sorted by trading date descending) so the tool never fails with
// a bare 400 for callers that do not track session ids.
type traceReasoningInput struct {
	SessionID string `json:"session_id,omitempty" jsonschema:"optional session id; defaults to the most recent session"`
}

func (s *server) handleTraceGetReasoning(ctx context.Context, _ *mcp.CallToolRequest, in traceReasoningInput) (*mcp.CallToolResult, llmTraceBaseOutput, error) {
	sessionID := strings.TrimSpace(in.SessionID)
	if sessionID == "" {
		var sess struct {
			Sessions []map[string]any `json:"sessions"`
		}
		if err := s.cli.Get(ctx, "/api/dashboard/sessions", nil, &sess); err != nil {
			return nil, llmTraceBaseOutput{}, fmt.Errorf("list sessions for default reasoning trace: %w", err)
		}
		if len(sess.Sessions) == 0 {
			return nil, llmTraceBaseOutput{}, fmt.Errorf("no sessions available for default reasoning trace")
		}
		sessionID, _ = sess.Sessions[0]["session_id"].(string)
		if sessionID == "" {
			return nil, llmTraceBaseOutput{}, fmt.Errorf("latest session has empty session_id")
		}
	}
	q := url.Values{"session_id": {sessionID}}
	var out llmTraceBaseOutput
	if err := s.withAudit(ctx, "trace_get_reasoning", []string{"session_id"}, func() error {
		return s.cli.Get(ctx, "/api/dashboard/reasoning-trace", q, &out.Result)
	}); err != nil {
		return nil, llmTraceBaseOutput{}, err
	}
	return nil, out, nil
}
