package server

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerDataUniverseTools(mcpSrv *mcp.Server, s *server) {
	mcp.AddTool(mcpSrv, &mcp.Tool{
		Name:        "data_get_channels",
		Description: "List all data channels (fugle, twse, yahoo, finmind, internal) with status.",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleDataGetChannels)

	mcp.AddTool(mcpSrv, &mcp.Tool{
		Name:        "data_get_channel_detail",
		Description: "Detail for a single data channel by name (latency, error rate, last fetch).",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleDataGetChannelDetail)

	mcp.AddTool(mcpSrv, &mcp.Tool{
		Name:        "data_get_quality",
		Description: "Data quality metrics (gaps, stale symbols, completeness by source).",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleDataGetQuality)

	mcp.AddTool(mcpSrv, &mcp.Tool{
		Name:        "data_get_field_contract",
		Description: "Field contract schema introspection (field types, optionality) for a model.",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleDataGetFieldContract)

	mcp.AddTool(mcpSrv, &mcp.Tool{
		Name:        "universe_get_sessions",
		Description: "Recent simulation sessions (id, date, status).",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleUniverseGetSessions)

	mcp.AddTool(mcpSrv, &mcp.Tool{
		Name:        "universe_get_universe_overlap",
		Description: "Universe overlap analysis across recent simulation sessions.",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
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
		return s.cli.Get(ctx, "/api/field-contract", nil, &out.Result)
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

func (s *server) handleUniverseGetUniverseOverlap(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, dataUniverseBaseOutput, error) {
	var out dataUniverseBaseOutput
	if err := s.withAudit(ctx, "universe_get_universe_overlap", nil, func() error {
		return s.cli.Get(ctx, "/api/dashboard/universe-overlap", nil, &out.Result)
	}); err != nil {
		return nil, dataUniverseBaseOutput{}, err
	}
	return nil, out, nil
}
