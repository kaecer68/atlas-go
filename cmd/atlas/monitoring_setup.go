package main

import (
	"context"
	"time"

	"github.com/kaecer68/atlas-go/internal/monitoring"
)

// setupMonitor creates the application-wide *monitoring.Monitor and
// wires the Phase 2A alert pipeline: alert store, deduplicator
// (5-min window), auto-handler with 24h category suppress rules, and
// the console handler for stdout/stderr rendering.
//
// The returned autoHandler is consumed by BackgroundTaskManager's
// recovery handler (see background_tasks.go) and any other component
// that wants to trigger an automatic recovery flow.
func setupMonitor(alertStore *monitoring.AlertStore, suppressCategories []string, lifecycleCtx context.Context) (*monitoring.Monitor, *monitoring.AutoHandler) {
	monitor := monitoring.NewMonitor()
	if alertStore != nil {
		monitor.SetAlertStore(alertStore)
	}
	alertDeduplicator := monitoring.NewAlertDeduplicator(5*time.Minute, alertStore)
	var suppressRules []monitoring.SuppressRule
	for _, cat := range suppressCategories {
		suppressRules = append(suppressRules, monitoring.SuppressRule{
			Category: cat,
			Duration: 24 * time.Hour,
		})
	}
	autoHandler := monitoring.NewAutoHandler(alertStore, suppressRules)
	monitor.SetDeduplicator(alertDeduplicator)
	monitor.SetAutoHandler(autoHandler)
	monitor.RegisterHandler(monitoring.ConsoleHandler)
	// #1787: TTL auto-archival — open alerts whose condition stops recurring
	// are resolved automatically (WARNING 7d / ERROR+CRITICAL 30d) instead of
	// polluting the "需要決策" queue forever.
	monitoring.StartAlertTTLLifecycle(alertStore, lifecycleCtx, time.Hour)
	return monitor, autoHandler
}
