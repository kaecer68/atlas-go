package server

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const defaultSamplingTimeout = 30 * time.Second

// MCPSampleLLMInput is the request schema for mcp_sample_llm.
type MCPSampleLLMInput struct {
	Messages     []*mcp.SamplingMessage `json:"messages" jsonschema:"conversation messages to send to the client LLM"`
	MaxTokens    int64                  `json:"max_tokens" jsonschema:"maximum tokens to sample"`
	SystemPrompt string                 `json:"system_prompt,omitempty" jsonschema:"optional system prompt"`
	Temperature  float64                `json:"temperature,omitempty" jsonschema:"optional sampling temperature"`
	Tools        []*mcp.Tool            `json:"tools,omitempty" jsonschema:"optional tools available to the model"`
}

// MCPSampleLLMOutput is the response schema for mcp_sample_llm.
type MCPSampleLLMOutput struct {
	Content    mcp.Content   `json:"content,omitempty"`
	Contents   []mcp.Content `json:"contents,omitempty"`
	Model      string        `json:"model"`
	Role       mcp.Role      `json:"role"`
	StopReason string        `json:"stop_reason,omitempty"`
}

func registerSamplingTools(mcpSrv *mcp.Server, s *server) {
	if !s.cfg.SamplingEnabled {
		return
	}

	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "mcp_sample_llm",
		Description: autoDescOr("mcp_sample_llm", "Request the connected MCP client to sample a message from its LLM. Requires client sampling capability."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: new(false)},
	}, s.handleMCPSampleLLM)
}

func (s *server) handleMCPSampleLLM(ctx context.Context, req *mcp.CallToolRequest, in MCPSampleLLMInput) (*mcp.CallToolResult, MCPSampleLLMOutput, error) {
	if req == nil || req.Session == nil {
		return nil, MCPSampleLLMOutput{}, errors.New("mcp_sample_llm: missing server session")
	}
	ss := req.Session

	ip := ss.InitializeParams()
	if ip == nil || ip.Capabilities == nil || ip.Capabilities.Sampling == nil {
		return nil, MCPSampleLLMOutput{}, errors.New("mcp_sample_llm: client does not support sampling")
	}
	if len(in.Messages) == 0 {
		return nil, MCPSampleLLMOutput{}, errors.New("mcp_sample_llm: messages cannot be empty")
	}

	callCtx, cancel := context.WithTimeout(ctx, defaultSamplingTimeout)
	defer cancel()

	var out MCPSampleLLMOutput
	if err := s.withAudit(callCtx, "mcp_sample_llm", []string{"max_tokens"}, func() error {
		if len(in.Tools) > 0 {
			return s.sampleWithTools(callCtx, ss, in, &out)
		}
		return s.sampleWithoutTools(callCtx, ss, in, &out)
	}); err != nil {
		return nil, MCPSampleLLMOutput{}, err
	}
	return nil, out, nil
}

func (s *server) sampleWithoutTools(ctx context.Context, ss *mcp.ServerSession, in MCPSampleLLMInput, out *MCPSampleLLMOutput) error {
	res, err := ss.CreateMessage(ctx, &mcp.CreateMessageParams{
		Messages:     in.Messages,
		MaxTokens:    in.MaxTokens,
		SystemPrompt: in.SystemPrompt,
		Temperature:  in.Temperature,
	})
	if err != nil {
		return fmt.Errorf("mcp_sample_llm: create message: %w", err)
	}
	out.Model = res.Model
	out.Role = res.Role
	out.StopReason = res.StopReason
	out.Content = res.Content
	if res.Content != nil {
		out.Contents = []mcp.Content{res.Content}
	}
	return nil
}

func (s *server) sampleWithTools(ctx context.Context, ss *mcp.ServerSession, in MCPSampleLLMInput, out *MCPSampleLLMOutput) error {
	v2msgs := make([]*mcp.SamplingMessageV2, len(in.Messages))
	for i, m := range in.Messages {
		if m == nil {
			continue
		}
		var contents []mcp.Content
		if m.Content != nil {
			contents = []mcp.Content{m.Content}
		}
		v2msgs[i] = &mcp.SamplingMessageV2{Role: m.Role, Content: contents}
	}

	res, err := ss.CreateMessageWithTools(ctx, &mcp.CreateMessageWithToolsParams{
		Messages:     v2msgs,
		MaxTokens:    in.MaxTokens,
		SystemPrompt: in.SystemPrompt,
		Temperature:  in.Temperature,
		Tools:        in.Tools,
	})
	if err != nil {
		return fmt.Errorf("mcp_sample_llm: create message with tools: %w", err)
	}
	out.Model = res.Model
	out.Role = res.Role
	out.StopReason = res.StopReason
	out.Contents = res.Content
	if len(res.Content) > 0 {
		out.Content = res.Content[0]
	}
	return nil
}
