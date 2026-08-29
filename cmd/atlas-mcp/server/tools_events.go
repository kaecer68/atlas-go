package server

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerEventTools(mcpSrv *mcp.Server, s *server) {
	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "event_calendar",
		Description: autoDescOr("event_calendar", "Upcoming Taiwan market events calendar (ETF rebalances, MSCI adjustments, revenue announcements, window dressing, holidays). 14-day forward look.  HTTP: GET /api/events/calendar. Alternative: event_flow_prediction."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: new(false)},
	}, s.handleEventCalendar)

	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "event_flow_prediction",
		Description: autoDescOr("event_flow_prediction", "5-day event-driven capital flow prediction. Maps upcoming events (ETF rebalances, revenue, MSCI, etc.) to predicted capital flow directions by day with confidence scores.  HTTP: GET /api/events/prediction. Alternative: event_calendar, narrative_get_events."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: new(false)},
	}, s.handleEventFlowPrediction)
}

func (s *server) handleEventCalendar(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, map[string]any, error) {
	var out map[string]any
	if err := s.withAudit(ctx, "event_calendar", nil, func() error {
		return s.cli.Get(ctx, "/api/events/calendar", nil, &out)
	}); err != nil {
		return nil, nil, err
	}
	return nil, out, nil
}

func (s *server) handleEventFlowPrediction(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, map[string]any, error) {
	var out map[string]any
	if err := s.withAudit(ctx, "event_flow_prediction", nil, func() error {
		return s.cli.Get(ctx, "/api/events/prediction", nil, &out)
	}); err != nil {
		return nil, nil, err
	}
	return nil, out, nil
}
