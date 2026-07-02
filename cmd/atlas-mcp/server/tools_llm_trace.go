package server

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerLLMTraceTools(mcpSrv *mcp.Server, s *server) {
	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "llm_get_cost",
		Description: autoDescOr("llm_get_cost", "LLM cost snapshot — recent spend, by model, by capability."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleLLMGetCost)

	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "llm_get_health",
		Description: autoDescOr("llm_get_health", "LLM router health — provider status (DeepSeek, MiniMax/M3), circuit-breaker, fallback chain."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleLLMGetHealth)

	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "trace_get_sim_latest",
		Description: autoDescOr("trace_get_sim_latest", "Latest simulation reasoning trace (most recent agent run)."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleTraceGetSimLatest)

	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "trace_get_agent_observatory",
		Description: autoDescOr("trace_get_agent_observatory", "Current agent activity observatory (which agents are active, what they're working on)."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleTraceGetAgentObservatory)

	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "trace_get_decision_chain",
		Description: autoDescOr("trace_get_decision_chain", "Decision chain for a symbol (full causal trace from macro → sector → agent recommendation)."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleTraceGetDecisionChain)

	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "trace_get_reasoning",
		Description: autoDescOr("trace_get_reasoning", "Reasoning trace (RAG/CoT steps) for a recent recommendation."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
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

func (s *server) handleTraceGetSimLatest(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, llmTraceBaseOutput, error) {
	var out llmTraceBaseOutput
	if err := s.withAudit(ctx, "trace_get_sim_latest", nil, func() error {
		return s.cli.Get(ctx, "/api/traces/sim-latest", nil, &out.Result)
	}); err != nil {
		return nil, llmTraceBaseOutput{}, err
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

func (s *server) handleTraceGetReasoning(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, llmTraceBaseOutput, error) {
	var out llmTraceBaseOutput
	if err := s.withAudit(ctx, "trace_get_reasoning", nil, func() error {
		return s.cli.Get(ctx, "/api/dashboard/reasoning-trace", nil, &out.Result)
	}); err != nil {
		return nil, llmTraceBaseOutput{}, err
	}
	return nil, out, nil
}
