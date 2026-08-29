package server

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerSchedulerTaskTools(mcpSrv *mcp.Server, s *server) {
	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "scheduler_get_status",
		Description: autoDescOr("scheduler_get_status", "Background scheduler status: which tasks ran, when, last result. Read-only overview of the dispatcher state."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: new(false)},
	}, s.handleSchedulerGetStatus)

	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "task_list",
		Description: autoDescOr("task_list", "List background tasks (filtered by status)."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: new(false)},
	}, s.handleTaskList)

	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "task_get",
		Description: autoDescOr("task_get", "Single task by id (status, progress, last result)."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: new(false)},
	}, s.handleTaskGet)

	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "task_get_events",
		Description: autoDescOr("task_get_events", "Event stream of a single task (ordered lifecycle events)."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: new(false)},
	}, s.handleTaskGetEvents)
}

type schedulerTaskBaseOutput struct {
	Result *map[string]any `json:"result"`
}

// schedulerStatusOutput wraps the raw task array with computed summary.
type schedulerStatusOutput struct {
	Tasks   []map[string]any `json:"tasks"`
	Summary struct {
		Total    int `json:"total"`
		Enabled  int `json:"enabled"`
		Disabled int `json:"disabled"`
		Pending  int `json:"pending"` // enabled but never started (next_run is zero)
		Errored  int `json:"errored"` // consecutive_failures > 0
	} `json:"summary"`
}

// taskListOutput decodes the JSON array returned by GET /api/tasks.
type taskListOutput struct {
	Tasks []map[string]any `json:"tasks"`
}

type taskIDInput struct {
	TaskID string `json:"task_id" jsonschema:"the task id"`
}

func (s *server) handleSchedulerGetStatus(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, schedulerStatusOutput, error) {
	var out schedulerStatusOutput
	if err := s.withAudit(ctx, "scheduler_get_status", nil, func() error {
		return s.cli.Get(ctx, "/api/scheduler/status", nil, &out.Tasks)
	}); err != nil {
		return nil, schedulerStatusOutput{}, err
	}
	// Compute summary from raw task data for agent-friendly overview.
	out.Summary.Total = len(out.Tasks)
	for _, t := range out.Tasks {
		enabled, _ := t["enabled"].(bool)
		failures, _ := t["consecutive_failures"].(float64)
		nextRun, _ := t["next_run"].(string)
		lastRun, _ := t["last_run"].(string)

		if !enabled {
			out.Summary.Disabled++
			continue
		}
		out.Summary.Enabled++
		if nextRun == "" || nextRun == "0001-01-01T00:00:00Z" || nextRun[0:4] == "0001" {
			if lastRun == "" || lastRun == "0001-01-01T00:00:00Z" || lastRun[0:4] == "0001" {
				out.Summary.Pending++
			}
		}
		if failures > 0 {
			out.Summary.Errored++
		}
	}
	return nil, out, nil
}

func (s *server) handleTaskList(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, taskListOutput, error) {
	var out taskListOutput
	if err := s.withAudit(ctx, "task_list", nil, func() error {
		return s.cli.Get(ctx, "/api/tasks", nil, &out.Tasks)
	}); err != nil {
		return nil, taskListOutput{}, err
	}
	return nil, out, nil
}

func (s *server) handleTaskGet(ctx context.Context, _ *mcp.CallToolRequest, in taskIDInput) (*mcp.CallToolResult, schedulerTaskBaseOutput, error) {
	var out schedulerTaskBaseOutput
	if err := s.withAudit(ctx, "task_get", []string{"task_id"}, func() error {
		return s.cli.Get(ctx, "/api/tasks/"+in.TaskID, nil, &out.Result)
	}); err != nil {
		return nil, schedulerTaskBaseOutput{}, err
	}
	return nil, out, nil
}

func (s *server) handleTaskGetEvents(ctx context.Context, _ *mcp.CallToolRequest, in taskIDInput) (*mcp.CallToolResult, schedulerTaskBaseOutput, error) {
	var out schedulerTaskBaseOutput
	if err := s.withAudit(ctx, "task_get_events", []string{"task_id"}, func() error {
		// /events is a text/event-stream (SSE) endpoint which this HTTP
		// client cannot decode; /events/snapshot is its JSON variant.
		return s.cli.Get(ctx, "/api/tasks/"+in.TaskID+"/events/snapshot", nil, &out.Result)
	}); err != nil {
		return nil, schedulerTaskBaseOutput{}, err
	}
	return nil, out, nil
}
