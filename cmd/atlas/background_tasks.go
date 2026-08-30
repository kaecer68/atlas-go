package main

import (
	"fmt"

	"github.com/kaecer68/atlas-go/internal/apigateway"
	"github.com/kaecer68/atlas-go/internal/logging"
	"github.com/kaecer68/atlas-go/internal/monitoring"
)

// setupBackgroundTaskManager creates a BackgroundTaskManager and wires
// its failure/recovery event handlers. The actual task Register() calls
// stay in main.go for now (Wave 3 PR10 will move them into a dedicated
// registration helper).
//
// Failure handler escalates to monitor.Alert(AlertLevelError) after 3
// consecutive failures. Recovery handler triggers the auto-recovery
// flow (if configured) and logs the event at INFO level.
func setupBackgroundTaskManager(gw *apigateway.Gateway, monitor *monitoring.Monitor, autoHandler *monitoring.AutoHandler) *apigateway.BackgroundTaskManager {
	var taskMgr *apigateway.BackgroundTaskManager
	if gw != nil {
		taskMgr = apigateway.NewBackgroundTaskManager(gw)
	} else {
		taskMgr = apigateway.NewBackgroundTaskManager(nil)
	}
	// Production: stagger the first runs of a fresh process start across a
	// bounded window so all tasks don't stampede the shared rate limiters and
	// upstreams simultaneously (#1763) — the restart window otherwise shows
	// ~30 minutes of false channel alarms on the dashboard.
	taskMgr.WithStartupStagger(true)
	taskMgr.SetFailureHandler(func(name string, consecutiveFailures int, err error) {
		if consecutiveFailures >= 3 {
			monitor.Alert(monitoring.AlertLevelError, "background_task",
				fmt.Sprintf("Task %s failed %d consecutive times: %v", name, consecutiveFailures, err),
				map[string]any{"task": name, "consecutive_failures": consecutiveFailures})
		}
	})
	taskMgr.SetRecoveryHandler(func(name string, recoveredFrom int) {
		if autoHandler != nil {
			autoHandler.Recover("background_task")
		}
		logging.Info("main", "task_recovered",
			"task", name,
			"recovered_from", recoveredFrom)
	})
	return taskMgr
}
