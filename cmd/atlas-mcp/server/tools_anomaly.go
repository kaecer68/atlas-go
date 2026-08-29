package server

import (
	"context"
	"errors"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kaecer68/atlas-go/internal/mcp/anomaly"
)

func registerAnomalyTools(mcpSrv *mcp.Server, s *server) {
	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "mcp_anomaly_get_recent",
		Description: autoDescOr("mcp_anomaly_get_recent", "List the most recent anomaly events detected by the MCP observability subsystem."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: new(false)},
	}, s.handleAnomalyGetRecent)

	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "mcp_anomaly_ack",
		Description: autoDescOr("mcp_anomaly_ack", "Acknowledge an anomaly alert via the atlas alert store."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: new(true)},
	}, s.handleAnomalyAck)
}

// AnomalyGetRecentInput is the input schema for mcp_anomaly_get_recent.
type AnomalyGetRecentInput struct {
	Limit *int `json:"limit,omitempty" jsonschema:"how many events to return; default 10, max 100"`
}

// AnomalyGetRecentOutput is the output schema for mcp_anomaly_get_recent.
type AnomalyGetRecentOutput struct {
	Events []anomaly.AnomalyEvent `json:"events"`
}

// AnomalyAckInput is the input schema for mcp_anomaly_ack.
type AnomalyAckInput struct {
	AlertID string `json:"alert_id" jsonschema:"the alert id to acknowledge"`
}

// AnomalyAckOutput is the output schema for mcp_anomaly_ack.
type AnomalyAckOutput struct {
	Acknowledged bool `json:"acknowledged"`
}

func (s *server) handleAnomalyGetRecent(ctx context.Context, _ *mcp.CallToolRequest, in AnomalyGetRecentInput) (*mcp.CallToolResult, AnomalyGetRecentOutput, error) {
	limit := 10
	if in.Limit != nil && *in.Limit > 0 {
		limit = *in.Limit
	}
	if limit > 100 {
		limit = 100
	}

	var out AnomalyGetRecentOutput
	if err := s.withAudit(ctx, "mcp_anomaly_get_recent", []string{"limit"}, func() error {
		if s.detector == nil {
			return errors.New("anomaly detector not initialized")
		}
		out.Events = s.detector.Store().Recent(limit)
		return nil
	}); err != nil {
		return nil, AnomalyGetRecentOutput{}, err
	}
	return nil, out, nil
}

func (s *server) handleAnomalyAck(ctx context.Context, _ *mcp.CallToolRequest, in AnomalyAckInput) (*mcp.CallToolResult, AnomalyAckOutput, error) {
	if in.AlertID == "" {
		return nil, AnomalyAckOutput{}, errors.New("mcp_anomaly_ack: alert_id is required")
	}

	var out AnomalyAckOutput
	if err := s.withAudit(ctx, "mcp_anomaly_ack", []string{"alert_id"}, func() error {
		body := map[string]string{"alert_id": in.AlertID}
		return s.cli.PostJSON(ctx, "/api/alerts/acknowledge", body, &out)
	}); err != nil {
		return nil, AnomalyAckOutput{}, err
	}
	out.Acknowledged = true
	return nil, out, nil
}
