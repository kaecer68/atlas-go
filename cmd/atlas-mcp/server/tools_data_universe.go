package server

import (
	"context"
	"encoding/json"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerDataUniverseTools(mcpSrv *mcp.Server, s *server) {
	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "data_get_channels",
		Description: autoDescOr("data_get_channels", "List all data channels (fugle, twse, yahoo, finmind, internal) with status."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: new(false)},
	}, s.handleDataGetChannels)

	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "data_get_channel_detail",
		Description: autoDescOr("data_get_channel_detail", "Detail for a single data channel by name (latency, error rate, last fetch)."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: new(false)},
	}, s.handleDataGetChannelDetail)

	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "data_get_quality",
		Description: autoDescOr("data_get_quality", "Data quality metrics (gaps, stale symbols, completeness by source)."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: new(false)},
	}, s.handleDataGetQuality)

	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "data_get_field_contract",
		Description: autoDescOr("data_get_field_contract", "Field contract schema introspection (field types, optionality) for a model."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: new(false)},
	}, s.handleDataGetFieldContract)

	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "universe_get_sessions",
		Description: autoDescOr("universe_get_sessions", "Recent simulation sessions (id, date, status, top_strategies)."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: new(false)},
	}, s.handleUniverseGetSessions)

	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "universe_get_session_detail",
		Description: autoDescOr("universe_get_session_detail", "Per-strategy drill-down for one session (full recommendation outcomes + summary)."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: new(false)},
	}, s.handleUniverseGetSessionDetail)

	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "universe_get_universe_overlap",
		Description: autoDescOr("universe_get_universe_overlap", "Universe overlap analysis across recent simulation sessions."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: new(false)},
	}, s.handleUniverseGetUniverseOverlap)
}

type dataUniverseBaseOutput struct {
	Result *map[string]any `json:"result"`
}

type channelNameInput struct {
	ChannelName string `json:"channel_name" jsonschema:"the data channel name (e.g. 'fugle', 'twse')"`
}

func (s *server) handleDataGetChannels(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, dataUniverseBaseOutput, error) {
	var out dataUniverseBaseOutput
	if err := s.withAudit(ctx, "data_get_channels", nil, func() error {
		return s.cli.Get(ctx, "/api/dashboard/data-channels", nil, &out.Result)
	}); err != nil {
		return nil, dataUniverseBaseOutput{}, err
	}
	return nil, out, nil
}

func (s *server) handleDataGetChannelDetail(ctx context.Context, _ *mcp.CallToolRequest, in channelNameInput) (*mcp.CallToolResult, dataUniverseBaseOutput, error) {
	var out dataUniverseBaseOutput
	if err := s.withAudit(ctx, "data_get_channel_detail", []string{"channel_name"}, func() error {
		return s.cli.Get(ctx, "/api/dashboard/data-channels/"+in.ChannelName, nil, &out.Result)
	}); err != nil {
		return nil, dataUniverseBaseOutput{}, err
	}
	return nil, out, nil
}

func (s *server) handleDataGetQuality(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, dataUniverseBaseOutput, error) {
	var out dataUniverseBaseOutput
	if err := s.withAudit(ctx, "data_get_quality", nil, func() error {
		return s.cli.Get(ctx, "/api/dashboard/data-quality", nil, &out.Result)
	}); err != nil {
		return nil, dataUniverseBaseOutput{}, err
	}
	return nil, out, nil
}

func (s *server) handleDataGetFieldContract(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, dataUniverseBaseOutput, error) {
	var out dataUniverseBaseOutput
	if err := s.withAudit(ctx, "data_get_field_contract", nil, func() error {
		raw, err := s.cli.GetRaw(ctx, "/api/field-contract", nil)
		if err != nil {
			return err
		}
		// /api/field-contract returns a JSON array of field names.
		// Wrap it in an object so the MCP client can unmarshal it.
		wrapper := map[string]any{"fields": json.RawMessage(raw)}
		out.Result = &wrapper
		return nil
	}); err != nil {
		return nil, dataUniverseBaseOutput{}, err
	}
	return nil, out, nil
}

func (s *server) handleUniverseGetSessions(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, dataUniverseBaseOutput, error) {
	var out dataUniverseBaseOutput
	if err := s.withAudit(ctx, "universe_get_sessions", nil, func() error {
		return s.cli.Get(ctx, "/api/dashboard/sessions", nil, &out.Result)
	}); err != nil {
		return nil, dataUniverseBaseOutput{}, err
	}
	return nil, out, nil
}

type sessionIDInput struct {
	SessionID string `json:"session_id" jsonschema:"the session_id from universe_get_sessions"`
}

func (s *server) handleUniverseGetSessionDetail(ctx context.Context, _ *mcp.CallToolRequest, in sessionIDInput) (*mcp.CallToolResult, dataUniverseBaseOutput, error) {
	var out dataUniverseBaseOutput
	if err := s.withAudit(ctx, "universe_get_session_detail", []string{"session_id"}, func() error {
		return s.cli.Get(ctx, "/api/dashboard/sessions/"+in.SessionID, nil, &out.Result)
	}); err != nil {
		return nil, dataUniverseBaseOutput{}, err
	}
	return nil, out, nil
}

func (s *server) handleUniverseGetUniverseOverlap(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, dataUniverseBaseOutput, error) {
	var out dataUniverseBaseOutput
	if err := s.withAudit(ctx, "universe_get_universe_overlap", nil, func() error {
		return s.cli.Get(ctx, "/api/dashboard/universe-overlap", nil, &out.Result)
	}); err != nil {
		return nil, dataUniverseBaseOutput{}, err
	}
	return nil, out, nil
}
