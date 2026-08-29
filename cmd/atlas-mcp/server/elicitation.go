package server

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const defaultElicitationTimeout = 120 * time.Second

// MCPElicitUserInput is the request schema for mcp_elicit_user.
type MCPElicitUserInput struct {
	Mode          string         `json:"mode,omitempty" jsonschema:"elicitation mode: form or url; inferred from other fields if empty"`
	Message       string         `json:"message" jsonschema:"message to present to the user"`
	Schema        map[string]any `json:"schema,omitempty" jsonschema:"JSON schema for form mode (top-level properties only)"`
	URL           string         `json:"url,omitempty" jsonschema:"URL to present for url mode"`
	ElicitationID string         `json:"elicitation_id,omitempty" jsonschema:"unique id for url mode"`
}

// MCPElicitUserOutput is the response schema for mcp_elicit_user.
type MCPElicitUserOutput struct {
	Action  string         `json:"action"`
	Content map[string]any `json:"content,omitempty"`
}

func registerElicitationTools(mcpSrv *mcp.Server, s *server) {
	if !s.cfg.ElicitationEnabled {
		return
	}

	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "mcp_elicit_user",
		Description: autoDescOr("mcp_elicit_user", "Ask the connected MCP client to elicit input from the user (form or url). Requires client elicitation capability."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: new(false)},
	}, s.handleMCPElicitUser)
}

func (s *server) handleMCPElicitUser(ctx context.Context, req *mcp.CallToolRequest, in MCPElicitUserInput) (*mcp.CallToolResult, MCPElicitUserOutput, error) {
	if req == nil || req.Session == nil {
		return nil, MCPElicitUserOutput{}, errors.New("mcp_elicit_user: missing server session")
	}
	ss := req.Session

	ip := ss.InitializeParams()
	if ip == nil || ip.Capabilities == nil || ip.Capabilities.Elicitation == nil {
		return nil, MCPElicitUserOutput{}, errors.New("mcp_elicit_user: client does not support elicitation")
	}
	if in.Message == "" {
		return nil, MCPElicitUserOutput{}, errors.New("mcp_elicit_user: message is required")
	}

	mode := in.Mode
	if mode == "" {
		if in.URL != "" {
			mode = "url"
		} else {
			mode = "form"
		}
	}

	params := &mcp.ElicitParams{
		Mode:    mode,
		Message: in.Message,
	}
	switch mode {
	case "form":
		if len(in.Schema) == 0 {
			return nil, MCPElicitUserOutput{}, errors.New("mcp_elicit_user: form mode requires schema")
		}
		if err := validateElicitSchema(in.Schema); err != nil {
			return nil, MCPElicitUserOutput{}, fmt.Errorf("mcp_elicit_user: invalid schema: %w", err)
		}
		params.RequestedSchema = in.Schema
	case "url":
		if in.URL == "" {
			return nil, MCPElicitUserOutput{}, errors.New("mcp_elicit_user: url mode requires url")
		}
		if in.ElicitationID == "" {
			return nil, MCPElicitUserOutput{}, errors.New("mcp_elicit_user: url mode requires elicitation_id")
		}
		params.URL = in.URL
		params.ElicitationID = in.ElicitationID
	default:
		return nil, MCPElicitUserOutput{}, fmt.Errorf("mcp_elicit_user: unsupported mode %q", mode)
	}

	callCtx, cancel := context.WithTimeout(ctx, defaultElicitationTimeout)
	defer cancel()

	var out MCPElicitUserOutput
	if err := s.withAudit(callCtx, "mcp_elicit_user", []string{"mode"}, func() error {
		res, err := ss.Elicit(callCtx, params)
		if err != nil {
			return fmt.Errorf("elicit: %w", err)
		}
		out.Action = res.Action
		out.Content = res.Content
		return nil
	}); err != nil {
		return nil, MCPElicitUserOutput{}, err
	}
	return nil, out, nil
}
