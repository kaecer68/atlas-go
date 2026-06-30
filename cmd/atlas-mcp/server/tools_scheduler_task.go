package server

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerSchedulerTaskTools(mcpSrv *mcp.Server, s *server) {
	mcp.AddTool(mcpSrv, &mcp.Tool{
		Name:        "scheduler_get_status",
		Description: autoDescOr("scheduler_get_status", "Background scheduler status: which tasks ran, when, last result. Read-only overview of the dispatcher state."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleSchedulerGetStatus)

	mcp.AddTool(mcpSrv, &mcp.Tool{
		Name:        "task_list",
		Description: autoDescOr("task_list", "List background tasks (filtered by status)."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleTaskList)

	mcp.AddTool(mcpSrv, &mcp.Tool{
		Name:        "task_get",
		Description: autoDescOr("task_get", "Single task by id (status, progress, last result)."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleTaskGet)

	mcp.AddTool(mcpSrv, &mcp.Tool{
		Name:        "task_get_events",
		Description: autoDescOr("task_get_events", "Event stream of a single task (ordered lifecycle events)."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)},
	}, s.handleTaskGetEvents)
}

type schedulerTaskBaseOutput struct {
	Result *map[string]any `json:"result"`
}

type taskIDInput struct {
	TaskID string `json:"task_id" jsonschema:"the task id"`
}

func (s *server) handleSchedulerGetStatus(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, schedulerTaskBaseOutput, error) {
	var out schedulerTaskBaseOutput
	if err := s.withAudit(ctx, "scheduler_get_status", nil, func() error {
		return s.cli.Get(ctx, "/api/scheduler/status", nil, &out.Result)
	}); err != nil {
		return nil, schedulerTaskBaseOutput{}, err
	}
	return nil, out, nil
}

func (s *server) handleTaskList(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, schedulerTaskBaseOutput, error) {
	var out schedulerTaskBaseOutput
	if err := s.withAudit(ctx, "task_list", nil, func() error {
		return s.cli.Get(ctx, "/api/tasks", nil, &out.Result)
	}); err != nil {
		return nil, schedulerTaskBaseOutput{}, err
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
		return s.cli.Get(ctx, "/api/tasks/"+in.TaskID+"/events", nil, &out.Result)
	}); err != nil {
		return nil, schedulerTaskBaseOutput{}, err
	}
	return nil, out, nil
}
